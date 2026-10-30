package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ganova/mymoney/internal/period"
)

// Order adalah satu pesanan/proyek dari daftar pesanan platform freelance
// (mis. Fastwork) yang dananya belum cair.
type Order struct {
	Ref     string `json:"ref"`      // ID pesanan
	Name    string `json:"name"`     // judul proyek
	Client  string `json:"client"`   // platform · nama klien
	Amount  int64  `json:"amount"`   // harga rupiah
	DueDate string `json:"due_date"` // YYYY-MM-DD atau kosong
	Status  string `json:"status"`   // teks status di layar
	Stage   string `json:"stage"`    // mis. "Tahap 1/2"
	Exists  bool   `json:"exists"`   // sudah tercatat sebelumnya
}

// OrderResult adalah hasil pembacaan daftar pesanan.
type OrderResult struct {
	Orders []Order `json:"orders"`
	Notes  string  `json:"notes"`
	Mode   string  `json:"mode"` // ai | offline
}

const ordersPrompt = `Kamu membaca screenshot daftar pesanan di platform freelance (mis. Fastwork: kolom ID Pesanan, Tanggal jatuh tempo, Partner kerja, Status, Diperbarui pada, Harga). Ekstrak SETIAP baris pesanan yang dananya belum cair menjadi data terstruktur.

Aturan:
- ref: ID pesanan persis seperti tertulis (mis. JIS7X6N8). Kosong bila tidak terlihat.
- name: judul proyek di kolom pertama. Bila terpotong dengan "...", tulis apa adanya tanpa "..." dan jangan mengarang kelanjutannya.
- client: "<platform> · <nama partner kerja>", mis. "Fastwork · Ujang". Bila nama platform tidak terlihat tetapi tata letaknya khas Fastwork, pakai "Fastwork".
- amount: kolom Harga sebagai bilangan bulat rupiah (Rp4.750.000,00 → 4750000).
- due_date: dari kolom tanggal jatuh tempo, format YYYY-MM-DD. Tanggal tanpa tahun: pilih tahun agar tanggal tidak lebih dari 2 bulan di masa lalu relatif terhadap hari ini. Tanda "—" atau kosong → string kosong. Abaikan keterangan seperti "(sisa 1 hari)".
- status: teks status persis di layar. stage: label tahap seperti "Tahap 1/2" bila ada, selain itu kosong.
- Lewati pesanan berstatus selesai, dibatalkan, ditolak, atau kadaluarsa. Sertakan yang sedang dikerjakan, menunggu persetujuan, atau menunggu pembayaran.
- Satu baris layar = satu pesanan; jangan menggandakan. Bila ada beberapa gambar yang tumpang tindih, tulis setiap pesanan sekali.
- notes: catatan singkat untuk pengguna (mis. baris yang dilewati), boleh kosong.`

func ordersSchema() map[string]any {
	order := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"ref", "name", "client", "amount", "due_date", "status", "stage"},
		"properties": map[string]any{
			"ref":      map[string]any{"type": "string"},
			"name":     map[string]any{"type": "string"},
			"client":   map[string]any{"type": "string"},
			"amount":   map[string]any{"type": "integer"},
			"due_date": map[string]any{"type": "string"},
			"status":   map[string]any{"type": "string"},
			"stage":    map[string]any{"type": "string"},
		},
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"orders", "notes"},
		"properties": map[string]any{
			"orders": map[string]any{"type": "array", "items": order},
			"notes":  map[string]any{"type": "string"},
		},
	}
}

// ExtractOrders membaca daftar pesanan dari screenshot memakai AI (API Claude
// atau Codex CLI). Tanpa AI mengembalikan ErrDisabled.
func (s *Service) ExtractOrders(ctx context.Context, images []Image) (*OrderResult, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("tidak ada gambar untuk dibaca")
	}
	today := s.store.Today()
	hdr := fmt.Sprintf("Hari ini: %s (%s).", today.Format("2006-01-02"), period.MonthName(today.Month()))
	var raw []byte
	if s.apiReady() {
		var content []anthropic.BetaContentBlockParamUnion
		for i, img := range images {
			content = append(content, anthropic.NewBetaTextBlock(fmt.Sprintf("Gambar %d:", i+1)))
			content = append(content, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
				Data:      base64.StdEncoding.EncodeToString(img.Data),
				MediaType: anthropic.BetaBase64ImageSourceMediaType(img.MediaType),
			}))
		}
		content = append(content, anthropic.NewBetaTextBlock(hdr+"\nEkstrak semua pesanannya sekarang."))
		p := s.baseParams(s.cfg.VisionEffort, 16000)
		p.System = []anthropic.BetaTextBlockParam{{Text: ordersPrompt}}
		p.Messages = []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(content...)}
		p.OutputConfig.Format = jsonFormat(ordersSchema())
		msg, err := s.send(ctx, "orders", p)
		if err != nil {
			return nil, err
		}
		raw = []byte(textOf(msg))
	} else {
		var err error
		raw, err = s.runCodex(ctx, ordersPrompt+"\n\nJangan membaca atau menjalankan perintah apa pun. Gambar terlampir adalah sumber datanya.\n\n"+hdr+"\nEkstrak semua pesanannya sekarang.", ordersSchema(), images)
		if err != nil {
			return nil, err
		}
	}
	var out OrderResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("jawaban AI tidak valid: %w", err)
	}
	out.Mode = "ai"
	out.Orders = cleanOrders(out.Orders)
	return &out, nil
}

func cleanOrders(in []Order) []Order {
	out := make([]Order, 0, len(in))
	seen := map[string]bool{}
	for _, o := range in {
		o.Name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(o.Name), "..."))
		o.Ref = strings.ToUpper(strings.TrimSpace(o.Ref))
		if _, err := time.Parse("2006-01-02", o.DueDate); err != nil {
			o.DueDate = ""
		}
		if o.Name == "" || o.Amount <= 0 {
			continue
		}
		key := o.Ref + "|" + strings.ToLower(o.Name) + "|" + strconv.FormatInt(o.Amount, 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, o)
	}
	return out
}

var (
	orderPrice  = regexp.MustCompile(`Rp\s?([0-9][0-9.]*)(?:,\d{1,2})?`)
	orderDue    = regexp.MustCompile(`(?i)(\d{1,2})\s+(jan|feb|mar|apr|mei|jun|jul|agu|ags|sep|okt|nov|des)[a-z]*`)
	orderStage  = regexp.MustCompile(`(?i)\bt.hap\s*(\d)\s*/\s*(\d)`)
	orderID     = regexp.MustCompile(`\b[A-Za-z0-9]{8}\b`)
	orderSkip   = regexp.MustCompile(`(?i)selesai|dibatalkan|batal|ditolak|kadaluarsa|kedaluwarsa`)
	orderHeader = regexp.MustCompile(`(?i)id\s*pesanan|partner\s*kerja|diperbarui`)
	orderNoise  = regexp.MustCompile(`(?i)\b(sedang\s*dikerjakan|menunggu\s*persetujuan|menunggu\s*pembayaran|yang\s*lalu|jatuh\s*tempo|sisa\s*\d+\s*hari)\b`)
)

// findOrderID mencari token 8 karakter yang memuat angka (ID pesanan), bukan kata biasa.
func findOrderID(s string) []int {
	for _, m := range orderID.FindAllStringIndex(s, -1) {
		if strings.IndexFunc(s[m[0]:m[1]], func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 {
			return m
		}
	}
	return nil
}

var monthIdx = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "mei": 5, "jun": 6, "jul": 7, "agu": 8, "ags": 8, "sep": 9, "okt": 10, "nov": 11, "des": 12}

// ParseOrdersOffline membaca daftar pesanan dari teks OCR tanpa AI. Hasilnya
// perkiraan (OCR sering salah baca ID dan nama partner), jadi pengguna
// diminta mengecek sebelum menyimpan.
func ParseOrdersOffline(text string, today time.Time) []Order {
	var out []Order
	var buf []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || orderHeader.MatchString(line) {
			continue
		}
		if m := orderStage.FindStringSubmatch(line); m != nil && len(out) > 0 && !orderPrice.MatchString(line) {
			out[len(out)-1].Stage = "Tahap " + m[1] + "/" + m[2]
			continue
		}
		pm := orderPrice.FindStringSubmatchIndex(line)
		if pm == nil {
			buf = append(buf, line)
			continue
		}
		amount, _ := strconv.ParseInt(strings.ReplaceAll(line[pm[2]:pm[3]], ".", ""), 10, 64)
		if amount <= 0 || orderSkip.MatchString(line) {
			buf = nil
			continue
		}
		o := Order{Amount: amount, Client: "Fastwork"}
		head := line[:pm[0]]
		switch {
		case strings.Contains(strings.ToLower(head), "menunggu"):
			o.Status = "Menunggu persetujuan"
		case strings.Contains(strings.ToLower(head), "sedang"):
			o.Status = "Sedang dikerjakan"
		}
		if m := orderDue.FindStringSubmatch(head); m != nil && strings.Contains(strings.ToLower(head), "jatuh") {
			day, _ := strconv.Atoi(m[1])
			mo := monthIdx[strings.ToLower(m[2][:3])]
			y := today.Year()
			d := time.Date(y, mo, day, 0, 0, 0, 0, time.UTC)
			if d.Before(today.AddDate(0, -2, 0)) {
				d = d.AddDate(1, 0, 0)
			}
			if d.Month() == mo {
				o.DueDate = d.Format("2006-01-02")
			}
		}
		idLoc := findOrderID(head)
		if idLoc != nil {
			o.Ref = strings.ToUpper(head[idLoc[0]:idLoc[1]])
		}
		// judul: teks sebelum ID/status di baris harga, ditambah baris sebelumnya
		title := head
		if idLoc != nil {
			title = head[:idLoc[0]]
		} else if loc := orderNoise.FindStringIndex(head); loc != nil {
			title = head[:loc[0]]
		}
		parts := append(append([]string{}, buf...), title)
		var words []string
		for _, w := range strings.Fields(strings.Join(parts, " ")) {
			// buang sisa OCR dari logo/thumbnail: simbol, angka tunggal, dan token 1-2 huruf
			if len([]rune(w)) <= 2 && w != "&" && !strings.EqualFold(w, "di") && !strings.EqualFold(w, "ke") {
				continue
			}
			words = append(words, w)
		}
		name := strings.Trim(strings.Join(words, " "), " -—_|©@()[]{},.")
		o.Name = strings.TrimSpace(strings.TrimSuffix(name, "..."))
		buf = nil
		if o.Name != "" {
			out = append(out, o)
		}
	}
	return cleanOrders(out)
}

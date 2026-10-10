// Package money berisi utilitas nominal Rupiah: membaca teks seperti
// "25rb", "1,5jt", "Rp 11.217.866" menjadi angka, dan memformatnya kembali.
package money

import (
	"regexp"
	"strconv"
	"strings"
)

// amountRe menangkap angka dengan pemisah ribuan/desimal dan satuan opsional.
var amountRe = regexp.MustCompile(`(?i)(?:rp\.?\s*)?(\d+(?:[.,]\d+)*)\s*(rb|ribu|rbu|k|jt|juta|jeti|m|miliar|milyar)?\b`)

// Match adalah satu nominal yang ditemukan di dalam teks.
type Match struct {
	Value int64
	Start int // posisi byte di teks asli
	End   int
	Unit  string
}

// Find mencari semua nominal di dalam teks.
func Find(text string) []Match {
	var out []Match
	for _, loc := range amountRe.FindAllStringSubmatchIndex(text, -1) {
		num := text[loc[2]:loc[3]]
		unit := ""
		if loc[4] >= 0 {
			unit = strings.ToLower(text[loc[4]:loc[5]])
		}
		v, ok := parseNumber(num, unit)
		if !ok || v <= 0 {
			continue
		}
		out = append(out, Match{Value: v, Start: loc[0], End: loc[1], Unit: unit})
	}
	return out
}

// Parse membaca satu nominal dari string; mengembalikan false bila tidak ada.
func Parse(s string) (int64, bool) {
	m := Find(s)
	if len(m) == 0 {
		return 0, false
	}
	return m[0].Value, true
}

func multiplier(unit string) float64 {
	switch unit {
	case "rb", "ribu", "rbu", "k":
		return 1e3
	case "jt", "juta", "jeti":
		return 1e6
	case "m", "miliar", "milyar":
		return 1e9
	}
	return 1
}

// parseNumber menafsirkan pemisah titik/koma ala Indonesia.
func parseNumber(num, unit string) (int64, bool) {
	mult := multiplier(unit)
	dots := strings.Count(num, ".")
	commas := strings.Count(num, ",")
	var normalized string
	switch {
	case dots == 0 && commas == 0:
		normalized = num
	case dots > 0 && commas > 0:
		// format "11.217.866,00" (ID) atau "11,217,866.00" (EN)
		if strings.LastIndex(num, ",") > strings.LastIndex(num, ".") {
			normalized = strings.ReplaceAll(num, ".", "")
			normalized = strings.Replace(normalized, ",", ".", 1)
		} else {
			normalized = strings.ReplaceAll(num, ",", "")
		}
	default:
		sep := "."
		if commas > 0 {
			sep = ","
		}
		parts := strings.Split(num, sep)
		last := parts[len(parts)-1]
		if len(parts) > 2 || (len(last) == 3 && mult == 1) {
			// pemisah ribuan: "25.000", "1.500.000"
			normalized = strings.Join(parts, "")
		} else if len(last) == 2 && mult == 1 && len(parts) == 2 && sep == "," {
			// sen: "25000,00"
			normalized = parts[0] + "." + last
		} else {
			// desimal: "1,5jt", "2.5k"
			normalized = parts[0] + "." + last
		}
	}
	f, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, false
	}
	return int64(f*mult + 0.5), true
}

// Format menghasilkan "Rp25.000" (negatif: "-Rp25.000").
func Format(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
	}
	if neg {
		return "-Rp" + b.String()
	}
	return "Rp" + b.String()
}

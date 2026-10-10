package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ganova/mymoney/internal/store"
)

// fakeClaude adalah server tiruan Messages API (SSE) untuk menguji alur AI
// tanpa memanggil API sungguhan.
type fakeClaude struct {
	mu        sync.Mutex
	requests  []map[string]any
	headers   []http.Header
	responses []func(w io.Writer)
}

func (f *fakeClaude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.headers = append(f.headers, r.Header.Clone())
	i := len(f.requests) - 1
	f.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	if i >= len(f.responses) {
		http.Error(w, "no more responses", 500)
		return
	}
	f.responses[i](w)
}

func sse(w io.Writer, event string, data any) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}

func start(w io.Writer) {
	sse(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5", "content": []any{},
		"stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 1200, "output_tokens": 1, "cache_read_input_tokens": 1000},
	}})
}

func finish(w io.Writer, stop string) {
	sse(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 80}})
	sse(w, "message_stop", map[string]any{"type": "message_stop"})
}

func toolUse(id, name string, input any) func(w io.Writer) {
	return func(w io.Writer) {
		start(w)
		b, _ := json.Marshal(input)
		sse(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}}})
		sse(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(b)}})
		sse(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		finish(w, "tool_use")
	}
}

func text(t string) func(w io.Writer) {
	return func(w io.Writer) {
		start(w)
		sse(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		sse(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": t}})
		sse(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		finish(w, "end_turn")
	}
}

func newFake(t *testing.T, st *store.Store, responses ...func(w io.Writer)) (*Service, *fakeClaude) {
	f := &fakeClaude{responses: responses}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return New(Config{APIKey: "test-key", BaseURL: srv.URL, Fallbacks: true}, st), f
}

func ids(t *testing.T, st *store.Store) (map[string]int64, map[string]int64) {
	acc, cat := names(t, st)
	a, c := map[string]int64{}, map[string]int64{}
	for id, n := range acc {
		a[n] = id
	}
	for id, n := range cat {
		c[n] = id
	}
	return a, c
}

func TestAskRecordsTransaction(t *testing.T) {
	st := testStore(t)
	a, c := ids(t, st)
	svc, f := newFake(t, st, toolUse("toolu_1", "catat_transaksi", map[string]any{
		"message": "Siap, makan siang Rp25.000 dari Tunai 👍",
		"transactions": []any{map[string]any{
			"type": "expense", "amount": 25000, "amount_estimated": false, "date": "2026-10-10",
			"description": "Makan siang", "category_id": c["Makan & Minum"], "account_id": a["Tunai"],
			"to_account_id": nil, "debt_id": 999, "note": "", "confidence": 0.9,
		}},
	}))
	res, err := svc.Ask(context.Background(), []HistoryItem{{Role: "assistant", Text: "halo"}}, "beli makan siang 25rb cash", "ai_text")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drafts) != 1 || res.Drafts[0].Amount != 25000 || res.Drafts[0].AccountID != a["Tunai"] || res.Drafts[0].DebtID != nil {
		t.Fatalf("drafts %+v", res.Drafts)
	}
	if !strings.Contains(res.Reply, "makan siang") {
		t.Fatalf("reply %q", res.Reply)
	}
	if len(f.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(f.requests))
	}
	req := f.requests[0]
	if req["model"] != "claude-opus-5-5" || req["fallbacks"] != "default" {
		t.Fatalf("model/fallbacks: %v %v", req["model"], req["fallbacks"])
	}
	if !strings.Contains(f.headers[0].Get("anthropic-beta"), "server-side-fallback-2026-07-01") {
		t.Fatalf("beta header %q", f.headers[0].Get("anthropic-beta"))
	}
	if oc, _ := req["output_config"].(map[string]any); oc["effort"] != "low" {
		t.Fatalf("output_config %v", req["output_config"])
	}
	if _, ok := req["thinking"]; ok {
		t.Fatal("thinking must be omitted for Opus 5.5")
	}
	tools := req["tools"].([]any)
	first := tools[0].(map[string]any)
	if first["name"] != "catat_transaksi" || first["strict"] != true {
		t.Fatalf("tool %v", first)
	}
	if schema := first["input_schema"].(map[string]any); schema["additionalProperties"] != false {
		t.Fatalf("schema %v", schema)
	}
	sys := req["system"].([]any)
	if last := sys[len(sys)-1].(map[string]any); last["cache_control"] == nil || !strings.Contains(last["text"].(string), "Makan & Minum") {
		t.Fatalf("system %v", last)
	}
	// riwayat yang diawali asisten dibuang; pesan pertama harus dari user
	msgs := req["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["role"] != "user" {
		t.Fatalf("messages %v", msgs)
	}
}

func TestAskAnswersWithTools(t *testing.T) {
	st := testStore(t)
	svc, f := newFake(t, st,
		toolUse("toolu_1", "ringkasan_periode", map[string]any{"period": ""}),
		text("Pengeluaranmu periode ini masih Rp0 😄"),
	)
	res, err := svc.Ask(context.Background(), nil, "pengeluaran bulan ini berapa?", "ai_text")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reply != "Pengeluaranmu periode ini masih Rp0 😄" || len(res.Drafts) != 0 {
		t.Fatalf("res %+v", res)
	}
	if len(f.requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(f.requests))
	}
	msgs := f.requests[1]["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	block := last["content"].([]any)[0].(map[string]any)
	if block["type"] != "tool_result" || block["tool_use_id"] != "toolu_1" {
		t.Fatalf("tool result %v", block)
	}
	content := block["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(content, "Pemasukan Rp0") {
		t.Fatalf("tool output %q", content)
	}
}

func TestExtractMarksDuplicates(t *testing.T) {
	st := testStore(t)
	a, c := ids(t, st)
	ctx := context.Background()
	existing, err := st.CreateTransaction(ctx, store.Transaction{Date: "2026-10-05", Type: "expense", Amount: 44000, AccountID: a["BCA"], Description: "Nasi Bebek"})
	if err != nil {
		t.Fatal(err)
	}
	row := func(desc string, amount int64, date, raw string) map[string]any {
		return map[string]any{"type": "expense", "amount": amount, "amount_estimated": false, "date": date, "description": desc,
			"category_id": c["Makan & Minum"], "account_id": a["BCA"], "to_account_id": nil, "debt_id": nil, "note": "", "confidence": 0.9, "raw": raw}
	}
	out, _ := json.Marshal(map[string]any{
		"source": "myBCA", "account_id": a["BCA"], "notes": "",
		"transactions": []any{
			row("Nasi Bebek", 44000, "2026-10-05", "TRANSAKSI DEBIT QR 914 Nasi Bebek"),
			row("Ikan & Ayam", 29000, "2026-10-06", "TRANSAKSI DEBIT QR 914 Ikan & Aya"),
			row("Ikan & Ayam", 29000, "2026-10-06", "TRANSAKSI DEBIT QR 914 Ikan & Aya"),
		},
	})
	svc, f := newFake(t, st, text(string(out)))
	res, err := svc.Extract(ctx, ExtractInput{Images: []Image{{Data: []byte("fakepng"), MediaType: "image/png"}}, AccountID: a["BCA"], Kind: "ai_image"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drafts) != 3 || res.Drafts[0].DuplicateOf != existing.ID || res.Drafts[1].DuplicateOf != 0 || !res.Drafts[2].DuplicateInScan {
		t.Fatalf("drafts %+v", res.Drafts)
	}
	req := f.requests[0]
	oc := req["output_config"].(map[string]any)
	if oc["effort"] != "medium" || oc["format"].(map[string]any)["type"] != "json_schema" {
		t.Fatalf("output_config %v", oc)
	}
	content := req["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if content[1].(map[string]any)["type"] != "image" {
		t.Fatalf("content %v", content)
	}
}

func TestRefusal(t *testing.T) {
	st := testStore(t)
	svc, _ := newFake(t, st, func(w io.Writer) { start(w); finish(w, "refusal") })
	if _, err := svc.Ask(context.Background(), nil, "x", "ai_text"); err == nil || !strings.Contains(err.Error(), "menolak") {
		t.Fatalf("err %v", err)
	}
}

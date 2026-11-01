//go:build !windows

package ai

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeCodex meniru `codex exec`: menulis JSON ke berkas -o dan memeriksa argumen.
func fakeCodex(t *testing.T, reply string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\nout=\nwhile [ $# -gt 0 ]; do [ \"$1\" = -o ] && out=$2; shift; done\ncat >/dev/null\ncat > \"$out\" <<'EOF2'\n" + reply + "\nEOF2\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCodexAsk(t *testing.T) {
	st := testStore(t)
	reply := `{"reply":"Dicatat.","transactions":[{"type":"expense","amount":25000,"amount_estimated":false,"date":"2026-10-29","description":"Makan siang","category_id":null,"account_id":1,"to_account_id":null,"debt_id":null,"note":"","confidence":0.9}]}`
	s := New(Config{CodexPath: fakeCodex(t, reply)}, st)
	if !s.Enabled() {
		t.Fatal("Codex seharusnya mengaktifkan AI")
	}
	res, err := s.Ask(context.Background(), nil, "makan siang 25rb", "ai_text")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drafts) != 1 || res.Drafts[0].Amount != 25000 || res.Reply != "Dicatat." {
		t.Fatalf("hasil tak terduga: %+v", res)
	}
}

func TestCodexBudgetAndInsight(t *testing.T) {
	budget := `{"lines":[{"label":"Cicilan rumah","category_id":null,"new_category":{"name":"Cicilan Rumah","group":"Tagihan Bulanan","icon":"🏠","kind":"expense"},"amount":4650000,"due_day":28,"note":""}],"summary":"Total pengeluaran Rp4.650.000."}`
	s := New(Config{CodexPath: fakeCodex(t, budget)}, testStore(t))
	out, err := s.SuggestBudget(t.Context(), "cicilan rumah 4650000 tiap bulan")
	if err != nil || len(out.Lines) != 1 || out.Lines[0].Amount != 4650000 || out.Lines[0].NewCategory == nil {
		t.Fatalf("budget via codex: %+v %v", out, err)
	}
	s = New(Config{CodexPath: fakeCodex(t, `{"text":"### Sorotan\n- Aman."}`)}, testStore(t))
	txt, err := s.Insight(t.Context(), "2026-11")
	if err != nil || txt != "### Sorotan\n- Aman." {
		t.Fatalf("insight via codex: %q %v", txt, err)
	}
}

package period

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestForStartDay28(t *testing.T) {
	cases := []struct {
		date, key, start, end string
	}{
		{"2026-10-10", "2026-10", "2026-09-28", "2026-10-27"},
		{"2026-09-28", "2026-10", "2026-09-28", "2026-10-27"},
		{"2026-10-27", "2026-10", "2026-09-28", "2026-10-27"},
		{"2026-10-28", "2026-11", "2026-10-28", "2026-11-27"},
		{"2026-12-30", "2027-01", "2026-12-28", "2027-01-27"},
		{"2027-01-05", "2027-01", "2026-12-28", "2027-01-27"},
		{"2027-02-28", "2027-03", "2027-02-28", "2027-03-27"},
	}
	for _, c := range cases {
		p := For(d(c.date), 28)
		if p.Key != c.key || p.StartStr() != c.start || p.EndStr() != c.end {
			t.Errorf("%s: got %s %s..%s, want %s %s..%s", c.date, p.Key, p.StartStr(), p.EndStr(), c.key, c.start, c.end)
		}
	}
}

func TestLabelAndRange(t *testing.T) {
	p := For(d("2026-10-10"), 28)
	if p.Label != "Oktober 2026" {
		t.Errorf("label %q", p.Label)
	}
	if p.Range() != "28 Sep – 27 Okt 2026" {
		t.Errorf("range %q", p.Range())
	}
	if p.Days() != 30 {
		t.Errorf("days %d", p.Days())
	}
	if p.Next().Key != "2026-11" || p.Prev().Key != "2026-09" {
		t.Errorf("next/prev %s %s", p.Next().Key, p.Prev().Key)
	}
	if p.Prev().StartStr() != "2026-08-28" || p.Prev().EndStr() != "2026-09-27" {
		t.Errorf("prev bounds %s %s", p.Prev().StartStr(), p.Prev().EndStr())
	}
}

func TestStartDay1(t *testing.T) {
	p := For(d("2026-02-14"), 1)
	if p.Key != "2026-02" || p.StartStr() != "2026-02-01" || p.EndStr() != "2026-02-28" {
		t.Errorf("got %s %s..%s", p.Key, p.StartStr(), p.EndStr())
	}
}

func TestStartDay31Clamped(t *testing.T) {
	// periode Maret: mulai 28 Feb (dijepit) s/d 30 Maret
	p := For(d("2026-03-10"), 31)
	if p.Key != "2026-03" || p.StartStr() != "2026-02-28" || p.EndStr() != "2026-03-30" {
		t.Errorf("got %s %s..%s", p.Key, p.StartStr(), p.EndStr())
	}
	// berurutan tanpa celah
	for i, q := 0, p; i < 24; i++ {
		n := q.Next()
		if n.Start.Sub(q.End) != 24*time.Hour {
			t.Fatalf("gap between %s and %s", q.Key, n.Key)
		}
		q = n
	}
}

func TestParse(t *testing.T) {
	p, err := Parse("2026-01", 28)
	if err != nil || p.StartStr() != "2025-12-28" || p.EndStr() != "2026-01-27" {
		t.Fatalf("got %v %s..%s", err, p.StartStr(), p.EndStr())
	}
	if _, err := Parse("bad", 28); err == nil {
		t.Fatal("expected error")
	}
}

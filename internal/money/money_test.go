package money

import "testing"

func TestParse(t *testing.T) {
	cases := map[string]int64{
		"25rb":                25000,
		"25 rb":               25000,
		"25ribu":              25000,
		"25k":                 25000,
		"25K":                 25000,
		"1,5jt":               1500000,
		"1.5 juta":            1500000,
		"2jt":                 2000000,
		"11.217.866":          11217866,
		"Rp 11.217.866,00":    11217866,
		"Rp25.000":            25000,
		"25.000":              25000,
		"25,000":              25000,
		"189000.00":           189000,
		"4,499,000.00":        4499000,
		"makan siang 24500":   24500,
		"bensin 50 ribu cash": 50000,
		"2.5k":                2500,
	}
	for in, want := range cases {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
	if _, ok := Parse("makan siang"); ok {
		t.Error("expected no amount")
	}
}

func TestFindMultiple(t *testing.T) {
	m := Find("makan siang 25rb, parkir 5rb dan bensin 50.000")
	if len(m) != 3 || m[0].Value != 25000 || m[1].Value != 5000 || m[2].Value != 50000 {
		t.Fatalf("got %+v", m)
	}
}

func TestFormat(t *testing.T) {
	cases := map[int64]string{0: "Rp0", 500: "Rp500", 25000: "Rp25.000", 11217866: "Rp11.217.866", -1500: "-Rp1.500"}
	for in, want := range cases {
		if got := Format(in); got != want {
			t.Errorf("Format(%d) = %q want %q", in, got, want)
		}
	}
}

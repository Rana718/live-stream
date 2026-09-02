package money

import (
	"encoding/json"
	"math/rand"
	"testing"
)

func TestParseRupees(t *testing.T) {
	cases := []struct {
		in    string
		want  int64
		isErr bool
	}{
		{"499", 49900, false},
		{"499.00", 49900, false},
		{"499.5", 49950, false},
		{"499.55", 49955, false},
		{"0", 0, false},
		{"0.01", 1, false},
		{"1,299.00", 129900, false},
		{"₹2999", 299900, false},
		{"INR 100.00", 10000, false},
		{"-50.25", -5025, false},
		{"  12.50  ", 1250, false},
		{"499.555", 0, true},
		{"", 0, true},
		{"abc", 0, true},
		{"1.2.3", 0, true},
		{"12.", 0, true},
	}
	for _, c := range cases {
		got, err := ParseRupees(c.in)
		if c.isErr {
			if err == nil {
				t.Errorf("ParseRupees(%q) expected error, got %d", c.in, got.Minor)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRupees(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got.Minor != c.want {
			t.Errorf("ParseRupees(%q) = %d, want %d", c.in, got.Minor, c.want)
		}
		if got.Currency != INR {
			t.Errorf("ParseRupees(%q) currency = %q", c.in, got.Currency)
		}
	}
}

func TestRupeesRoundTrip(t *testing.T) {
	for _, minor := range []int64{0, 1, 99, 100, 49900, 129900, -5025, 100000000} {
		m := New(minor, INR)
		back, err := ParseRupees(m.Rupees())
		if err != nil {
			t.Fatalf("ParseRupees(%q): %v", m.Rupees(), err)
		}
		if back.Minor != minor {
			t.Errorf("round trip %d -> %q -> %d", minor, m.Rupees(), back.Minor)
		}
	}
}

func TestMulBps(t *testing.T) {
	cases := []struct {
		minor, bps, want int64
	}{
		{10000, 1800, 1800}, // 18% of ₹100 = ₹18
		{49900, 1000, 4990}, // 10% of ₹499
		{100, 1800, 18},     // 18% of ₹1
		{101, 1800, 18},     // 18.18p -> 18
		{103, 1800, 19},     // 18.54p -> 19 (half away from zero)
		{10000, 0, 0},
		{-10000, 1800, -1800}, // symmetric
	}
	for _, c := range cases {
		got := New(c.minor, INR).MulBps(c.bps).Minor
		if got != c.want {
			t.Errorf("New(%d).MulBps(%d) = %d, want %d", c.minor, c.bps, got, c.want)
		}
	}
}

func TestSplitSumsExactly(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 5000; i++ {
		minor := rng.Int63n(20_000_000) - 5_000_000 // -50k .. 150k rupees
		n := rng.Intn(12) + 1
		parts := New(minor, INR).Split(n)
		if len(parts) != n {
			t.Fatalf("Split(%d) returned %d parts", n, len(parts))
		}
		var total int64
		for _, p := range parts {
			total += p.Minor
		}
		if total != minor {
			t.Fatalf("Split(%d) of %d summed to %d", n, minor, total)
		}
		// parts differ by at most one minor unit
		min, max := parts[0].Minor, parts[0].Minor
		for _, p := range parts {
			if p.Minor < min {
				min = p.Minor
			}
			if p.Minor > max {
				max = p.Minor
			}
		}
		if max-min > 1 {
			t.Fatalf("Split(%d) of %d parts spread %d..%d", n, minor, min, max)
		}
	}
}

func TestSplitFeeInstallments(t *testing.T) {
	// ₹10,000 / 3 -> 3334 + 3333 + 3333 (paise: 1000000/3)
	parts := MustParseRupees("10000").Split(3)
	got := []int64{parts[0].Minor, parts[1].Minor, parts[2].Minor}
	want := []int64{333334, 333333, 333333}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("installment %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestSplitGST_IntraState(t *testing.T) {
	g := SplitGST(MustParseRupees("100"), 1800, false)
	if g.CGST.Minor != 900 || g.SGST.Minor != 900 || g.IGST.Minor != 0 {
		t.Fatalf("intra 18%% of ₹100: cgst=%d sgst=%d igst=%d", g.CGST.Minor, g.SGST.Minor, g.IGST.Minor)
	}
	if g.Total.Minor != 11800 {
		t.Fatalf("total = %d, want 11800", g.Total.Minor)
	}
	// odd-paise case: ₹101 @ 18% = 1818p; half = 909 (of 909); SGST absorbs
	g = SplitGST(New(10100, INR), 1800, false)
	if g.CGST.Minor+g.SGST.Minor != 1818 {
		t.Fatalf("cgst+sgst = %d, want 1818", g.CGST.Minor+g.SGST.Minor)
	}
}

func TestSplitGST_InterState(t *testing.T) {
	g := SplitGST(MustParseRupees("100"), 1800, true)
	if g.IGST.Minor != 1800 || g.CGST.Minor != 0 || g.SGST.Minor != 0 {
		t.Fatalf("inter 18%% of ₹100: igst=%d cgst=%d sgst=%d", g.IGST.Minor, g.CGST.Minor, g.SGST.Minor)
	}
	if g.Total.Minor != 11800 {
		t.Fatalf("total = %d, want 11800", g.Total.Minor)
	}
}

func TestSplitGSTInclusive(t *testing.T) {
	// ₹4999 GST-inclusive @ 18%: taxable = 4999*10000/11800 = ₹4236.44,
	// tax = ₹762.56, split CGST ₹381.28 / SGST ₹381.28.
	g := SplitGSTInclusive(New(499900, INR), 1800, false)
	if g.Taxable.Minor != 423644 {
		t.Fatalf("taxable = %d, want 423644", g.Taxable.Minor)
	}
	if g.CGST.Minor != 38128 || g.SGST.Minor != 38128 {
		t.Fatalf("cgst=%d sgst=%d, want 38128 each", g.CGST.Minor, g.SGST.Minor)
	}
	// Components + taxable always reconstitute the gross exactly.
	if g.Taxable.Minor+g.CGST.Minor+g.SGST.Minor+g.IGST.Minor != 499900 {
		t.Fatalf("components do not sum to gross")
	}
	// inter-state: all tax in IGST
	gi := SplitGSTInclusive(New(499900, INR), 1800, true)
	if gi.IGST.Minor != 76256 || gi.CGST.Minor != 0 {
		t.Fatalf("inter: igst=%d cgst=%d", gi.IGST.Minor, gi.CGST.Minor)
	}
	// zero rate: everything is taxable, no tax
	g0 := SplitGSTInclusive(New(10000, INR), 0, false)
	if g0.Taxable.Minor != 10000 || g0.CGST.Minor != 0 {
		t.Fatalf("zero rate: taxable=%d cgst=%d", g0.Taxable.Minor, g0.CGST.Minor)
	}
}

func TestSplitGSTInclusiveReconstitutes(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 5000; i++ {
		gross := New(rng.Int63n(9_000_000)+1, INR)
		rate := []int64{0, 500, 1200, 1800, 2800}[rng.Intn(5)]
		inter := rng.Intn(2) == 0
		g := SplitGSTInclusive(gross, rate, inter)
		if g.Taxable.Minor+g.CGST.Minor+g.SGST.Minor+g.IGST.Minor != gross.Minor {
			t.Fatalf("gross=%d rate=%d: components sum to %d",
				gross.Minor, rate, g.Taxable.Minor+g.CGST.Minor+g.SGST.Minor+g.IGST.Minor)
		}
		if g.Total.Minor != gross.Minor {
			t.Fatalf("total %d != gross %d", g.Total.Minor, gross.Minor)
		}
	}
}

func TestGSTComponentsPlusRoundOffEqualTotal(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 3000; i++ {
		taxable := New(rng.Int63n(5_000_000)+1, INR)
		rate := []int64{0, 500, 1200, 1800, 2800}[rng.Intn(5)]
		inter := rng.Intn(2) == 0
		g := SplitGST(taxable, rate, inter)
		sum := g.Taxable.Add(g.CGST).Add(g.SGST).Add(g.IGST)
		if sum.Cmp(g.Total) != 0 {
			t.Fatalf("components %d != total %d", sum.Minor, g.Total.Minor)
		}
		rounded, roundOff := g.Total.RoundToRupee()
		if g.Total.Add(roundOff).Cmp(rounded) != 0 {
			t.Fatalf("total+roundOff != rounded: %d + %d != %d", g.Total.Minor, roundOff.Minor, rounded.Minor)
		}
		if rounded.Minor%100 != 0 {
			t.Fatalf("rounded %d not whole rupees", rounded.Minor)
		}
	}
}

func TestArithmeticCurrencyMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on currency mismatch")
		}
	}()
	New(100, INR).Add(New(100, "USD"))
}

func TestJSONRoundTrip(t *testing.T) {
	m := MustParseRupees("2999.50")
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"amount_minor":299950,"currency":"INR"}` {
		t.Fatalf("marshal = %s", b)
	}
	var back Money
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Cmp(m) != 0 || back.Currency != INR {
		t.Fatalf("unmarshal = %+v", back)
	}
	// accepts a {"rupees": "..."} form too
	var r Money
	if err := json.Unmarshal([]byte(`{"rupees":"499.00"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Minor != 49900 {
		t.Fatalf("rupees form = %d", r.Minor)
	}
}

func TestString(t *testing.T) {
	if got := MustParseRupees("1299").String(); got != "₹1299.00" {
		t.Fatalf("String = %q", got)
	}
	if got := New(5000, "USD").String(); got != "USD 50.00" {
		t.Fatalf("String = %q", got)
	}
}

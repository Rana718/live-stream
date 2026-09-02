// Package money is the single representation of monetary amounts across the
// backend. Amounts are integer minor units (paise for INR) plus a currency
// code — never float64, never NUMERIC-via-string. Every price, order line,
// payment, refund, tax component and payout total flows through this type.
//
// Storage: two columns, `<name>_minor bigint` + `currency text`. Build a
// Money from a row with New(row.AmountMinor, Currency(row.Currency)); write
// one back with m.Minor and string(m.Currency).
package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// divRound divides num by den rounding half away from zero. den must be non-zero.
func divRound(num, den int64) int64 {
	if den == 0 {
		panic("money: divide by zero")
	}
	if (num < 0) != (den < 0) {
		return (num - den/2) / den
	}
	return (num + den/2) / den
}

// Currency is an ISO-4217 code. The platform is INR-only today; the type
// exists so a mismatch is a compile-visible concept and multi-currency is a
// later additive change rather than a rewrite.
type Currency string

const INR Currency = "INR"

// Money is an exact monetary amount: Minor is the amount in the currency's
// smallest unit (paise for INR, i.e. 1/100 of a rupee).
type Money struct {
	Minor    int64    `json:"amount_minor"`
	Currency Currency `json:"currency"`
}

var (
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	ErrParse            = errors.New("money: cannot parse amount")
)

// New builds a Money from minor units.
func New(minor int64, cur Currency) Money {
	if cur == "" {
		cur = INR
	}
	return Money{Minor: minor, Currency: cur}
}

// Zero is a zero amount in the given currency.
func Zero(cur Currency) Money { return New(0, cur) }

// FromRupees builds a whole-rupee INR amount. For fractional input use
// ParseRupees.
func FromRupees(rupees int64) Money { return New(rupees*100, INR) }

// ParseRupees parses a decimal rupee string ("499", "499.5", "1,299.00",
// "₹499.00") into paise. Rejects more than two fractional digits.
func ParseRupees(s string) (Money, error) {
	raw := strings.TrimSpace(s)
	raw = strings.TrimPrefix(raw, "₹")
	raw = strings.TrimPrefix(raw, "INR")
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, ",", "")
	raw = strings.ReplaceAll(raw, " ", "")
	if raw == "" {
		return Money{}, ErrParse
	}

	neg := false
	if strings.HasPrefix(raw, "-") {
		neg = true
		raw = raw[1:]
	}

	intPart, fracPart, hasFrac := raw, "", false
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		intPart, fracPart, hasFrac = raw[:i], raw[i+1:], true
	}
	if intPart == "" {
		intPart = "0"
	}
	if hasFrac && (len(fracPart) == 0 || len(fracPart) > 2) {
		return Money{}, fmt.Errorf("%w: %q has more than 2 decimal places", ErrParse, s)
	}
	for _, r := range intPart + fracPart {
		if r < '0' || r > '9' {
			return Money{}, fmt.Errorf("%w: %q", ErrParse, s)
		}
	}

	var rupees int64
	for _, r := range intPart {
		rupees = rupees*10 + int64(r-'0')
	}
	var paise int64
	switch len(fracPart) {
	case 0:
		paise = 0
	case 1:
		paise = int64(fracPart[0]-'0') * 10
	case 2:
		paise = int64(fracPart[0]-'0')*10 + int64(fracPart[1]-'0')
	}

	minor := rupees*100 + paise
	if neg {
		minor = -minor
	}
	return New(minor, INR), nil
}

// MustParseRupees is ParseRupees that panics on error. Use ONLY in seeds,
// migrations helpers and tests — never on request-path input.
func MustParseRupees(s string) Money {
	m, err := ParseRupees(s)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) sameCurrency(n Money) {
	if m.Currency != n.Currency {
		panic(fmt.Sprintf("%v: %s vs %s", ErrCurrencyMismatch, m.Currency, n.Currency))
	}
}

// Add returns m + n. Panics on a currency mismatch (a programming error).
func (m Money) Add(n Money) Money {
	m.sameCurrency(n)
	return New(m.Minor+n.Minor, m.Currency)
}

// Sub returns m - n.
func (m Money) Sub(n Money) Money {
	m.sameCurrency(n)
	return New(m.Minor-n.Minor, m.Currency)
}

// Neg returns -m.
func (m Money) Neg() Money { return New(-m.Minor, m.Currency) }

// MulBps returns m * bps / 10000, rounded half away from zero. Used for
// percentage discounts, tax rates and revenue shares.
func (m Money) MulBps(bps int64) Money {
	return New(divRound(m.Minor*bps, 10_000), m.Currency)
}

// Split divides m into n parts that sum exactly back to m. The first
// (|m.Minor| mod n) parts are one minor unit larger so nothing is lost to
// rounding. n must be >= 1.
func (m Money) Split(n int) []Money {
	if n < 1 {
		panic("money: Split n < 1")
	}
	out := make([]Money, n)
	base := m.Minor / int64(n)
	rem := m.Minor % int64(n) // keeps sign of m.Minor
	step := int64(1)
	if rem < 0 {
		step, rem = -1, -rem
	}
	for i := 0; i < n; i++ {
		v := base
		if int64(i) < rem {
			v += step
		}
		out[i] = New(v, m.Currency)
	}
	return out
}

// RoundToRupee rounds m to the nearest whole rupee (half away from zero) and
// returns the rounded amount plus the round-off delta (rounded - original),
// for an invoice-level `round_off_minor` line.
func (m Money) RoundToRupee() (rounded, roundOff Money) {
	r := divRound(m.Minor, 100) * 100
	return New(r, m.Currency), New(r-m.Minor, m.Currency)
}

func (m Money) IsZero() bool     { return m.Minor == 0 }
func (m Money) IsNegative() bool { return m.Minor < 0 }
func (m Money) IsPositive() bool { return m.Minor > 0 }

// Cmp reports -1, 0, or +1 as m is less than, equal to, or greater than n.
func (m Money) Cmp(n Money) int {
	m.sameCurrency(n)
	switch {
	case m.Minor < n.Minor:
		return -1
	case m.Minor > n.Minor:
		return 1
	default:
		return 0
	}
}

// Rupees renders the amount as a plain decimal string, e.g. "499.00".
func (m Money) Rupees() string {
	minor := m.Minor
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	return fmt.Sprintf("%s%d.%02d", sign, minor/100, minor%100)
}

// String renders a human amount, e.g. "₹499.00" for INR.
func (m Money) String() string {
	if m.Currency == INR {
		return "₹" + m.Rupees()
	}
	return string(m.Currency) + " " + m.Rupees()
}

// MarshalJSON emits {"amount_minor": N, "currency": "INR"}.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		AmountMinor int64  `json:"amount_minor"`
		Currency    string `json:"currency"`
	}{m.Minor, string(orDefault(m.Currency))})
}

func (m *Money) UnmarshalJSON(b []byte) error {
	var raw struct {
		AmountMinor *int64  `json:"amount_minor"`
		Currency    string  `json:"currency"`
		Rupees      *string `json:"rupees"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	cur := Currency(raw.Currency)
	if cur == "" {
		cur = INR
	}
	switch {
	case raw.AmountMinor != nil:
		*m = New(*raw.AmountMinor, cur)
	case raw.Rupees != nil:
		parsed, err := ParseRupees(*raw.Rupees)
		if err != nil {
			return err
		}
		parsed.Currency = cur
		*m = parsed
	default:
		return fmt.Errorf("%w: object has neither amount_minor nor rupees", ErrParse)
	}
	return nil
}

func orDefault(c Currency) Currency {
	if c == "" {
		return INR
	}
	return c
}

// GST is a tax breakup for one taxable amount.
type GST struct {
	Taxable Money `json:"taxable"`
	CGST    Money `json:"cgst"`
	SGST    Money `json:"sgst"`
	IGST    Money `json:"igst"`
	Total   Money `json:"total"` // Taxable + CGST + SGST + IGST
}

// SplitGST computes the CGST/SGST (intra-state) or IGST (inter-state) on a
// taxable amount at rateBps (e.g. 1800 = 18%). Components are rounded to
// paise half away from zero; for intra-state the rate is halved for each of
// CGST and SGST so the two always sum to the full tax.
func SplitGST(taxable Money, rateBps int64, interState bool) GST {
	g := GST{Taxable: taxable}
	if interState {
		g.IGST = taxable.MulBps(rateBps)
		g.CGST = Zero(taxable.Currency)
		g.SGST = Zero(taxable.Currency)
	} else {
		half := taxable.MulBps(rateBps / 2)
		full := taxable.MulBps(rateBps)
		g.CGST = half
		g.SGST = full.Sub(half) // absorb the odd paise into SGST
		g.IGST = Zero(taxable.Currency)
	}
	g.Total = taxable.Add(g.CGST).Add(g.SGST).Add(g.IGST)
	return g
}

// SplitGSTInclusive back-computes the GST breakup for a GST-INCLUSIVE gross
// amount (the price the customer actually pays) at rateBps. taxable is
// gross * 10000 / (10000 + rateBps), rounded to paise; the remainder is the
// tax, split CGST/SGST for intra-state or all IGST for inter-state.
func SplitGSTInclusive(gross Money, rateBps int64, interState bool) GST {
	if rateBps <= 0 {
		return GST{Taxable: gross, Total: gross,
			CGST: Zero(gross.Currency), SGST: Zero(gross.Currency), IGST: Zero(gross.Currency)}
	}
	taxableMinor := gross.Minor * 10000 / (10000 + rateBps)
	taxable := New(taxableMinor, gross.Currency)
	tax := gross.Sub(taxable)
	g := GST{Taxable: taxable, Total: gross}
	if interState {
		g.IGST = tax
		g.CGST = Zero(gross.Currency)
		g.SGST = Zero(gross.Currency)
	} else {
		half := New(tax.Minor/2, gross.Currency)
		g.CGST = half
		g.SGST = tax.Sub(half) // odd paise → SGST
		g.IGST = Zero(gross.Currency)
	}
	return g
}

// Sum adds any number of same-currency amounts. Empty input returns a zero
// INR amount.
func Sum(ms ...Money) Money {
	if len(ms) == 0 {
		return Zero(INR)
	}
	total := Zero(ms[0].Currency)
	for _, m := range ms {
		total = total.Add(m)
	}
	return total
}

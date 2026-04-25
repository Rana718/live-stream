package tenants

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"unicode"
)

// letterPrefix pulls up to `n` ASCII letters from `s` (uppercased, ignoring
// spaces, digits and punctuation). If the input has fewer than `n` letters
// we right-pad with 'X' so we always return a fixed-width prefix — keeps
// org codes the same length across every tenant for predictable UI.
func letterPrefix(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= n {
			break
		}
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		} else if r >= 'a' && r <= 'z' {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	for b.Len() < n {
		b.WriteByte('X')
	}
	return b.String()
}

// randomFourDigit returns 0..9999. Crypto/rand is overkill for org-code
// disambiguation but it's already imported elsewhere and predictable
// `math/rand` would let an attacker enumerate tenants by guessing the
// next code. Cost is the same; default to the safe choice.
func randomFourDigit() int {
	var buf [4]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return 0
	}
	return int(binary.BigEndian.Uint32(buf[:]) % 10000)
}

// slugify is the conventional kebab-case transform: lowercase, ASCII
// letters/digits only, hyphens between words. We collapse runs of
// non-alphanumerics to a single hyphen so "Excel  Coaching!!" → "excel-coaching".
func slugify(s string) string {
	var b strings.Builder
	prevHyphen := true // suppress leading hyphen
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

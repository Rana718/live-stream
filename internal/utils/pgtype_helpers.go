package utils

import (
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- write helpers (Go → pgtype) ---

func UUIDToPg(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func UUIDPtrToPg(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func TextToPg(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func TextPtrToPg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func BoolToPg(b bool) pgtype.Bool {
	return pgtype.Bool{Bool: b, Valid: true}
}

func Int4ToPg(n int32) pgtype.Int4 {
	return pgtype.Int4{Int32: n, Valid: true}
}

func Int8ToPg(n int64) pgtype.Int8 {
	return pgtype.Int8{Int64: n, Valid: true}
}

func TimestampToPg(t time.Time) pgtype.Timestamp {
	if t.IsZero() {
		return pgtype.Timestamp{Valid: false}
	}
	return pgtype.Timestamp{Time: t, Valid: true}
}

func TimestampPtrToPg(t *time.Time) pgtype.Timestamp {
	if t == nil || t.IsZero() {
		return pgtype.Timestamp{Valid: false}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}

func DateToPg(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// NumericFromFloat converts a float64 into a pgtype.Numeric.
func NumericFromFloat(f float64) pgtype.Numeric {
	s := fmt.Sprintf("%.4f", f)
	var n pgtype.Numeric
	_ = n.Scan(s)
	return n
}

// NumericFromString converts a decimal string into a pgtype.Numeric.
func NumericFromString(s string) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(s)
	return n
}

// --- read helpers (pgtype → Go) ---

func UUIDFromPg(p pgtype.UUID) string {
	if !p.Valid {
		return ""
	}
	return uuid.UUID(p.Bytes).String()
}

func TextFromPg(p pgtype.Text) string {
	if !p.Valid {
		return ""
	}
	return p.String
}

func BoolFromPg(p pgtype.Bool) bool {
	return p.Valid && p.Bool
}

func Int4FromPg(p pgtype.Int4) int32 {
	if !p.Valid {
		return 0
	}
	return p.Int32
}

func Int8FromPg(p pgtype.Int8) int64 {
	if !p.Valid {
		return 0
	}
	return p.Int64
}

func TimestampFromPg(p pgtype.Timestamp) *time.Time {
	if !p.Valid {
		return nil
	}
	t := p.Time
	return &t
}

func NumericToFloat(p pgtype.Numeric) float64 {
	if !p.Valid {
		return 0
	}
	if p.NaN || p.InfinityModifier != pgtype.Finite {
		return 0
	}
	// Convert the integer mantissa + exponent into float64.
	if p.Int == nil {
		return 0
	}
	f, _ := new(big.Float).SetInt(p.Int).Float64()
	if p.Exp == 0 {
		return f
	}
	scale := bigPow10(p.Exp)
	return f * scale
}

func bigPow10(exp int32) float64 {
	f := 1.0
	if exp >= 0 {
		for i := int32(0); i < exp; i++ {
			f *= 10
		}
		return f
	}
	for i := int32(0); i < -exp; i++ {
		f /= 10
	}
	return f
}

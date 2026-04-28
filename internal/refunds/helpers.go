package refunds

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

// decodeJSON safely decodes the bytes form of a jsonb column into an any.
// Used by ListPayments to expose the refund block in metadata to the UI.
func decodeJSON(raw []byte) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func uuidStringOrEmpty(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuidString(u.Bytes)
}

func uuidString(b [16]byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	pos := 0
	for i, by := range b {
		out[pos] = hex[by>>4]
		out[pos+1] = hex[by&0x0f]
		pos += 2
		switch i {
		case 3, 5, 7, 9:
			out[pos] = '-'
			pos++
		}
	}
	return string(out)
}

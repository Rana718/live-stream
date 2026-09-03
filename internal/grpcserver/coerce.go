package grpcserver

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Short aliases for the sqlc column wrappers, used across adapters.
type (
	pgText = pgtype.Text
	pgUUID = pgtype.UUID
	pgTs   = pgtype.Timestamptz
	pgDate = pgtype.Date
	pgInt4 = pgtype.Int4
	pgInt8 = pgtype.Int8
)

// Coercion helpers for the handful of service methods that still return
// map[string]any (internal/tests attempts, parts of platformadmin/billing).
// Each tolerates the pgtype wrappers, bare Go types, pointers and nil.

func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case uuid.UUID:
		return x.String()
	case pgtype.UUID:
		if !x.Valid {
			return ""
		}
		return uuid.UUID(x.Bytes).String()
	case pgtype.Text:
		return x.String
	case *string:
		if x == nil {
			return ""
		}
		return *x
	default:
		return ""
	}
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case pgtype.Float8:
		if !x.Valid {
			return 0
		}
		return x.Float64
	case pgtype.Numeric:
		f, _ := x.Float64Value()
		return f.Float64
	default:
		return 0
	}
}

func asInt(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case pgtype.Int4:
		if !x.Valid {
			return 0
		}
		return int64(x.Int32)
	case pgtype.Int8:
		if !x.Valid {
			return 0
		}
		return x.Int64
	default:
		return 0
	}
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case pgtype.Bool:
		return x.Valid && x.Bool
	case *bool:
		return x != nil && *x
	default:
		return false
	}
}

// tsFromPgtz / tsFromPgts convert sqlc pgtype timestamp columns to proto.
func tsFromPgtz(t pgtype.Timestamptz) *timestamppb.Timestamp {
	if !t.Valid {
		return nil
	}
	return timestamppb.New(t.Time)
}

func tsFromPgts(t pgtype.Timestamp) *timestamppb.Timestamp {
	if !t.Valid {
		return nil
	}
	return timestamppb.New(t.Time)
}

func asTimestamp(v any) *timestamppb.Timestamp {
	switch x := v.(type) {
	case time.Time:
		if x.IsZero() {
			return nil
		}
		return timestamppb.New(x)
	case *time.Time:
		if x == nil || x.IsZero() {
			return nil
		}
		return timestamppb.New(*x)
	case pgtype.Timestamptz:
		if !x.Valid {
			return nil
		}
		return timestamppb.New(x.Time)
	case pgtype.Timestamp:
		if !x.Valid {
			return nil
		}
		return timestamppb.New(x.Time)
	default:
		return nil
	}
}

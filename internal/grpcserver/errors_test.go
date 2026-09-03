package grpcserver

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"nil", nil, codes.OK},
		{"no rows", pgx.ErrNoRows, codes.NotFound},
		{"deadline", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"canceled", context.Canceled, codes.Canceled},
		{"unique", &pgconn.PgError{Code: "23505"}, codes.AlreadyExists},
		{"fk", &pgconn.PgError{Code: "23503"}, codes.InvalidArgument},
		{"check", &pgconn.PgError{Code: "23514"}, codes.InvalidArgument},
		{"serialization", &pgconn.PgError{Code: "40001"}, codes.Aborted},
		{"rls deny", &pgconn.PgError{Code: "42501"}, codes.PermissionDenied},
		{"passthrough status", status.Error(codes.PermissionDenied, "x"), codes.PermissionDenied},
		{"wrapped no rows", errors.New("query: " + pgx.ErrNoRows.Error()), codes.Internal}, // string, not wrapped — stays Internal
		{"unknown", errors.New("boom"), codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := status.Code(toStatus(tc.err))
			if got != tc.want {
				t.Fatalf("toStatus(%v) = %s, want %s", tc.err, got, tc.want)
			}
		})
	}
}

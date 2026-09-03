package grpcserver

import (
	"context"
	"errors"
	"log/slog"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toStatus maps an internal/service-layer error to a gRPC status. It is the
// single place adapters translate errors — a handler never returns a raw
// err.Error() to the client for an Internal failure (it is logged instead).
//
// Business errors that services report as plain fmt.Errorf strings cannot be
// classified here; adapters pre-map those known cases before calling toStatus.
func toStatus(err error) error {
	if err == nil {
		return nil
	}

	// Already a gRPC status (an adapter pre-mapped it) — pass through.
	if _, ok := status.FromError(err); ok {
		return err
	}

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return status.Error(codes.NotFound, "not found")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "canceled")
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return status.Error(codes.AlreadyExists, "already exists")
		case "23503": // foreign_key_violation
			return status.Error(codes.InvalidArgument, "referenced record does not exist")
		case "23514", "23502": // check_violation / not_null_violation
			return status.Error(codes.InvalidArgument, "invalid field value")
		case "40001", "40P01": // serialization_failure / deadlock_detected
			return status.Error(codes.Aborted, "write conflict, retry")
		case "42501": // insufficient_privilege — an RLS policy denied the row
			return status.Error(codes.PermissionDenied, "not permitted")
		}
	}

	var verr validator.ValidationErrors
	if errors.As(err, &verr) {
		return status.Error(codes.InvalidArgument, verr.Error())
	}

	slog.Error("grpc unhandled error", "err", err)
	return status.Error(codes.Internal, "internal error")
}

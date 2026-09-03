package grpcserver

import (
	"time"

	"live-platform/internal/middleware"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// validate runs go-playground/validator over a mapped internal request struct
// (the same structs + `validate:` tags the REST handlers use) and returns
// InvalidArgument on failure.
func validate(v any) error {
	if err := middleware.ValidateStruct(v); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return nil
}

// tsFromTime converts a *time.Time to a proto Timestamp (nil-safe).
func tsFromTime(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
}

// tsFromValue converts a time.Time to a proto Timestamp (zero → nil).
func tsFromValue(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

package middleware

import "testing"

type sampleReq struct {
	Email string `validate:"required,email"`
	Age   int    `validate:"gte=0,lte=150"`
}

func TestValidatorRejectsInvalid(t *testing.T) {
	if err := ValidateStruct(&sampleReq{Email: "not-an-email", Age: -1}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidatorAcceptsValid(t *testing.T) {
	if err := ValidateStruct(&sampleReq{Email: "user@example.com", Age: 20}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

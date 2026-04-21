package utils

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

const testSecret = "test-secret-longer-than-default"

func TestGenerateAndValidateAccessToken(t *testing.T) {
	uid := uuid.New()
	tok, err := GenerateAccessToken(uid, "u@test.local", "student", testSecret, 5*time.Minute)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	claims, err := ValidateToken(tok, testSecret)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if claims.UserID != uid {
		t.Errorf("got userID %s, want %s", claims.UserID, uid)
	}
	if claims.Email != "u@test.local" {
		t.Errorf("got email %q, want u@test.local", claims.Email)
	}
	if claims.Role != "student" {
		t.Errorf("got role %q, want student", claims.Role)
	}
}

func TestValidateTokenRejectsWrongSecret(t *testing.T) {
	tok, _ := GenerateAccessToken(uuid.New(), "x@x", "student", testSecret, time.Minute)
	if _, err := ValidateToken(tok, "different-secret"); err == nil {
		t.Fatal("wrong secret must fail validation")
	}
}

func TestValidateTokenRejectsExpired(t *testing.T) {
	tok, _ := GenerateAccessToken(uuid.New(), "x@x", "student", testSecret, -time.Second)
	if _, err := ValidateToken(tok, testSecret); err == nil {
		t.Fatal("expired token must fail validation")
	}
}

func TestRefreshTokenRoundtrip(t *testing.T) {
	uid := uuid.New()
	tok, err := GenerateRefreshToken(uid, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("generate refresh failed: %v", err)
	}
	claims, err := ValidateRefreshToken(tok, testSecret)
	if err != nil {
		t.Fatalf("validate refresh failed: %v", err)
	}
	if claims.Subject != uid.String() {
		t.Errorf("got subject %s, want %s", claims.Subject, uid.String())
	}
}

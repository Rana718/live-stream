package utils

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

const testSecret = "test-secret-longer-than-default"

func TestGenerateAndValidateAccessToken(t *testing.T) {
	uid := uuid.New()
	tid := uuid.New()
	tok, err := GenerateAccessToken(uid, "u@test.local", "student", tid, 3, testSecret, 5*time.Minute)
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
	if claims.TenantID != tid {
		t.Errorf("got tenantID %s, want %s", claims.TenantID, tid)
	}
	if claims.Ver != 3 {
		t.Errorf("got ver %d, want 3", claims.Ver)
	}
}

func TestValidateTokenRejectsWrongSecret(t *testing.T) {
	tok, _ := GenerateAccessToken(uuid.New(), "x@x", "student", uuid.New(), 0, testSecret, time.Minute)
	if _, err := ValidateToken(tok, "different-secret"); err == nil {
		t.Fatal("wrong secret must fail validation")
	}
}

func TestValidateTokenRejectsExpired(t *testing.T) {
	tok, _ := GenerateAccessToken(uuid.New(), "x@x", "student", uuid.New(), 0, testSecret, -time.Second)
	if _, err := ValidateToken(tok, testSecret); err == nil {
		t.Fatal("expired token must fail validation")
	}
}

func TestOpaqueTokenHashing(t *testing.T) {
	raw, hash, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken: %v", err)
	}
	if raw == "" || hash == "" || raw == hash {
		t.Fatalf("bad token/hash: %q / %q", raw, hash)
	}
	if HashToken(raw) != hash {
		t.Fatal("HashToken not deterministic with NewOpaqueToken")
	}
	if len(hash) != 64 {
		t.Fatalf("hash len %d, want 64 (hex sha256)", len(hash))
	}
}

package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims is the access-token payload. Role is resolved from tenant_users for
// the chosen tenant at mint time (schema v2 — users has no role column). Ver
// mirrors users.token_version so deactivating a user / rotating their
// password / changing their role invalidates every outstanding access token
// on its next use.
type Claims struct {
	UserID   uuid.UUID `json:"uid"`
	Email    string    `json:"email,omitempty"`
	Role     string    `json:"role"`
	TenantID uuid.UUID `json:"tid"`
	Ver      int32     `json:"ver"`
	jwt.RegisteredClaims
}

// GenerateAccessToken issues a tenant-scoped access token.
func GenerateAccessToken(userID uuid.UUID, email, role string, tenantID uuid.UUID, ver int32, secret string, expiry time.Duration) (string, error) {
	claims := Claims{
		UserID:   userID,
		Email:    email,
		Role:     role,
		TenantID: tenantID,
		Ver:      ver,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID.String(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ValidateToken parses and verifies an access token.
func ValidateToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

// NewOpaqueToken returns a fresh refresh-token secret and its storage hash.
// The raw value goes to the client once; only HashToken(raw) is persisted in
// refresh_tokens.token_hash.
func NewOpaqueToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashToken(raw), nil
}

// HashToken is the deterministic hash stored for an opaque token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

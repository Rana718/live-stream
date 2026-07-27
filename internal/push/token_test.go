package push

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func testServiceAccountJSON(t *testing.T, tokenURI string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: mustMarshalPKCS8(t, key),
	})
	sa := serviceAccountKey{
		ProjectID:   "test-project",
		PrivateKey:  string(pemBlock),
		ClientEmail: "svc@test-project.iam.gserviceaccount.com",
		TokenURI:    tokenURI,
	}
	raw, err := json.Marshal(sa)
	if err != nil {
		t.Fatalf("marshal service account: %v", err)
	}
	return raw
}

func mustMarshalPKCS8(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	return b
}

func TestNewTokenSourceRejectsIncompleteKey(t *testing.T) {
	_, err := newTokenSource([]byte(`{"project_id":"p"}`), http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for missing client_email/private_key")
	}
}

func TestNewTokenSourceDefaultsTokenURI(t *testing.T) {
	raw := testServiceAccountJSON(t, "")
	ts, err := newTokenSource(raw, http.DefaultClient)
	if err != nil {
		t.Fatalf("newTokenSource: %v", err)
	}
	if ts.key.TokenURI != "https://oauth2.googleapis.com/token" {
		t.Fatalf("expected default token URI, got %q", ts.key.TokenURI)
	}
}

func TestSignedJWTHasThreeSegments(t *testing.T) {
	raw := testServiceAccountJSON(t, "https://oauth2.googleapis.com/token")
	ts, err := newTokenSource(raw, http.DefaultClient)
	if err != nil {
		t.Fatalf("newTokenSource: %v", err)
	}
	jwt, err := ts.signedJWT()
	if err != nil {
		t.Fatalf("signedJWT: %v", err)
	}
	if got := strings.Count(jwt, "."); got != 2 {
		t.Fatalf("expected header.claims.signature (2 dots), got %d dots", got)
	}
}

func TestTokenFetchesAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Fatalf("unexpected grant_type: %s", r.Form.Get("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-abc","expires_in":3600}`))
	}))
	defer srv.Close()

	raw := testServiceAccountJSON(t, srv.URL)
	ts, err := newTokenSource(raw, srv.Client())
	if err != nil {
		t.Fatalf("newTokenSource: %v", err)
	}

	ctx := context.Background()
	tok1, err := ts.Token(ctx)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok1 != "tok-abc" {
		t.Fatalf("expected tok-abc, got %s", tok1)
	}

	tok2, err := ts.Token(ctx)
	if err != nil {
		t.Fatalf("Token (cached): %v", err)
	}
	if tok2 != "tok-abc" || calls != 1 {
		t.Fatalf("expected cached token without a second HTTP call, got %d calls", calls)
	}
}

func TestTokenRefetchesAfterExpiry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		// expires_in=1 with a 2-minute refresh buffer means the very next
		// call should treat this as already-expired and refetch.
		_, _ = w.Write([]byte(`{"access_token":"tok-` + strconv.Itoa(calls) + `","expires_in":1}`))
	}))
	defer srv.Close()

	raw := testServiceAccountJSON(t, srv.URL)
	ts, err := newTokenSource(raw, srv.Client())
	if err != nil {
		t.Fatalf("newTokenSource: %v", err)
	}

	ctx := context.Background()
	if _, err := ts.Token(ctx); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if _, err := ts.Token(ctx); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected a refetch once cached token is within the refresh buffer, got %d calls", calls)
	}
}

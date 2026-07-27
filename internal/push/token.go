package push

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// fcmMessagingScope is the OAuth2 scope FCM HTTP v1 requires.
const fcmMessagingScope = "https://www.googleapis.com/auth/firebase.messaging"

// serviceAccountKey mirrors the fields we need from a Firebase/GCP
// service-account JSON key file. The rest (private_key_id, client_id, …)
// isn't needed for the JWT-bearer flow.
type serviceAccountKey struct {
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

// tokenSource mints and caches Google OAuth2 access tokens via the
// JWT-bearer grant (RFC 7523) so we don't need golang.org/x/oauth2/google
// as a dependency — everything here is stdlib crypto, matching the rest
// of this package's provider clients.
type tokenSource struct {
	key    serviceAccountKey
	rsaKey *rsa.PrivateKey
	http   *http.Client

	mu     sync.Mutex
	cached string
	expiry time.Time
}

func newTokenSource(rawJSON []byte, httpClient *http.Client) (*tokenSource, error) {
	var key serviceAccountKey
	if err := json.Unmarshal(rawJSON, &key); err != nil {
		return nil, fmt.Errorf("parse service account json: %w", err)
	}
	if key.ClientEmail == "" || key.PrivateKey == "" || key.ProjectID == "" {
		return nil, errors.New("service account json missing client_email/private_key/project_id")
	}
	if key.TokenURI == "" {
		key.TokenURI = "https://oauth2.googleapis.com/token"
	}
	rsaKey, err := parseRSAPrivateKey(key.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return &tokenSource{key: key, rsaKey: rsaKey, http: httpClient}, nil
}

func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("invalid PEM block")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return rsaKey, nil
}

// Token returns a cached access token, minting a new one via the token
// endpoint if the cached one expires within 2 minutes.
func (t *tokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	if t.cached != "" && time.Now().Before(t.expiry.Add(-2*time.Minute)) {
		tok := t.cached
		t.mu.Unlock()
		return tok, nil
	}
	t.mu.Unlock()

	jwt, err := t.signedJWT()
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", jwt)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.key.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode >= 400 || out.AccessToken == "" {
		return "", fmt.Errorf("token exchange failed: %s %s", out.Error, out.ErrorDesc)
	}

	t.mu.Lock()
	t.cached = out.AccessToken
	t.expiry = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	t.mu.Unlock()
	return out.AccessToken, nil
}

// signedJWT builds a self-signed assertion per RFC 7523 §3: header.claims
// signed with the service account's RSA private key.
func (t *tokenSource) signedJWT() (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   t.key.ClientEmail,
		"scope": fcmMessagingScope,
		"aud":   t.key.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := b64(headerJSON) + "." + b64(claimsJSON)

	hashed := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, t.rsaKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return unsigned + "." + b64(sig), nil
}

func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

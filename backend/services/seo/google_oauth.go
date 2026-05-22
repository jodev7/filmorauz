package seo

import (
	"bytes"
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
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// googleServiceAccount is the JSON layout that gcloud generates when you
// download a service-account key. Only the fields we need are parsed.
type googleServiceAccount struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// googleTokenSource fetches and caches a Google OAuth2 access token for a
// service account, signing the assertion as RS256 ourselves so we don't
// pull in the golang.org/x/oauth2 dependency.
type googleTokenSource struct {
	account googleServiceAccount
	scopes  []string
	key     *rsa.PrivateKey

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newGoogleTokenSource(credentialsPath string, scopes ...string) (*googleTokenSource, error) {
	credentialsPath = strings.TrimSpace(credentialsPath)
	if credentialsPath == "" {
		return nil, errors.New("google: credentials path is empty")
	}
	raw, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("google: read credentials: %w", err)
	}
	var account googleServiceAccount
	if err := json.Unmarshal(raw, &account); err != nil {
		return nil, fmt.Errorf("google: parse credentials: %w", err)
	}
	if account.ClientEmail == "" || account.PrivateKey == "" {
		return nil, errors.New("google: credentials missing client_email or private_key")
	}
	if account.TokenURI == "" {
		account.TokenURI = "https://oauth2.googleapis.com/token"
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if block == nil {
		return nil, errors.New("google: private_key is not PEM-encoded")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Some keys are PKCS1.
		if k, err2 := x509.ParsePKCS1PrivateKey(block.Bytes); err2 == nil {
			parsed = k
		} else {
			return nil, fmt.Errorf("google: parse private_key: %w", err)
		}
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("google: private_key is not RSA")
	}
	return &googleTokenSource{account: account, scopes: scopes, key: rsaKey}, nil
}

func (g *googleTokenSource) Token() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token != "" && time.Now().Before(g.expiresAt.Add(-60*time.Second)) {
		return g.token, nil
	}

	now := time.Now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   g.account.ClientEmail,
		"scope": strings.Join(g.scopes, " "),
		"aud":   g.account.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(60 * time.Minute).Unix(),
	}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("google: sign jwt: %w", err)
	}
	assertion := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequest(http.MethodPost, g.account.TokenURI, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google: token endpoint status=%d body=%s", resp.StatusCode, truncate(string(body), 300))
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("google: parse token: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("google: empty access_token from token endpoint")
	}
	g.token = tokenResp.AccessToken
	g.expiresAt = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return g.token, nil
}

func (g *googleTokenSource) Email() string { return g.account.ClientEmail }

// GoogleTokenSource is the public-facing handle returned by
// NewGoogleTokenSourceForTool. Used by cmd/seo-verify-sa so the verification
// CLI can reuse the same RS256 signing path as the in-process services.
type GoogleTokenSource struct{ inner *googleTokenSource }

// NewGoogleTokenSourceForTool exposes the package-private constructor for
// out-of-process tools. Not intended for runtime services — they should keep
// using the unexported helpers.
func NewGoogleTokenSourceForTool(credentialsPath string, scopes ...string) (*GoogleTokenSource, error) {
	ts, err := newGoogleTokenSource(credentialsPath, scopes...)
	if err != nil {
		return nil, err
	}
	return &GoogleTokenSource{inner: ts}, nil
}

func (g *GoogleTokenSource) Token() (string, error) { return g.inner.Token() }
func (g *GoogleTokenSource) Email() string          { return g.inner.Email() }

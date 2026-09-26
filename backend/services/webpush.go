package services

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Minimal Web Push sender (RFC 8030) with aes128gcm payload encryption
// (RFC 8291) and VAPID authentication (RFC 8292), stdlib only.

// PushSubscription is what the browser's PushManager.subscribe() returns.
type PushSubscription struct {
	Endpoint string `json:"endpoint" bson:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh" bson:"p256dh"`
		Auth   string `json:"auth" bson:"auth"`
	} `json:"keys" bson:"keys"`
}

// VAPIDKeys is the server's application-server key pair.
type VAPIDKeys struct {
	private *ecdsa.PrivateKey
	// PublicKey is the base64url uncompressed point handed to the browser.
	PublicKey string
	Subject   string // "mailto:..." or an https URL
}

// ErrPushGone means the subscription expired or was revoked (404/410): the
// caller should delete it.
var ErrPushGone = errors.New("push subscription gone")

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// decodeB64 accepts both padded/unpadded, standard/url alphabets.
func decodeB64(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	s = strings.NewReplacer("+", "-", "/", "_").Replace(s)
	return base64.RawURLEncoding.DecodeString(s)
}

// ParseVAPIDPrivateKey loads a base64url raw 32-byte P-256 private scalar
// (the format `web-push generate-vapid-keys` prints).
func ParseVAPIDPrivateKey(privB64, subject string) (*VAPIDKeys, error) {
	d, err := decodeB64(privB64)
	if err != nil || len(d) != 32 {
		return nil, fmt.Errorf("VAPID private key must be 32 bytes base64url")
	}
	curve := elliptic.P256()
	priv := &ecdsa.PrivateKey{D: new(big.Int).SetBytes(d)}
	priv.PublicKey.Curve = curve
	priv.PublicKey.X, priv.PublicKey.Y = curve.ScalarBaseMult(d)
	ecdhPriv, err := priv.ECDH()
	if err != nil {
		return nil, err
	}
	if subject == "" {
		subject = "mailto:admin@filmorauz.net"
	}
	return &VAPIDKeys{private: priv, PublicKey: b64(ecdhPriv.PublicKey().Bytes()), Subject: subject}, nil
}

// GenerateVAPIDKeys makes a new key pair (used to print keys for .env).
func GenerateVAPIDKeys() (privB64, pubB64 string, err error) {
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return b64(k.Bytes()), b64(k.PublicKey().Bytes()), nil
}

// vapidJWT signs the ES256 token for the push service's origin.
func (v *VAPIDKeys) vapidJWT(endpoint string, now time.Time) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("bad endpoint")
	}
	header := b64([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, _ := json.Marshal(map[string]interface{}{
		"aud": u.Scheme + "://" + u.Host,
		"exp": now.Add(12 * time.Hour).Unix(),
		"sub": v.Subject,
	})
	signing := header + "." + b64(claims)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, v.private, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + b64(sig), nil
}

// encryptPayload implements RFC 8291 (aes128gcm, single record). salt and
// the ephemeral key are parameters so tests can use the RFC vectors.
func encryptPayload(plaintext []byte, sub PushSubscription, salt []byte, asPriv *ecdh.PrivateKey) ([]byte, error) {
	uaPubBytes, err := decodeB64(sub.Keys.P256dh)
	if err != nil {
		return nil, fmt.Errorf("bad p256dh")
	}
	authSecret, err := decodeB64(sub.Keys.Auth)
	if err != nil || len(authSecret) == 0 {
		return nil, fmt.Errorf("bad auth")
	}
	uaPub, err := ecdh.P256().NewPublicKey(uaPubBytes)
	if err != nil {
		return nil, fmt.Errorf("bad p256dh point")
	}
	ecdhSecret, err := asPriv.ECDH(uaPub)
	if err != nil {
		return nil, err
	}
	asPub := asPriv.PublicKey().Bytes()

	keyInfo := append(append([]byte("WebPush: info\x00"), uaPubBytes...), asPub...)
	prkKey, err := hkdf.Extract(sha256.New, ecdhSecret, authSecret)
	if err != nil {
		return nil, err
	}
	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	record := append(append([]byte{}, plaintext...), 0x02) // last-record delimiter
	ct := gcm.Seal(nil, nonce, record, nil)

	var buf bytes.Buffer
	buf.Write(salt)
	rs := make([]byte, 4)
	binary.BigEndian.PutUint32(rs, 4096)
	buf.Write(rs)
	buf.WriteByte(byte(len(asPub)))
	buf.Write(asPub)
	buf.Write(ct)
	return buf.Bytes(), nil
}

// WebPusher sends notifications to browser push services.
type WebPusher struct {
	keys   *VAPIDKeys
	client *http.Client
}

func NewWebPusher(keys *VAPIDKeys) *WebPusher {
	return &WebPusher{keys: keys, client: &http.Client{Timeout: 10 * time.Second}}
}

// PublicKey is the VAPID key the browser needs for subscribe().
func (p *WebPusher) PublicKey() string {
	if p == nil || p.keys == nil {
		return ""
	}
	return p.keys.PublicKey
}

// Send delivers one payload (≤ ~3 KB) to one subscription.
func (p *WebPusher) Send(sub PushSubscription, payload []byte, ttl time.Duration) error {
	if p == nil || p.keys == nil {
		return fmt.Errorf("web push not configured")
	}
	if !strings.HasPrefix(sub.Endpoint, "https://") {
		return fmt.Errorf("endpoint must be https")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	asPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	body, err := encryptPayload(payload, sub, salt, asPriv)
	if err != nil {
		return err
	}
	token, err := p.keys.vapidJWT(sub.Endpoint, time.Now())
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(int(ttl.Seconds())))
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", "vapid t="+token+", k="+p.keys.PublicKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrPushGone
	case resp.StatusCode >= 300:
		return fmt.Errorf("push service returned %d", resp.StatusCode)
	}
	return nil
}

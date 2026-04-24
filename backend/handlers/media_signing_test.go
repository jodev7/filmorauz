package handlers

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildMediaTokenIncludesBindingClaims(t *testing.T) {
	token, _ := buildMediaToken("secret", "/videos/movies/demo/master.m3u8", 60, mediaTokenOptions{
		ClientIP: "203.0.113.10",
		UAHash:   hashUserAgent("Mozilla/5.0 Test Browser"),
	})

	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}

	parts := strings.Split(string(decoded), "\n")
	if len(parts) != 5 {
		t.Fatalf("expected 5 token fields, got %d", len(parts))
	}
	if parts[0] != "/videos/movies/demo/" {
		t.Fatalf("unexpected scope: %q", parts[0])
	}
	if parts[3] != "203.0.113.10" {
		t.Fatalf("unexpected client ip: %q", parts[3])
	}
	if parts[4] == "" {
		t.Fatal("expected ua hash to be present")
	}

	expectedSig := signMediaScope("secret", parts[0], parts[1], parts[3], parts[4])
	if parts[2] != expectedSig {
		t.Fatalf("unexpected signature: got %q want %q", parts[2], expectedSig)
	}
}

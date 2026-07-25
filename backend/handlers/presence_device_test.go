package handlers

import (
	"strings"
	"testing"
)

func TestParseDevice(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want string
	}{
		{
			"chrome windows",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36",
			"Chrome · Windows",
		},
		{
			"safari iphone",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			"Safari · iPhone",
		},
		{
			"chrome android",
			"Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36",
			"Chrome · Android",
		},
		{
			"edge wins over chrome token",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36 Edg/120.0",
			"Edge · Windows",
		},
		{"empty", "", "Noma'lum"},
		{"garbage", "curl/8.0", "Noma'lum"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseDevice(tc.ua); got != tc.want {
				t.Fatalf("parseDevice(%q) = %q, want %q", tc.ua, got, tc.want)
			}
		})
	}
}

func TestToSessionActivity(t *testing.T) {
	if got := toSessionActivity(nil); got != nil {
		t.Fatalf("nil payload should stay nil, got %+v", got)
	}
	if got := toSessionActivity(&heartbeatContent{Type: "homepage", Title: "x"}); got != nil {
		t.Fatalf("unknown type should be rejected, got %+v", got)
	}
	if got := toSessionActivity(&heartbeatContent{Type: "movie"}); got != nil {
		t.Fatalf("empty payload should be rejected, got %+v", got)
	}

	// Absolute/protocol-relative URLs are dropped so nobody can plant an
	// off-site link in the admin panel.
	got := toSessionActivity(&heartbeatContent{
		Type: "MOVIE", ContentID: "m1", Title: " Interstellar ", Slug: "interstellar",
		URL: "https://evil.example/x",
	})
	if got == nil || got.Type != "movie" || got.Title != "Interstellar" || got.URL != "" {
		t.Fatalf("unexpected activity: %+v", got)
	}
	if got := toSessionActivity(&heartbeatContent{Type: "movie", Slug: "a", URL: "//evil.example"}); got.URL != "" {
		t.Fatalf("protocol-relative URL kept: %q", got.URL)
	}
	if got := toSessionActivity(&heartbeatContent{Type: "episode", Slug: "a", URL: "/episode/1"}); got.URL != "/episode/1" {
		t.Fatalf("relative URL dropped: %q", got.URL)
	}

	long := toSessionActivity(&heartbeatContent{Type: "movie", Title: strings.Repeat("a", 500)})
	if len(long.Title) != maxActivityField {
		t.Fatalf("title len = %d, want %d", len(long.Title), maxActivityField)
	}
}

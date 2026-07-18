package handlers

import "testing"

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

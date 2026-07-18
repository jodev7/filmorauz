package handlers

import "testing"

func TestPickBestDownloadQuality(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"prefers 1080p", []string{"360p", "480p", "720p", "1080p"}, "1080p"},
		{"falls back to 720p", []string{"360p", "480p", "720p"}, "720p"},
		{"highest numeric when no 1080/720", []string{"360p", "480p"}, "480p"},
		{"empty", nil, ""},
		{"case insensitive", []string{"1080P"}, "1080p"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickBestDownloadQuality(tc.in); got != tc.want {
				t.Fatalf("pickBestDownloadQuality(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenditionPlaylistURL(t *testing.T) {
	cases := []struct {
		name    string
		master  string
		quality string
		want    string
	}{
		{
			"index.m3u8 master",
			"https://cdn.example.com/videos/movies/abc/index.m3u8",
			"1080p",
			"https://cdn.example.com/videos/movies/abc/1080p/index.m3u8",
		},
		{
			"legacy master.m3u8",
			"https://cdn.example.com/videos/movies/abc/master.m3u8",
			"720p",
			"https://cdn.example.com/videos/movies/abc/720p/index.m3u8",
		},
		{
			"empty quality returns master unchanged",
			"https://cdn.example.com/videos/movies/abc/index.m3u8",
			"",
			"https://cdn.example.com/videos/movies/abc/index.m3u8",
		},
		{
			"unexpected filename returns master unchanged",
			"https://cdn.example.com/videos/movies/abc/playlist.m3u8",
			"1080p",
			"https://cdn.example.com/videos/movies/abc/playlist.m3u8",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renditionPlaylistURL(tc.master, tc.quality); got != tc.want {
				t.Fatalf("renditionPlaylistURL(%q, %q) = %q, want %q", tc.master, tc.quality, got, tc.want)
			}
		})
	}
}

func TestSanitizeDownloadName(t *testing.T) {
	cases := map[string]string{
		"Interstellar (2014)": "Interstellar_2014",
		"  ":                  "video",
		"кино":                "video", // non-ASCII stripped → fallback
		"my/movie:name":       "mymoviename",
	}
	for in, want := range cases {
		if got := sanitizeDownloadName(in); got != want {
			t.Fatalf("sanitizeDownloadName(%q) = %q, want %q", in, got, want)
		}
	}
}

package services

import "testing"

func TestNormalizeTelegramImageURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "images path",
			raw:  "/images/posters/xxx.webp",
			want: "https://cdn.filmorauz.net/file/filmorauznet/images/posters/xxx.webp",
		},
		{
			name: "legacy media images path",
			raw:  "/media/images/posters/xxx.webp",
			want: "https://cdn.filmorauz.net/file/filmorauznet/images/posters/xxx.webp",
		},
		{
			name: "absolute media url",
			raw:  "https://cdn.filmorauz.net/media/images/posters/xxx.webp",
			want: "https://cdn.filmorauz.net/file/filmorauznet/images/posters/xxx.webp",
		},
		{
			name: "absolute file url unchanged",
			raw:  "https://cdn.filmorauz.net/file/filmorauznet/images/posters/xxx.webp",
			want: "https://cdn.filmorauz.net/file/filmorauznet/images/posters/xxx.webp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeTelegramImageURL(tt.raw)
			if got != tt.want {
				t.Fatalf("NormalizeTelegramImageURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

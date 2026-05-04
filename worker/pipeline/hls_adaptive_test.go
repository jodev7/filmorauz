package pipeline

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetApplicableRenditions(t *testing.T) {
	p := &Pipeline{}

	tests := []struct {
		name   string
		width  int
		height int
		want   []string
	}{
		{name: "1080p source", width: 1920, height: 1080, want: []string{"1080p", "720p", "480p", "360p"}},
		{name: "720p source", width: 1280, height: 720, want: []string{"720p", "480p", "360p"}},
		{name: "480p source", width: 854, height: 480, want: []string{"480p", "360p"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getRenditionNames(p.getApplicableRenditions(tc.width, tc.height))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("getApplicableRenditions(%d, %d) = %v, want %v", tc.width, tc.height, got, tc.want)
			}
		})
	}
}

func TestVerifyExpectedRenditionsFailsWhenMissing(t *testing.T) {
	p := &Pipeline{}
	tmpDir := t.TempDir()
	expected := []RenditionConfig{
		{Name: "1080p", Width: 1920, Height: 1080},
		{Name: "720p", Width: 1280, Height: 720},
	}

	makeRendition := func(name string) {
		dir := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte("#EXTM3U\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "segment_001.ts"), []byte("segment"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	makeRendition("1080p")

	_, _, err := p.verifyExpectedRenditions(tmpDir, expected)
	if err == nil {
		t.Fatal("expected verifyExpectedRenditions to fail when 720p is missing")
	}
}

func TestParseMasterPlaylistVariantPaths(t *testing.T) {
	content := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5300000,RESOLUTION=1920x1080\n1080p/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=3020000,RESOLUTION=1280x720\n720p/index.m3u8\n"
	got := parseMasterPlaylistVariantPaths(content)
	want := []string{"1080p/index.m3u8", "720p/index.m3u8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMasterPlaylistVariantPaths() = %v, want %v", got, want)
	}
}

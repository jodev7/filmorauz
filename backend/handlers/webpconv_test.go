package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"testing"
)

func TestIsWebPConvertibleImage(t *testing.T) {
	cases := map[string]bool{
		"image/png":   true,
		"image/jpeg":  true,
		"image/jpg":   true,
		"IMAGE/PNG":   true,
		" image/png ": true,
		"image/webp":  false, // already target format
		"image/gif":   false, // animation not supported by cwebp
		"video/mp4":   false,
		"":            false,
	}
	for ct, want := range cases {
		if got := isWebPConvertibleImage(ct); got != want {
			t.Errorf("isWebPConvertibleImage(%q) = %v, want %v", ct, got, want)
		}
	}
}

func samplePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode sample png: %v", err)
	}
	return buf.Bytes()
}

func TestConvertToWebP(t *testing.T) {
	if _, err := exec.LookPath(cwebpBinary); err != nil {
		t.Skipf("cwebp not installed: %v", err)
	}

	out, err := convertToWebP(samplePNG(t))
	if err != nil {
		t.Fatalf("convertToWebP returned error: %v", err)
	}
	// WebP files start with the RIFF container header followed by "WEBP".
	if len(out) < 12 || !bytes.Equal(out[0:4], []byte("RIFF")) || !bytes.Equal(out[8:12], []byte("WEBP")) {
		t.Fatalf("output is not a valid WebP file: % x", out[:min(12, len(out))])
	}
}

func TestConvertToWebPEmpty(t *testing.T) {
	if _, err := convertToWebP(nil); err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestConvertToWebPMissingBinary(t *testing.T) {
	orig := cwebpBinary
	cwebpBinary = "cwebp_definitely_not_a_real_binary_xyz"
	defer func() { cwebpBinary = orig }()

	if _, err := convertToWebP(samplePNG(t)); err == nil {
		t.Fatal("expected error when cwebp binary is missing, got nil")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

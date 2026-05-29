package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"testing"
)

// stubWebPEncoder installs a fake WebP encoder for the duration of a test so
// handler tests don't require the cwebp binary to be installed. It prepends the
// RIFF/WEBP container magic so output still looks like a real WebP file.
func stubWebPEncoder(t *testing.T) {
	t.Helper()
	orig := webpEncoder
	webpEncoder = func(data []byte) ([]byte, error) {
		out := []byte("RIFF\x00\x00\x00\x00WEBP")
		return append(out, data...), nil
	}
	t.Cleanup(func() { webpEncoder = orig })
}

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

func TestMaybeConvertImageToWebP(t *testing.T) {
	stubWebPEncoder(t)

	// PNG is converted.
	out, ct, err := maybeConvertImageToWebP([]byte("pngbytes"), "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "image/webp" {
		t.Errorf("content type = %q, want image/webp", ct)
	}
	if !bytes.HasPrefix(out, []byte("RIFF")) {
		t.Errorf("output not webp: % x", out[:min(8, len(out))])
	}

	// Already-webp and gif pass through untouched.
	for _, ct := range []string{"image/webp", "image/gif", "video/mp4"} {
		in := []byte("original")
		out, gotCT, err := maybeConvertImageToWebP(in, ct)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", ct, err)
		}
		if gotCT != ct || !bytes.Equal(out, in) {
			t.Errorf("%q was modified: ct=%q", ct, gotCT)
		}
	}
}

func TestMaybeConvertImageToWebPError(t *testing.T) {
	orig := webpEncoder
	webpEncoder = func([]byte) ([]byte, error) { return nil, errStub }
	defer func() { webpEncoder = orig }()

	if _, _, err := maybeConvertImageToWebP([]byte("x"), "image/jpeg"); err == nil {
		t.Fatal("expected error to propagate, got nil")
	}
}

var errStub = errStubType("boom")

type errStubType string

func (e errStubType) Error() string { return string(e) }

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

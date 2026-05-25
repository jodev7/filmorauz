package handlers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// webpQuality is the cwebp quality factor (0-100) used for poster/backdrop
// conversion. 80 is a good balance of size vs. visual quality for photos.
const webpQuality = 80

// cwebpBinary is the name of the libwebp encoder binary. Overridable in tests.
var cwebpBinary = "cwebp"

// isWebPConvertibleImage reports whether the given content type is an image we
// convert to WebP. Animated GIFs are intentionally excluded (cwebp does not
// handle animation; gif2webp would be required), as is image/webp itself
// (already in target format).
func isWebPConvertibleImage(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg", "image/png":
		return true
	}
	return false
}

// convertToWebP encodes the given image bytes to lossy WebP using the cwebp
// binary. It returns an error if cwebp is missing or the conversion fails;
// callers must treat a conversion failure as a hard error (no silent fallback
// to the original format).
func convertToWebP(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}

	inPath, err := writeTempFile(data, "webpconv_in_")
	if err != nil {
		return nil, err
	}
	defer os.Remove(inPath)

	outPath := inPath + ".webp"
	defer os.Remove(outPath)

	var stderr bytes.Buffer
	cmd := exec.Command(cwebpBinary, "-quiet", "-q", strconv.Itoa(webpQuality), inPath, "-o", outPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cwebp failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cwebp output: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cwebp produced empty output")
	}
	return out, nil
}

// writeTempFile writes data to a uniquely named temp file and returns its path.
func writeTempFile(data []byte, prefix string) (string, error) {
	f, err := os.CreateTemp("", prefix+strconv.FormatInt(time.Now().UnixNano(), 10)+"_*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	return f.Name(), nil
}

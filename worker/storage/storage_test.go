package storage

import "testing"

func TestEncodeB2InfoValueEscapesCommaAndSpace(t *testing.T) {
	got := encodeB2InfoValue("public, max-age=604800")
	want := "public%2C%20max-age%3D604800"
	if got != want {
		t.Fatalf("encodeB2InfoValue() = %q, want %q", got, want)
	}
}

func TestSha1Hex(t *testing.T) {
	got := sha1Hex([]byte("filmorauz"))
	want := "71575cdf8f4554b5487b92df2e200a696ad93dea"
	if got != want {
		t.Fatalf("sha1Hex() = %q, want %q", got, want)
	}
}

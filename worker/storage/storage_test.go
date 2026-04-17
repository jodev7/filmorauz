package storage

import "testing"

func TestEncodeB2InfoValueEscapesCommaAndSpace(t *testing.T) {
	got := encodeB2InfoValue("public, max-age=604800")
	want := "public%2C%20max-age%3D604800"
	if got != want {
		t.Fatalf("encodeB2InfoValue() = %q, want %q", got, want)
	}
}

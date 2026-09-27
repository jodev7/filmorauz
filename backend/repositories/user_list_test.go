package repositories

import (
	"strings"
	"testing"
)

func TestCleanListText(t *testing.T) {
	title, desc, err := CleanListText("  Oilaviy   kechalar ", "  tavsif ")
	if err != nil || title != "Oilaviy kechalar" || desc != "tavsif" {
		t.Fatalf("got %q %q %v", title, desc, err)
	}
	if _, _, err := CleanListText("   ", ""); err != ErrListTitle {
		t.Fatalf("empty title: %v", err)
	}
	if _, _, err := CleanListText(strings.Repeat("я", 61), ""); err != ErrListTitle {
		t.Fatalf("long title: %v", err)
	}
	if _, _, err := CleanListText("ok", strings.Repeat("a", 301)); err != ErrListDesc {
		t.Fatalf("long desc: %v", err)
	}
}

func TestShareSlugAndNames(t *testing.T) {
	s := newShareSlug()
	if len(s) != 8 || strings.Trim(s, slugAlphabet) != "" {
		t.Fatalf("bad slug %q", s)
	}
	if got := firstValidName(".", " - ", "", "ali"); got != "ali" {
		t.Fatalf("got %q", got)
	}
}

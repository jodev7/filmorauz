package repositories

import (
	"testing"

	"github.com/filmorauz/backend/models"
)

func TestCreditsScore(t *testing.T) {
	a := models.Movie{Cast: []string{"Tom Hanks", "Meg Ryan"}, Director: "Nora Ephron"}
	b := models.Movie{Cast: []string{"tom hanks ", "Bill Pullman"}, Director: "nora ephron"}
	if got := creditsScore(a, b); got != 10 {
		t.Fatalf("got %d want 10", got)
	}
	if got := creditsScore(models.Movie{}, b); got != 0 {
		t.Fatalf("empty current should score 0, got %d", got)
	}
	if similarCandidateQuery(models.Movie{}) != nil {
		t.Fatal("no genre/cast/director → nil filter")
	}
}

func TestExactNameRegex(t *testing.T) {
	rx := exactNameRegex("  Tom   Hanks.* ")
	if rx.Pattern != `^\s*Tom\s+Hanks\.\*\s*$` || rx.Options != "i" {
		t.Fatalf("unexpected %q", rx.Pattern)
	}
}

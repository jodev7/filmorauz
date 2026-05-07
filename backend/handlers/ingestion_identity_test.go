package handlers

import "testing"

func TestIdentityConfidenceHighForExactMatch(t *testing.T) {
	selected := importIdentitySnapshot{
		Source:    "asilmedia",
		SourceID:  "9140",
		DetailURL: "https://asilmedia.org/9140-interstellar-uzbek-tarjima.html",
		Title:     "Interstellar",
		Year:      2014,
		Type:      "movie",
		Poster:    "https://asilmedia.org/poster.jpg",
	}
	fetched := selected
	got := identityConfidence(selected, fetched)
	if got < 0.99 {
		t.Fatalf("expected near-perfect confidence, got %.3f", got)
	}
}

func TestIdentityConfidenceLowForWrongMovie(t *testing.T) {
	selected := importIdentitySnapshot{
		Source:    "asilmedia",
		SourceID:  "9140",
		DetailURL: "https://asilmedia.org/9140-interstellar-uzbek-tarjima.html",
		Title:     "Interstellar",
		Year:      2014,
		Type:      "movie",
	}
	fetched := importIdentitySnapshot{
		Source:    "asilmedia",
		SourceID:  "7777",
		DetailURL: "https://asilmedia.org/7777-inception-uzbek-tarjima.html",
		Title:     "Inception",
		Year:      2010,
		Type:      "movie",
	}
	got := identityConfidence(selected, fetched)
	if got >= 0.85 {
		t.Fatalf("expected low confidence for wrong movie, got %.3f", got)
	}
}

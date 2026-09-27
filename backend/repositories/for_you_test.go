package repositories

import "testing"

func TestGenreProfileAndScore(t *testing.T) {
	p := GenreProfile{}
	p.add([]string{"Drama", "komediya"}, weightWatched)
	p.add([]string{"drama"}, weightFavorite)
	p.add([]string{"anime"}, weightWatchlist)

	top := p.Top(2)
	if len(top) != 2 || top[0] != "drama" || top[1] != "anime" {
		t.Fatalf("top = %v", top)
	}
	dramaHit := ScoreCandidate(p, []string{"drama"}, 100, 4)
	comedyPopular := ScoreCandidate(p, []string{"komediya"}, 1_000_000, 5)
	none := ScoreCandidate(p, []string{"horror"}, 1_000_000, 5)
	if !(dramaHit > comedyPopular) {
		t.Errorf("affinity should beat popularity: drama=%v comedy=%v", dramaHit, comedyPopular)
	}
	if none != 0 {
		t.Errorf("no-affinity candidate should score 0, got %v", none)
	}
}

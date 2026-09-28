package repositories

import (
	"testing"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func mv(title string, genre []string, country string, year int, views int64) models.Movie {
	return models.Movie{ID: primitive.NewObjectID(), Title: title, Slug: title, Genre: genre, Country: country, Year: year, Views: views, VideoURL: "x"}
}

func TestCanonicalNamesMatchAcrossLanguagesAndSpelling(t *testing.T) {
	a := canonGenres(models.Movie{Genre: []string{"Jangari", "Drama"}})
	b := canonGenres(models.Movie{Genre: []string{"action, drama"}})
	if !a["action"] || !a["drama"] || !b["action"] || !b["drama"] {
		t.Fatalf("genres not canonicalised: %v %v", a, b)
	}
	for _, c := range []string{"USA", "AQSH", "United States of America", "Amerika"} {
		if got := canonCountry(c); got != "united states" {
			t.Fatalf("%q -> %q", c, got)
		}
	}
	if !canonCountries(models.Movie{Country: "Janubiy Koreya, Yaponiya"})["south korea"] {
		t.Fatal("multi-country list not split")
	}
}

// Regression: an unrelated but popular, same-year title used to outrank a
// title that actually shares the genres.
func TestRankSimilarPrefersSharedGenreOverPopularity(t *testing.T) {
	cur := mv("current", []string{"horror", "thriller"}, "USA", 2020, 100)
	unrelatedPopular := mv("comedy-hit", []string{"comedy"}, "USA", 2020, 5_000_000)
	unrelatedPopular.RatingAvg = 5
	similar := mv("scary", []string{"Qo'rqinchli", "Triller"}, "Janubiy Koreya", 2009, 10)
	partly := mv("thriller-only", []string{"thriller"}, "AQSH", 2019, 500)

	got := rankSimilar(cur, []models.Movie{unrelatedPopular, partly, similar, cur}, 3)
	if len(got) != 3 {
		t.Fatalf("want 3 results, got %d", len(got))
	}
	if got[0].Title != "scary" && got[0].Title != "thriller-only" {
		t.Fatalf("first result should share genres, got %q", got[0].Title)
	}
	if got[2].Title != "comedy-hit" {
		t.Fatalf("unrelated title must come last, got order %q %q %q", got[0].Title, got[1].Title, got[2].Title)
	}
	for _, m := range got {
		if m.ID == cur.ID {
			t.Fatal("current movie must not recommend itself")
		}
	}
}

func TestRankSimilarSkipsUnplayable(t *testing.T) {
	cur := mv("current", []string{"drama"}, "", 2020, 0)
	noVideo := mv("no-video", []string{"drama"}, "", 2020, 0)
	noVideo.VideoURL = ""
	if got := rankSimilar(cur, []models.Movie{noVideo}, 5); len(got) != 0 {
		t.Fatalf("unplayable title returned: %v", got)
	}
}

func TestSimilarCandidateQueryCoversAllGenreFields(t *testing.T) {
	q := similarCandidateQuery(models.Movie{Genre: []string{"drama"}})
	or, ok := q["$or"].([]bson.M)
	if !ok || len(or) != 4 {
		t.Fatalf("want 4 genre field clauses (genre, genres, movie_genre, genres_uz), got %v", q["$or"])
	}
}

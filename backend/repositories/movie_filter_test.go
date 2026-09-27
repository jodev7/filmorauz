package repositories

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestMovieListFilterBuildsConstraints(t *testing.T) {
	f := MovieListFilter{YearFrom: 2010, YearTo: 2020, MinRating: 4, Country: "AQSH", Duration: "long", FreeOnly: true, Sort: "rating"}
	if f.IsZero() {
		t.Fatal("filter with options reported zero")
	}
	and := f.mongoFilter()["$and"].([]bson.M)
	// published + year + rating + country + duration + free
	if len(and) != 6 {
		t.Fatalf("expected 6 clauses, got %d: %v", len(and), and)
	}
	if got := f.sort()[0].Key; got != "rating_avg" {
		t.Errorf("sort key = %s", got)
	}
	if !(MovieListFilter{Sort: "new"}).IsZero() {
		t.Error("default filter should be zero")
	}
}

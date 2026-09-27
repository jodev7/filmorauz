package repositories

import (
	"testing"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestDecodeCredits(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	// Round-trip through BSON so the values have the driver's real types.
	raw, _ := bson.Marshal(bson.M{
		"cast_details":         []models.CastMember{{Name: "Robert Downey Jr.", Character: "Tony Stark", ProfileURL: "https://x/a.jpg", TMDBID: 3223}},
		"director_profile_url": "https://x/r.jpg",
		"credits_status":       "ok",
		"credits_fetched_at":   primitive.NewDateTimeFromTime(now),
	})
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var m models.Movie
	decodeCredits(doc, &m)
	if len(m.CastDetails) != 1 || m.CastDetails[0].Character != "Tony Stark" || m.CastDetails[0].TMDBID != 3223 {
		t.Fatalf("cast_details: %+v", m.CastDetails)
	}
	if m.DirectorProfileURL != "https://x/r.jpg" || m.CreditsStatus != "ok" || m.CreditsFetchedAt == nil || !m.CreditsFetchedAt.Equal(now) {
		t.Fatalf("fields: %+v", m)
	}
}

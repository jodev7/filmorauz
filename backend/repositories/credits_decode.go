package repositories

import (
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
)

// decodeCredits copies the TMDB credit fields from a raw movie document
// (normalizeMovieFromBSON decodes field by field).
func decodeCredits(doc bson.M, movie *models.Movie) {
	if raw, ok := doc["cast_details"]; ok && raw != nil {
		if b, err := bson.Marshal(bson.M{"v": raw}); err == nil {
			var w struct {
				V []models.CastMember `bson:"v"`
			}
			if bson.Unmarshal(b, &w) == nil {
				movie.CastDetails = w.V
			}
		}
	}
	if s, ok := doc["director_profile_url"].(string); ok {
		movie.DirectorProfileURL = s
	}
	if s, ok := doc["credits_status"].(string); ok {
		movie.CreditsStatus = s
	}
	switch t := doc["credits_fetched_at"].(type) {
	case time.Time:
		movie.CreditsFetchedAt = &t
	case interface{ Time() time.Time }:
		v := t.Time()
		movie.CreditsFetchedAt = &v
	}
}

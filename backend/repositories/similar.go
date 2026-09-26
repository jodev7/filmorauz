package repositories

import (
	"strings"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
)

// similarCandidateFilter matches movies sharing a genre, an actor or the
// director with m; nil when m has none of those.
func similarCandidateFilter(m models.Movie) bson.M {
	or := []bson.M{}
	if len(m.Genre) > 0 {
		or = append(or, bson.M{"genre": bson.M{"$in": m.Genre}})
	}
	if len(m.Cast) > 0 {
		cast := m.Cast
		if len(cast) > 8 {
			cast = cast[:8] // leads matter most
		}
		or = append(or, bson.M{"cast": bson.M{"$in": cast}})
	}
	if d := strings.TrimSpace(m.Director); d != "" {
		or = append(or, bson.M{"director": d})
	}
	if len(or) == 0 {
		return nil
	}
	return bson.M{"$or": or}
}

// creditsScore rewards shared people: +4 per shared actor (max +12) and +6
// for the same director.
func creditsScore(current, candidate models.Movie) int {
	score := 0
	if len(current.Cast) > 0 && len(candidate.Cast) > 0 {
		names := make(map[string]bool, len(current.Cast))
		for _, n := range current.Cast {
			names[strings.ToLower(strings.TrimSpace(n))] = true
		}
		shared := 0
		for _, n := range candidate.Cast {
			if names[strings.ToLower(strings.TrimSpace(n))] {
				shared++
			}
		}
		if shared > 3 {
			shared = 3
		}
		score += 4 * shared
	}
	if d := strings.TrimSpace(current.Director); d != "" && strings.EqualFold(d, strings.TrimSpace(candidate.Director)) {
		score += 6
	}
	return score
}

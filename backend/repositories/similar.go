package repositories

import (
	"strings"

	"github.com/filmorauz/backend/models"
)

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

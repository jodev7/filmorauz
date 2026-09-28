package repositories

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ─── Canonical genre / country keys ─────────────────────────────────────
//
// Genres and countries were imported from several scrapers, so the same value
// can appear as "drama", "Drama", "Dramma", "Jangari"/"action", "USA"/"AQSH",
// "United States" or a comma-separated list. Similarity must compare the
// meaning, not the spelling.

var (
	genreUzToEn   = invert(genreEnToUz)
	countryUzToEn = invert(countryEnToUz)
	countryAlias  = map[string]string{
		"usa": "united states", "us": "united states", "u.s.a.": "united states", "united states of america": "united states", "aqsh": "united states", "amerika": "united states",
		"uk": "united kingdom", "great britain": "united kingdom", "britain": "united kingdom", "england": "united kingdom", "angliya": "united kingdom", "buyuk britaniya": "united kingdom",
		"korea": "south korea", "republic of korea": "south korea", "korea, republic of": "south korea", "koreya": "south korea", "janubiy koreya": "south korea",
		"russian federation": "russia", "rossiya": "russia", "rf": "russia",
		"uzbekistan": "uzbekistan", "o'zbekiston": "uzbekistan", "ozbekiston": "uzbekistan", "o‘zbekiston": "uzbekistan",
		"turkiye": "turkey", "türkiye": "turkey", "turkiya": "turkey",
		"hindiston": "india", "xitoy": "china", "yaponiya": "japan", "fransiya": "france", "germaniya": "germany", "ispaniya": "spain", "italiya": "italy",
	}
	genreAlias = map[string]string{
		"sci-fi": "science fiction", "scifi": "science fiction", "fantastic": "fantasy", "cartoon": "animation", "multfilm": "animation", "multfilmlar": "animation", "multfilm-lar": "animation",
		"romantic": "romance", "melodrama": "romance", "melodrama-romance": "romance", "dramma": "drama", "komediya": "comedy", "triller": "thriller", "qo'rqinchli": "horror", "qorqinchli": "horror",
		"jangovar": "action", "boevik": "action", "detektiv": "crime", "kriminal": "crime", "tarixiy": "history", "biografiya": "history", "harbiy": "war",
	}
	listSplit = regexp.MustCompile(`\s*[,;/|]\s*`)
)

func invert(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for en, uz := range m {
		if _, taken := out[uz]; !taken {
			out[uz] = en
		}
	}
	return out
}

func canonGenre(g string) string {
	k := strings.ToLower(strings.TrimSpace(g))
	k = strings.Join(strings.Fields(strings.ReplaceAll(k, "_", " ")), " ")
	if k == "" {
		return ""
	}
	if en, ok := genreUzToEn[k]; ok {
		k = en
	}
	dashed := strings.ReplaceAll(k, " ", "-")
	if a, ok := genreAlias[dashed]; ok {
		k = a
	} else if a, ok := genreAlias[k]; ok {
		k = a
	}
	return k
}

// canonGenres returns the title's genres as a de-duplicated set of English keys.
func canonGenres(m models.Movie) map[string]bool {
	out := map[string]bool{}
	for _, list := range [][]string{m.Genre, m.GenresUz} {
		for _, raw := range list {
			for _, part := range listSplit.Split(raw, -1) {
				if g := canonGenre(part); g != "" {
					out[g] = true
				}
			}
		}
	}
	return out
}

func canonCountry(c string) string {
	k := strings.ToLower(strings.Join(strings.Fields(c), " "))
	if k == "" {
		return ""
	}
	if a, ok := countryAlias[k]; ok {
		return a
	}
	if en, ok := countryUzToEn[k]; ok {
		k = en
	}
	if a, ok := countryAlias[k]; ok {
		return a
	}
	return k
}

func canonCountries(m models.Movie) map[string]bool {
	out := map[string]bool{}
	for _, part := range listSplit.Split(m.Country, -1) {
		if c := canonCountry(part); c != "" {
			out[c] = true
		}
	}
	for _, raw := range m.CountriesUz {
		if c := canonCountry(raw); c != "" {
			out[c] = true
		}
	}
	return out
}

// genreQueryValues lists every stored spelling of the given canonical genres
// (English/Uzbek, any case) as case-insensitive exact-match regexes.
func genreQueryValues(genres map[string]bool) []interface{} {
	seen := map[string]bool{}
	var vals []interface{}
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		vals = append(vals, primitive.Regex{Pattern: "^" + regexp.QuoteMeta(s) + "$", Options: "i"})
	}
	for g := range genres {
		add(g)
		add(strings.ReplaceAll(g, " ", "-"))
		if uz, ok := genreEnToUz[g]; ok {
			add(uz)
		}
		for alias, canon := range genreAlias {
			if canon == g {
				add(alias)
			}
		}
	}
	return vals
}

// similarCandidateQuery matches titles sharing a genre (in any stored field
// or spelling), a lead actor or the director.
func similarCandidateQuery(m models.Movie) bson.M {
	or := []bson.M{}
	if vals := genreQueryValues(canonGenres(m)); len(vals) > 0 {
		or = append(or,
			bson.M{"genre": bson.M{"$in": vals}},
			bson.M{"genres": bson.M{"$in": vals}},
			bson.M{"movie_genre": bson.M{"$in": vals}},
			bson.M{"genres_uz": bson.M{"$in": vals}},
		)
	}
	if len(m.Cast) > 0 {
		cast := m.Cast
		if len(cast) > 8 {
			cast = cast[:8]
		}
		or = append(or, bson.M{"cast": bson.M{"$in": cast}})
	}
	if d := strings.TrimSpace(m.Director); d != "" {
		or = append(or, bson.M{"director": exactNameRegex(d)})
	}
	if len(or) == 0 {
		return nil
	}
	return bson.M{"$or": or}
}

// ─── Scoring ────────────────────────────────────────────────────────────

type similarity struct {
	score        float64
	sharedGenres int
	related      bool // shares a genre, an actor or the director
}

// similarityOf scores how alike two titles are. Genre overlap dominates;
// country, people and era refine it; popularity only breaks ties.
func similarityOf(cur models.Movie, curGenres, curCountries map[string]bool, cand models.Movie) similarity {
	var s similarity
	candGenres := canonGenres(cand)
	for g := range candGenres {
		if curGenres[g] {
			s.sharedGenres++
		}
	}
	if s.sharedGenres > 0 {
		union := len(curGenres) + len(candGenres) - s.sharedGenres
		s.score += 10*float64(s.sharedGenres) + 12*float64(s.sharedGenres)/float64(union)
		if s.sharedGenres == len(curGenres) {
			s.score += 4 // covers every genre of the current title
		}
	}

	credits := creditsScore(cur, cand)
	s.score += float64(credits)

	candCountries := canonCountries(cand)
	for c := range candCountries {
		if curCountries[c] {
			s.score += 6
			break
		}
	}

	if cur.Year > 0 && cand.Year > 0 {
		switch d := int(math.Abs(float64(cur.Year - cand.Year))); {
		case d <= 2:
			s.score += 3
		case d <= 5:
			s.score += 2
		case d <= 10:
			s.score += 1
		}
	}

	if cand.RatingAvg >= 4 {
		s.score += 1
	}
	// log-scaled popularity, at most +2
	s.score += math.Min(2, math.Log10(float64(cand.Views)+1)/2.5)

	s.related = s.sharedGenres > 0 || credits > 0
	return s
}

func isPlayable(m models.Movie) bool {
	return m.VideoURL != "" || m.EmbedURL != "" || m.MasterPlaylistURL != ""
}

// rankSimilar orders candidates for "O'xshash kinolar": titles that share a
// genre or people first (by similarity), unrelated ones only as a last resort.
func rankSimilar(cur models.Movie, candidates []models.Movie, limit int) []models.Movie {
	curGenres := canonGenres(cur)
	curCountries := canonCountries(cur)
	type scored struct {
		m models.Movie
		s similarity
	}
	seen := map[primitive.ObjectID]bool{cur.ID: true}
	seenSlug := map[string]bool{}
	if cur.Slug != "" {
		seenSlug[cur.Slug] = true
	}
	var list []scored
	for _, m := range candidates {
		if seen[m.ID] || (m.Slug != "" && seenSlug[m.Slug]) || !isPlayable(m) {
			continue
		}
		seen[m.ID] = true
		if m.Slug != "" {
			seenSlug[m.Slug] = true
		}
		list = append(list, scored{m, similarityOf(cur, curGenres, curCountries, m)})
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i].s, list[j].s
		if a.related != b.related {
			return a.related
		}
		if a.score != b.score {
			return a.score > b.score
		}
		return list[i].m.Views > list[j].m.Views
	})
	out := make([]models.Movie, 0, limit)
	for _, sc := range list {
		if len(out) >= limit {
			break
		}
		out = append(out, sc.m)
	}
	return out
}

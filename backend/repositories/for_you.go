package repositories

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// "Siz uchun" — personal movie recommendations.
//
// Signals (weights): favorites 3, rated ≥4 stars 3, watch-later 2, watched
// 1 (movies and series). They build a genre-affinity profile; candidates are
// published movies in the user's top genres that they haven't watched or
// favorited, scored by genre affinity, popularity and rating.

const (
	weightFavorite  = 3.0
	weightHighRated = 3.0
	weightWatchlist = 2.0
	weightWatched   = 1.0
)

// GenreProfile maps normalized genre → affinity weight.
type GenreProfile map[string]float64

func (p GenreProfile) add(genres []string, w float64) {
	for _, g := range genres {
		g = strings.ToLower(strings.TrimSpace(g))
		if g != "" {
			p[g] += w
		}
	}
}

// Top returns the n strongest genres.
func (p GenreProfile) Top(n int) []string {
	type kv struct {
		k string
		v float64
	}
	var all []kv
	for k, v := range p {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].v != all[j].v {
			return all[i].v > all[j].v
		}
		return all[i].k < all[j].k
	})
	out := []string{}
	for i := 0; i < len(all) && i < n; i++ {
		out = append(out, all[i].k)
	}
	return out
}

// ScoreCandidate ranks a candidate movie for a profile. Genre affinity
// dominates; popularity (log views) and rating break ties.
func ScoreCandidate(p GenreProfile, genres []string, views int64, rating float64) float64 {
	var affinity float64
	for _, g := range genres {
		affinity += p[strings.ToLower(strings.TrimSpace(g))]
	}
	if affinity == 0 {
		return 0
	}
	return affinity*10 + math.Log10(float64(views)+10)*2 + rating
}

type ForYouRepository struct {
	db *mongo.Database
}

func NewForYouRepository(db *mongo.Database) *ForYouRepository {
	return &ForYouRepository{db: db}
}

func objectIDs(docs []bson.M, keys ...string) []primitive.ObjectID {
	var out []primitive.ObjectID
	for _, d := range docs {
		for _, k := range keys {
			if id, ok := d[k].(primitive.ObjectID); ok && !id.IsZero() {
				out = append(out, id)
				break
			}
		}
	}
	return out
}

func (r *ForYouRepository) find(ctx context.Context, col string, filter bson.M, proj bson.M, limit int64) []bson.M {
	cur, err := r.db.Collection(col).Find(ctx, filter, options.Find().SetProjection(proj).SetLimit(limit).SetSort(bson.D{{Key: "_id", Value: -1}}))
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)
	var docs []bson.M
	_ = cur.All(ctx, &docs)
	return docs
}

func (r *ForYouRepository) genresOf(ctx context.Context, col string, ids []primitive.ObjectID) map[primitive.ObjectID][]string {
	out := map[primitive.ObjectID][]string{}
	if len(ids) == 0 {
		return out
	}
	for _, d := range r.find(ctx, col, bson.M{"_id": bson.M{"$in": ids}}, bson.M{"genre": 1}, int64(len(ids))) {
		id, _ := d["_id"].(primitive.ObjectID)
		if arr, ok := d["genre"].(bson.A); ok {
			for _, g := range arr {
				if s, ok := g.(string); ok {
					out[id] = append(out[id], s)
				}
			}
		}
	}
	return out
}

// ForYou returns up to limit recommended movies for a user, plus the top
// genres used (for the "because you like …" hint). Empty when the user has
// no history yet.
func (r *ForYouRepository) ForYou(ctx context.Context, userID primitive.ObjectID, limit int) ([]models.Movie, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if limit <= 0 || limit > 40 {
		limit = 18
	}

	history := r.find(ctx, "watch_history", bson.M{"user_id": userID}, bson.M{"target_type": 1, "target_id": 1, "movie_id": 1, "series_id": 1}, 300)
	favorites := r.find(ctx, "favorites", bson.M{"user_id": userID}, bson.M{"target_type": 1, "target_id": 1, "movie_id": 1, "series_id": 1}, 300)
	ratings := r.find(ctx, "movie_ratings", bson.M{"user_id": userID, "rating": bson.M{"$gte": 4}}, bson.M{"movie_id": 1}, 300)
	watchlist := r.find(ctx, "watchlist", bson.M{"user_id": userID}, bson.M{"target_type": 1, "target_id": 1}, 300)

	split := func(docs []bson.M) (movies, series []primitive.ObjectID) {
		for _, d := range docs {
			t, _ := d["target_type"].(string)
			if sid, ok := d["series_id"].(primitive.ObjectID); ok && !sid.IsZero() {
				series = append(series, sid)
				continue
			}
			if t == "series" {
				if id, ok := d["target_id"].(primitive.ObjectID); ok {
					series = append(series, id)
				}
				continue
			}
			if t == "episode" {
				continue
			}
			movies = append(movies, objectIDs([]bson.M{d}, "movie_id", "target_id")...)
		}
		return
	}
	histM, histS := split(history)
	favM, favS := split(favorites)
	wlM, wlS := split(watchlist)
	rated := objectIDs(ratings, "movie_id")

	allMovies := append(append(append(append([]primitive.ObjectID{}, histM...), favM...), wlM...), rated...)
	allSeries := append(append(append([]primitive.ObjectID{}, histS...), favS...), wlS...)
	mg := r.genresOf(ctx, "movies", allMovies)
	sg := r.genresOf(ctx, "series", allSeries)

	profile := GenreProfile{}
	for _, id := range histM {
		profile.add(mg[id], weightWatched)
	}
	for _, id := range histS {
		profile.add(sg[id], weightWatched)
	}
	for _, id := range favM {
		profile.add(mg[id], weightFavorite)
	}
	for _, id := range favS {
		profile.add(sg[id], weightFavorite)
	}
	for _, id := range rated {
		profile.add(mg[id], weightHighRated)
	}
	for _, id := range wlM {
		profile.add(mg[id], weightWatchlist)
	}
	for _, id := range wlS {
		profile.add(sg[id], weightWatchlist)
	}
	top := profile.Top(5)
	if len(top) == 0 {
		return []models.Movie{}, []string{}, nil
	}

	// Exclude what they've already watched / saved.
	exclude := map[primitive.ObjectID]bool{}
	for _, id := range append(append(append(histM, favM...), wlM...), rated...) {
		exclude[id] = true
	}
	excl := make([]primitive.ObjectID, 0, len(exclude))
	for id := range exclude {
		excl = append(excl, id)
	}

	cur, err := r.db.Collection("movies").Find(ctx, bson.M{"$and": []bson.M{
		{"$or": []bson.M{{"is_published": true}, {"is_published": bson.M{"$exists": false}}}},
		{"genre": bson.M{"$in": top}},
		{"_id": bson.M{"$nin": excl}},
	}}, options.Find().SetSort(bson.D{{Key: "views", Value: -1}}).SetLimit(250))
	if err != nil {
		return nil, top, err
	}
	defer cur.Close(ctx)
	var raw []bson.M
	if err := cur.All(ctx, &raw); err != nil {
		return nil, top, err
	}

	type scored struct {
		m models.Movie
		s float64
	}
	cands := make([]scored, 0, len(raw))
	for _, doc := range raw {
		m, err := normalizeMovieFromBSON(doc)
		if err != nil {
			continue
		}
		cands = append(cands, scored{*m, ScoreCandidate(profile, m.Genre, m.Views, m.RatingAvg)})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].s > cands[j].s })
	out := make([]models.Movie, 0, limit)
	for i := 0; i < len(cands) && len(out) < limit; i++ {
		out = append(out, cands[i].m)
	}
	return out, top, nil
}

package repositories

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var publishedMovieFilter = bson.M{"$or": []bson.M{
	{"is_published": true},
	{"is_published": bson.M{"$exists": false}},
}}

// exactNameRegex matches a whole name case-insensitively (collapsing
// internal whitespace), with user input escaped.
func exactNameRegex(name string) primitive.Regex {
	parts := strings.Fields(name)
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return primitive.Regex{Pattern: `^\s*` + strings.Join(parts, `\s+`) + `\s*$`, Options: "i"}
}

// PersonCredits is everything a person appears in.
type PersonCredits struct {
	Name     string         `json:"name"`
	Acted    []models.Movie `json:"acted"`
	Directed []models.Movie `json:"directed"`
}

// FindPersonCredits returns published movies where name is in the cast or is
// the director. The display name is taken from the stored spelling.
func (r *MovieRepository) FindPersonCredits(name string, limit int) (*PersonCredits, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || len(name) > 80 {
		return nil, fmt.Errorf("invalid name")
	}
	if limit <= 0 || limit > 200 {
		limit = 120
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rx := exactNameRegex(name)

	load := func(field string) ([]models.Movie, error) {
		cur, err := r.col.Find(ctx,
			bson.M{"$and": []bson.M{publishedMovieFilter, {field: rx}}},
			options.Find().SetSort(bson.D{{Key: "year", Value: -1}, {Key: "views", Value: -1}}).SetLimit(int64(limit)))
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return nil, err
		}
		out := make([]models.Movie, 0, len(docs))
		for _, d := range docs {
			if m, err := normalizeMovieFromBSON(d); err == nil {
				out = append(out, *m)
			}
		}
		return out, nil
	}
	acted, err := load("cast")
	if err != nil {
		return nil, err
	}
	directed, err := load("director")
	if err != nil {
		return nil, err
	}
	res := &PersonCredits{Name: name, Acted: acted, Directed: directed}
	// Prefer the spelling stored on the content.
	needle := strings.ToLower(name)
	for _, m := range acted {
		for _, c := range m.Cast {
			if strings.ToLower(strings.Join(strings.Fields(c), " ")) == needle {
				res.Name = strings.Join(strings.Fields(c), " ")
				return res, nil
			}
		}
	}
	if len(directed) > 0 {
		res.Name = strings.Join(strings.Fields(directed[0].Director), " ")
	}
	return res, nil
}

// RandomMovie returns one random published, playable movie, optionally of a
// genre and excluding some ids (the ones the user already skipped).
func (r *MovieRepository) RandomMovie(genre string, exclude []primitive.ObjectID) (*models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	and := []bson.M{publishedMovieFilter, {"$or": []bson.M{
		{"video_url": bson.M{"$nin": bson.A{"", nil}}},
		{"embed_url": bson.M{"$nin": bson.A{"", nil}}},
		{"master_playlist_url": bson.M{"$nin": bson.A{"", nil}}},
	}}}
	if g := normalizeGenreValues([]string{genre}); len(g) > 0 && g[0] != "" {
		and = append(and, bson.M{"genre": g[0]})
	}
	if len(exclude) > 0 {
		and = append(and, bson.M{"_id": bson.M{"$nin": exclude}})
	}
	cur, err := r.col.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"$and": and}}},
		{{Key: "$sample", Value: bson.M{"size": 1}}},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}
	return normalizeMovieFromBSON(docs[0])
}

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

// MovieListFilter drives the public "kengaytirilgan filtr" on /movies.
// Zero values mean "no constraint".
type MovieListFilter struct {
	Genre     string
	YearFrom  int
	YearTo    int
	MinRating float64 // 1..5
	Country   string
	Duration  string // "short" (<90), "medium" (90-120), "long" (>120)
	FreeOnly  bool
	Sort      string // "new" (default), "popular", "rating", "year"
}

// IsZero reports whether no advanced option is set (only genre/sort default).
func (f MovieListFilter) IsZero() bool {
	return f.YearFrom == 0 && f.YearTo == 0 && f.MinRating == 0 && f.Country == "" &&
		f.Duration == "" && !f.FreeOnly && (f.Sort == "" || f.Sort == "new")
}

func (f MovieListFilter) mongoFilter() bson.M {
	and := []bson.M{{"$or": []bson.M{
		{"is_published": true},
		{"is_published": bson.M{"$exists": false}},
	}}}
	if normalized := normalizeGenreValues([]string{f.Genre}); len(normalized) > 0 && normalized[0] != "" {
		and = append(and, bson.M{"genre": bson.M{"$in": []string{normalized[0]}}})
	}
	year := bson.M{}
	if f.YearFrom > 0 {
		year["$gte"] = f.YearFrom
	}
	if f.YearTo > 0 {
		year["$lte"] = f.YearTo
	}
	if len(year) > 0 {
		and = append(and, bson.M{"year": year})
	}
	if f.MinRating > 0 {
		and = append(and, bson.M{"rating_avg": bson.M{"$gte": f.MinRating}})
	}
	if c := strings.TrimSpace(f.Country); c != "" {
		rx := primitive.Regex{Pattern: "^" + regexp.QuoteMeta(c) + "$", Options: "i"}
		and = append(and, bson.M{"$or": []bson.M{{"country": rx}, {"countries_uz": rx}}})
	}
	switch f.Duration {
	case "short":
		and = append(and, bson.M{"duration": bson.M{"$gt": 0, "$lt": 90}})
	case "medium":
		and = append(and, bson.M{"duration": bson.M{"$gte": 90, "$lte": 120}})
	case "long":
		and = append(and, bson.M{"duration": bson.M{"$gt": 120}})
	}
	if f.FreeOnly {
		and = append(and, bson.M{"is_premium": bson.M{"$ne": true}})
	}
	return bson.M{"$and": and}
}

func (f MovieListFilter) sort() bson.D {
	switch f.Sort {
	case "popular":
		return bson.D{{Key: "views", Value: -1}, {Key: "created_at", Value: -1}}
	case "rating":
		return bson.D{{Key: "rating_avg", Value: -1}, {Key: "rating_count", Value: -1}}
	case "year":
		return bson.D{{Key: "year", Value: -1}, {Key: "created_at", Value: -1}}
	default:
		return bson.D{{Key: "updated_at", Value: -1}, {Key: "created_at", Value: -1}}
	}
}

// ListFiltered returns published movies matching f, paged.
func (r *MovieRepository) ListFiltered(f MovieListFilter, page, limit int) ([]models.Movie, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 24
	}
	filter := f.mongoFilter()
	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count filtered movies: %w", err)
	}
	cursor, err := r.col.Find(ctx, filter, options.Find().
		SetSort(f.sort()).
		SetSkip(int64((page-1)*limit)).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, 0, fmt.Errorf("find filtered movies: %w", err)
	}
	defer cursor.Close(ctx)
	var raw []bson.M
	if err := cursor.All(ctx, &raw); err != nil {
		return nil, 0, fmt.Errorf("decode filtered movies: %w", err)
	}
	movies := make([]models.Movie, 0, len(raw))
	for _, doc := range raw {
		if m, err := normalizeMovieFromBSON(doc); err == nil {
			movies = append(movies, *m)
		}
	}
	return movies, total, nil
}

// MovieFilterFacets lists the values the filter UI can offer.
type MovieFilterFacets struct {
	Countries []string `json:"countries"`
	YearMin   int      `json:"year_min"`
	YearMax   int      `json:"year_max"`
}

func (r *MovieRepository) FilterFacets() (*MovieFilterFacets, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	published := bson.M{"$or": []bson.M{{"is_published": true}, {"is_published": bson.M{"$exists": false}}}}

	out := &MovieFilterFacets{Countries: []string{}}
	cur, err := r.col.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: published}},
		{{Key: "$match", Value: bson.M{"country": bson.M{"$nin": bson.A{"", nil}}}}},
		{{Key: "$group", Value: bson.M{"_id": "$country", "n": bson.M{"$sum": 1}}}},
		{{Key: "$sort", Value: bson.D{{Key: "n", Value: -1}}}},
		{{Key: "$limit", Value: 30}},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID string `bson:"_id"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if strings.TrimSpace(row.ID) != "" {
			out.Countries = append(out.Countries, row.ID)
		}
	}

	cur2, err := r.col.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: published}},
		{{Key: "$match", Value: bson.M{"year": bson.M{"$gt": 1900}}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "min": bson.M{"$min": "$year"}, "max": bson.M{"$max": "$year"}}}},
	})
	if err == nil {
		var yr []struct {
			Min int `bson:"min"`
			Max int `bson:"max"`
		}
		if cur2.All(ctx, &yr) == nil && len(yr) > 0 {
			out.YearMin, out.YearMax = yr[0].Min, yr[0].Max
		}
	}
	return out, nil
}

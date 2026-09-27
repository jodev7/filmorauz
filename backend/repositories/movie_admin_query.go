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
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AdminMovieQuery drives the server-side paged /admin/movies list.
// Zero values mean "no constraint".
type AdminMovieQuery struct {
	Page    int
	Limit   int
	Status  string // "", "all", "pending", "approved", "rejected", "scheduled"
	Search  string
	Premium string // "", "premium", "free"
	Media   string // "", "missing" (no playable source yet)
	Sort    string // "newest" (default), "oldest", "title", "views", "rating", "schedule"
}

// AdminMovieStatusCounts are the tab counters shown above the admin list.
// They respect the search/premium/media filters but not the status tab.
type AdminMovieStatusCounts struct {
	All       int64 `json:"all"`
	Pending   int64 `json:"pending"`
	Approved  int64 `json:"approved"`
	Rejected  int64 `json:"rejected"`
	Scheduled int64 `json:"scheduled"`
}

// Legacy documents have no approval_status (or an empty one) and count as
// approved everywhere else in the app, so the filter must agree.
var approvedStatusFilter = bson.M{"$or": []bson.M{
	{"approval_status": "approved"},
	{"approval_status": bson.M{"$exists": false}},
	{"approval_status": ""},
	{"approval_status": nil},
}}

var scheduledFilter = bson.M{"scheduled_publish_at": bson.M{"$type": "date"}}

func adminStatusFilter(status string) bson.M {
	switch status {
	case "pending", "rejected":
		return bson.M{"approval_status": status}
	case "approved":
		return approvedStatusFilter
	case "scheduled":
		return scheduledFilter
	}
	return nil
}

// baseFilter is everything except the status tab.
func (q AdminMovieQuery) baseFilter() []bson.M {
	and := []bson.M{}
	if s := strings.TrimSpace(q.Search); s != "" {
		match := BuildTitleSearchFilter(s, []string{"title", "title_uz", "original_title", "cast", "director"}, false)
		or, _ := match["$or"].(bson.A)
		or = append(or, bson.M{"slug": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(strings.ToLower(s)), Options: ""}})
		and = append(and, bson.M{"$or": or})
	}
	switch q.Premium {
	case "premium":
		and = append(and, bson.M{"is_premium": true})
	case "free":
		and = append(and, bson.M{"is_premium": bson.M{"$ne": true}})
	}
	if q.Media == "missing" {
		for _, f := range []string{"video_url", "embed_url", "master_playlist_url"} {
			and = append(and, bson.M{"$or": []bson.M{{f: bson.M{"$exists": false}}, {f: ""}, {f: nil}}})
		}
	}
	return and
}

func combine(parts []bson.M) bson.M {
	switch len(parts) {
	case 0:
		return bson.M{}
	case 1:
		return parts[0]
	}
	return bson.M{"$and": parts}
}

// Filter returns the full Mongo filter for the query (base + status tab).
func (q AdminMovieQuery) Filter() bson.M {
	parts := q.baseFilter()
	if st := adminStatusFilter(q.Status); st != nil {
		parts = append(parts, st)
	}
	return combine(parts)
}

func (q AdminMovieQuery) sortSpec() bson.D {
	switch q.Sort {
	case "oldest":
		return bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}
	case "title":
		return bson.D{{Key: "title", Value: 1}, {Key: "_id", Value: 1}}
	case "views":
		return bson.D{{Key: "views", Value: -1}, {Key: "_id", Value: -1}}
	case "rating":
		return bson.D{{Key: "rating_avg", Value: -1}, {Key: "rating_count", Value: -1}, {Key: "_id", Value: -1}}
	case "schedule":
		return bson.D{{Key: "scheduled_publish_at", Value: 1}, {Key: "_id", Value: 1}}
	}
	return bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}
}

// ListAdminQuery returns one page of movies (any approval status) matching q.
func (r *MovieRepository) ListAdminQuery(q AdminMovieQuery) ([]models.Movie, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 20
	}
	filter := q.Filter()

	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin movies: %w", err)
	}

	opts := options.Find().
		SetSort(q.sortSpec()).
		SetSkip(int64((q.Page - 1) * q.Limit)).
		SetLimit(int64(q.Limit))
	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("find admin movies: %w", err)
	}
	defer cursor.Close(ctx)

	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		return nil, 0, fmt.Errorf("decode admin movies: %w", err)
	}
	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			continue
		}
		movies = append(movies, *movie)
	}
	return movies, total, nil
}

// AdminStatusCounts returns per-tab counts for the query's base filter.
func (r *MovieRepository) AdminStatusCounts(q AdminMovieQuery) (*AdminMovieStatusCounts, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	base := q.baseFilter()
	pipeline := []bson.M{
		{"$match": combine(base)},
		{"$group": bson.M{
			"_id": "$approval_status",
			"n":   bson.M{"$sum": 1},
			"scheduled": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$eq": bson.A{bson.M{"$type": "$scheduled_publish_at"}, "date"}}, 1, 0,
			}}},
		}},
	}
	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("admin status counts: %w", err)
	}
	defer cursor.Close(ctx)

	var rows []struct {
		ID        interface{} `bson:"_id"`
		N         int64       `bson:"n"`
		Scheduled int64       `bson:"scheduled"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode admin status counts: %w", err)
	}
	out := &AdminMovieStatusCounts{}
	for _, row := range rows {
		out.All += row.N
		out.Scheduled += row.Scheduled
		status, _ := row.ID.(string)
		switch status {
		case "pending":
			out.Pending += row.N
		case "rejected":
			out.Rejected += row.N
		case "", "approved":
			out.Approved += row.N
		}
	}
	return out, nil
}

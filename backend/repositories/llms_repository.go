package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Data for /llms.txt (AI-readable site index): every published movie and
// series with the fields a language model needs to describe it.

type LLMSTitle struct {
	Kind          string // "movie" | "series"
	Slug          string
	Code          string
	Title         string
	TitleUz       string
	OriginalTitle string
	Description   string
	DescriptionUz string
	Year          int
	Duration      int // minutes (movies)
	Country       string
	Genres        []string
	GenresUz      []string
	Cast          []string
	Director      string
	Quality       string
	RatingAvg     float64
	RatingCount   int
	IsPremium     bool
	Seasons       int
	Episodes      int
	UpdatedAt     time.Time
}

// DisplayTitle prefers the Uzbek title the site shows.
func (t LLMSTitle) DisplayTitle() string {
	if s := strings.TrimSpace(t.TitleUz); s != "" {
		return s
	}
	return strings.TrimSpace(t.Title)
}

// DisplayDescription prefers the Uzbek description.
func (t LLMSTitle) DisplayDescription() string {
	if s := strings.TrimSpace(t.DescriptionUz); s != "" {
		return s
	}
	return strings.TrimSpace(t.Description)
}

type LLMSRepository struct {
	db *mongo.Database
}

func NewLLMSRepository(db *mongo.Database) *LLMSRepository { return &LLMSRepository{db: db} }

var llmsPublished = bson.M{"$or": []bson.M{{"is_published": true}, {"is_published": bson.M{"$exists": false}}}}

var llmsProjection = bson.M{
	"slug": 1, "code": 1, "title": 1, "title_uz": 1, "original_title": 1, "description": 1, "description_uz": 1,
	"year": 1, "duration": 1, "country": 1, "genre": 1, "genres_uz": 1, "cast": 1, "director": 1, "quality": 1,
	"rating_avg": 1, "rating_count": 1, "is_premium": 1, "updated_at": 1,
}

func llmsFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func llmsTime(v interface{}) time.Time {
	switch t := v.(type) {
	case primitive.DateTime:
		return t.Time()
	case time.Time:
		return t
	}
	return time.Time{}
}

func llmsTitleFromDoc(kind string, d bson.M) LLMSTitle {
	premium, _ := d["is_premium"].(bool)
	return LLMSTitle{
		Kind:          kind,
		Slug:          strings.TrimSpace(normalizeFieldToString(d["slug"])),
		Code:          normalizeFieldToString(d["code"]),
		Title:         normalizeFieldToString(d["title"]),
		TitleUz:       normalizeFieldToString(d["title_uz"]),
		OriginalTitle: normalizeFieldToString(d["original_title"]),
		Description:   normalizeFieldToString(d["description"]),
		DescriptionUz: normalizeFieldToString(d["description_uz"]),
		Year:          normalizeFieldToInt(d["year"]),
		Duration:      normalizeFieldToInt(d["duration"]),
		Country:       normalizeFieldToString(d["country"]),
		Genres:        normalizeGenreValues(decodeBSONStringArray(d["genre"])),
		GenresUz:      decodeBSONStringArray(d["genres_uz"]),
		Cast:          decodeBSONStringArray(d["cast"]),
		Director:      normalizeFieldToString(d["director"]),
		Quality:       normalizeFieldToString(d["quality"]),
		RatingAvg:     llmsFloat(d["rating_avg"]),
		RatingCount:   normalizeFieldToInt(d["rating_count"]),
		IsPremium:     premium,
		UpdatedAt:     llmsTime(d["updated_at"]),
	}
}

func (r *LLMSRepository) find(ctx context.Context, kind string, filter bson.M, limit int64) ([]LLMSTitle, error) {
	col := "movies"
	if kind == "series" {
		col = "series"
	}
	opts := options.Find().SetProjection(llmsProjection).SetSort(bson.D{{Key: "_id", Value: -1}}) // newest first; _id is always indexed
	if limit > 0 {
		opts.SetLimit(limit)
	}
	cur, err := r.db.Collection(col).Find(ctx, bson.M{"$and": []bson.M{llmsPublished, filter}}, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]LLMSTitle, 0, len(docs))
	ids := make([]primitive.ObjectID, 0, len(docs))
	for _, d := range docs {
		t := llmsTitleFromDoc(kind, d)
		if t.Slug == "" || t.DisplayTitle() == "" {
			continue
		}
		id, _ := d["_id"].(primitive.ObjectID)
		out = append(out, t)
		ids = append(ids, id)
	}
	if kind == "series" && len(ids) > 0 {
		seasons := r.countBySeries(ctx, "seasons", ids)
		episodes := r.countBySeries(ctx, "episodes", ids)
		for i, id := range ids {
			out[i].Seasons = seasons[id]
			out[i].Episodes = episodes[id]
		}
	}
	return out, nil
}

func (r *LLMSRepository) countBySeries(ctx context.Context, col string, ids []primitive.ObjectID) map[primitive.ObjectID]int {
	out := map[primitive.ObjectID]int{}
	cur, err := r.db.Collection(col).Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"series_id": bson.M{"$in": ids}}}},
		{{Key: "$group", Value: bson.M{"_id": "$series_id", "n": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return out
	}
	var rows []struct {
		ID primitive.ObjectID `bson:"_id"`
		N  int                `bson:"n"`
	}
	if cur.All(ctx, &rows) == nil {
		for _, r := range rows {
			out[r.ID] = r.N
		}
	}
	return out
}

// Titles lists all published movies or series, newest first.
func (r *LLMSRepository) Titles(ctx context.Context, kind string) ([]LLMSTitle, error) {
	return r.find(ctx, kind, bson.M{}, 0)
}

// BySlug returns one published title, or nil.
func (r *LLMSRepository) BySlug(ctx context.Context, kind, slug string) (*LLMSTitle, error) {
	list, err := r.find(ctx, kind, bson.M{"slug": slug}, 1)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// Version changes whenever a movie/series is added, removed or edited, so
// the generated files can be rebuilt right away instead of on a timer.
func (r *LLMSRepository) Version(ctx context.Context) string {
	var parts []string
	for _, col := range []string{"movies", "series"} {
		c := r.db.Collection(col)
		n, _ := c.CountDocuments(ctx, llmsPublished)
		var last struct {
			UpdatedAt time.Time `bson:"updated_at"`
		}
		_ = c.FindOne(ctx, llmsPublished, options.FindOne().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetProjection(bson.M{"updated_at": 1})).Decode(&last)
		parts = append(parts, col, fmt.Sprint(n), last.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	return strings.Join(parts, "|")
}

package repositories

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ClipAIUsageRepository reads the clip_ai_usage collection that the worker
// writes one row into per Gemini analysis (see worker recordGeminiUsage).
// Each row: content_kind, content_id, title, model, *_tokens, cost_usd,
// clip_count, created_at. Nothing in the backend writes this collection — it
// is read-only here, purely for the admin cost dashboard.
type ClipAIUsageRepository struct {
	col *mongo.Collection
}

func NewClipAIUsageRepository(db *mongo.Database) *ClipAIUsageRepository {
	return &ClipAIUsageRepository{col: db.Collection("clip_ai_usage")}
}

// AIUsageTotals is the aggregate across every recorded analysis.
type AIUsageTotals struct {
	CostUSD      float64 `json:"cost_usd" bson:"cost_usd"`
	TotalTokens  int64   `json:"total_tokens" bson:"total_tokens"`
	AudioTokens  int64   `json:"audio_tokens" bson:"audio_tokens"`
	TextTokens   int64   `json:"text_tokens" bson:"text_tokens"`
	OutputTokens int64   `json:"output_tokens" bson:"output_tokens"`
	ClipCount    int64   `json:"clip_count" bson:"clip_count"`
	Analyses     int64   `json:"analyses" bson:"analyses"`
}

// AIUsageItem is the spend rolled up per piece of content (movie or episode).
type AIUsageItem struct {
	ContentKind  string  `json:"content_kind" bson:"_id_kind"`
	ContentID    string  `json:"content_id" bson:"_id_content"`
	Title        string  `json:"title" bson:"title"`
	Model        string  `json:"model" bson:"model"`
	CostUSD      float64 `json:"cost_usd" bson:"cost_usd"`
	CostPerClip  float64 `json:"cost_per_clip" bson:"-"`
	TotalTokens  int64   `json:"total_tokens" bson:"total_tokens"`
	ClipCount    int64   `json:"clip_count" bson:"clip_count"`
	Analyses     int64   `json:"analyses" bson:"analyses"`
	LastAnalyzed any     `json:"last_analyzed" bson:"last_analyzed"`
}

// Totals sums spend across all analyses.
func (r *ClipAIUsageRepository) Totals(ctx context.Context) (*AIUsageTotals, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "cost_usd", Value: bson.D{{Key: "$sum", Value: "$cost_usd"}}},
			{Key: "total_tokens", Value: bson.D{{Key: "$sum", Value: "$total_tokens"}}},
			{Key: "audio_tokens", Value: bson.D{{Key: "$sum", Value: "$audio_tokens"}}},
			{Key: "text_tokens", Value: bson.D{{Key: "$sum", Value: "$text_tokens"}}},
			{Key: "output_tokens", Value: bson.D{{Key: "$sum", Value: "$output_tokens"}}},
			{Key: "clip_count", Value: bson.D{{Key: "$sum", Value: "$clip_count"}}},
			{Key: "analyses", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}
	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	totals := &AIUsageTotals{}
	if cursor.Next(ctx) {
		if err := cursor.Decode(totals); err != nil {
			return nil, err
		}
	}
	return totals, nil
}

// PerContent rolls spend up per content_id (one row per movie/episode),
// newest activity first. limit <= 0 means no limit.
func (r *ClipAIUsageRepository) PerContent(ctx context.Context, limit int64) ([]AIUsageItem, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "kind", Value: "$content_kind"},
				{Key: "content", Value: "$content_id"},
			}},
			{Key: "title", Value: bson.D{{Key: "$last", Value: "$title"}}},
			{Key: "model", Value: bson.D{{Key: "$last", Value: "$model"}}},
			{Key: "cost_usd", Value: bson.D{{Key: "$sum", Value: "$cost_usd"}}},
			{Key: "total_tokens", Value: bson.D{{Key: "$sum", Value: "$total_tokens"}}},
			{Key: "clip_count", Value: bson.D{{Key: "$sum", Value: "$clip_count"}}},
			{Key: "analyses", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "last_analyzed", Value: bson.D{{Key: "$max", Value: "$created_at"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "last_analyzed", Value: -1}}}},
	}
	if limit > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: limit}})
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []AIUsageItem
	for cursor.Next(ctx) {
		var raw struct {
			ID struct {
				Kind    string `bson:"kind"`
				Content any    `bson:"content"`
			} `bson:"_id"`
			Title        string  `bson:"title"`
			Model        string  `bson:"model"`
			CostUSD      float64 `bson:"cost_usd"`
			TotalTokens  int64   `bson:"total_tokens"`
			ClipCount    int64   `bson:"clip_count"`
			Analyses     int64   `bson:"analyses"`
			LastAnalyzed any     `bson:"last_analyzed"`
		}
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		item := AIUsageItem{
			ContentKind:  raw.ID.Kind,
			ContentID:    stringifyID(raw.ID.Content),
			Title:        raw.Title,
			Model:        raw.Model,
			CostUSD:      raw.CostUSD,
			TotalTokens:  raw.TotalTokens,
			ClipCount:    raw.ClipCount,
			Analyses:     raw.Analyses,
			LastAnalyzed: raw.LastAnalyzed,
		}
		if raw.ClipCount > 0 {
			item.CostPerClip = raw.CostUSD / float64(raw.ClipCount)
		}
		items = append(items, item)
	}
	return items, nil
}

// stringifyID renders a content_id (ObjectID or string) as a hex/plain string.
func stringifyID(v any) string {
	switch id := v.(type) {
	case string:
		return id
	case interface{ Hex() string }:
		return id.Hex()
	default:
		return ""
	}
}

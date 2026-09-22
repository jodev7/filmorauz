package repositories

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AnalyticsRepository struct {
	searchEvents        *mongo.Collection
	contentViewEvents   *mongo.Collection
	premiumFunnelEvents *mongo.Collection
	playbackReports     *mongo.Collection
	watchHistory        *mongo.Collection
	movies              *mongo.Collection
	series              *mongo.Collection
	episodes            *mongo.Collection
	premiumSessions     *mongo.Collection
	premiumPayments     *mongo.Collection
}

type SearchTermStat struct {
	Query       string  `json:"query"`
	Count       int64   `json:"count"`
	AvgResults  float64 `json:"avg_results"`
	ZeroResults int64   `json:"zero_results"`
}

type TopContentPeriodStat struct {
	TargetType string  `json:"target_type"`
	TargetID   string  `json:"target_id"`
	Title      string  `json:"title"`
	Slug       string  `json:"slug,omitempty"`
	PosterURL  string  `json:"poster_url,omitempty"`
	Views      int64   `json:"views"`
	PeriodDays int     `json:"period_days"`
	Score      float64 `json:"score,omitempty"`
}

type CompletionStat struct {
	TargetType     string  `json:"target_type"`
	TargetID       string  `json:"target_id"`
	Title          string  `json:"title"`
	Slug           string  `json:"slug,omitempty"`
	PosterURL      string  `json:"poster_url,omitempty"`
	Starts         int64   `json:"starts"`
	Completed      int64   `json:"completed"`
	AvgProgress    float64 `json:"avg_progress"`
	CompletionRate float64 `json:"completion_rate"`
}

type PremiumFunnelSummary struct {
	PeriodDays      int   `json:"period_days"`
	LockViews       int64 `json:"lock_views"`
	CTAClicks       int64 `json:"cta_clicks"`
	SessionsStarted int64 `json:"sessions_started"`
	PaidSessions    int64 `json:"paid_sessions"`
	StarsRevenue    int64 `json:"stars_revenue"`
}

func NewAnalyticsRepository(db *mongo.Database) *AnalyticsRepository {
	return &AnalyticsRepository{
		searchEvents:        db.Collection("search_events"),
		contentViewEvents:   db.Collection("content_view_events"),
		premiumFunnelEvents: db.Collection("premium_funnel_events"),
		playbackReports:     db.Collection("playback_reports"),
		watchHistory:        db.Collection("watch_history"),
		movies:              db.Collection("movies"),
		series:              db.Collection("series"),
		episodes:            db.Collection("episodes"),
		premiumSessions:     db.Collection("premium_purchase_sessions"),
		premiumPayments:     db.Collection("telegram_stars_payments"),
	}
}

func (r *AnalyticsRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []struct {
		col *mongo.Collection
		idx []mongo.IndexModel
	}{
		{r.searchEvents, []mongo.IndexModel{
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "query", Value: 1}, {Key: "created_at", Value: -1}}},
		}},
		{r.contentViewEvents, []mongo.IndexModel{
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "target_type", Value: 1}, {Key: "target_id", Value: 1}, {Key: "created_at", Value: -1}}},
		}},
		{r.premiumFunnelEvents, []mongo.IndexModel{
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "event_type", Value: 1}, {Key: "created_at", Value: -1}}},
		}},
		{r.playbackReports, []mongo.IndexModel{
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
		}},
	}
	for _, item := range indexes {
		if _, err := item.col.Indexes().CreateMany(ctx, item.idx); err != nil {
			return err
		}
	}
	return nil
}

func (r *AnalyticsRepository) RecordSearchEvent(ctx context.Context, event models.SearchEvent) error {
	if strings.TrimSpace(event.Query) == "" {
		return nil
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	_, err := r.searchEvents.InsertOne(ctx, event)
	return err
}

func (r *AnalyticsRepository) RecordContentView(ctx context.Context, event models.ContentViewEvent) error {
	if event.TargetID.IsZero() || event.TargetType == "" {
		return nil
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	_, err := r.contentViewEvents.InsertOne(ctx, event)
	return err
}

func (r *AnalyticsRepository) RecordPremiumFunnelEvent(ctx context.Context, event models.PremiumFunnelEvent) error {
	if event.UserID.IsZero() || strings.TrimSpace(event.EventType) == "" {
		return nil
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	_, err := r.premiumFunnelEvents.InsertOne(ctx, event)
	return err
}

func (r *AnalyticsRepository) CreatePlaybackReport(ctx context.Context, report models.PlaybackReport) error {
	if report.TargetID.IsZero() || report.TargetType == "" {
		return nil
	}
	now := time.Now()
	if report.CreatedAt.IsZero() {
		report.CreatedAt = now
	}
	if report.UpdatedAt.IsZero() {
		report.UpdatedAt = now
	}
	if report.Status == "" {
		report.Status = "new"
	}
	_, err := r.playbackReports.InsertOne(ctx, report)
	return err
}

func (r *AnalyticsRepository) SearchSummary(ctx context.Context, days int, limit int) ([]SearchTermStat, []SearchTermStat, error) {
	if days <= 0 {
		days = 30
	}
	if limit <= 0 {
		limit = 10
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	top, err := r.searchTerms(ctx, bson.M{"created_at": bson.M{"$gte": since}}, limit)
	if err != nil {
		return nil, nil, err
	}
	zero, err := r.searchTerms(ctx, bson.M{"created_at": bson.M{"$gte": since}, "result_count": 0}, limit)
	if err != nil {
		return nil, nil, err
	}
	return top, zero, nil
}

func (r *AnalyticsRepository) searchTerms(ctx context.Context, match bson.M, limit int) ([]SearchTermStat, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":          "$query",
			"count":        bson.M{"$sum": 1},
			"avg_results":  bson.M{"$avg": "$result_count"},
			"zero_results": bson.M{"$sum": bson.M{"$cond": []interface{}{bson.M{"$eq": []interface{}{"$result_count", 0}}, 1, 0}}},
		}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		{{Key: "$limit", Value: limit}},
	}
	cursor, err := r.searchEvents.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var raw []struct {
		Query       string  `bson:"_id"`
		Count       int64   `bson:"count"`
		AvgResults  float64 `bson:"avg_results"`
		ZeroResults int64   `bson:"zero_results"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return nil, err
	}
	out := make([]SearchTermStat, 0, len(raw))
	for _, row := range raw {
		out = append(out, SearchTermStat{
			Query:       row.Query,
			Count:       row.Count,
			AvgResults:  math.Round(row.AvgResults*10) / 10,
			ZeroResults: row.ZeroResults,
		})
	}
	return out, nil
}

func (r *AnalyticsRepository) TopContentByPeriod(ctx context.Context, days int, limit int) ([]TopContentPeriodStat, error) {
	if days <= 0 {
		days = 7
	}
	if limit <= 0 {
		limit = 10
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"created_at": bson.M{"$gte": since}}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"target_type": "$target_type",
				"target_id":   "$target_id",
			},
			"views": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{"views": -1}}},
		{{Key: "$limit", Value: limit}},
	}
	cursor, err := r.contentViewEvents.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var raw []struct {
		ID struct {
			TargetType string             `bson:"target_type"`
			TargetID   primitive.ObjectID `bson:"target_id"`
		} `bson:"_id"`
		Views int64 `bson:"views"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return nil, err
	}
	out := make([]TopContentPeriodStat, 0, len(raw))
	for _, row := range raw {
		title, slug, poster := r.contentLabel(ctx, row.ID.TargetType, row.ID.TargetID)
		out = append(out, TopContentPeriodStat{
			TargetType: row.ID.TargetType,
			TargetID:   row.ID.TargetID.Hex(),
			Title:      title,
			Slug:       slug,
			PosterURL:  poster,
			Views:      row.Views,
			PeriodDays: days,
		})
	}
	return out, nil
}

func (r *AnalyticsRepository) CompletionSummary(ctx context.Context, limit int) ([]CompletionStat, error) {
	if limit <= 0 {
		limit = 10
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"target_id":        bson.M{"$exists": true},
			"progress_percent": bson.M{"$gt": 0},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"target_type": bson.M{"$ifNull": []interface{}{"$target_type", "movie"}},
				"target_id":   bson.M{"$ifNull": []interface{}{"$target_id", "$movie_id"}},
			},
			"starts":       bson.M{"$sum": 1},
			"completed":    bson.M{"$sum": bson.M{"$cond": []interface{}{"$completed", 1, 0}}},
			"avg_progress": bson.M{"$avg": "$progress_percent"},
		}}},
		{{Key: "$sort", Value: bson.M{"starts": -1}}},
		{{Key: "$limit", Value: limit}},
	}
	cursor, err := r.watchHistory.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var raw []struct {
		ID struct {
			TargetType string             `bson:"target_type"`
			TargetID   primitive.ObjectID `bson:"target_id"`
		} `bson:"_id"`
		Starts      int64   `bson:"starts"`
		Completed   int64   `bson:"completed"`
		AvgProgress float64 `bson:"avg_progress"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return nil, err
	}
	out := make([]CompletionStat, 0, len(raw))
	for _, row := range raw {
		if row.ID.TargetID.IsZero() {
			continue
		}
		title, slug, poster := r.contentLabel(ctx, row.ID.TargetType, row.ID.TargetID)
		rate := float64(0)
		if row.Starts > 0 {
			rate = float64(row.Completed) / float64(row.Starts) * 100
		}
		out = append(out, CompletionStat{
			TargetType:     row.ID.TargetType,
			TargetID:       row.ID.TargetID.Hex(),
			Title:          title,
			Slug:           slug,
			PosterURL:      poster,
			Starts:         row.Starts,
			Completed:      row.Completed,
			AvgProgress:    math.Round(row.AvgProgress*10) / 10,
			CompletionRate: math.Round(rate*10) / 10,
		})
	}
	return out, nil
}

func (r *AnalyticsRepository) PremiumFunnel(ctx context.Context, days int) (PremiumFunnelSummary, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	out := PremiumFunnelSummary{PeriodDays: days}
	var err error
	if out.LockViews, err = r.premiumFunnelEvents.CountDocuments(ctx, bson.M{"created_at": bson.M{"$gte": since}, "event_type": "lock_view"}); err != nil {
		return out, err
	}
	if out.CTAClicks, err = r.premiumFunnelEvents.CountDocuments(ctx, bson.M{"created_at": bson.M{"$gte": since}, "event_type": "cta_click"}); err != nil {
		return out, err
	}
	if out.SessionsStarted, err = r.premiumSessions.CountDocuments(ctx, bson.M{"created_at": bson.M{"$gte": since}}); err != nil {
		return out, err
	}
	if out.PaidSessions, err = r.premiumSessions.CountDocuments(ctx, bson.M{"completed_at": bson.M{"$gte": since}, "status": "paid"}); err != nil {
		return out, err
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"created_at": bson.M{"$gte": since}, "status": "succeeded"}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "stars": bson.M{"$sum": "$stars_amount"}}}},
	}
	cursor, err := r.premiumPayments.Aggregate(ctx, pipeline)
	if err != nil {
		return out, err
	}
	defer cursor.Close(ctx)
	var raw []struct {
		Stars int64 `bson:"stars"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return out, err
	}
	if len(raw) > 0 {
		out.StarsRevenue = raw[0].Stars
	}
	return out, nil
}

func (r *AnalyticsRepository) ListPlaybackReports(ctx context.Context, status string, limit int) ([]models.PlaybackReport, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	filter := bson.M{}
	if strings.TrimSpace(status) != "" && status != "all" {
		filter["status"] = status
	}
	cursor, err := r.playbackReports.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var reports []models.PlaybackReport
	if err := cursor.All(ctx, &reports); err != nil {
		return nil, err
	}
	if reports == nil {
		reports = []models.PlaybackReport{}
	}
	return reports, nil
}

func (r *AnalyticsRepository) PlaybackReportCounts(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{"new": 0, "reviewing": 0, "resolved": 0}
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{"_id": "$status", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := r.playbackReports.Aggregate(ctx, pipeline)
	if err != nil {
		return out, err
	}
	defer cursor.Close(ctx)
	var raw []struct {
		Status string `bson:"_id"`
		Count  int64  `bson:"count"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return out, err
	}
	for _, row := range raw {
		out[row.Status] = row.Count
	}
	return out, nil
}

func (r *AnalyticsRepository) contentLabel(ctx context.Context, targetType string, id primitive.ObjectID) (string, string, string) {
	col := r.movies
	if targetType == "series" {
		col = r.series
	} else if targetType == "episode" {
		col = r.episodes
	}
	var doc struct {
		Title         string             `bson:"title"`
		Slug          string             `bson:"slug"`
		PosterURL     string             `bson:"poster_url"`
		SeriesID      primitive.ObjectID `bson:"series_id"`
		EpisodeNumber int                `bson:"episode_number"`
	}
	err := col.FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetProjection(bson.M{
		"title":          1,
		"slug":           1,
		"poster_url":     1,
		"series_id":      1,
		"episode_number": 1,
	})).Decode(&doc)
	if err != nil {
		return "O'chirilgan kontent", "", ""
	}
	if targetType == "episode" && !doc.SeriesID.IsZero() {
		var s struct {
			Title     string `bson:"title"`
			Slug      string `bson:"slug"`
			PosterURL string `bson:"poster_url"`
		}
		if err := r.series.FindOne(ctx, bson.M{"_id": doc.SeriesID}, options.FindOne().SetProjection(bson.M{"title": 1, "slug": 1, "poster_url": 1})).Decode(&s); err == nil {
			title := strings.TrimSpace(s.Title)
			if doc.EpisodeNumber > 0 {
				title = fmt.Sprintf("%s - %d-qism", title, doc.EpisodeNumber)
			}
			if doc.Title != "" {
				title = s.Title + " - " + doc.Title
			}
			if title == "" {
				title = s.Title
			}
			return title, s.Slug, s.PosterURL
		}
	}
	return doc.Title, doc.Slug, doc.PosterURL
}

package handlers

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AdminOverviewHandler powers the dashboard's "at a glance" blocks:
// items needing attention, pipeline health, content quality and (for
// superadmins) a monthly finance summary. Everything is a cheap count or a
// small aggregation so the dashboard can call it on every load.
type AdminOverviewHandler struct {
	db      *mongo.Database
	jobRepo *repositories.JobRepository
}

func NewAdminOverviewHandler(db *mongo.Database, jobRepo *repositories.JobRepository) *AdminOverviewHandler {
	return &AdminOverviewHandler{db: db, jobRepo: jobRepo}
}

// starsUSDRate is the approximate USD value of one Telegram Star, used only
// for the rough revenue estimate on the dashboard. Override with
// STARS_USD_RATE if Telegram's payout rate changes.
func starsUSDRate() float64 {
	if v := strings.TrimSpace(os.Getenv("STARS_USD_RATE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return 0.013
}

func (h *AdminOverviewHandler) count(ctx context.Context, collection string, filter bson.M) int64 {
	n, err := h.db.Collection(collection).CountDocuments(ctx, filter)
	if err != nil {
		log.Printf("[OVERVIEW] count %s failed: %v", collection, err)
		return 0
	}
	return n
}

func (h *AdminOverviewHandler) sum(ctx context.Context, collection string, match bson.M, field string) float64 {
	cursor, err := h.db.Collection(collection).Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{"_id": nil, "total": bson.M{"$sum": "$" + field}}}},
	})
	if err != nil {
		log.Printf("[OVERVIEW] sum %s.%s failed: %v", collection, field, err)
		return 0
	}
	defer cursor.Close(ctx)
	var rows []struct {
		Total float64 `bson:"total"`
	}
	if err := cursor.All(ctx, &rows); err != nil || len(rows) == 0 {
		return 0
	}
	return rows[0].Total
}

type qualityItem struct {
	ID    string `json:"id" bson:"_id"`
	Title string `json:"title" bson:"title"`
	Slug  string `json:"slug" bson:"slug"`
	Views int64  `json:"views" bson:"views"`
}

func (h *AdminOverviewHandler) sampleMovies(ctx context.Context, filter bson.M, sort bson.D, limit int64) []qualityItem {
	opts := options.Find().
		SetProjection(bson.M{"title": 1, "slug": 1, "views": 1}).
		SetSort(sort).
		SetLimit(limit)
	cursor, err := h.db.Collection("movies").Find(ctx, filter, opts)
	if err != nil {
		log.Printf("[OVERVIEW] sample movies failed: %v", err)
		return []qualityItem{}
	}
	defer cursor.Close(ctx)
	var raw []bson.M
	if err := cursor.All(ctx, &raw); err != nil {
		return []qualityItem{}
	}
	out := make([]qualityItem, 0, len(raw))
	for _, d := range raw {
		item := qualityItem{}
		if id, ok := d["_id"].(interface{ Hex() string }); ok {
			item.ID = id.Hex()
		}
		item.Title, _ = d["title"].(string)
		item.Slug, _ = d["slug"].(string)
		switch v := d["views"].(type) {
		case int64:
			item.Views = v
		case int32:
			item.Views = int64(v)
		case float64:
			item.Views = int64(v)
		}
		out = append(out, item)
	}
	return out
}

// emptyField matches a missing, null or blank string field.
func emptyField(field string) bson.M {
	return bson.M{"$or": bson.A{
		bson.M{field: bson.M{"$exists": false}},
		bson.M{field: nil},
		bson.M{field: ""},
	}}
}

// Overview GET /api/admin/overview
func (h *AdminOverviewHandler) Overview(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	now := time.Now()
	dayAgo := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	// ── Needs attention ────────────────────────────────────────────────
	attention := gin.H{
		"pending_appeals":     h.count(ctx, "ban_appeals", bson.M{"status": models.BanAppealStatusPending}),
		"pending_suggestions": h.count(ctx, "suggestions", bson.M{"status": models.SuggestionStatusPending}),
		"pending_comments":    h.count(ctx, "movie_comments", bson.M{"status": models.CommentStatusPending}),
		"playback_reports":    h.count(ctx, "playback_reports", bson.M{"status": "new"}),
		"pending_approvals":   h.count(ctx, "movies", bson.M{"approval_status": "pending"}),
		"premium_expiring_3d": h.count(ctx, "users", bson.M{
			"is_premium":         true,
			"premium_expires_at": bson.M{"$gte": now, "$lte": now.Add(3 * 24 * time.Hour)},
		}),
		"failed_publish_jobs_7d": h.count(ctx, "publish_jobs", bson.M{
			"status":     models.PublishJobStatusFailed,
			"updated_at": bson.M{"$gte": weekAgo},
		}),
	}

	// ── Pipeline health (ingestion queue) ──────────────────────────────
	ingestion := gin.H{}
	for _, key := range []string{"active", "pending", "processing", "stuck"} {
		f := buildJobStatusFilter(key)
		f["content_type"] = bson.M{"$ne": "clip_only"}
		n, err := h.jobRepo.CountTopLevelGroups(ctx, f)
		if err != nil {
			log.Printf("[OVERVIEW] ingestion %s count failed: %v", key, err)
		}
		ingestion[key] = n
	}
	failed24 := buildJobStatusFilter("failed")
	failed24["updated_at"] = bson.M{"$gte": dayAgo}
	ingestion["failed_24h"] = h.count(ctx, "ingestion_jobs", failed24)
	completed24 := buildJobStatusFilter("completed")
	completed24["updated_at"] = bson.M{"$gte": dayAgo}
	ingestion["completed_24h"] = h.count(ctx, "ingestion_jobs", completed24)

	publishQueue := gin.H{
		"scheduled": h.count(ctx, "publish_jobs", bson.M{"status": models.PublishJobStatusPending}),
		"success_24h": h.count(ctx, "publish_jobs", bson.M{
			"status":     models.PublishJobStatusSuccess,
			"updated_at": bson.M{"$gte": dayAgo},
		}),
		"failed_24h": h.count(ctx, "publish_jobs", bson.M{
			"status":     models.PublishJobStatusFailed,
			"updated_at": bson.M{"$gte": dayAgo},
		}),
	}

	// ── Content quality ────────────────────────────────────────────────
	noVideo := bson.M{"$and": bson.A{
		emptyField("video_url"),
		emptyField("embed_url"),
		emptyField("master_playlist_url"),
	}}
	oldEnough := bson.M{"created_at": bson.M{"$lte": now.Add(-14 * 24 * time.Hour)}}
	lowViews := bson.M{"$and": bson.A{oldEnough, bson.M{"views": bson.M{"$lte": 5}}}}
	quality := gin.H{
		"missing_poster":      h.count(ctx, "movies", emptyField("poster_url")),
		"missing_description": h.count(ctx, "movies", bson.M{"$and": bson.A{emptyField("description"), emptyField("description_uz")}}),
		"missing_video":       h.count(ctx, "movies", noVideo),
		"low_views":           h.count(ctx, "movies", lowViews),
		"samples": gin.H{
			"missing_poster": h.sampleMovies(ctx, emptyField("poster_url"), bson.D{{Key: "created_at", Value: -1}}, 5),
			"missing_video":  h.sampleMovies(ctx, noVideo, bson.D{{Key: "created_at", Value: -1}}, 5),
			"low_views":      h.sampleMovies(ctx, lowViews, bson.D{{Key: "views", Value: 1}, {Key: "created_at", Value: 1}}, 5),
		},
	}

	resp := gin.H{
		"attention":     attention,
		"ingestion":     ingestion,
		"publish_queue": publishQueue,
		"quality":       quality,
	}

	// ── Finance (superadmin only — expenses are superadmin data) ───────
	if role, _ := c.Get("role"); strings.EqualFold(strings.TrimSpace(toString(role)), "superadmin") {
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		stars := h.sum(ctx, "telegram_stars_payments", bson.M{"status": "succeeded", "created_at": bson.M{"$gte": monthStart}}, "stars_amount")
		sales := h.count(ctx, "telegram_stars_payments", bson.M{"status": "succeeded", "created_at": bson.M{"$gte": monthStart}})
		recurring := h.sum(ctx, "expenses", bson.M{"recurring": true}, "amount_usd")
		oneOff := h.sum(ctx, "expenses", bson.M{"recurring": bson.M{"$ne": true}, "incurred_at": bson.M{"$gte": monthStart}}, "amount_usd")
		aiCost := h.sum(ctx, "clip_ai_usage", bson.M{"created_at": bson.M{"$gte": monthStart}}, "cost_usd")
		rate := starsUSDRate()
		revenueUSD := stars * rate
		expensesUSD := recurring + oneOff + aiCost
		resp["finance"] = gin.H{
			"month":              monthStart.Format("2006-01"),
			"premium_sales":      sales,
			"stars_revenue":      int64(stars),
			"stars_usd_rate":     rate,
			"revenue_usd":        revenueUSD,
			"recurring_expenses": recurring,
			"one_off_expenses":   oneOff,
			"ai_clip_cost":       aiCost,
			"expenses_usd":       expensesUSD,
			"net_usd":            revenueUSD - expensesUSD,
		}
	}

	c.JSON(http.StatusOK, resp)
}

func toString(v interface{}) string {
	s, _ := v.(string)
	return s
}

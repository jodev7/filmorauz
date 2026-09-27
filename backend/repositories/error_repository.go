package repositories

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Self-hosted error tracking. Every occurrence is folded into one document
// per fingerprint (source + message + top of stack), so the admin page shows
// "this error, N times, last seen …" instead of a raw log.

const errorRetention = 30 * 24 * time.Hour

type ErrorEvent struct {
	Source    string // "client" | "server"
	Kind      string // client: window|promise|react; server: panic|http5xx
	Message   string
	Stack     string
	URL       string // page URL (client) or route (server)
	Release   string
	UserID    string
	UserAgent string
	Status    int
}

type ErrorGroup struct {
	ID         primitive.ObjectID `bson:"_id" json:"id"`
	Source     string             `bson:"source" json:"source"`
	Kind       string             `bson:"kind" json:"kind"`
	Message    string             `bson:"message" json:"message"`
	Stack      string             `bson:"stack" json:"stack,omitempty"`
	LastURL    string             `bson:"last_url" json:"last_url"`
	Release    string             `bson:"release,omitempty" json:"release,omitempty"`
	Status     int                `bson:"status,omitempty" json:"status,omitempty"`
	Count      int64              `bson:"count" json:"count"`
	Users      []string           `bson:"users,omitempty" json:"-"`
	UserCount  int                `bson:"-" json:"user_count"`
	LastAgent  string             `bson:"last_user_agent,omitempty" json:"last_user_agent,omitempty"`
	FirstSeen  time.Time          `bson:"first_seen" json:"first_seen"`
	LastSeen   time.Time          `bson:"last_seen" json:"last_seen"`
	Resolved   bool               `bson:"resolved" json:"resolved"`
	ResolvedAt *time.Time         `bson:"resolved_at,omitempty" json:"resolved_at,omitempty"`
}

type ErrorRepository struct {
	col *mongo.Collection
}

func NewErrorRepository(db *mongo.Database) *ErrorRepository {
	return &ErrorRepository{col: db.Collection("error_groups")}
}

func (r *ErrorRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "fingerprint", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "last_seen", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(int32(errorRetention.Seconds()))},
		{Keys: bson.D{{Key: "source", Value: 1}, {Key: "resolved", Value: 1}, {Key: "last_seen", Value: -1}}},
	})
	return err
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Fingerprint groups occurrences: source + kind + message + first stack line.
func Fingerprint(e ErrorEvent) string {
	firstFrame := ""
	for _, line := range strings.Split(e.Stack, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.EqualFold(line, e.Message) && !strings.HasPrefix(line, "Error") {
			firstFrame = line
			break
		}
	}
	// Strip volatile bits (query strings, cache-busting hashes in chunk URLs).
	if i := strings.Index(firstFrame, "?"); i >= 0 {
		firstFrame = firstFrame[:i]
	}
	sum := sha1.Sum([]byte(e.Source + "|" + e.Kind + "|" + truncate(e.Message, 300) + "|" + firstFrame))
	return hex.EncodeToString(sum[:])
}

// Record upserts one occurrence. A resolved group that happens again is
// automatically reopened.
func (r *ErrorRepository) Record(ctx context.Context, e ErrorEvent) error {
	now := time.Now()
	set := bson.M{
		"last_seen":       now,
		"last_url":        truncate(e.URL, 500),
		"last_user_agent": truncate(e.UserAgent, 300),
		"resolved":        false,
	}
	if e.Release != "" {
		set["release"] = truncate(e.Release, 60)
	}
	update := bson.M{
		"$inc": bson.M{"count": 1},
		"$set": set,
		"$setOnInsert": bson.M{
			"source":     e.Source,
			"kind":       e.Kind,
			"message":    truncate(e.Message, 1000),
			"stack":      truncate(e.Stack, 4000),
			"status":     e.Status,
			"first_seen": now,
		},
	}
	if e.UserID != "" {
		// Distinct affected users (capped by only adding while small).
		update["$addToSet"] = bson.M{"users": e.UserID}
	}
	_, err := r.col.UpdateOne(ctx, bson.M{"fingerprint": Fingerprint(e)}, update, options.Update().SetUpsert(true))
	if err == nil && e.UserID != "" {
		// Keep the users array bounded.
		_, _ = r.col.UpdateOne(ctx, bson.M{"fingerprint": Fingerprint(e), "users.200": bson.M{"$exists": true}},
			bson.M{"$push": bson.M{"users": bson.M{"$each": bson.A{}, "$slice": -200}}})
	}
	return err
}

func (r *ErrorRepository) List(ctx context.Context, source string, includeResolved bool, limit int) ([]ErrorGroup, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	filter := bson.M{}
	if source == "client" || source == "server" {
		filter["source"] = source
	}
	if !includeResolved {
		filter["resolved"] = false
	}
	cur, err := r.col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "last_seen", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []ErrorGroup{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].UserCount = len(out[i].Users)
	}
	return out, nil
}

func (r *ErrorRepository) Resolve(ctx context.Context, id primitive.ObjectID) error {
	now := time.Now()
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"resolved": true, "resolved_at": now}})
	return err
}

// CountOpenSince counts unresolved groups seen since t (dashboard badge).
func (r *ErrorRepository) CountOpenSince(ctx context.Context, t time.Time) int64 {
	n, _ := r.col.CountDocuments(ctx, bson.M{"resolved": false, "last_seen": bson.M{"$gte": t}})
	return n
}

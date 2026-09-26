package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ChannelPrefs says where one category of notifications is delivered.
type ChannelPrefs struct {
	Site     bool `bson:"site" json:"site"`
	Telegram bool `bson:"telegram" json:"telegram"`
	Push     bool `bson:"push" json:"push"`
}

// NotifyPrefs is a user's settings per category.
type NotifyPrefs map[string]ChannelPrefs

// DefaultNotifyPrefs: everything on.
func DefaultNotifyPrefs() NotifyPrefs {
	p := NotifyPrefs{}
	for _, c := range models.NotificationCategories {
		p[c] = ChannelPrefs{Site: true, Telegram: true, Push: true}
	}
	return p
}

// Normalize fills missing categories with defaults, drops unknown ones and
// keeps account notices on the site.
func (p NotifyPrefs) Normalize() NotifyPrefs {
	out := DefaultNotifyPrefs()
	for c := range out {
		if v, ok := p[c]; ok {
			out[c] = v
		}
	}
	acc := out[models.NotifCatAccount]
	acc.Site = true
	out[models.NotifCatAccount] = acc
	return out
}

// For returns the channels for a notification type.
func (p NotifyPrefs) For(t models.NotificationType) ChannelPrefs {
	return p.Normalize()[models.NotificationCategoryOf(t)]
}

type PushSubscriptionDoc struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	UserID     primitive.ObjectID `bson:"user_id"`
	Endpoint   string             `bson:"endpoint"`
	P256dh     string             `bson:"p256dh"`
	Auth       string             `bson:"auth"`
	UserAgent  string             `bson:"user_agent,omitempty"`
	CreatedAt  time.Time          `bson:"created_at"`
	LastUsedAt time.Time          `bson:"last_used_at"`
}

type NotifyPrefsRepository struct {
	prefs *mongo.Collection
	subs  *mongo.Collection
}

func NewNotifyPrefsRepository(db *mongo.Database) *NotifyPrefsRepository {
	return &NotifyPrefsRepository{prefs: db.Collection("notification_prefs"), subs: db.Collection("push_subscriptions")}
}

func (r *NotifyPrefsRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.subs.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "endpoint", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
	})
	return err
}

// Get returns the user's settings (defaults when never saved).
func (r *NotifyPrefsRepository) Get(ctx context.Context, userID primitive.ObjectID) (NotifyPrefs, error) {
	var doc struct {
		Prefs NotifyPrefs `bson:"prefs"`
	}
	err := r.prefs.FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return DefaultNotifyPrefs(), nil
	}
	if err != nil {
		return DefaultNotifyPrefs(), err
	}
	return doc.Prefs.Normalize(), nil
}

func (r *NotifyPrefsRepository) Set(ctx context.Context, userID primitive.ObjectID, p NotifyPrefs) (NotifyPrefs, error) {
	p = p.Normalize()
	_, err := r.prefs.UpdateOne(ctx, bson.M{"_id": userID},
		bson.M{"$set": bson.M{"prefs": p, "updated_at": time.Now()}}, options.Update().SetUpsert(true))
	return p, err
}

// AddSubscription stores (or re-assigns) a browser push subscription.
func (r *NotifyPrefsRepository) AddSubscription(ctx context.Context, userID primitive.ObjectID, endpoint, p256dh, auth, ua string) error {
	now := time.Now()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	_, err := r.subs.UpdateOne(ctx, bson.M{"endpoint": endpoint}, bson.M{
		"$set":         bson.M{"user_id": userID, "p256dh": p256dh, "auth": auth, "user_agent": ua, "last_used_at": now},
		"$setOnInsert": bson.M{"created_at": now},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return err
	}
	// Keep at most 10 devices per user: drop the oldest.
	cur, err := r.subs.Find(ctx, bson.M{"user_id": userID},
		options.Find().SetSort(bson.D{{Key: "last_used_at", Value: -1}}).SetSkip(10).SetProjection(bson.M{"_id": 1}))
	if err == nil {
		var old []struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if cur.All(ctx, &old) == nil && len(old) > 0 {
			ids := make([]primitive.ObjectID, 0, len(old))
			for _, o := range old {
				ids = append(ids, o.ID)
			}
			_, _ = r.subs.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
		}
	}
	return nil
}

func (r *NotifyPrefsRepository) RemoveSubscription(ctx context.Context, userID primitive.ObjectID, endpoint string) error {
	_, err := r.subs.DeleteOne(ctx, bson.M{"endpoint": endpoint, "user_id": userID})
	return err
}

func (r *NotifyPrefsRepository) DeleteEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.subs.DeleteOne(ctx, bson.M{"endpoint": endpoint})
	return err
}

func (r *NotifyPrefsRepository) Subscriptions(ctx context.Context, userID primitive.ObjectID) ([]PushSubscriptionDoc, error) {
	cur, err := r.subs.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	var out []PushSubscriptionDoc
	err = cur.All(ctx, &out)
	return out, err
}

func (r *NotifyPrefsRepository) CountSubscriptions(ctx context.Context, userID primitive.ObjectID) int64 {
	n, _ := r.subs.CountDocuments(ctx, bson.M{"user_id": userID})
	return n
}

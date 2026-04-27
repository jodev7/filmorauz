package repositories

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/filmorauz/backend/models"
)

type TelegramPostRepository struct {
	col *mongo.Collection
}

func NewTelegramPostRepository(db *mongo.Database) *TelegramPostRepository {
	return &TelegramPostRepository{
		col: db.Collection("telegram_posts"),
	}
}

func (r *TelegramPostRepository) Create(post *models.TelegramPost) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	post.CreatedAt = time.Now()
	post.SentAt = time.Now()

	result, err := r.col.InsertOne(ctx, post)
	if err != nil {
		return err
	}

	post.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// StripLegacyMediaPrefix rewrites legacy "/media/<rest>" image_url values to "/<rest>".
// Frontend now assumes a clean path and prepends its own base URL.
func (r *TelegramPostRepository) StripLegacyMediaPrefix() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := bson.M{"image_url": bson.M{"$regex": "^/media/"}}
	cursor, err := r.col.Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		cursor.Close(ctx)
		return 0, err
	}
	cursor.Close(ctx)

	total := 0
	for _, doc := range docs {
		value, _ := doc["image_url"].(string)
		if !strings.HasPrefix(value, "/media/") {
			continue
		}
		cleaned := "/" + strings.TrimPrefix(value, "/media/")
		if _, err := r.col.UpdateOne(ctx, bson.M{"_id": doc["_id"]}, bson.M{"$set": bson.M{"image_url": cleaned}}); err != nil {
			return total, err
		}
		total++
	}
	return total, nil
}

func (r *TelegramPostRepository) List(limit int) ([]models.TelegramPost, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().
		SetSort(bson.D{{Key: "sent_at", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var posts []models.TelegramPost
	if err := cursor.All(ctx, &posts); err != nil {
		return nil, err
	}

	if posts == nil {
		posts = []models.TelegramPost{}
	}

	return posts, nil
}

package repositories

import (
	"context"
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

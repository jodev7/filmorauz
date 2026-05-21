package repositories

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ContentClipRepository struct {
	col *mongo.Collection
}

func NewContentClipRepository(db *mongo.Database) *ContentClipRepository {
	r := &ContentClipRepository{col: db.Collection("content_clips")}
	r.ensureIndexes()
	return r
}

func (r *ContentClipRepository) ensureIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "folder_id", Value: 1}, {Key: "created_at", Value: -1}}},
	})
	if err != nil {
		log.Printf("[content_clip] index create warn: %v", err)
	}
}

func (r *ContentClipRepository) Create(ctx context.Context, clip *models.ContentClip) error {
	clip.CreatedAt = time.Now()
	res, err := r.col.InsertOne(ctx, clip)
	if err != nil {
		return fmt.Errorf("insert content clip: %w", err)
	}
	clip.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *ContentClipRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.ContentClip, error) {
	var c models.ContentClip
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *ContentClipRepository) Update(ctx context.Context, id primitive.ObjectID, patch bson.M) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": patch})
	return err
}

func (r *ContentClipRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *ContentClipRepository) DeleteByFolder(ctx context.Context, folderID primitive.ObjectID) (int64, error) {
	res, err := r.col.DeleteMany(ctx, bson.M{"folder_id": folderID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (r *ContentClipRepository) ListByFolder(ctx context.Context, folderID primitive.ObjectID) ([]models.ContentClip, error) {
	cur, err := r.col.Find(ctx, bson.M{"folder_id": folderID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.ContentClip
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RecordUpload bumps the aggregate counters after a publish attempt.
func (r *ContentClipRepository) RecordUpload(ctx context.Context, id primitive.ObjectID, status string) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{"last_upload_status": status, "last_upload_at": now},
	}
	if status == "success" {
		update["$inc"] = bson.M{"upload_count": 1}
	}
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

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

type ContentFolderRepository struct {
	col *mongo.Collection
}

func NewContentFolderRepository(db *mongo.Database) *ContentFolderRepository {
	r := &ContentFolderRepository{col: db.Collection("content_folders")}
	r.ensureIndexes()
	return r
}

func (r *ContentFolderRepository) ensureIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "sort_order", Value: 1}, {Key: "created_at", Value: -1}}},
	})
	if err != nil {
		log.Printf("[content_folder] index create warn: %v", err)
	}
}

func (r *ContentFolderRepository) Create(ctx context.Context, folder *models.ContentFolder) error {
	now := time.Now()
	folder.CreatedAt = now
	folder.UpdatedAt = now
	res, err := r.col.InsertOne(ctx, folder)
	if err != nil {
		return fmt.Errorf("insert folder: %w", err)
	}
	folder.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *ContentFolderRepository) Update(ctx context.Context, id primitive.ObjectID, patch bson.M) error {
	patch["updated_at"] = time.Now()
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": patch})
	return err
}

func (r *ContentFolderRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *ContentFolderRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.ContentFolder, error) {
	var f models.ContentFolder
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&f)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (r *ContentFolderRepository) SlugExists(ctx context.Context, slug string, excludeID primitive.ObjectID) (bool, error) {
	filter := bson.M{"slug": slug}
	if excludeID != primitive.NilObjectID {
		filter["_id"] = bson.M{"$ne": excludeID}
	}
	n, err := r.col.CountDocuments(ctx, filter)
	return n > 0, err
}

func (r *ContentFolderRepository) ListAll(ctx context.Context) ([]models.ContentFolder, error) {
	cur, err := r.col.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{
		{Key: "sort_order", Value: 1}, {Key: "created_at", Value: -1},
	}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.ContentFolder
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// IncrementClipsCount adds delta (positive or negative) to clips_count.
func (r *ContentFolderRepository) IncrementClipsCount(ctx context.Context, id primitive.ObjectID, delta int) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$inc": bson.M{"clips_count": delta},
		"$set": bson.M{"updated_at": time.Now()},
	})
	return err
}

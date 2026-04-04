package repositories

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CollectionRepository struct {
	col *mongo.Collection
}

func NewCollectionRepository(db *mongo.Database) *CollectionRepository {
	repo := &CollectionRepository{col: db.Collection("collections")}
	repo.ensureIndexes()
	return repo
}

// ensureIndexes creates required indexes on the collections collection
func (r *CollectionRepository) ensureIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Unique index on slug
	slugIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "slug", Value: 1}},
		Options: options.Index().SetUnique(true),
	}

	// Index for featured collections query
	featuredIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "is_published", Value: 1},
			{Key: "is_featured", Value: 1},
			{Key: "sort_order", Value: 1},
		},
	}

	// Index for published collections query
	publishedIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "is_published", Value: 1},
			{Key: "sort_order", Value: 1},
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{slugIndex, featuredIndex, publishedIndex})
	if err != nil {
		log.Printf("Warning: failed to create collection indexes: %v", err)
	}
}

// Create inserts a new collection
func (r *CollectionRepository) Create(ctx context.Context, collection *models.Collection) error {
	collection.CreatedAt = time.Now()
	collection.UpdatedAt = time.Now()

	result, err := r.col.InsertOne(ctx, collection)
	if err != nil {
		return fmt.Errorf("failed to insert collection: %w", err)
	}

	collection.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// Update modifies an existing collection
func (r *CollectionRepository) Update(ctx context.Context, id primitive.ObjectID, collection *models.Collection) error {
	collection.UpdatedAt = time.Now()

	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": collection})
	if err != nil {
		return fmt.Errorf("failed to update collection: %w", err)
	}

	return nil
}

// Delete removes a collection by ID
func (r *CollectionRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	return nil
}

// GetByID retrieves a collection by its ID
func (r *CollectionRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.Collection, error) {
	var collection models.Collection
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&collection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get collection by id: %w", err)
	}

	return &collection, nil
}

// GetBySlug retrieves a collection by its slug
func (r *CollectionRepository) GetBySlug(ctx context.Context, slug string) (*models.Collection, error) {
	var collection models.Collection
	err := r.col.FindOne(ctx, bson.M{"slug": slug}).Decode(&collection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get collection by slug: %w", err)
	}

	return &collection, nil
}

// GetAll retrieves all collections (for admin)
func (r *CollectionRepository) GetAll(ctx context.Context) ([]models.Collection, error) {
	cursor, err := r.col.Find(ctx, bson.M{}, &options.FindOptions{
		Sort: bson.D{{Key: "sort_order", Value: 1}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get all collections: %w", err)
	}
	defer cursor.Close(ctx)

	var collections []models.Collection
	if err := cursor.All(ctx, &collections); err != nil {
		return nil, fmt.Errorf("failed to decode collections: %w", err)
	}

	return collections, nil
}

// GetPublished retrieves only published collections
func (r *CollectionRepository) GetPublished(ctx context.Context) ([]models.Collection, error) {
	cursor, err := r.col.Find(ctx, bson.M{"is_published": true}, &options.FindOptions{
		Sort: bson.D{{Key: "sort_order", Value: 1}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get published collections: %w", err)
	}
	defer cursor.Close(ctx)

	var collections []models.Collection
	if err := cursor.All(ctx, &collections); err != nil {
		return nil, fmt.Errorf("failed to decode collections: %w", err)
	}

	return collections, nil
}

// GetFeatured retrieves featured and published collections
func (r *CollectionRepository) GetFeatured(ctx context.Context) ([]models.Collection, error) {
	cursor, err := r.col.Find(ctx, bson.M{
		"is_published": true,
		"is_featured":  true,
	}, &options.FindOptions{
		Sort: bson.D{{Key: "sort_order", Value: 1}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get featured collections: %w", err)
	}
	defer cursor.Close(ctx)

	var collections []models.Collection
	if err := cursor.All(ctx, &collections); err != nil {
		return nil, fmt.Errorf("failed to decode collections: %w", err)
	}

	return collections, nil
}

// GetByIDHex retrieves a collection by its hex string ID (for admin)
func (r *CollectionRepository) GetByIDHex(ctx context.Context, idHex string) (*models.Collection, error) {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return nil, fmt.Errorf("invalid collection id: %w", err)
	}
	return r.GetByID(ctx, id)
}

// CheckSlugExists checks if a slug already exists (excluding a specific collection ID)
func (r *CollectionRepository) CheckSlugExists(ctx context.Context, slug string, excludeID primitive.ObjectID) (bool, error) {
	filter := bson.M{"slug": slug}
	if excludeID != primitive.NilObjectID {
		filter["_id"] = bson.M{"$ne": excludeID}
	}

	count, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return false, fmt.Errorf("failed to check slug exists: %w", err)
	}

	return count > 0, nil
}



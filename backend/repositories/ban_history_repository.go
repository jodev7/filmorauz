package repositories

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BanHistoryRepository struct {
	col *mongo.Collection
}

func NewBanHistoryRepository(db *mongo.Database) *BanHistoryRepository {
	col := db.Collection("ban_history")

	// Create indexes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "banned_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "unbanned_at", Value: 1}},
		},
	}

	col.Indexes().CreateMany(ctx, indexes)

	return &BanHistoryRepository{col: col}
}

// Collection returns the mongo collection for direct access
func (r *BanHistoryRepository) Collection() *mongo.Collection {
	return r.col
}

// Create creates a new ban history record
func (r *BanHistoryRepository) Create(banHistory *models.BanHistory) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	banHistory.CreatedAt = time.Now()
	result, err := r.col.InsertOne(ctx, banHistory)
	if err != nil {
		return err
	}

	banHistory.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// FindByID finds a ban history record by ID
func (r *BanHistoryRepository) FindByID(id string) (*models.BanHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var banHistory models.BanHistory
	err = r.col.FindOne(ctx, bson.M{"_id": objectID}).Decode(&banHistory)
	if err != nil {
		return nil, err
	}

	return &banHistory, nil
}

// FindByUserID finds all ban history records for a user
func (r *BanHistoryRepository) FindByUserID(userID string) ([]models.BanHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userObjectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, err
	}

	cursor, err := r.col.Find(ctx, bson.M{"user_id": userObjectID}, &options.FindOptions{
		Sort: bson.D{{Key: "banned_at", Value: -1}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []models.BanHistory
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}

	return records, nil
}

// FindAll finds all ban history records with optional filters
func (r *BanHistoryRepository) FindAll(params *BanHistoryQueryParams) ([]models.BanHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{}

	// Apply status filter
	if params.Status != "" && params.Status != "all" {
		if params.Status == "active" {
			// Active = not unbanned AND (permanent OR not expired)
			filter["$and"] = []bson.M{
				{"unbanned_at": nil},
				{"$or": []bson.M{
					{"is_permanent": true},
					{"banned_until": bson.M{"$gt": time.Now()}},
				}},
			}
		} else if params.Status == "unbanned" {
			filter["unbanned_at"] = bson.M{"$ne": nil}
		} else if params.Status == "expired" {
			// Expired = not unbanned AND not permanent AND expired
			filter["$and"] = []bson.M{
				{"unbanned_at": nil},
				{"is_permanent": false},
				{"banned_until": bson.M{"$lte": time.Now()}},
			}
		}
	}

	// Apply search filter
	if params.Search != "" {
		normalizedSearch := strings.TrimPrefix(params.Search, "@")
		orConditions := []bson.M{
			{"banned_by_username": bson.M{"$regex": normalizedSearch, "$options": "i"}},
			{"reason": bson.M{"$regex": normalizedSearch, "$options": "i"}},
		}

		if telegramID, err := strconv.ParseInt(normalizedSearch, 10, 64); err == nil {
			orConditions = append(orConditions, bson.M{"user_telegram_id": telegramID})
		}

		if objID, err := primitive.ObjectIDFromHex(normalizedSearch); err == nil {
			orConditions = append(orConditions, bson.M{"user_id": objID})
		}

		filter["$or"] = orConditions
	}

	cursor, err := r.col.Find(ctx, filter, &options.FindOptions{
		Sort:  bson.D{{Key: "banned_at", Value: -1}},
		Limit: func(i int64) *int64 { return &i }(1000), // Limit for safety
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []models.BanHistory
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}

	return records, nil
}

// UpdateUnban updates a ban history record when unbanned
func (r *BanHistoryRepository) UpdateUnban(id string, unbannedByUserID primitive.ObjectID, unbannedByUsername string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	now := time.Now()
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": objectID}, bson.M{
		"$set": bson.M{
			"unbanned_at":          now,
			"unbanned_by_user_id":  unbannedByUserID,
			"unbanned_by_username": unbannedByUsername,
		},
	})
	return err
}

// FindActiveBanByUserID finds the active ban for a user
func (r *BanHistoryRepository) FindActiveBanByUserID(userID string) (*models.BanHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userObjectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, err
	}

	// Find active ban: not unbanned AND (permanent OR not expired)
	filter := bson.M{
		"user_id":     userObjectID,
		"unbanned_at": nil,
		"$or": []bson.M{
			{"is_permanent": true},
			{"banned_until": bson.M{"$gt": time.Now()}},
		},
	}

	var banHistory models.BanHistory
	err = r.col.FindOne(ctx, filter, &options.FindOneOptions{
		Sort: bson.D{{Key: "banned_at", Value: -1}},
	}).Decode(&banHistory)

	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &banHistory, nil
}

// BanHistoryQueryParams holds query parameters for FindAll
type BanHistoryQueryParams struct {
	Search string
	Status string // "all", "active", "unbanned", "expired"
}

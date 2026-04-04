package repositories

import (
	"context"
	"math"
	"time"

	"github.com/filmorauz/backend/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BanAppealRepository struct {
	collection *mongo.Collection
}

func NewBanAppealRepository(db *mongo.Database) *BanAppealRepository {
	collection := db.Collection("ban_appeals")

	// Create indexes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "status", Value: 1},
			},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
	}

	collection.Indexes().CreateMany(ctx, indexes)

	return &BanAppealRepository{
		collection: collection,
	}
}

// Create creates a new ban appeal
func (r *BanAppealRepository) Create(ctx context.Context, appeal *models.BanAppeal) error {
	appeal.ID = primitive.NewObjectID()
	appeal.CreatedAt = time.Now()
	appeal.Status = models.BanAppealStatusPending

	_, err := r.collection.InsertOne(ctx, appeal)
	return err
}

// FindAll retrieves all appeals with pagination and filtering
func (r *BanAppealRepository) FindAll(ctx context.Context, status string, search string, page int, perPage int) (*models.GetBanAppealsResponse, error) {
	filter := bson.M{}

	// Status filter
	if status != "" && status != "all" {
		filter["status"] = status
	}

	// Search filter (by username, telegram_id, or user_id)
	if search != "" {
		filter["$or"] = []bson.M{
			{"username": bson.M{"$regex": search, "$options": "i"}},
			{"telegram_id": bson.M{"$regex": search, "$options": "i"}},
			{"user_id": bson.M{"$regex": search, "$options": "i"}},
		}
	}

	// Count total
	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Pagination
	skip := (page - 1) * perPage
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64(skip)).
		SetLimit(int64(perPage))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var appeals []models.BanAppeal
	if err := cursor.All(ctx, &appeals); err != nil {
		return nil, err
	}

	if appeals == nil {
		appeals = []models.BanAppeal{}
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	return &models.GetBanAppealsResponse{
		Appeals:    appeals,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

// FindByID retrieves a single appeal by ID
func (r *BanAppealRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.BanAppeal, error) {
	var appeal models.BanAppeal
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&appeal)
	if err != nil {
		return nil, err
	}
	return &appeal, nil
}

// FindByUserID retrieves all appeals for a specific user
func (r *BanAppealRepository) FindByUserID(ctx context.Context, userID primitive.ObjectID) (*models.GetMyAppealsResponse, error) {
	filter := bson.M{"user_id": userID}

	// Count total
	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var appeals []models.BanAppeal
	if err := cursor.All(ctx, &appeals); err != nil {
		return nil, err
	}

	if appeals == nil {
		appeals = []models.BanAppeal{}
	}

	return &models.GetMyAppealsResponse{
		Appeals: appeals,
		Total:   total,
	}, nil
}

// FindPendingAppealByUserID checks if user has a pending appeal for active ban
func (r *BanAppealRepository) FindPendingAppealByUserID(ctx context.Context, userID primitive.ObjectID) (*models.BanAppeal, error) {
	var appeal models.BanAppeal
	err := r.collection.FindOne(ctx, bson.M{
		"user_id": userID,
		"status":  models.BanAppealStatusPending,
	}).Decode(&appeal)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &appeal, nil
}

// UpdateReview updates the appeal with review information
func (r *BanAppealRepository) UpdateReview(ctx context.Context, id primitive.ObjectID, review *models.ReviewBanAppealRequest, reviewerID primitive.ObjectID, reviewerUsername string) error {
	now := time.Now()

	status := models.BanAppealStatusRejected
	if review.Action == "approve" {
		status = models.BanAppealStatusApproved
	}

	update := bson.M{
		"$set": bson.M{
			"status":               status,
			"admin_note":           review.AdminNote,
			"reviewed_at":          now,
			"reviewed_by_user_id":  reviewerID,
			"reviewed_by_username": reviewerUsername,
		},
	}

	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

// GetPendingCount returns the count of pending appeals
func (r *BanAppealRepository) GetPendingCount(ctx context.Context) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"status": models.BanAppealStatusPending})
}

// GetStats returns appeal statistics
func (r *BanAppealRepository) GetStats(ctx context.Context) (map[string]int64, error) {
	stats := make(map[string]int64)

	pending, err := r.collection.CountDocuments(ctx, bson.M{"status": models.BanAppealStatusPending})
	if err != nil {
		return nil, err
	}
	stats["pending"] = pending

	approved, err := r.collection.CountDocuments(ctx, bson.M{"status": models.BanAppealStatusApproved})
	if err != nil {
		return nil, err
	}
	stats["approved"] = approved

	rejected, err := r.collection.CountDocuments(ctx, bson.M{"status": models.BanAppealStatusRejected})
	if err != nil {
		return nil, err
	}
	stats["rejected"] = rejected

	total, err := r.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	return stats, nil
}

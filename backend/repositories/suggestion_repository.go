package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SuggestionRepository struct {
	col *mongo.Collection
}

func NewSuggestionRepository(db *mongo.Database) *SuggestionRepository {
	repo := &SuggestionRepository{col: db.Collection("suggestions")}
	repo.EnsureIndexes()
	return repo
}

func (r *SuggestionRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	statusIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "status", Value: 1}},
	}
	userIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}},
	}
	createdAtIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "created_at", Value: -1}},
	}

	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{statusIndex, userIndex, createdAtIndex})
	return err
}

func (r *SuggestionRepository) Create(suggestion *models.Suggestion) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	suggestion.CreatedAt = time.Now()
	suggestion.UpdatedAt = time.Now()
	suggestion.Status = models.SuggestionStatusPending

	result, err := r.col.InsertOne(ctx, suggestion)
	if err != nil {
		return err
	}

	suggestion.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *SuggestionRepository) FindByID(id primitive.ObjectID) (*models.Suggestion, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var suggestion models.Suggestion
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&suggestion)
	if err != nil {
		return nil, err
	}

	return &suggestion, nil
}

func (r *SuggestionRepository) FindByUserID(userID primitive.ObjectID, page, limit int) ([]models.Suggestion, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	filter := bson.M{"user_id": userID}

	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var suggestions []models.Suggestion
	if err := cursor.All(ctx, &suggestions); err != nil {
		return nil, 0, err
	}

	if suggestions == nil {
		suggestions = []models.Suggestion{}
	}

	return suggestions, total, nil
}

func (r *SuggestionRepository) List(page, limit int, status *models.SuggestionStatus) ([]models.Suggestion, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}

	filter := bson.M{}
	if status != nil && *status != "" {
		filter["status"] = *status
	}

	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var suggestions []models.Suggestion
	if err := cursor.All(ctx, &suggestions); err != nil {
		return nil, 0, err
	}

	if suggestions == nil {
		suggestions = []models.Suggestion{}
	}

	return suggestions, total, nil
}

func (r *SuggestionRepository) Update(id primitive.ObjectID, suggestion *models.Suggestion) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	suggestion.UpdatedAt = time.Now()

	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": suggestion},
	)
	return err
}

func (r *SuggestionRepository) UpdateStatus(id primitive.ObjectID, status models.SuggestionStatus, adminMessage, reviewedBy string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":        status,
			"admin_message": adminMessage,
			"reviewed_by":   reviewedBy,
			"reviewed_at":   now,
			"updated_at":    now,
		},
	}

	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

func (r *SuggestionRepository) FindByIDHex(idHex string) (*models.Suggestion, error) {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return nil, err
	}
	return r.FindByID(id)
}

func (r *SuggestionRepository) Delete(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *SuggestionRepository) CountByStatus(status models.SuggestionStatus) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"status": status}
	return r.col.CountDocuments(ctx, filter)
}

func (r *SuggestionRepository) GetStats() (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats := make(map[string]int64)

	total, err := r.col.CountDocuments(ctx, bson.M{})
	if err != nil {
		return stats, fmt.Errorf("count total: %w", err)
	}
	stats["total"] = total

	pending, err := r.col.CountDocuments(ctx, bson.M{"status": models.SuggestionStatusPending})
	if err != nil {
		return stats, fmt.Errorf("count pending: %w", err)
	}
	stats["pending"] = pending

	accepted, err := r.col.CountDocuments(ctx, bson.M{"status": models.SuggestionStatusAccepted})
	if err != nil {
		return stats, fmt.Errorf("count accepted: %w", err)
	}
	stats["accepted"] = accepted

	rejected, err := r.col.CountDocuments(ctx, bson.M{"status": models.SuggestionStatusRejected})
	if err != nil {
		return stats, fmt.Errorf("count rejected: %w", err)
	}
	stats["rejected"] = rejected

	return stats, nil
}

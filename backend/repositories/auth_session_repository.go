package repositories

import (
	"context"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AuthSessionRepository struct {
	col *mongo.Collection
}

func NewAuthSessionRepository(db *mongo.Database) *AuthSessionRepository {
	return &AuthSessionRepository{col: db.Collection("auth_sessions")}
}

// Create creates a new auth session
func (r *AuthSessionRepository) Create(session *models.AuthSession) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.InsertOne(ctx, session)
	return err
}

// FindByCode finds an auth session by its code (case-insensitive)
func (r *AuthSessionRepository) FindByCode(code string) (*models.AuthSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("[AUTH REPO] FindByCode: looking for code=%s (case-insensitive)", code)

	// Use case-insensitive regex search
	var session models.AuthSession
	err := r.col.FindOne(ctx, bson.M{"code": bson.M{"$regex": "^" + code + "$", "$options": "i"}}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("[AUTH REPO] FindByCode: no document found for code=%s (case-insensitive)", code)
		} else {
			log.Printf("[AUTH REPO] FindByCode: error for code=%s: %v", code, err)
		}
		return nil, err
	}
	log.Printf("[AUTH REPO] FindByCode: found session for code=%s, actual_code=%s, status=%s", code, session.Code, session.Status)
	return &session, nil
}

// UpdateStatus updates the status of an auth session
func (r *AuthSessionRepository) UpdateStatus(code string, status string, userID primitive.ObjectID, telegramID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	if status == models.AuthSessionStatusCompleted {
		update["$set"].(bson.M)["completed_at"] = time.Now()
	}

	if !userID.IsZero() {
		update["$set"].(bson.M)["user_id"] = userID
	}

	if telegramID > 0 {
		update["$set"].(bson.M)["telegram_id"] = telegramID
	}

	_, err := r.col.UpdateOne(ctx, bson.M{"code": code}, update)
	return err
}

// MarkAsCompleted marks an auth session as completed (case-insensitive)
func (r *AuthSessionRepository) MarkAsCompleted(code string, userID primitive.ObjectID, telegramID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("[AUTH REPO] MarkAsCompleted: code=%s, user_id=%s, telegram_id=%d", code, userID.Hex(), telegramID)

	_, err := r.col.UpdateOne(ctx, bson.M{"code": bson.M{"$regex": "^" + code + "$", "$options": "i"}}, bson.M{
		"$set": bson.M{
			"status":       models.AuthSessionStatusCompleted,
			"user_id":      userID,
			"telegram_id":  telegramID,
			"completed_at": time.Now(),
			"updated_at":   time.Now(),
		},
	})
	if err != nil {
		log.Printf("[AUTH REPO] MarkAsCompleted error: %v", err)
	}
	return err
}

// MarkAsExpired marks an auth session as expired (case-insensitive)
func (r *AuthSessionRepository) MarkAsExpired(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("[AUTH REPO] MarkAsExpired: code=%s", code)

	_, err := r.col.UpdateOne(ctx, bson.M{"code": bson.M{"$regex": "^" + code + "$", "$options": "i"}}, bson.M{
		"$set": bson.M{
			"status":     models.AuthSessionStatusExpired,
			"updated_at": time.Now(),
		},
	})
	if err != nil {
		log.Printf("[AUTH REPO] MarkAsExpired error: %v", err)
	}
	return err
}

// DeleteByCode deletes an auth session by code (case-insensitive)
func (r *AuthSessionRepository) DeleteByCode(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("[AUTH REPO] DeleteByCode: code=%s", code)

	_, err := r.col.DeleteOne(ctx, bson.M{"code": bson.M{"$regex": "^" + code + "$", "$options": "i"}})
	if err != nil {
		log.Printf("[AUTH REPO] DeleteByCode error: %v", err)
	}
	return err
}

// CleanupExpired deletes all expired auth sessions
func (r *AuthSessionRepository) CleanupExpired() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := r.col.DeleteMany(ctx, bson.M{
		"expires_at": bson.M{"$lt": time.Now()},
	})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

// EnsureIndexes creates required indexes for the auth_sessions collection
func (r *AuthSessionRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "code", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0), // TTL index - auto-delete after expiration
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

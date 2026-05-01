package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PremiumPurchaseSessionRepository struct {
	col *mongo.Collection
}

func NewPremiumPurchaseSessionRepository(db *mongo.Database) *PremiumPurchaseSessionRepository {
	return &PremiumPurchaseSessionRepository{col: db.Collection("premium_purchase_sessions")}
}

func (r *PremiumPurchaseSessionRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "token", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_token"),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetName("ttl_expires_at"),
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

func (r *PremiumPurchaseSessionRepository) Insert(session *models.PremiumPurchaseSession) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.col.InsertOne(ctx, session)
	return err
}

func (r *PremiumPurchaseSessionRepository) FindByToken(token string) (*models.PremiumPurchaseSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var session models.PremiumPurchaseSession
	err := r.col.FindOne(ctx, bson.M{"token": token}).Decode(&session)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (r *PremiumPurchaseSessionRepository) UpdateStatus(token string, status string, chargeID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	set := bson.M{
		"status":     status,
		"updated_at": now,
	}
	if chargeID != "" {
		set["telegram_payment_charge_id"] = chargeID
	}
	if status == models.PremiumPurchaseSessionStatusPaid {
		set["completed_at"] = now
	}
	_, err := r.col.UpdateOne(ctx, bson.M{"token": token}, bson.M{
		"$set": set,
	})
	return err
}

func (r *PremiumPurchaseSessionRepository) MarkPaid(token string, chargeID string) error {
	return r.UpdateStatus(token, models.PremiumPurchaseSessionStatusPaid, chargeID)
}

func (r *PremiumPurchaseSessionRepository) MarkExpired(token string) error {
	return r.UpdateStatus(token, models.PremiumPurchaseSessionStatusExpired, "")
}

func (r *PremiumPurchaseSessionRepository) MarkCancelled(token string) error {
	return r.UpdateStatus(token, models.PremiumPurchaseSessionStatusCancelled, "")
}

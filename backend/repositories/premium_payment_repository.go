package repositories

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/filmorauz/backend/models"
)

type PremiumPaymentRepository struct {
	col *mongo.Collection
}

func NewPremiumPaymentRepository(db *mongo.Database) *PremiumPaymentRepository {
	return &PremiumPaymentRepository{col: db.Collection("telegram_stars_payments")}
}

func (r *PremiumPaymentRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "telegram_payment_charge_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_tg_charge_id"),
	})
	return err
}

// FindByChargeID returns (nil, nil) when no record exists.
func (r *PremiumPaymentRepository) FindByChargeID(chargeID string) (*models.TelegramStarsPayment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var p models.TelegramStarsPayment
	err := r.col.FindOne(ctx, bson.M{"telegram_payment_charge_id": chargeID}).Decode(&p)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// Insert returns mongo.WriteException with code 11000 if charge_id already exists (duplicate).
func (r *PremiumPaymentRepository) Insert(p *models.TelegramStarsPayment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	_, err := r.col.InsertOne(ctx, p)
	return err
}

// IsDuplicateKey reports whether err is a mongo duplicate-key error (code 11000).
func IsDuplicateKey(err error) bool {
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			if e.Code == 11000 {
				return true
			}
		}
	}
	return false
}

package repositories

import (
	"context"
	"errors"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/filmorauz/backend/models"
)

type AdRepository struct {
	col      *mongo.Collection
	delivCol *mongo.Collection
	rotCol   *mongo.Collection
}

func NewAdRepository(db *mongo.Database) *AdRepository {
	return &AdRepository{
		col:      db.Collection("ads"),
		delivCol: db.Collection("ad_deliveries"),
		rotCol:   db.Collection("ad_rotation"),
	}
}

// activeAdsFilter returns the filter for active, non-expired, in-schedule ads for a placement.
func activeAdsFilter(placement string) bson.M {
	now := time.Now()
	return bson.M{
		"status":     models.AdStatusActive,
		"placements": placement,
		"$and": []bson.M{
			{
				"$or": []bson.M{
					{"starts_at": bson.M{"$exists": false}},
					{"starts_at": bson.M{"$lte": now}},
				},
			},
			{
				"$or": []bson.M{
					{"ends_at": nil},
					{"ends_at": bson.M{"$exists": false}},
					{"ends_at": bson.M{"$gte": now}},
				},
			},
		},
	}
}

// NextAdForPlacement returns the next ad in round-robin order for the placement.
// Rotation state (last_index) is persisted in the ad_rotation collection.
func (r *AdRepository) NextAdForPlacement(placement string) (*models.Ad, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Load ordered active ads: stable order by _id ASC
	cursor, err := r.col.Find(ctx, activeAdsFilter(placement),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var ads []models.Ad
	if err := cursor.All(ctx, &ads); err != nil {
		return nil, err
	}
	if len(ads) == 0 {
		return nil, nil
	}
	if len(ads) == 1 {
		return &ads[0], nil
	}

	// Fetch current rotation state
	var state models.AdRotationState
	err = r.rotCol.FindOne(ctx, bson.M{"placement": placement}).Decode(&state)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	// state.LastIndex is 0 on first call (ErrNoDocuments) — next will be index 0
	nextIdx := (state.LastIndex + 1) % len(ads)

	// Persist updated index (non-fatal on failure)
	_, updErr := r.rotCol.UpdateOne(ctx,
		bson.M{"placement": placement},
		bson.M{"$set": bson.M{"last_index": nextIdx, "updated_at": time.Now()}},
		options.Update().SetUpsert(true),
	)
	if updErr != nil {
		log.Printf("[AD-ROTATION] failed to persist state for %s: %v", placement, updErr)
	}

	return &ads[nextIdx], nil
}

func (r *AdRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "placements", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: -1}}},
	}
	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return err
	}
	_, err = r.rotCol.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "placement", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
	return err
}

// Create inserts a new ad
func (r *AdRepository) Create(ad *models.Ad) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ad.ID = primitive.NewObjectID()
	ad.CreatedAt = time.Now()
	ad.UpdatedAt = time.Now()
	ad.Impressions = 0
	ad.Clicks = 0

	_, err := r.col.InsertOne(ctx, ad)
	return err
}

// FindByID returns an ad by its ID
func (r *AdRepository) FindByID(id primitive.ObjectID) (*models.Ad, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var ad models.Ad
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&ad)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &ad, nil
}

// ExpireEnded flips status from active → expired for all ads whose ends_at has passed.
// Call this before List() and GetStats() so counts stay accurate without a background job.
func (r *AdRepository) ExpireEnded() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	_, err := r.col.UpdateMany(ctx,
		bson.M{
			"status":  models.AdStatusActive,
			"ends_at": bson.M{"$lt": now, "$ne": nil},
		},
		bson.M{"$set": bson.M{"status": models.AdStatusExpired, "updated_at": now}},
	)
	return err
}

// List returns all ads, sorted by created_at desc
func (r *AdRepository) List() ([]models.Ad, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := r.col.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var ads []models.Ad
	if err := cursor.All(ctx, &ads); err != nil {
		return nil, err
	}
	return ads, nil
}

// FindByPlacement returns all active ads for a placement (used internally / admin).
func (r *AdRepository) FindByPlacement(placement string) ([]models.Ad, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := r.col.Find(ctx, activeAdsFilter(placement),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var ads []models.Ad
	if err := cursor.All(ctx, &ads); err != nil {
		return nil, err
	}
	return ads, nil
}

// Update replaces an ad's editable fields
func (r *AdRepository) Update(id primitive.ObjectID, update bson.M) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update["updated_at"] = time.Now()
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	return err
}

// Delete removes an ad by ID
func (r *AdRepository) Delete(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// LogDelivery stores a delivery record and atomically increments telegram_deliveries
func (r *AdRepository) LogDelivery(d *models.AdDelivery) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d.ID = primitive.NewObjectID()
	if d.SentAt.IsZero() {
		d.SentAt = time.Now()
	}
	if _, err := r.delivCol.InsertOne(ctx, d); err != nil {
		return err
	}
	// Atomically increment counter + set last_sent_at
	inc := bson.M{"telegram_deliveries": 1}
	upd := bson.M{
		"$inc": inc,
		"$set": bson.M{"telegram_last_sent_at": d.SentAt},
	}
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": d.AdID}, upd)
	return err
}

// GetDeliveryHistory returns recent delivery records for an ad
func (r *AdRepository) GetDeliveryHistory(adID primitive.ObjectID, limit int) ([]models.AdDelivery, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	l := int64(limit)
	cursor, err := r.delivCol.Find(ctx, bson.M{"ad_id": adID},
		options.Find().SetSort(bson.D{{Key: "sent_at", Value: -1}}).SetLimit(l))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []models.AdDelivery
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// IncrementImpression atomically increments the impression counter
func (r *AdRepository) IncrementImpression(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"impressions": 1}})
	return err
}

// IncrementClick atomically increments the click counter
func (r *AdRepository) IncrementClick(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"clicks": 1}})
	return err
}

// GetStats returns aggregate stats across all ads
func (r *AdRepository) GetStats() (*models.AdStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	totalAds, err := r.col.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	activeAds, err := r.col.CountDocuments(ctx, bson.M{"status": models.AdStatusActive})
	if err != nil {
		return nil, err
	}
	expiredAds, err := r.col.CountDocuments(ctx, bson.M{"status": models.AdStatusExpired})
	if err != nil {
		return nil, err
	}

	// Aggregate impressions, clicks, revenue, telegram deliveries
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "impressions", Value: bson.D{{Key: "$sum", Value: "$impressions"}}},
			{Key: "clicks", Value: bson.D{{Key: "$sum", Value: "$clicks"}}},
			{Key: "revenue", Value: bson.D{{Key: "$sum", Value: "$price"}}},
			{Key: "telegram_deliveries", Value: bson.D{{Key: "$sum", Value: "$telegram_deliveries"}}},
		}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var aggResult []struct {
		Impressions        int64   `bson:"impressions"`
		Clicks             int64   `bson:"clicks"`
		Revenue            float64 `bson:"revenue"`
		TelegramDeliveries int64   `bson:"telegram_deliveries"`
	}
	if err := cursor.All(ctx, &aggResult); err != nil {
		return nil, err
	}

	// Count failed deliveries from ad_deliveries collection
	telegramFailed, _ := r.delivCol.CountDocuments(ctx, bson.M{"status": "failed"})

	stats := &models.AdStats{
		TotalAds:       totalAds,
		ActiveAds:      activeAds,
		ExpiredAds:     expiredAds,
		TelegramFailed: telegramFailed,
	}
	if len(aggResult) > 0 {
		stats.Impressions = aggResult[0].Impressions
		stats.Clicks = aggResult[0].Clicks
		stats.Revenue = aggResult[0].Revenue
		stats.TelegramDeliveries = aggResult[0].TelegramDeliveries
	}
	return stats, nil
}

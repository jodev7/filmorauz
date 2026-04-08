package repositories

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/filmorauz/backend/models"
)

type AdRepository struct {
	col *mongo.Collection
}

func NewAdRepository(db *mongo.Database) *AdRepository {
	return &AdRepository{col: db.Collection("ads")}
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

// FindByPlacement returns active ads for a given placement (respects schedule)
func (r *AdRepository) FindByPlacement(placement string) ([]models.Ad, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	filter := bson.M{
		"status":     models.AdStatusActive,
		"placements": placement,
		"$or": []bson.M{
			{"starts_at": bson.M{"$exists": false}},
			{"starts_at": bson.M{"$lte": now}},
		},
	}
	// Also check ends_at
	filter2 := bson.M{
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
	_ = filter

	cursor, err := r.col.Find(ctx, filter2)
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

	// Aggregate impressions, clicks, revenue
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "impressions", Value: bson.D{{Key: "$sum", Value: "$impressions"}}},
			{Key: "clicks", Value: bson.D{{Key: "$sum", Value: "$clicks"}}},
			{Key: "revenue", Value: bson.D{{Key: "$sum", Value: "$price"}}},
		}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var aggResult []struct {
		Impressions int64   `bson:"impressions"`
		Clicks      int64   `bson:"clicks"`
		Revenue     float64 `bson:"revenue"`
	}
	if err := cursor.All(ctx, &aggResult); err != nil {
		return nil, err
	}

	stats := &models.AdStats{
		TotalAds:   totalAds,
		ActiveAds:  activeAds,
		ExpiredAds: expiredAds,
	}
	if len(aggResult) > 0 {
		stats.Impressions = aggResult[0].Impressions
		stats.Clicks = aggResult[0].Clicks
		stats.Revenue = aggResult[0].Revenue
	}
	return stats, nil
}

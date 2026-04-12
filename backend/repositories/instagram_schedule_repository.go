package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type InstagramScheduleRepository struct {
	col *mongo.Collection
}

func NewInstagramScheduleRepository(db *mongo.Database) *InstagramScheduleRepository {
	return &InstagramScheduleRepository{col: db.Collection("instagram_schedules")}
}

func (r *InstagramScheduleRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "clip_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "scheduled_for", Value: 1}}},
	})
	return err
}

func (r *InstagramScheduleRepository) Create(s *models.InstagramSchedule) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.ID = primitive.NewObjectID()
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now
	s.Status = models.InstagramScheduleStatusPending
	_, err := r.col.InsertOne(ctx, s)
	return err
}

func (r *InstagramScheduleRepository) FindByID(id primitive.ObjectID) (*models.InstagramSchedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var s models.InstagramSchedule
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &s, err
}

func (r *InstagramScheduleRepository) FindByClipID(clipID primitive.ObjectID) ([]models.InstagramSchedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cursor, err := r.col.Find(ctx, bson.M{"clip_id": clipID},
		options.Find().SetSort(bson.D{{Key: "scheduled_for", Value: -1}}).SetLimit(20))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []models.InstagramSchedule
	return results, cursor.All(ctx, &results)
}

func (r *InstagramScheduleRepository) ListAll(limit, offset int64) ([]models.InstagramSchedule, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	total, err := r.col.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, 0, err
	}
	cursor, err := r.col.Find(ctx, bson.M{},
		options.Find().
			SetSort(bson.D{{Key: "scheduled_for", Value: -1}}).
			SetSkip(offset).
			SetLimit(limit))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var results []models.InstagramSchedule
	return results, total, cursor.All(ctx, &results)
}

// ClaimDue atomically finds one pending schedule that is due (scheduled_for <= now)
// and sets it to "processing". Returns nil if nothing is ready.
// The atomic FindOneAndUpdate prevents double-execution across concurrent calls.
func (r *InstagramScheduleRepository) ClaimDue() (*models.InstagramSchedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	filter := bson.M{
		"status":        models.InstagramScheduleStatusPending,
		"scheduled_for": bson.M{"$lte": now},
	}
	update := bson.M{"$set": bson.M{
		"status":     models.InstagramScheduleStatusProcessing,
		"updated_at": now,
	}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var s models.InstagramSchedule
	err := r.col.FindOneAndUpdate(ctx, filter, update, opts).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *InstagramScheduleRepository) MarkSuccess(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"status":      models.InstagramScheduleStatusSuccess,
		"executed_at": now,
		"updated_at":  now,
	}})
	return err
}

func (r *InstagramScheduleRepository) MarkFailed(id primitive.ObjectID, errMsg string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"status":      models.InstagramScheduleStatusFailed,
		"error":       errMsg,
		"executed_at": now,
		"updated_at":  now,
	}})
	return err
}

// UpdateScheduledTime changes the scheduled_for time of a pending schedule.
// Returns false if the schedule is not found or is no longer pending.
func (r *InstagramScheduleRepository) UpdateScheduledTime(id primitive.ObjectID, newTime time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := r.col.UpdateOne(ctx,
		bson.M{"_id": id, "status": models.InstagramScheduleStatusPending},
		bson.M{"$set": bson.M{"scheduled_for": newTime.UTC(), "updated_at": time.Now().UTC()}},
	)
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// Cancel deletes a pending schedule. Returns false if not found or already executing/done.
func (r *InstagramScheduleRepository) Cancel(id primitive.ObjectID) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := r.col.DeleteOne(ctx, bson.M{"_id": id, "status": models.InstagramScheduleStatusPending})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

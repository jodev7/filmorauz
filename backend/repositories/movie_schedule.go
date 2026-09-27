package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrMovieAlreadyPublished is returned when scheduling a movie that is
// already approved (there is nothing left to publish).
var ErrMovieAlreadyPublished = errors.New("movie is already published")

// EnsureScheduleIndex backs the scheduler's "due now" lookup. Sparse, so it
// only holds the handful of scheduled movies.
func (r *MovieRepository) EnsureScheduleIndex() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "scheduled_publish_at", Value: 1}},
		Options: options.Index().SetSparse(true).SetName("scheduled_publish_at_sparse"),
	})
	return err
}

// SetSchedule schedules an unpublished movie for automatic publishing at
// `at`. The movie is (re)set to pending so it stays hidden until then.
func (r *MovieRepository) SetSchedule(idHex string, at time.Time, byUserID string) error {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return fmt.Errorf("invalid movie id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := r.col.UpdateOne(ctx,
		bson.M{"_id": id, "approval_status": bson.M{"$in": []string{"pending", "rejected"}}},
		bson.M{
			"$set": bson.M{
				"scheduled_publish_at": at.UTC(),
				"scheduled_by":         byUserID,
				"approval_status":      "pending",
				"is_published":         false,
				"updated_at":           time.Now(),
			},
			"$unset": bson.M{"schedule_error": ""},
		},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		n, err := r.col.CountDocuments(ctx, bson.M{"_id": id})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("movie not found")
		}
		return ErrMovieAlreadyPublished
	}
	return nil
}

// ClearSchedule cancels a pending schedule (no-op when none is set).
func (r *MovieRepository) ClearSchedule(idHex string) error {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return fmt.Errorf("invalid movie id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$unset": bson.M{"scheduled_publish_at": "", "scheduled_by": "", "schedule_error": ""},
		"$set":   bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("movie not found")
	}
	return nil
}

// ClaimDueScheduled atomically takes one movie whose publish time has come,
// removing its schedule so no other instance (or the next tick) publishes it
// again. Returns ("", "", nil) when nothing is due.
func (r *MovieRepository) ClaimDueScheduled(now time.Time) (idHex, scheduledBy string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var doc struct {
		ID          primitive.ObjectID `bson:"_id"`
		ScheduledBy string             `bson:"scheduled_by"`
	}
	err = r.col.FindOneAndUpdate(ctx,
		bson.M{"scheduled_publish_at": bson.M{"$lte": now.UTC()}},
		bson.M{"$unset": bson.M{"scheduled_publish_at": ""}},
		options.FindOneAndUpdate().
			SetSort(bson.D{{Key: "scheduled_publish_at", Value: 1}}).
			SetProjection(bson.M{"_id": 1, "scheduled_by": 1}),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	return doc.ID.Hex(), doc.ScheduledBy, nil
}

// SetScheduleError records why an automatic publish failed so the admin
// list can show it next to the movie.
func (r *MovieRepository) SetScheduleError(idHex, msg string) error {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"schedule_error": msg}})
	return err
}

// CountScheduled returns how many movies are waiting for a scheduled publish.
func (r *MovieRepository) CountScheduled(ctx context.Context) (int64, error) {
	return r.col.CountDocuments(ctx, scheduledFilter)
}

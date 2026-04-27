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

type ClipRepository struct {
	col *mongo.Collection
}

func NewClipRepository(db *mongo.Database) *ClipRepository {
	col := db.Collection("clips")
	return &ClipRepository{col: col}
}

func (r *ClipRepository) Collection() *mongo.Collection {
	return r.col
}

func (r *ClipRepository) Create(ctx context.Context, clip *models.Clip) error {
	clip.CreatedAt = time.Now()
	result, err := r.col.InsertOne(ctx, clip)
	if err != nil {
		return err
	}
	clip.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *ClipRepository) CreateMany(ctx context.Context, clips []models.Clip) error {
	if len(clips) == 0 {
		return nil
	}
	docs := make([]interface{}, len(clips))
	now := time.Now()
	for i := range clips {
		clips[i].CreatedAt = now
		docs[i] = clips[i]
	}
	_, err := r.col.InsertMany(ctx, docs)
	return err
}

func (r *ClipRepository) FindByMovieID(ctx context.Context, movieID primitive.ObjectID) ([]models.Clip, error) {
	cursor, err := r.col.Find(ctx, bson.M{"movie_id": movieID}, options.Find().SetSort(bson.M{"sequence": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, err
	}
	return clips, nil
}

func (r *ClipRepository) List(ctx context.Context, limit, offset int64) ([]models.Clip, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	filter := bson.M{}

	// Debug: log database and collection info
	log.Printf("[ClipRepo] List: db=%s, coll=%s", r.col.Database().Name(), r.col.Name())

	total, countErr := r.col.CountDocuments(ctx, filter)
	if countErr != nil {
		log.Printf("[ClipRepo] List: CountDocuments error=%v", countErr)
	} else {
		log.Printf("[ClipRepo] List: total clips in DB=%d, limit=%d, offset=%d", total, limit, offset)
	}

	// Try raw bson.M to see if there's a struct mismatch
	var rawDocs []bson.M
	rawCursor, rawErr := r.col.Find(ctx, filter)
	if rawErr != nil {
		log.Printf("[ClipRepo] List: Raw Find error=%v", rawErr)
	} else {
		if err := rawCursor.All(ctx, &rawDocs); err != nil {
			log.Printf("[ClipRepo] List: Raw decode error=%v", err)
		} else {
			log.Printf("[ClipRepo] List: Raw query returned %d docs", len(rawDocs))
			for i, doc := range rawDocs {
				log.Printf("[ClipRepo] List: doc[%d] _id=%v, movie_id=%v", i, doc["_id"], doc["movie_id"])
				if i >= 2 {
					break // Only log first 3
				}
			}
		}
		rawCursor.Close(ctx)
	}

	opts := options.Find().
		SetSort(bson.M{"created_at": -1}).
		SetLimit(limit).
		SetSkip(offset)

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		log.Printf("[ClipRepo] List: Find error=%v", err)
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		log.Printf("[ClipRepo] List: Decode error=%v", err)
		return nil, 0, err
	}

	log.Printf("[ClipRepo] List: returned %d clips (countErr=%v)", len(clips), countErr)

	if countErr != nil {
		total = int64(len(clips))
	}

	return clips, total, nil
}

func (r *ClipRepository) DeleteByMovieID(ctx context.Context, movieID primitive.ObjectID) error {
	_, err := r.col.DeleteMany(ctx, bson.M{"movie_id": movieID})
	return err
}

func (r *ClipRepository) FindBySeriesID(ctx context.Context, seriesID primitive.ObjectID) ([]models.Clip, error) {
	cursor, err := r.col.Find(ctx, bson.M{"series_id": seriesID}, options.Find().SetSort(bson.M{"sequence": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, err
	}
	return clips, nil
}

func (r *ClipRepository) DeleteBySeriesID(ctx context.Context, seriesID primitive.ObjectID) error {
	_, err := r.col.DeleteMany(ctx, bson.M{"series_id": seriesID})
	return err
}

// FindByEpisodeID returns clips linked to a single episode. Used during
// series cascade delete so legacy clip rows that only carry episode_id
// (no series_id) are still picked up.
func (r *ClipRepository) FindByEpisodeID(ctx context.Context, episodeID primitive.ObjectID) ([]models.Clip, error) {
	cursor, err := r.col.Find(ctx, bson.M{"episode_id": episodeID}, options.Find().SetSort(bson.M{"sequence": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, err
	}
	return clips, nil
}

// DeleteBySeriesAndEpisodeIDs removes every clip whose series_id matches
// or whose episode_id is in the given list. The OR query catches legacy
// clip rows that lack a series_id but reference one of the deleted
// episodes — without it those rows would survive a series delete and
// keep pointing at vanished content.
func (r *ClipRepository) DeleteBySeriesAndEpisodeIDs(ctx context.Context, seriesID primitive.ObjectID, episodeIDs []primitive.ObjectID) error {
	filter := bson.M{"series_id": seriesID}
	if len(episodeIDs) > 0 {
		filter = bson.M{
			"$or": []bson.M{
				{"series_id": seriesID},
				{"episode_id": bson.M{"$in": episodeIDs}},
			},
		}
	}
	_, err := r.col.DeleteMany(ctx, filter)
	return err
}

func (r *ClipRepository) CountByMovieID(ctx context.Context, movieID primitive.ObjectID) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{"movie_id": movieID})
}

// RecordInstagramUpload increments the upload counter and sets status/timestamp.
// status must be "success" or "failed".
func (r *ClipRepository) RecordInstagramUpload(ctx context.Context, clipID primitive.ObjectID, status string) error {
	now := time.Now()
	uploaded := status == "success"
	_, err := r.col.UpdateOne(ctx,
		bson.M{"_id": clipID},
		bson.M{
			"$inc": bson.M{"instagram_upload_count": 1},
			"$set": bson.M{
				"uploaded_to_instagram":        uploaded,
				"last_instagram_upload_at":     now,
				"last_instagram_upload_status": status,
			},
		},
	)
	return err
}

// FindByID returns a single clip by its ObjectID.
func (r *ClipRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.Clip, error) {
	var clip models.Clip
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&clip)
	if err != nil {
		return nil, err
	}
	return &clip, nil
}

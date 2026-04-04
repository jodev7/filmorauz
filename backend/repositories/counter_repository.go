package repositories

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Counter document stored in the "counters" collection
type Counter struct {
	ID  string `bson:"_id"`
	Seq int64  `bson:"seq"`
}

type CounterRepository struct {
	col *mongo.Collection
}

func NewCounterRepository(db *mongo.Database) *CounterRepository {
	return &CounterRepository{col: db.Collection("counters")}
}

// NextSequence atomically increments and returns the next value for the given key.
// Uses findOneAndUpdate with upsert=true for production-safe atomic operation.
func (r *CounterRepository) NextSequence(key string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": key}
	update := bson.M{"$inc": bson.M{"seq": int64(1)}}
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var result Counter
	err := r.col.FindOneAndUpdate(ctx, filter, update, opts).Decode(&result)
	if err != nil {
		return 0, err
	}
	return result.Seq, nil
}

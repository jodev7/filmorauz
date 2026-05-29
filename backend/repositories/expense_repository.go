package repositories

import (
	"context"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ExpenseRepository is the DB layer for manually recorded project costs.
type ExpenseRepository struct {
	col *mongo.Collection
}

func NewExpenseRepository(db *mongo.Database) *ExpenseRepository {
	return &ExpenseRepository{col: db.Collection("expenses")}
}

func (r *ExpenseRepository) Create(ctx context.Context, e *models.Expense) error {
	e.CreatedAt = time.Now()
	if e.IncurredAt.IsZero() {
		e.IncurredAt = e.CreatedAt
	}
	res, err := r.col.InsertOne(ctx, e)
	if err != nil {
		return err
	}
	e.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

// List returns all expenses, newest incurred first.
func (r *ExpenseRepository) List(ctx context.Context) ([]models.Expense, error) {
	cursor, err := r.col.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"incurred_at": -1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	expenses := []models.Expense{}
	if err := cursor.All(ctx, &expenses); err != nil {
		return nil, err
	}
	return expenses, nil
}

func (r *ExpenseRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// ExpenseCategoryTotal is spend rolled up per category.
type ExpenseCategoryTotal struct {
	Category  string  `json:"category" bson:"_id"`
	AmountUSD float64 `json:"amount_usd" bson:"amount_usd"`
	Count     int64   `json:"count" bson:"count"`
}

// TotalsByCategory sums manual expenses grouped by category, largest first.
func (r *ExpenseRepository) TotalsByCategory(ctx context.Context) ([]ExpenseCategoryTotal, float64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$category"},
			{Key: "amount_usd", Value: bson.D{{Key: "$sum", Value: "$amount_usd"}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "amount_usd", Value: -1}}}},
	}
	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	cats := []ExpenseCategoryTotal{}
	var total float64
	for cursor.Next(ctx) {
		var c ExpenseCategoryTotal
		if err := cursor.Decode(&c); err != nil {
			return nil, 0, err
		}
		if c.Category == "" {
			c.Category = "other"
		}
		total += c.AmountUSD
		cats = append(cats, c)
	}
	return cats, total, nil
}

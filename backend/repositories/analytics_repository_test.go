package repositories

import (
	"context"
	"testing"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestAnalyticsRepo(t *testing.T) {
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	
	db := client.Database("filmorauz_test")
	repo := NewAnalyticsRepository(db)
	
	// Ensure indexes
	err = repo.EnsureIndexes()
	if err != nil {
		t.Logf("Index err: %v", err)
	}
	
	top, zero, err := repo.SearchSummary(context.Background(), 30, 10)
	if err != nil {
		t.Fatalf("SearchSummary err: %v", err)
	}
	t.Logf("Top: %+v", top)
	t.Logf("Zero: %+v", zero)
}

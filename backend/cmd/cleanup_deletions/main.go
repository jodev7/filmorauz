package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoURI := "mongodb://localhost:27017"
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		mongoURI = uri
	}
	mongoDB := "filmorauz"
	if db := os.Getenv("MONGO_DB"); db != "" {
		mongoDB = db
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database(mongoDB)
	repo := repositories.NewDeleteJobRepository(db)

	log.Println("Resetting stale deletion jobs...")
	count, err := repo.FailStaleJobs(context.Background(), 5*time.Minute)
	if err != nil {
		log.Fatalf("Error resetting jobs: %v", err)
	}

	log.Printf("Successfully marked %d stale deletion jobs as failed.", count)
}

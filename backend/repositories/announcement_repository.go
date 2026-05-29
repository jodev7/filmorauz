package repositories

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AnnouncementRepository struct {
	col *mongo.Collection
}

func NewAnnouncementRepository(db *mongo.Database) *AnnouncementRepository {
	repo := &AnnouncementRepository{col: db.Collection("announcements")}
	repo.ensureIndexes()
	return repo
}

func (r *AnnouncementRepository) ensureIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	idx := []mongo.IndexModel{
		{Keys: bson.D{{Key: "is_active", Value: 1}, {Key: "starts_at", Value: 1}, {Key: "ends_at", Value: 1}}},
		{Keys: bson.D{{Key: "priority", Value: -1}, {Key: "created_at", Value: -1}}},
	}
	if _, err := r.col.Indexes().CreateMany(ctx, idx); err != nil {
		log.Printf("Warning: failed to create announcement indexes: %v", err)
	}
}

func (r *AnnouncementRepository) Create(ctx context.Context, a *models.Announcement) error {
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now
	res, err := r.col.InsertOne(ctx, a)
	if err != nil {
		return fmt.Errorf("insert announcement: %w", err)
	}
	a.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *AnnouncementRepository) Update(ctx context.Context, id primitive.ObjectID, input *models.AnnouncementInput) (*models.Announcement, error) {
	now := time.Now()
	update := bson.M{"$set": bson.M{
		"title":        input.Title,
		"body":         input.Body,
		"link_url":     input.LinkURL,
		"link_label":   input.LinkLabel,
		"starts_at":    input.StartsAt,
		"ends_at":      input.EndsAt,
		"dismissible":  input.Dismissible,
		"is_active":    input.IsActive,
		"priority":     input.Priority,
		"updated_at":   now,
	}}
	if _, err := r.col.UpdateByID(ctx, id, update); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *AnnouncementRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *AnnouncementRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.Announcement, error) {
	var a models.Announcement
	if err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&a); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// List returns every announcement sorted newest first. Used by the admin UI.
func (r *AnnouncementRepository) List(ctx context.Context) ([]models.Announcement, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(200)
	cur, err := r.col.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.Announcement
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []models.Announcement{}, nil
	}
	return out, nil
}

// ListActive returns announcements currently within their time window and
// flagged active, sorted by priority desc then newest. The public modal
// endpoint shows the first entry from this list.
func (r *AnnouncementRepository) ListActive(ctx context.Context, now time.Time) ([]models.Announcement, error) {
	filter := bson.M{
		"is_active": true,
		"starts_at": bson.M{"$lte": now},
		"ends_at":   bson.M{"$gte": now},
	}
	opts := options.Find().SetSort(bson.D{
		{Key: "priority", Value: -1},
		{Key: "created_at", Value: -1},
	}).SetLimit(20)
	cur, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.Announcement
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []models.Announcement{}, nil
	}
	return out, nil
}

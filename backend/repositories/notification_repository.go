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

type NotificationRepository struct {
	collection *mongo.Collection
}

func NewNotificationRepository(db *mongo.Database) *NotificationRepository {
	collection := db.Collection("notifications")

	// Create indexes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "is_read", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "type", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetBackground(true),
		},
		// For deduplication of expiring soon notifications
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "type", Value: 1}, {Key: "created_at", Value: 1}},
			Options: options.Index().SetBackground(true).SetExpireAfterSeconds(86400 * 2), // TTL: 2 days for expiry checks
		},
	}

	collection.Indexes().CreateMany(ctx, indexes)

	return &NotificationRepository{
		collection: collection,
	}
}

// Create creates a new notification
func (r *NotificationRepository) Create(ctx context.Context, notification *models.Notification) error {
	notification.ID = primitive.NewObjectID()
	notification.CreatedAt = time.Now()
	notification.IsRead = false

	_, err := r.collection.InsertOne(ctx, notification)
	return err
}

// FindByUserID retrieves notifications for a user with pagination
func (r *NotificationRepository) FindByUserID(ctx context.Context, userID primitive.ObjectID, limit int) ([]models.Notification, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var notifications []models.Notification
	if err := cursor.All(ctx, &notifications); err != nil {
		return nil, err
	}

	if notifications == nil {
		notifications = []models.Notification{}
	}

	return notifications, nil
}

// FindByUserIDPaginated retrieves notifications with pagination
func (r *NotificationRepository) FindByUserIDPaginated(ctx context.Context, userID primitive.ObjectID, page, perPage int) ([]models.Notification, int64, error) {
	filter := bson.M{"user_id": userID}

	// Count total
	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	// Pagination
	skip := (page - 1) * perPage
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64(skip)).
		SetLimit(int64(perPage))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var notifications []models.Notification
	if err := cursor.All(ctx, &notifications); err != nil {
		return nil, 0, err
	}

	if notifications == nil {
		notifications = []models.Notification{}
	}

	return notifications, total, nil
}

// CountUnread counts unread notifications for a user
func (r *NotificationRepository) CountUnread(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{
		"user_id": userID,
		"is_read": false,
	})
}

// MarkAsRead marks a notification as read
func (r *NotificationRepository) MarkAsRead(ctx context.Context, notificationID primitive.ObjectID, userID primitive.ObjectID) error {
	now := time.Now()
	_, err := r.collection.UpdateOne(ctx,
		bson.M{"_id": notificationID, "user_id": userID},
		bson.M{"$set": bson.M{"is_read": true, "read_at": now}},
	)
	return err
}

// MarkAllAsRead marks all notifications as read for a user
func (r *NotificationRepository) MarkAllAsRead(ctx context.Context, userID primitive.ObjectID) error {
	now := time.Now()
	_, err := r.collection.UpdateMany(ctx,
		bson.M{"user_id": userID, "is_read": false},
		bson.M{"$set": bson.M{"is_read": true, "read_at": now}},
	)
	return err
}

// MarkMultipleAsRead marks multiple notifications as read
func (r *NotificationRepository) MarkMultipleAsRead(ctx context.Context, ids []primitive.ObjectID, userID primitive.ObjectID) error {
	now := time.Now()
	_, err := r.collection.UpdateMany(ctx,
		bson.M{"_id": bson.M{"$in": ids}, "user_id": userID},
		bson.M{"$set": bson.M{"is_read": true, "read_at": now}},
	)
	return err
}

// CheckRecentNotification checks if a similar notification was sent recently (for deduplication)
func (r *NotificationRepository) CheckRecentNotification(ctx context.Context, userID primitive.ObjectID, notificationType models.NotificationType, withinHours int) (bool, error) {
	cutoff := time.Now().Add(-time.Duration(withinHours) * time.Hour)

	count, err := r.collection.CountDocuments(ctx, bson.M{
		"user_id":    userID,
		"type":       notificationType,
		"created_at": bson.M{"$gte": cutoff},
	})

	return count > 0, err
}

// GetUnreadByType gets unread notifications of a specific type
func (r *NotificationRepository) GetUnreadByType(ctx context.Context, userID primitive.ObjectID, notificationType models.NotificationType) ([]models.Notification, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{
		"user_id": userID,
		"type":    notificationType,
		"is_read": false,
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var notifications []models.Notification
	if err := cursor.All(ctx, &notifications); err != nil {
		return nil, err
	}

	return notifications, nil
}

// DeleteByID deletes a single notification owned by `userID`. Used for
// one-shot notifications (e.g. ROOM_INVITE) that should disappear from
// the dropdown the moment the user clicks them — without this, the row
// stays and a later re-click follows a now-expired invite link.
func (r *NotificationRepository) DeleteByID(ctx context.Context, notificationID primitive.ObjectID, userID primitive.ObjectID) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": notificationID, "user_id": userID})
	return err
}

// DeleteOld deletes notifications older than the specified days
func (r *NotificationRepository) DeleteOld(ctx context.Context, daysOld int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -daysOld)

	result, err := r.collection.DeleteMany(ctx, bson.M{
		"created_at": bson.M{"$lt": cutoff},
		"is_read":    true,
	})

	if err != nil {
		return 0, err
	}

	return result.DeletedCount, nil
}

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

type WatchRoomRepository struct {
	rooms    *mongo.Collection
	invites  *mongo.Collection
	messages *mongo.Collection
}

func NewWatchRoomRepository(db *mongo.Database) *WatchRoomRepository {
	r := &WatchRoomRepository{
		rooms:    db.Collection("watch_rooms"),
		invites:  db.Collection("watch_room_invites"),
		messages: db.Collection("watch_room_messages"),
	}
	r.ensureIndexes(context.Background())
	return r
}

func (r *WatchRoomRepository) ensureIndexes(ctx context.Context) {
	_, _ = r.rooms.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "owner_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "visibility", Value: 1}, {Key: "status", Value: 1}, {Key: "updated_at", Value: -1}}},
		// TTL — Mongo auto-deletes the row after ExpiresAt.
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	})
	_, _ = r.invites.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "code", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "room_id", Value: 1}}},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	})
	_, _ = r.messages.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "room_id", Value: 1}, {Key: "created_at", Value: 1}},
	})
}

func (r *WatchRoomRepository) RoomsCollection() *mongo.Collection { return r.rooms }
func (r *WatchRoomRepository) InvitesCollection() *mongo.Collection { return r.invites }
func (r *WatchRoomRepository) MessagesCollection() *mongo.Collection { return r.messages }

// ── Rooms ──────────────────────────────────────────────────────────────────

func (r *WatchRoomRepository) CreateRoom(ctx context.Context, room *models.WatchRoom) error {
	res, err := r.rooms.InsertOne(ctx, room)
	if err != nil {
		return err
	}
	room.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *WatchRoomRepository) GetRoomByID(ctx context.Context, id primitive.ObjectID) (*models.WatchRoom, error) {
	var room models.WatchRoom
	if err := r.rooms.FindOne(ctx, bson.M{"_id": id}).Decode(&room); err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *WatchRoomRepository) CountRoomsCreatedSince(ctx context.Context, ownerID primitive.ObjectID, since time.Time) (int64, error) {
	return r.rooms.CountDocuments(ctx, bson.M{
		"owner_id":   ownerID,
		"created_at": bson.M{"$gte": since},
	})
}

func (r *WatchRoomRepository) UpdatePlaybackState(ctx context.Context, id primitive.ObjectID, position float64, isPlaying bool) error {
	now := time.Now()
	_, err := r.rooms.UpdateByID(ctx, id, bson.M{"$set": bson.M{
		"position_seconds":  position,
		"is_playing":        isPlaying,
		"last_state_update": now,
		"updated_at":        now,
	}})
	return err
}

func (r *WatchRoomRepository) CloseRoom(ctx context.Context, id primitive.ObjectID) error {
	now := time.Now()
	_, err := r.rooms.UpdateByID(ctx, id, bson.M{"$set": bson.M{
		"status":     "closed",
		"closed_at":  now,
		"updated_at": now,
	}})
	return err
}

// ListPublicRooms returns active public rooms ordered by most-recent activity.
func (r *WatchRoomRepository) ListPublicRooms(ctx context.Context, limit int) ([]models.WatchRoom, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	cursor, err := r.rooms.Find(ctx,
		bson.M{"visibility": "public", "status": "active"},
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rooms []models.WatchRoom
	if err := cursor.All(ctx, &rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

// ── Invites ────────────────────────────────────────────────────────────────

func (r *WatchRoomRepository) CreateInvite(ctx context.Context, inv *models.WatchRoomInvite) error {
	res, err := r.invites.InsertOne(ctx, inv)
	if err != nil {
		return err
	}
	inv.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *WatchRoomRepository) GetInviteByCode(ctx context.Context, code string) (*models.WatchRoomInvite, error) {
	var inv models.WatchRoomInvite
	if err := r.invites.FindOne(ctx, bson.M{"code": code}).Decode(&inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *WatchRoomRepository) IncrementInviteUses(ctx context.Context, code string) error {
	_, err := r.invites.UpdateOne(ctx, bson.M{"code": code}, bson.M{"$inc": bson.M{"uses": 1}})
	return err
}

// ── Messages ───────────────────────────────────────────────────────────────

func (r *WatchRoomRepository) CreateMessage(ctx context.Context, msg *models.WatchRoomMessage) error {
	res, err := r.messages.InsertOne(ctx, msg)
	if err != nil {
		return err
	}
	msg.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *WatchRoomRepository) ListRoomMessages(ctx context.Context, roomID primitive.ObjectID, limit int) ([]models.WatchRoomMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	cursor, err := r.messages.Find(ctx,
		bson.M{"room_id": roomID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var msgs []models.WatchRoomMessage
	if err := cursor.All(ctx, &msgs); err != nil {
		return nil, err
	}
	// Return chronological order (oldest first).
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

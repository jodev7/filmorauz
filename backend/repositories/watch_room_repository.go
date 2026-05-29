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
	rooms   *mongo.Collection
	invites *mongo.Collection
	stats   *mongo.Collection
}

func NewWatchRoomRepository(db *mongo.Database) *WatchRoomRepository {
	r := &WatchRoomRepository{
		rooms:   db.Collection("watch_rooms"),
		invites: db.Collection("watch_room_invites"),
		stats:   db.Collection("watch_room_stats"),
	}
	r.ensureIndexes(context.Background())
	// Bootstrap the stats doc from whatever rows already exist in the
	// rooms collection on first run. Subsequent boots are no-ops because
	// the doc is left untouched once it exists.
	_ = r.backfillStatsIfMissing(context.Background())
	return r
}

// backfillStatsIfMissing seeds the singleton stats doc from the current
// rooms collection. Runs only when no doc exists yet — otherwise the
// live increments in CreateRoom / CloseRoom are the source of truth.
func (r *WatchRoomRepository) backfillStatsIfMissing(ctx context.Context) error {
	count, err := r.stats.CountDocuments(ctx, bson.M{"_id": statsDocID})
	if err != nil || count > 0 {
		return err
	}
	total, _ := r.rooms.CountDocuments(ctx, bson.M{})
	closed, _ := r.rooms.CountDocuments(ctx, bson.M{"status": "closed"})
	// Per-month buckets: aggregate created_at by YYYY-MM.
	byMonth := map[string]int64{}
	cur, err := r.rooms.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"created_at": 1}))
	if err == nil {
		defer cur.Close(ctx)
		var docs []struct {
			CreatedAt time.Time `bson:"created_at"`
		}
		if err := cur.All(ctx, &docs); err == nil {
			for _, d := range docs {
				byMonth[d.CreatedAt.Format("2006-01")]++
			}
		}
	}
	_, _ = r.stats.UpdateOne(ctx,
		bson.M{"_id": statsDocID},
		bson.M{"$set": bson.M{
			"total_created": total,
			"total_closed":  closed,
			"by_month":      byMonth,
		}},
		options.Update().SetUpsert(true),
	)
	return nil
}

func (r *WatchRoomRepository) ensureIndexes(ctx context.Context) {
	// Drop the legacy TTL index on watch_rooms if it still exists — rooms
	// are now kept forever (closed ones are the historical record that
	// drives the admin stats). Best-effort: missing index is fine.
	_, _ = r.rooms.Indexes().DropOne(ctx, "expires_at_1")
	_, _ = r.rooms.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "owner_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "visibility", Value: 1}, {Key: "status", Value: 1}, {Key: "updated_at", Value: -1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
		// Pinned-premiere listing: active featured rooms by priority.
		{Keys: bson.D{{Key: "is_featured", Value: 1}, {Key: "status", Value: 1}, {Key: "pin_priority", Value: -1}}},
	})
	_, _ = r.invites.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "code", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "room_id", Value: 1}}},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	})
}

func (r *WatchRoomRepository) RoomsCollection() *mongo.Collection { return r.rooms }
func (r *WatchRoomRepository) InvitesCollection() *mongo.Collection { return r.invites }

// ── Rooms ──────────────────────────────────────────────────────────────────

func (r *WatchRoomRepository) CreateRoom(ctx context.Context, room *models.WatchRoom) error {
	res, err := r.rooms.InsertOne(ctx, room)
	if err != nil {
		return err
	}
	room.ID = res.InsertedID.(primitive.ObjectID)
	// Bump the lifetime counters. Best-effort — a failed stats update
	// must not block the room from being created.
	_ = r.bumpStats(ctx, "created", room.CreatedAt)
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

// SetTheme persists a host-chosen background gradient on the room doc.
func (r *WatchRoomRepository) SetTheme(ctx context.Context, id primitive.ObjectID, from, to string) error {
	_, err := r.rooms.UpdateByID(ctx, id, bson.M{"$set": bson.M{
		"theme.from": from,
		"theme.to":   to,
		"updated_at": time.Now(),
	}})
	return err
}

// SetCurrentEpisode mutates a series room's currently-playing episode and
// resets the playback head to zero. Called both from the host's manual
// "switch episode" action and from the auto-advance flow when an episode
// ends.
func (r *WatchRoomRepository) SetCurrentEpisode(ctx context.Context, id, episodeID primitive.ObjectID, episodeTitle string, seasonID primitive.ObjectID) error {
	now := time.Now()
	_, err := r.rooms.UpdateByID(ctx, id, bson.M{"$set": bson.M{
		"current_episode_id":    episodeID,
		"current_episode_title": episodeTitle,
		"season_id":             seasonID,
		"position_seconds":      0,
		"is_playing":            false,
		"last_state_update":     now,
		"updated_at":            now,
	}})
	return err
}

func (r *WatchRoomRepository) CloseRoom(ctx context.Context, id primitive.ObjectID) error {
	now := time.Now()
	res, err := r.rooms.UpdateByID(ctx, id, bson.M{"$set": bson.M{
		"status":     "closed",
		"closed_at":  now,
		"updated_at": now,
	}})
	if err != nil {
		return err
	}
	// Only count the close transition once — UpdateByID is idempotent so
	// without ModifiedCount==1 we'd over-count if CloseRoom is called
	// twice on the same room (e.g. host_disconnect + empty).
	if res != nil && res.ModifiedCount == 1 {
		_ = r.bumpStats(ctx, "closed", now)
	}
	return nil
}

// FindActiveByOwner returns the most-recent active room owned by `ownerID`.
// Used by the "you have an open room" pill in the navbar so a host who
// clicked Back/Home can hop back into their still-open session.
func (r *WatchRoomRepository) FindActiveByOwner(ctx context.Context, ownerID primitive.ObjectID) (*models.WatchRoom, error) {
	var room models.WatchRoom
	err := r.rooms.FindOne(ctx,
		bson.M{"owner_id": ownerID, "status": "active"},
		options.FindOne().SetSort(bson.D{{Key: "updated_at", Value: -1}}),
	).Decode(&room)
	if err != nil {
		return nil, err
	}
	return &room, nil
}

// RoomStats is the aggregate counts shown on the admin dashboard.
type RoomStats struct {
	Active    int64 `json:"active"`
	Closed    int64 `json:"closed"`
	Total     int64 `json:"total"`
	ThisMonth int64 `json:"this_month"`
}

// TopRoomContent is one row of "most-roomed" content for the admin
// dashboard leaderboard.
type TopRoomContent struct {
	ContentID    string `json:"content_id"    bson:"content_id"`
	ContentType  string `json:"content_type"  bson:"content_type"`
	ContentTitle string `json:"content_title" bson:"content_title"`
	ContentPoster string `json:"content_poster" bson:"content_poster"`
	RoomCount    int64  `json:"room_count"    bson:"room_count"`
}

// TopRoomHost is one row of "most-hosting" users.
type TopRoomHost struct {
	OwnerID    string `json:"owner_id"    bson:"owner_id"`
	OwnerName  string `json:"owner_name"  bson:"owner_name"`
	OwnerAvatar string `json:"owner_avatar" bson:"owner_avatar"`
	RoomCount  int64  `json:"room_count"  bson:"room_count"`
}

// GetTopContent returns the most-roomed movies/series across all time
// for the admin dashboard leaderboard.
func (r *WatchRoomRepository) GetTopContent(ctx context.Context, limit int) ([]TopRoomContent, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	pipeline := []bson.M{
		{"$group": bson.M{
			"_id": bson.M{
				"content_id":   "$content_id",
				"content_type": "$content_type",
			},
			"room_count":    bson.M{"$sum": 1},
			"content_title": bson.M{"$first": "$content_title"},
			"content_poster": bson.M{"$first": "$content_poster"},
		}},
		{"$sort": bson.M{"room_count": -1}},
		{"$limit": int64(limit)},
		{"$project": bson.M{
			"_id":          0,
			"content_id":   bson.M{"$toString": "$_id.content_id"},
			"content_type": "$_id.content_type",
			"content_title": 1,
			"content_poster": 1,
			"room_count":   1,
		}},
	}
	cur, err := r.rooms.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []TopRoomContent
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTopHosts returns the users who've hosted the most rooms.
func (r *WatchRoomRepository) GetTopHosts(ctx context.Context, limit int) ([]TopRoomHost, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	pipeline := []bson.M{
		{"$group": bson.M{
			"_id":          "$owner_id",
			"room_count":   bson.M{"$sum": 1},
			"owner_name":   bson.M{"$last": "$owner_name"},
			"owner_avatar": bson.M{"$last": "$owner_avatar"},
		}},
		{"$sort": bson.M{"room_count": -1}},
		{"$limit": int64(limit)},
		{"$project": bson.M{
			"_id":          0,
			"owner_id":     bson.M{"$toString": "$_id"},
			"owner_name":   1,
			"owner_avatar": 1,
			"room_count":   1,
		}},
	}
	cur, err := r.rooms.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []TopRoomHost
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// statsDoc is the shape of the singleton document in watch_room_stats.
// total_created / total_closed are lifetime counters; by_month is a map of
// "YYYY-MM" → created-count so we can compute "this month" cheaply.
type statsDoc struct {
	TotalCreated int64            `bson:"total_created"`
	TotalClosed  int64            `bson:"total_closed"`
	ByMonth      map[string]int64 `bson:"by_month"`
}

const statsDocID = "global"

// bumpStats atomically increments the appropriate counter on the singleton
// stats doc. `kind` is "created" or "closed"; for "created" the per-month
// bucket is also bumped using the room's created_at timestamp so the doc
// stays consistent even when an old room is inserted out-of-order.
func (r *WatchRoomRepository) bumpStats(ctx context.Context, kind string, when time.Time) error {
	inc := bson.M{}
	switch kind {
	case "created":
		inc["total_created"] = 1
		monthKey := when.Format("2006-01")
		inc["by_month."+monthKey] = 1
	case "closed":
		inc["total_closed"] = 1
	default:
		return nil
	}
	_, err := r.stats.UpdateOne(ctx,
		bson.M{"_id": statsDocID},
		bson.M{"$inc": inc},
		options.Update().SetUpsert(true),
	)
	return err
}

// GetRoomStats returns the dashboard counters. Lifetime totals come from
// the dedicated watch_room_stats singleton (so the rooms collection can
// be pruned without losing history); the "active" count is still a live
// query on the rooms collection because that's the only source of truth
// for currently-open sessions.
func (r *WatchRoomRepository) GetRoomStats(ctx context.Context) (RoomStats, error) {
	var s RoomStats
	active, err := r.rooms.CountDocuments(ctx, bson.M{"status": "active"})
	if err != nil {
		return s, err
	}
	var doc statsDoc
	err = r.stats.FindOne(ctx, bson.M{"_id": statsDocID}).Decode(&doc)
	if err != nil && err != mongo.ErrNoDocuments {
		return s, err
	}
	monthKey := time.Now().Format("2006-01")
	s.Active = active
	s.Closed = doc.TotalClosed
	s.Total = doc.TotalCreated
	if doc.ByMonth != nil {
		s.ThisMonth = doc.ByMonth[monthKey]
	}
	return s, nil
}

// ListPublicRooms returns active public, non-featured rooms ordered by
// most-recent activity. Featured (premiere) rooms are excluded here so they
// don't double up — they have their own pinned section via ListFeaturedRooms.
func (r *WatchRoomRepository) ListPublicRooms(ctx context.Context, limit int) ([]models.WatchRoom, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	cursor, err := r.rooms.Find(ctx,
		bson.M{"visibility": "public", "status": "active", "is_featured": bson.M{"$ne": true}},
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

// ListFeaturedRooms returns active featured (premiere) rooms, pinned-first:
// highest pin_priority, then most recently scheduled/created.
func (r *WatchRoomRepository) ListFeaturedRooms(ctx context.Context, limit int) ([]models.WatchRoom, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	cursor, err := r.rooms.Find(ctx,
		bson.M{"status": "active", "is_featured": true},
		options.Find().
			SetSort(bson.D{{Key: "pin_priority", Value: -1}, {Key: "scheduled_start_at", Value: 1}, {Key: "created_at", Value: -1}}).
			SetLimit(int64(limit)),
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

// Chat messages are intentionally NOT persisted — they live only in the
// hub's in-memory ring buffer (see services/watch_room_hub.go) and vanish
// when the room closes.

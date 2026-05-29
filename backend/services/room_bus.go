package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RoomBus is the cross-instance backbone for watch-rooms. With multiple
// backend instances behind a load balancer, each instance holds only a
// subset of a room's WebSocket connections; the bus lets a message produced
// on one instance reach the clients connected to every other instance, and
// keeps the authoritative playback state / presence count / chat history /
// roster in Redis so any instance can serve a fresh joiner.
//
// When REDIS_URL is unset the hub runs without a bus (single-instance,
// in-memory) and none of this code path is exercised.
type RoomBus struct {
	rdb        *redis.Client
	instanceID string
	// keyTTL bounds every per-room key so a crashed instance or an abandoned
	// room can't leak Redis memory forever. Refreshed on activity.
	keyTTL time.Duration
}

// busEnvelope is what travels over the pub/sub channel. Raw is an
// already-marshalled hub message; ExceptUser lets the receiver skip the
// original sender's own connections (typing / member_joined semantics).
type busEnvelope struct {
	ExceptUser string          `json:"eu,omitempty"`
	Raw        json.RawMessage `json:"m"`
}

// NewRoomBus dials Redis and returns a bus, or an error if the URL is bad or
// the server is unreachable. Callers treat a nil bus as "disabled".
func NewRoomBus(redisURL string) (*RoomBus, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	idb := make([]byte, 8)
	_, _ = rand.Read(idb)
	return &RoomBus{
		rdb:        rdb,
		instanceID: hex.EncodeToString(idb),
		keyTTL:     24 * time.Hour,
	}, nil
}

// ── keys ─────────────────────────────────────────────────────────────────

func bcastChannel(roomID string) string { return "wr:bcast:" + roomID }
func stateKey(roomID string) string     { return "wr:state:" + roomID }
func presenceKey(roomID string) string  { return "wr:presence:" + roomID }
func chatKey(roomID string) string      { return "wr:chat:" + roomID }
func rosterKey(roomID string) string    { return "wr:roster:" + roomID }

// ── pub/sub ──────────────────────────────────────────────────────────────

// Publish sends a raw hub message to every subscriber of the room channel
// (including this instance — the caller does NOT also fan out locally).
func (b *RoomBus) Publish(roomID string, raw []byte, exceptUser string) {
	env, err := json.Marshal(busEnvelope{ExceptUser: exceptUser, Raw: raw})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := b.rdb.Publish(ctx, bcastChannel(roomID), env).Err(); err != nil {
		log.Printf("[bus] publish room=%s failed: %v", roomID, err)
	}
}

// Subscribe returns a cancel func and a channel of decoded envelopes for a
// room. The caller fans each envelope out to its local connections.
func (b *RoomBus) Subscribe(roomID string) (func(), <-chan busEnvelope) {
	ctx, cancel := context.WithCancel(context.Background())
	pubsub := b.rdb.Subscribe(ctx, bcastChannel(roomID))
	out := make(chan busEnvelope, 256)
	go func() {
		defer close(out)
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				_ = pubsub.Close()
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var env busEnvelope
				if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
					continue
				}
				select {
				case out <- env:
				default:
					// Local consumer is slow — drop; clients recover on the
					// next heartbeat.
				}
			}
		}
	}()
	return cancel, out
}

// ── playback state ─────────────────────────────────────────────────────────

// SetPlayback writes the authoritative playback head for a room.
func (b *RoomBus) SetPlayback(roomID string, position float64, isPlaying bool, asOfMs int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	key := stateKey(roomID)
	playing := "0"
	if isPlaying {
		playing = "1"
	}
	pipe := b.rdb.Pipeline()
	pipe.HSet(ctx, key, map[string]any{
		"position":   strconv.FormatFloat(position, 'f', -1, 64),
		"is_playing": playing,
		"as_of_ms":   strconv.FormatInt(asOfMs, 10),
	})
	pipe.Expire(ctx, key, b.keyTTL)
	_, _ = pipe.Exec(ctx)
}

// GetPlayback reads the shared playback head. ok=false when no state exists.
func (b *RoomBus) GetPlayback(roomID string) (position float64, isPlaying bool, asOfMs int64, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m, err := b.rdb.HGetAll(ctx, stateKey(roomID)).Result()
	if err != nil || len(m) == 0 {
		return 0, false, 0, false
	}
	position, _ = strconv.ParseFloat(m["position"], 64)
	isPlaying = m["is_playing"] == "1"
	asOfMs, _ = strconv.ParseInt(m["as_of_ms"], 10, 64)
	return position, isPlaying, asOfMs, true
}

// ── presence ─────────────────────────────────────────────────────────────

func (b *RoomBus) IncrPresence(roomID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pipe := b.rdb.Pipeline()
	pipe.Incr(ctx, presenceKey(roomID))
	pipe.Expire(ctx, presenceKey(roomID), b.keyTTL)
	_, _ = pipe.Exec(ctx)
}

func (b *RoomBus) DecrPresence(roomID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Floor at zero — a decrement past zero (e.g. after a reconciliation)
	// would render a negative count.
	if n, err := b.rdb.Decr(ctx, presenceKey(roomID)).Result(); err == nil && n < 0 {
		b.rdb.Set(ctx, presenceKey(roomID), 0, b.keyTTL)
	}
}

func (b *RoomBus) GetPresence(roomID string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	n, err := b.rdb.Get(ctx, presenceKey(roomID)).Int()
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// ── chat ring buffer ───────────────────────────────────────────────────────

// PushChat appends one message (already JSON) to the room's capped list.
func (b *RoomBus) PushChat(roomID string, raw []byte, cap int) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	key := chatKey(roomID)
	pipe := b.rdb.Pipeline()
	pipe.RPush(ctx, key, raw)
	pipe.LTrim(ctx, key, int64(-cap), -1)
	pipe.Expire(ctx, key, b.keyTTL)
	_, _ = pipe.Exec(ctx)
}

// RecentChat returns the buffered messages oldest-first (raw JSON entries).
func (b *RoomBus) RecentChat(roomID string) [][]byte {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	vals, err := b.rdb.LRange(ctx, chatKey(roomID), 0, -1).Result()
	if err != nil {
		return nil
	}
	out := make([][]byte, 0, len(vals))
	for _, v := range vals {
		out = append(out, []byte(v))
	}
	return out
}

// ── roster ─────────────────────────────────────────────────────────────────

func (b *RoomBus) AddRosterMember(roomID, userID string, member []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pipe := b.rdb.Pipeline()
	pipe.HSet(ctx, rosterKey(roomID), userID, member)
	pipe.Expire(ctx, rosterKey(roomID), b.keyTTL)
	_, _ = pipe.Exec(ctx)
}

func (b *RoomBus) RemoveRosterMember(roomID, userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	b.rdb.HDel(ctx, rosterKey(roomID), userID)
}

// Roster returns every member entry (raw JSON). Caller sorts + paginates.
func (b *RoomBus) Roster(roomID string) [][]byte {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m, err := b.rdb.HGetAll(ctx, rosterKey(roomID)).Result()
	if err != nil {
		return nil
	}
	out := make([][]byte, 0, len(m))
	for _, v := range m {
		out = append(out, []byte(v))
	}
	return out
}

// Cleanup removes all keys for a room when it closes for good.
func (b *RoomBus) Cleanup(roomID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	b.rdb.Del(ctx, stateKey(roomID), presenceKey(roomID), chatKey(roomID), rosterKey(roomID))
}

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// isAllowedGifURL guards the gif chat kind: only https URLs hosted on GIPHY's
// own CDN are accepted, so a client can't smuggle an arbitrary image/tracker
// URL (or a non-image payload) into every member's chat. The picker only ever
// hands us giphy.com media URLs; anything else is dropped.
func isAllowedGifURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "giphy.com" || strings.HasSuffix(host, ".giphy.com")
}

var _ = primitive.NilObjectID // keep primitive imported even if every direct use is via models

// HostDisconnectGrace is how long we wait for the host to reconnect after
// their websocket drops before tearing down the room. Bumped to 5 minutes
// per UX request — a host who tab-closes by mistake or loses connectivity
// briefly should be able to walk back in and resume.
const HostDisconnectGrace = 5 * time.Minute

// chatBufferSize is how many of the most-recent chat/emoji entries each room
// keeps in memory. Chat is NOT persisted to Mongo — this in-memory ring is
// the only history. A user who leaves and rejoins a still-open room replays
// these on entry; once the room closes the buffer is GC'd with the HubRoom
// and the conversation is gone for good (by design).
const chatBufferSize = 100

// HubClient is a single websocket connection inside a room. One per browser
// tab; the same user may have multiple clients (multi-device).
type HubClient struct {
	UserID     primitive.ObjectID
	UserName   string
	UserAvatar string
	IsHost     bool
	Conn       *websocket.Conn
	send       chan []byte
	// JoinedAt orders clients for host-transfer: when the host drops and the
	// grace window expires, the earliest-joined remaining guest is promoted.
	JoinedAt time.Time
}

// HubRoom is the live, in-memory state for one watch-room. Persisted state
// (rooms collection) is the source of truth on cold-start; the hub takes
// over while at least one connection is open.
type HubRoom struct {
	ID         primitive.ObjectID
	OwnerID    primitive.ObjectID
	MaxMembers int

	Position        float64
	IsPlaying       bool
	LastStateUpdate time.Time
	// hostDisconnectDeadline is set when the host drops; clients use it to
	// render the "Host qaytmoqda… (mm:ss qoldi)" countdown. Zero when the
	// host is currently connected.
	HostDisconnectDeadline time.Time

	// presenceMode is on for large/premiere rooms. In this mode the hub
	// stops broadcasting per-member join/left events and the full member
	// snapshot (which would be O(N) per churn at thousands of viewers) —
	// clients track the live count via state_sync.member_count and fetch the
	// roster over REST instead.
	presenceMode bool
	// keepAlive is on for premiere rooms: the room is NOT torn down when it
	// empties or the host (admin) disconnects. The timeline keeps advancing
	// (guests extrapolate from as_of_ms) until the admin explicitly closes
	// it or it expires.
	keepAlive bool

	mu      sync.Mutex
	clients map[*HubClient]struct{}
	// recentMessages is the in-memory chat ring buffer (last chatBufferSize
	// entries). Guarded by mu. Never persisted.
	recentMessages []models.WatchRoomMessage
	// lastChat throttles chat in presenceMode rooms: one message per user per
	// chatSlowModeInterval. Guarded by mu.
	lastChat map[primitive.ObjectID]time.Time
	// hostGraceTimer fires when the host has been gone for HostDisconnectGrace
	// without reconnecting. Cancelled if the host re-joins in time.
	hostGraceTimer *time.Timer
	closed         bool
	// busCancel tears down this room's Redis subscription (cluster mode only).
	busCancel func()
}

// Scaling thresholds.
const (
	// largeRoomThreshold is the member-capacity above which a room switches
	// to presenceMode (aggregate count instead of per-member events).
	largeRoomThreshold = 50
	// chatSlowModeInterval is the minimum gap between two chat messages from
	// the same user in a presenceMode room.
	chatSlowModeInterval = 2 * time.Second
	// writeFlushInterval is how often each client's write pump coalesces
	// queued messages into a single batched frame.
	writeFlushInterval = 200 * time.Millisecond
	// writeBatchMax bounds a single batch so a burst can't build an unbounded
	// frame or stall latency — flush early once this many messages queue up.
	writeBatchMax = 64
)

// WatchRoomHub multiplexes every active room. Methods are safe to call from
// any goroutine — the rooms map is protected by mu, and each HubRoom has
// its own mutex for member/state mutation.
type WatchRoomHub struct {
	repo *repositories.WatchRoomRepository
	// bus is the cross-instance backbone. Nil → single-instance in-memory.
	bus *RoomBus

	mu    sync.RWMutex
	rooms map[primitive.ObjectID]*HubRoom
}

// NewWatchRoomHub builds the hub. Pass a non-nil bus to run in cluster mode
// (Redis pub/sub + shared state); nil keeps the legacy single-instance path.
func NewWatchRoomHub(repo *repositories.WatchRoomRepository, bus *RoomBus) *WatchRoomHub {
	return &WatchRoomHub{
		repo:  repo,
		bus:   bus,
		rooms: make(map[primitive.ObjectID]*HubRoom),
	}
}

// GetOrLoadRoom returns the live HubRoom, loading it from Mongo on first use.
func (h *WatchRoomHub) GetOrLoadRoom(ctx context.Context, roomID primitive.ObjectID) (*HubRoom, error) {
	h.mu.RLock()
	if rm, ok := h.rooms[roomID]; ok {
		h.mu.RUnlock()
		return rm, nil
	}
	h.mu.RUnlock()

	persisted, err := h.repo.GetRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if rm, ok := h.rooms[roomID]; ok {
		return rm, nil
	}
	rm := &HubRoom{
		ID:              persisted.ID,
		OwnerID:         persisted.OwnerID,
		MaxMembers:      persisted.MaxMembers,
		Position:        persisted.PositionSeconds,
		IsPlaying:       persisted.IsPlaying,
		LastStateUpdate: persisted.LastStateUpdate,
		presenceMode:    persisted.Kind == "premiere" || persisted.MaxMembers > largeRoomThreshold,
		keepAlive:       persisted.Kind == "premiere",
		clients:         make(map[*HubClient]struct{}),
		lastChat:        make(map[primitive.ObjectID]time.Time),
	}
	h.rooms[roomID] = rm
	// Cluster mode: subscribe to this room's Redis channel and fan every
	// inbound envelope out to our local connections.
	if h.bus != nil {
		cancel, ch := h.bus.Subscribe(roomID.Hex())
		rm.busCancel = cancel
		go func() {
			for env := range ch {
				// Intercept control messages (e.g. __kick) before they reach
				// clients; everything else is fanned out verbatim.
				var probe struct {
					Type    string `json:"type"`
					Payload struct {
						UserID string `json:"user_id"`
					} `json:"payload"`
				}
				if json.Unmarshal(env.Raw, &probe) == nil {
					switch probe.Type {
					case "__kick":
						if uid, err := primitive.ObjectIDFromHex(probe.Payload.UserID); err == nil {
							h.localKick(rm, uid)
						}
						continue
					case "__close":
						h.closeLocalOnly(rm, "closed")
						return
					}
				}
				h.localBroadcastRaw(rm, env.Raw, env.ExceptUser)
			}
		}()
	}
	return rm, nil
}

// AddClient registers a websocket into a room. Returns error when the room
// is at capacity. Triggers a state_sync to the new client and a
// member_joined broadcast to the rest.
func (h *WatchRoomHub) AddClient(rm *HubRoom, c *HubClient) error {
	rm.mu.Lock()
	if rm.closed {
		rm.mu.Unlock()
		return ErrRoomClosed
	}
	if rm.MaxMembers > 0 && len(rm.clients)+1 > rm.MaxMembers {
		rm.mu.Unlock()
		return ErrRoomFull
	}
	// Bumped from 32 → 256. A busy chat plus the 2s state-sync heartbeat
	// could fill a 32-slot buffer fast and start dropping messages —
	// observed as "chat freezes after a flurry, reappears after refresh"
	// and "sync lags behind the host." A larger buffer absorbs the burst.
	c.send = make(chan []byte, 256)
	rm.clients[c] = struct{}{}
	// Host reconnected — cancel any pending teardown and clear the
	// "host disconnected" deadline so guests stop seeing the countdown.
	hostReconnected := false
	if c.IsHost && rm.hostGraceTimer != nil {
		rm.hostGraceTimer.Stop()
		rm.hostGraceTimer = nil
		rm.HostDisconnectDeadline = time.Time{}
		hostReconnected = true
	}
	rm.mu.Unlock()

	// Cluster mode: register this connection in the shared presence counter
	// and roster so every instance's count/roster reflects it.
	if h.bus != nil {
		h.bus.IncrPresence(rm.ID.Hex())
		if member, mErr := json.Marshal(map[string]any{
			"user_id":     c.UserID.Hex(),
			"user_name":   c.UserName,
			"user_avatar": c.UserAvatar,
			"is_host":     c.IsHost,
		}); mErr == nil {
			h.bus.AddRosterMember(rm.ID.Hex(), c.UserID.Hex(), member)
		}
	}

	if hostReconnected {
		h.broadcast(rm, hubMessage{Type: "host_reconnected", Payload: map[string]any{}}, nil)
	}

	go h.writePump(c)
	h.sendState(rm, c)

	// Send a snapshot of every current member (including the just-joined
	// client themselves) so the client UI has the full roster on entry,
	// without waiting for someone else to (re)join.
	//
	// presenceMode rooms skip this: a 5000-entry snapshot on every join is
	// pure waste. Those clients render the count from state_sync and pull
	// the roster page-by-page over REST.
	if !rm.presenceMode {
		var snapshot []map[string]any
		if h.bus != nil {
			// Cluster: roster is shared in Redis, so the snapshot is complete
			// even when members are spread across instances.
			for _, raw := range h.bus.Roster(rm.ID.Hex()) {
				var m map[string]any
				if json.Unmarshal(raw, &m) == nil {
					snapshot = append(snapshot, m)
				}
			}
		} else {
			rm.mu.Lock()
			snapshot = make([]map[string]any, 0, len(rm.clients))
			for cc := range rm.clients {
				snapshot = append(snapshot, map[string]any{
					"user_id":     cc.UserID.Hex(),
					"user_name":   cc.UserName,
					"user_avatar": cc.UserAvatar,
					"is_host":     cc.IsHost,
				})
			}
			rm.mu.Unlock()
		}
		h.sendTo(c, hubMessage{Type: "member_snapshot", Payload: map[string]any{"members": snapshot}})
	}

	// Replay the chat history so a (re)joining client sees the recent
	// conversation. Cluster mode reads the shared Redis ring; otherwise the
	// in-memory buffer. Sent only to the newcomer.
	var history []map[string]any
	if h.bus != nil {
		for _, raw := range h.bus.RecentChat(rm.ID.Hex()) {
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				history = append(history, m)
			}
		}
	} else {
		rm.mu.Lock()
		history = make([]map[string]any, 0, len(rm.recentMessages))
		for _, m := range rm.recentMessages {
			history = append(history, map[string]any{
				"user_id":     m.UserID.Hex(),
				"user_name":   m.UserName,
				"user_avatar": m.UserAvatar,
				"kind":        m.Kind,
				"text":        m.Text,
				"emoji":       m.Emoji,
				"gif_url":     m.GifURL,
				"created_at":  m.CreatedAt,
			})
		}
		rm.mu.Unlock()
	}
	if len(history) > 0 {
		h.sendTo(c, hubMessage{Type: "chat_history", Payload: map[string]any{"messages": history}})
	}

	// Then announce the new client to everyone else — small rooms only.
	// presenceMode rooms rely on the periodic state_sync member_count.
	if !rm.presenceMode {
		h.broadcast(rm, hubMessage{
			Type: "member_joined",
			Payload: map[string]any{
				"user_id":     c.UserID.Hex(),
				"user_name":   c.UserName,
				"user_avatar": c.UserAvatar,
				"is_host":     c.IsHost,
			},
		}, c)
	}
	return nil
}

// RemoveClient drops a client. If the leaver is the host, starts the grace
// timer that closes the room if no host reconnects in time.
func (h *WatchRoomHub) RemoveClient(rm *HubRoom, c *HubClient) {
	rm.mu.Lock()
	if _, ok := rm.clients[c]; !ok {
		rm.mu.Unlock()
		return
	}
	delete(rm.clients, c)
	close(c.send)
	delete(rm.lastChat, c.UserID)
	isHostGone := c.IsHost && !rm.hasHostLocked()
	// Does this user still have another local connection? If so, keep them
	// in the shared roster (multi-tab).
	sameUserStillHere := false
	for cc := range rm.clients {
		if cc.UserID == c.UserID {
			sameUserStillHere = true
			break
		}
	}
	rm.mu.Unlock()

	// Cluster mode: drop one presence count, and the roster entry once the
	// user has no remaining local connection.
	if h.bus != nil {
		h.bus.DecrPresence(rm.ID.Hex())
		if !sameUserStillHere {
			h.bus.RemoveRosterMember(rm.ID.Hex(), c.UserID.Hex())
		}
	}

	// Per-member leave events only in small rooms — and only once the user
	// has no other live connection. Otherwise closing one of two devices
	// (same account) would broadcast member_left and drop the user from
	// everyone's roster even though their other device is still in the room.
	if !rm.presenceMode && !sameUserStillHere {
		h.broadcast(rm, hubMessage{
			Type:    "member_left",
			Payload: map[string]any{"user_id": c.UserID.Hex()},
		}, nil)
	}

	// Premiere rooms are never torn down by churn — they keep running
	// (timeline advances via extrapolation) until the admin closes them
	// or they expire. No host-grace, no empty-teardown.
	if rm.keepAlive {
		return
	}

	if isHostGone {
		rm.mu.Lock()
		if rm.hostGraceTimer == nil {
			deadline := time.Now().Add(HostDisconnectGrace)
			rm.HostDisconnectDeadline = deadline
			rm.hostGraceTimer = time.AfterFunc(HostDisconnectGrace, func() {
				h.promoteHostOrClose(rm)
			})
			rm.mu.Unlock()
			// Tell every remaining client when the room will be torn down
			// so they can render a countdown instead of a silent buffer.
			h.broadcast(rm, hubMessage{
				Type: "host_disconnected",
				Payload: map[string]any{
					"deadline_ms":   deadline.UnixMilli(),
					"grace_seconds": int(HostDisconnectGrace.Seconds()),
				},
			}, nil)
		} else {
			rm.mu.Unlock()
		}
	} else {
		rm.mu.Lock()
		empty := len(rm.clients) == 0
		rm.mu.Unlock()
		if empty {
			h.CloseRoom(rm, "empty")
		}
	}
}

// promoteHostOrClose runs when the host's grace window expires. If any
// guests are still connected, the earliest-joined one is promoted to host
// (and the new owner persisted) so the room keeps going instead of being
// torn down. With nobody left, the room closes as before.
func (h *WatchRoomHub) promoteHostOrClose(rm *HubRoom) {
	rm.mu.Lock()
	if rm.closed {
		rm.mu.Unlock()
		return
	}
	// Host reconnected just as the timer fired — nothing to do.
	if rm.hasHostLocked() {
		rm.hostGraceTimer = nil
		rm.HostDisconnectDeadline = time.Time{}
		rm.mu.Unlock()
		return
	}
	var newHost *HubClient
	for c := range rm.clients {
		if newHost == nil || c.JoinedAt.Before(newHost.JoinedAt) {
			newHost = c
		}
	}
	if newHost == nil {
		rm.mu.Unlock()
		h.CloseRoom(rm, "host_disconnect")
		return
	}
	newHost.IsHost = true
	rm.OwnerID = newHost.UserID
	rm.hostGraceTimer = nil
	rm.HostDisconnectDeadline = time.Time{}
	newOwnerID := newHost.UserID
	newOwnerName := newHost.UserName
	rm.mu.Unlock()

	// Persist the new owner so reconnects / cold-start resolve host status.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.repo.UpdateOwner(ctx, rm.ID, newOwnerID); err != nil {
		log.Printf("[hub] room=%s host-transfer persist failed: %v", rm.ID.Hex(), err)
	}

	h.broadcast(rm, hubMessage{
		Type: "host_changed",
		Payload: map[string]any{
			"owner_id":  newOwnerID.Hex(),
			"user_name": newOwnerName,
		},
	}, nil)
	log.Printf("[hub] room=%s host transferred to user=%s", rm.ID.Hex(), newOwnerID.Hex())
}

// recordChat buffers one chat/emoji entry for replay on rejoin. In cluster
// mode it pushes the canonical chat-history payload onto the shared Redis
// ring (so a join on any instance replays it); otherwise it appends to the
// in-memory ring. Never touches Mongo.
func (h *WatchRoomHub) recordChat(rm *HubRoom, m models.WatchRoomMessage) {
	if h.bus != nil {
		payload, err := json.Marshal(map[string]any{
			"user_id":     m.UserID.Hex(),
			"user_name":   m.UserName,
			"user_avatar": m.UserAvatar,
			"kind":        m.Kind,
			"text":        m.Text,
			"emoji":       m.Emoji,
			"gif_url":     m.GifURL,
			"created_at":  m.CreatedAt,
		})
		if err == nil {
			h.bus.PushChat(rm.ID.Hex(), payload, chatBufferSize)
		}
		return
	}
	rm.appendMessage(m)
}

// appendMessage adds one entry to the room's in-memory chat ring buffer,
// trimming the oldest once it exceeds chatBufferSize. Safe to call without
// holding rm.mu — it locks internally.
func (rm *HubRoom) appendMessage(m models.WatchRoomMessage) {
	rm.mu.Lock()
	rm.recentMessages = append(rm.recentMessages, m)
	if len(rm.recentMessages) > chatBufferSize {
		rm.recentMessages = rm.recentMessages[len(rm.recentMessages)-chatBufferSize:]
	}
	rm.mu.Unlock()
}

// allowChat enforces slow-mode in presenceMode rooms: at most one chat/
// reaction per user per chatSlowModeInterval. Small rooms are unthrottled.
// Returns true if the message is allowed (and records the timestamp).
func (rm *HubRoom) allowChat(userID primitive.ObjectID) bool {
	if !rm.presenceMode {
		return true
	}
	now := time.Now()
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if last, ok := rm.lastChat[userID]; ok && now.Sub(last) < chatSlowModeInterval {
		return false
	}
	rm.lastChat[userID] = now
	return true
}

func (rm *HubRoom) hasHostLocked() bool {
	for c := range rm.clients {
		if c.IsHost {
			return true
		}
	}
	return false
}

// HandleCommand processes one inbound message from a client. Host-only
// commands sent by guests are silently dropped (logged).
func (h *WatchRoomHub) HandleCommand(rm *HubRoom, c *HubClient, raw []byte) {
	var msg hubMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	switch msg.Type {
	case "host_action":
		if !c.IsHost {
			log.Printf("[hub] non-host attempted host_action room=%s user=%s", rm.ID.Hex(), c.UserID.Hex())
			return
		}
		var p struct {
			Action   string  `json:"action"`
			Position float64 `json:"position"`
		}
		if err := decodePayload(msg.Payload, &p); err != nil {
			return
		}
		now := time.Now()
		rm.mu.Lock()
		rm.Position = p.Position
		switch p.Action {
		case "play":
			rm.IsPlaying = true
		case "pause":
			rm.IsPlaying = false
		case "seek":
			// keep current play/pause as-is; just move the head
		default:
			rm.mu.Unlock()
			return
		}
		rm.LastStateUpdate = now
		pos, playing, asOf := rm.Position, rm.IsPlaying, now.UnixMilli()
		rm.mu.Unlock()
		// Cluster mode: write the authoritative head to Redis so other
		// instances (and fresh joiners anywhere) read the correct position.
		if h.bus != nil {
			h.bus.SetPlayback(rm.ID.Hex(), pos, playing, asOf)
		}
		// Persist async — don't block the broadcast.
		go func(roomID primitive.ObjectID, pos float64, playing bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.repo.UpdatePlaybackState(ctx, roomID, pos, playing)
		}(rm.ID, pos, playing)
		h.broadcastState(rm, nil)

	case "chat_send":
		var p struct {
			Kind   string `json:"kind"`
			Text   string `json:"text"`
			Emoji  string `json:"emoji"`
			GifURL string `json:"gif_url"`
		}
		if err := decodePayload(msg.Payload, &p); err != nil {
			return
		}
		if p.Kind != "text" && p.Kind != "emoji" && p.Kind != "gif" {
			p.Kind = "text"
		}
		// A gif message must carry a valid GIPHY URL; otherwise drop it so a
		// malformed/forged gif can't render a broken or hostile image.
		if p.Kind == "gif" && !isAllowedGifURL(p.GifURL) {
			return
		}
		// Slow-mode in large/premiere rooms — drop and notify the sender.
		if !rm.allowChat(c.UserID) {
			h.sendTo(c, hubMessage{
				Type:    "chat_rate_limited",
				Payload: map[string]any{"interval_ms": chatSlowModeInterval.Milliseconds()},
			})
			return
		}
		entry := models.WatchRoomMessage{
			RoomID:     rm.ID,
			UserID:     c.UserID,
			UserName:   c.UserName,
			UserAvatar: c.UserAvatar,
			Kind:       p.Kind,
			Text:       p.Text,
			Emoji:      p.Emoji,
			GifURL:     p.GifURL,
			CreatedAt:  time.Now(),
		}
		// Never persisted to Mongo — buffered for replay on rejoin (shared
		// Redis ring in cluster mode, in-memory slice otherwise).
		chatPayload := map[string]any{
			"user_id":     c.UserID.Hex(),
			"user_name":   c.UserName,
			"user_avatar": c.UserAvatar,
			"kind":        entry.Kind,
			"text":        entry.Text,
			"emoji":       entry.Emoji,
			"gif_url":     entry.GifURL,
			"created_at":  entry.CreatedAt,
		}
		h.recordChat(rm, entry)
		h.broadcast(rm, hubMessage{Type: "chat_message", Payload: chatPayload}, nil)

	case "typing":
		// Lightweight ephemeral broadcast — never persisted. Suppressed in
		// presenceMode: thousands of typing toggles would drown the channel.
		if rm.presenceMode {
			return
		}
		var p struct {
			IsTyping bool `json:"is_typing"`
		}
		_ = decodePayload(msg.Payload, &p)
		h.broadcast(rm, hubMessage{
			Type: "typing",
			Payload: map[string]any{
				"user_id":   c.UserID.Hex(),
				"user_name": c.UserName,
				"is_typing": p.IsTyping,
			},
		}, c)

	case "reaction":
		// Floats over the video for all members. Persisted as a single
		// chat row with kind=emoji so the timeline survives reloads.
		var p struct {
			Emoji string `json:"emoji"`
		}
		_ = decodePayload(msg.Payload, &p)
		if p.Emoji == "" {
			return
		}
		// Reactions share the chat slow-mode budget in presenceMode rooms.
		if !rm.allowChat(c.UserID) {
			return
		}
		entry := models.WatchRoomMessage{
			RoomID:     rm.ID,
			UserID:     c.UserID,
			UserName:   c.UserName,
			UserAvatar: c.UserAvatar,
			Kind:       "emoji",
			Emoji:      p.Emoji,
			CreatedAt:  time.Now(),
		}
		// Buffered for replay on rejoin (as an emoji chat entry).
		h.recordChat(rm, entry)
		h.broadcast(rm, hubMessage{
			Type: "reaction",
			Payload: map[string]any{
				"user_id":   c.UserID.Hex(),
				"user_name": c.UserName,
				"emoji":     p.Emoji,
				"ts":        time.Now().UnixMilli(),
			},
		}, nil)

	case "episode_request":
		// Guests use this to nudge the host to switch episodes (typically
		// auto-advance or "let's watch the next one"). Hub just forwards
		// to the host as a typed message; host UI decides whether to ack.
		var p struct {
			TargetEpisodeID string `json:"target_episode_id"`
			Reason          string `json:"reason"` // "next" | "prev" | "ended"
		}
		_ = decodePayload(msg.Payload, &p)
		rm.mu.Lock()
		var host *HubClient
		for cc := range rm.clients {
			if cc.IsHost {
				host = cc
				break
			}
		}
		rm.mu.Unlock()
		if host != nil {
			h.sendTo(host, hubMessage{
				Type: "episode_request",
				Payload: map[string]any{
					"user_id":           c.UserID.Hex(),
					"user_name":         c.UserName,
					"target_episode_id": p.TargetEpisodeID,
					"reason":            p.Reason,
				},
			})
		}

	case "host_kick":
		if !c.IsHost {
			return
		}
		var p struct {
			UserID string `json:"user_id"`
		}
		if err := decodePayload(msg.Payload, &p); err != nil {
			return
		}
		target, _ := primitive.ObjectIDFromHex(p.UserID)
		if target.IsZero() || target == c.UserID {
			return
		}
		h.KickUser(rm, target)

	case "ping":
		h.sendTo(c, hubMessage{Type: "pong", Payload: map[string]any{"ts": time.Now().UnixMilli()}})
	}
}

// BroadcastTheme pushes a theme_change WS event so every connected
// guest repaints their background without a page reload.
func (h *WatchRoomHub) BroadcastTheme(rm *HubRoom, from, to string) {
	h.broadcast(rm, hubMessage{
		Type: "theme_change",
		Payload: map[string]any{
			"from": from,
			"to":   to,
		},
	}, nil)
}

// BroadcastEpisodeChange tells every client in the room to reload its
// player against a new episode. Also resets the in-memory playback state
// so the next state_sync isn't stuck on the old episode's position.
func (h *WatchRoomHub) BroadcastEpisodeChange(rm *HubRoom, episodeID, episodeTitle string) {
	rm.mu.Lock()
	rm.Position = 0
	rm.IsPlaying = false
	rm.LastStateUpdate = time.Now()
	rm.mu.Unlock()
	h.broadcast(rm, hubMessage{
		Type: "episode_change",
		Payload: map[string]any{
			"episode_id":    episodeID,
			"episode_title": episodeTitle,
		},
	}, nil)
}

// KickUser disconnects every connection owned by `userID` from the room and
// notifies the rest of the members. The kicked side sees a "kicked" event
// and the WS closes immediately after.
func (h *WatchRoomHub) KickUser(rm *HubRoom, userID primitive.ObjectID) {
	// Disconnect the target's connections on this instance.
	h.localKick(rm, userID)
	// Cluster mode: the target may be connected to another instance — tell
	// every instance to drop their local copies too.
	if h.bus != nil {
		ctrl, _ := json.Marshal(hubMessage{Type: "__kick", Payload: map[string]any{"user_id": userID.Hex()}})
		h.bus.Publish(rm.ID.Hex(), ctrl, "")
	}
	h.broadcast(rm, hubMessage{
		Type:    "member_left",
		Payload: map[string]any{"user_id": userID.Hex(), "kicked": true},
	}, nil)
}

// localKick closes this instance's connections for userID (non-host only),
// sending them a "kicked" frame first.
func (h *WatchRoomHub) localKick(rm *HubRoom, userID primitive.ObjectID) {
	rm.mu.Lock()
	targets := make([]*HubClient, 0)
	for c := range rm.clients {
		if c.UserID == userID && !c.IsHost {
			targets = append(targets, c)
		}
	}
	rm.mu.Unlock()
	payload, _ := json.Marshal(hubMessage{Type: "kicked", Payload: map[string]any{"reason": "host kicked"}})
	for _, c := range targets {
		_ = c.Conn.WriteMessage(websocket.TextMessage, payload)
		_ = c.Conn.Close()
	}
}

// SnapshotMembers returns the current member list — used by the admin REST
// endpoint that shows who's in each room.
func (h *WatchRoomHub) SnapshotMembers(roomID primitive.ObjectID) []map[string]any {
	h.mu.RLock()
	rm, ok := h.rooms[roomID]
	h.mu.RUnlock()
	if !ok {
		return nil
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	out := make([]map[string]any, 0, len(rm.clients))
	for c := range rm.clients {
		out = append(out, map[string]any{
			"user_id":     c.UserID.Hex(),
			"user_name":   c.UserName,
			"user_avatar": c.UserAvatar,
			"is_host":     c.IsHost,
		})
	}
	return out
}

// MemberCount returns the cluster-wide live member count (Redis presence) in
// cluster mode, or this instance's local count otherwise. Used by the room
// listing handlers.
func (h *WatchRoomHub) MemberCount(roomID primitive.ObjectID) int {
	if h.bus != nil {
		return h.bus.GetPresence(roomID.Hex())
	}
	return len(h.SnapshotMembers(roomID))
}

// SnapshotMembersPage returns a slice of the roster plus the total live
// member count — used by the paginated roster REST endpoint that powers the
// virtualized member list in large/premiere rooms. Hosts are surfaced first
// so they're always visible on the first page.
func (h *WatchRoomHub) SnapshotMembersPage(roomID primitive.ObjectID, offset, limit int) ([]map[string]any, int) {
	// Build the full roster as a list of maps. Cluster mode reads the shared
	// Redis roster (so any instance answers completely); otherwise it reads
	// this instance's local connections.
	var all []map[string]any
	if h.bus != nil {
		for _, raw := range h.bus.Roster(roomID.Hex()) {
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				all = append(all, m)
			}
		}
	} else {
		h.mu.RLock()
		rm, ok := h.rooms[roomID]
		h.mu.RUnlock()
		if !ok {
			return []map[string]any{}, 0
		}
		rm.mu.Lock()
		for c := range rm.clients {
			all = append(all, map[string]any{
				"user_id":     c.UserID.Hex(),
				"user_name":   c.UserName,
				"user_avatar": c.UserAvatar,
				"is_host":     c.IsHost,
			})
		}
		rm.mu.Unlock()
	}

	isHost := func(m map[string]any) bool { v, _ := m["is_host"].(bool); return v }
	uid := func(m map[string]any) string { v, _ := m["user_id"].(string); return v }
	// Hosts first, then stable by user id so pages don't reshuffle wildly.
	sort.Slice(all, func(i, j int) bool {
		if isHost(all[i]) != isHost(all[j]) {
			return isHost(all[i])
		}
		return uid(all[i]) < uid(all[j])
	})

	total := len(all)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return all[offset:end], total
}

// CloseRoom tears the room down everywhere and marks it closed in Mongo.
// In cluster mode it first tells peer instances to drop their local copies,
// then performs the one-time Mongo close + Redis cleanup here.
func (h *WatchRoomHub) CloseRoom(rm *HubRoom, reason string) {
	// Tell peers to tear down their local connections + subscription.
	if h.bus != nil {
		ctrl, _ := json.Marshal(hubMessage{Type: "__close", Payload: map[string]any{"reason": reason}})
		h.bus.Publish(rm.ID.Hex(), ctrl, "")
	}
	if !h.closeLocalOnly(rm, reason) {
		// Someone already tore it down — don't double-persist/clean.
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = h.repo.CloseRoom(ctx, rm.ID)
	// Chat/state/presence/roster live only in memory or Redis — wipe the
	// Redis keys (in-memory is GC'd with the HubRoom).
	if h.bus != nil {
		h.bus.Cleanup(rm.ID.Hex())
	}
	log.Printf("[hub] room=%s closed reason=%s", rm.ID.Hex(), reason)
}

// closeLocalOnly drops this instance's connections + subscription for the
// room and removes it from the local map. Idempotent — returns true only on
// the call that actually performed the teardown. Does NOT touch Mongo/Redis.
func (h *WatchRoomHub) closeLocalOnly(rm *HubRoom, reason string) bool {
	rm.mu.Lock()
	if rm.closed {
		rm.mu.Unlock()
		return false
	}
	rm.closed = true
	clients := make([]*HubClient, 0, len(rm.clients))
	for c := range rm.clients {
		clients = append(clients, c)
	}
	rm.mu.Unlock()

	payload, _ := json.Marshal(hubMessage{Type: "room_closed", Payload: map[string]any{"reason": reason}})
	for _, c := range clients {
		_ = c.Conn.WriteMessage(websocket.TextMessage, payload)
		_ = c.Conn.Close()
	}
	if rm.busCancel != nil {
		rm.busCancel()
	}
	h.mu.Lock()
	delete(h.rooms, rm.ID)
	h.mu.Unlock()
	return true
}

// ── broadcast helpers ─────────────────────────────────────────────────────

// CRITICAL: `as_of_ms` MUST be the timestamp of the host action that
// last set `Position` — NOT time.Now() at broadcast time. The guest
// computes `target = position + (now - as_of_ms)/1000`. If as_of_ms is
// "now" while position is stale, elapsed=0 and the guest seeks back
// to the OLD position every heartbeat — visible as the "last segment
// loops forever" bug on every non-host viewer.
func (h *WatchRoomHub) sendState(rm *HubRoom, c *HubClient) {
	pos, playing, asOf := h.playbackState(rm)
	state := hubMessage{
		Type: "state_sync",
		Payload: map[string]any{
			"position":     pos,
			"is_playing":   playing,
			"as_of_ms":     asOf,
			"member_count": h.memberCount(rm),
		},
	}
	h.sendTo(c, state)
}

// stateMsg builds a state_sync message from the room's authoritative head.
func (h *WatchRoomHub) stateMsg(rm *HubRoom) hubMessage {
	pos, playing, asOf := h.playbackState(rm)
	return hubMessage{
		Type: "state_sync",
		Payload: map[string]any{
			"position":     pos,
			"is_playing":   playing,
			"as_of_ms":     asOf,
			"member_count": h.memberCount(rm),
		},
	}
}

// broadcastState publishes a state_sync cluster-wide (used after host actions).
func (h *WatchRoomHub) broadcastState(rm *HubRoom, except *HubClient) {
	h.broadcast(rm, h.stateMsg(rm), except)
}

// playbackState returns the authoritative head — from Redis in cluster mode
// (so any instance is correct), else from this room's in-memory fields.
func (h *WatchRoomHub) playbackState(rm *HubRoom) (position float64, isPlaying bool, asOfMs int64) {
	if h.bus != nil {
		if pos, playing, asOf, ok := h.bus.GetPlayback(rm.ID.Hex()); ok {
			return pos, playing, asOf
		}
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.Position, rm.IsPlaying, rm.LastStateUpdate.UnixMilli()
}

// memberCount is the cluster-wide live count in bus mode, else the local one.
func (h *WatchRoomHub) memberCount(rm *HubRoom) int {
	if h.bus != nil {
		return h.bus.GetPresence(rm.ID.Hex())
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	// Count distinct users, not raw connections — one account opened on two
	// devices is still one member. Without this the roster count showed "2"
	// while the deduped member LIST showed the user once.
	seen := make(map[primitive.ObjectID]struct{}, len(rm.clients))
	for c := range rm.clients {
		seen[c.UserID] = struct{}{}
	}
	return len(seen)
}

// broadcast delivers msg to every member of the room. In cluster mode it
// publishes to Redis (the per-room subscriber on every instance, including
// this one, then fans it out locally — so we do NOT also send locally here).
// In single-instance mode it fans out to local connections directly.
func (h *WatchRoomHub) broadcast(rm *HubRoom, msg hubMessage, except *HubClient) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	exceptUser := ""
	if except != nil {
		exceptUser = except.UserID.Hex()
	}
	if h.bus != nil {
		h.bus.Publish(rm.ID.Hex(), data, exceptUser)
		return
	}
	h.localBroadcastRaw(rm, data, exceptUser)
}

// localBroadcastRaw fans an already-marshalled frame out to this instance's
// connections, skipping every connection owned by exceptUser (empty = none).
func (h *WatchRoomHub) localBroadcastRaw(rm *HubRoom, data []byte, exceptUser string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	for c := range rm.clients {
		if exceptUser != "" && c.UserID.Hex() == exceptUser {
			continue
		}
		select {
		case c.send <- data:
		default:
			// Slow client — drop to keep the broadcast fast. The client
			// will recover state on the next heartbeat.
		}
	}
}

func (h *WatchRoomHub) sendTo(c *HubClient, msg hubMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

// ── connection pumps ──────────────────────────────────────────────────────

// writePump drains a client's send channel and writes to the socket. To keep
// fan-out cheap at thousands of viewers it COALESCES queued messages into one
// batched frame every writeFlushInterval (or sooner once writeBatchMax
// queue up) instead of one syscall per message. A batch of N messages is sent
// as a single {"type":"batch","payload":{"messages":[<raw>,...]}} envelope;
// a lone message is sent as-is. The client unwraps "batch" and dispatches
// each inner message in order.
func (h *WatchRoomHub) writePump(c *HubClient) {
	defer c.Conn.Close()
	ticker := time.NewTicker(writeFlushInterval)
	defer ticker.Stop()

	pending := make([][]byte, 0, writeBatchMax)
	flush := func() bool {
		if len(pending) == 0 {
			return true
		}
		_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		var frame []byte
		if len(pending) == 1 {
			frame = pending[0]
		} else {
			frame = encodeBatch(pending)
		}
		pending = pending[:0]
		return c.Conn.WriteMessage(websocket.TextMessage, frame) == nil
	}

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				flush()
				return
			}
			pending = append(pending, msg)
			if len(pending) >= writeBatchMax {
				if !flush() {
					return
				}
			}
		case <-ticker.C:
			if !flush() {
				return
			}
		}
	}
}

// encodeBatch wraps several already-marshalled messages into one envelope
// frame without re-parsing them: {"type":"batch","payload":{"messages":[…]}}.
func encodeBatch(msgs [][]byte) []byte {
	var b bytes.Buffer
	b.WriteString(`{"type":"batch","payload":{"messages":[`)
	for i, m := range msgs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(m)
	}
	b.WriteString(`]}}`)
	return b.Bytes()
}

// ── periodic heartbeat ────────────────────────────────────────────────────

// StartHeartbeat broadcasts a state_sync to every room every 5s so guests
// can recover from buffer drift without explicit host commands. Intended
// to be called once at backend startup.
func (h *WatchRoomHub) StartHeartbeat() {
	go func() {
		// 2s instead of 5s — guests reported a noticeable lag between
		// host actions and their own playhead. A tighter heartbeat
		// catches drift faster without measurable bandwidth cost
		// (one tiny JSON per room every 2s).
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			h.mu.RLock()
			rooms := make([]*HubRoom, 0, len(h.rooms))
			for _, r := range h.rooms {
				rooms = append(rooms, r)
			}
			h.mu.RUnlock()
			for _, rm := range rooms {
				// Heartbeat fans out to LOCAL clients only. In cluster mode
				// every instance runs its own heartbeat, so publishing here
				// would duplicate the frame across the whole fleet; instead
				// each instance reads the shared head (Redis) and pushes it
				// to the connections it owns.
				data, err := json.Marshal(h.stateMsg(rm))
				if err != nil {
					continue
				}
				h.localBroadcastRaw(rm, data, "")
			}
		}
	}()
}

// ── helpers ────────────────────────────────────────────────────────────────

type hubMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

func decodePayload(in interface{}, out interface{}) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// Sentinel errors.
var (
	ErrRoomClosed = wsError("room closed")
	ErrRoomFull   = wsError("room full")
)

type wsError string

func (e wsError) Error() string { return string(e) }

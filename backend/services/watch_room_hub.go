package services

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var _ = primitive.NilObjectID // keep primitive imported even if every direct use is via models

// HostDisconnectGrace is how long we wait for the host to reconnect after
// their websocket drops before tearing down the room. Bumped to 5 minutes
// per UX request — a host who tab-closes by mistake or loses connectivity
// briefly should be able to walk back in and resume.
const HostDisconnectGrace = 5 * time.Minute

// HubClient is a single websocket connection inside a room. One per browser
// tab; the same user may have multiple clients (multi-device).
type HubClient struct {
	UserID     primitive.ObjectID
	UserName   string
	UserAvatar string
	IsHost     bool
	Conn       *websocket.Conn
	send       chan []byte
}

// HubRoom is the live, in-memory state for one watch-room. Persisted state
// (rooms collection) is the source of truth on cold-start; the hub takes
// over while at least one connection is open.
type HubRoom struct {
	ID         primitive.ObjectID
	OwnerID    primitive.ObjectID
	MaxMembers int

	Position         float64
	IsPlaying        bool
	LastStateUpdate  time.Time
	// hostDisconnectDeadline is set when the host drops; clients use it to
	// render the "Host qaytmoqda… (mm:ss qoldi)" countdown. Zero when the
	// host is currently connected.
	HostDisconnectDeadline time.Time

	mu      sync.Mutex
	clients map[*HubClient]struct{}
	// hostGraceTimer fires when the host has been gone for HostDisconnectGrace
	// without reconnecting. Cancelled if the host re-joins in time.
	hostGraceTimer *time.Timer
	closed         bool
}

// WatchRoomHub multiplexes every active room. Methods are safe to call from
// any goroutine — the rooms map is protected by mu, and each HubRoom has
// its own mutex for member/state mutation.
type WatchRoomHub struct {
	repo *repositories.WatchRoomRepository

	mu    sync.RWMutex
	rooms map[primitive.ObjectID]*HubRoom
}

func NewWatchRoomHub(repo *repositories.WatchRoomRepository) *WatchRoomHub {
	return &WatchRoomHub{
		repo:  repo,
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
		clients:         make(map[*HubClient]struct{}),
	}
	h.rooms[roomID] = rm
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
	c.send = make(chan []byte, 32)
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
	if hostReconnected {
		h.broadcast(rm, hubMessage{Type: "host_reconnected", Payload: map[string]any{}}, nil)
	}

	go h.writePump(c)
	h.sendState(rm, c)

	// Send a snapshot of every current member (including the just-joined
	// client themselves) so the client UI has the full roster on entry,
	// without waiting for someone else to (re)join.
	rm.mu.Lock()
	snapshot := make([]map[string]any, 0, len(rm.clients))
	for cc := range rm.clients {
		snapshot = append(snapshot, map[string]any{
			"user_id":     cc.UserID.Hex(),
			"user_name":   cc.UserName,
			"user_avatar": cc.UserAvatar,
			"is_host":     cc.IsHost,
		})
	}
	rm.mu.Unlock()
	h.sendTo(c, hubMessage{Type: "member_snapshot", Payload: map[string]any{"members": snapshot}})

	// Then announce the new client to everyone else.
	h.broadcast(rm, hubMessage{
		Type: "member_joined",
		Payload: map[string]any{
			"user_id":     c.UserID.Hex(),
			"user_name":   c.UserName,
			"user_avatar": c.UserAvatar,
			"is_host":     c.IsHost,
		},
	}, c)
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
	isHostGone := c.IsHost && !rm.hasHostLocked()
	rm.mu.Unlock()

	h.broadcast(rm, hubMessage{
		Type: "member_left",
		Payload: map[string]any{"user_id": c.UserID.Hex()},
	}, nil)

	if isHostGone {
		rm.mu.Lock()
		if rm.hostGraceTimer == nil {
			deadline := time.Now().Add(HostDisconnectGrace)
			rm.HostDisconnectDeadline = deadline
			rm.hostGraceTimer = time.AfterFunc(HostDisconnectGrace, func() {
				h.CloseRoom(rm, "host_disconnect")
			})
			rm.mu.Unlock()
			// Tell every remaining client when the room will be torn down
			// so they can render a countdown instead of a silent buffer.
			h.broadcast(rm, hubMessage{
				Type: "host_disconnected",
				Payload: map[string]any{
					"deadline_ms":    deadline.UnixMilli(),
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
		rm.mu.Unlock()
		// Persist async — don't block the broadcast.
		go func(roomID primitive.ObjectID, pos float64, playing bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.repo.UpdatePlaybackState(ctx, roomID, pos, playing)
		}(rm.ID, rm.Position, rm.IsPlaying)
		h.broadcastState(rm, nil)

	case "chat_send":
		var p struct {
			Kind  string `json:"kind"`
			Text  string `json:"text"`
			Emoji string `json:"emoji"`
		}
		if err := decodePayload(msg.Payload, &p); err != nil {
			return
		}
		if p.Kind != "text" && p.Kind != "emoji" {
			p.Kind = "text"
		}
		entry := &models.WatchRoomMessage{
			RoomID:     rm.ID,
			UserID:     c.UserID,
			UserName:   c.UserName,
			UserAvatar: c.UserAvatar,
			Kind:       p.Kind,
			Text:       p.Text,
			Emoji:      p.Emoji,
			CreatedAt:  time.Now(),
		}
		// Persist (best-effort).
		go func(m *models.WatchRoomMessage) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.repo.CreateMessage(ctx, m)
		}(entry)
		h.broadcast(rm, hubMessage{
			Type: "chat_message",
			Payload: map[string]any{
				"user_id":     c.UserID.Hex(),
				"user_name":   c.UserName,
				"user_avatar": c.UserAvatar,
				"kind":        entry.Kind,
				"text":        entry.Text,
				"emoji":       entry.Emoji,
				"created_at":  entry.CreatedAt,
			},
		}, nil)

	case "typing":
		// Lightweight ephemeral broadcast — never persisted.
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
		entry := &models.WatchRoomMessage{
			RoomID:     rm.ID,
			UserID:     c.UserID,
			UserName:   c.UserName,
			UserAvatar: c.UserAvatar,
			Kind:       "emoji",
			Emoji:      p.Emoji,
			CreatedAt:  time.Now(),
		}
		go func(m *models.WatchRoomMessage) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.repo.CreateMessage(ctx, m)
		}(entry)
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
	rm.mu.Lock()
	targets := make([]*HubClient, 0)
	for c := range rm.clients {
		if c.UserID == userID && !c.IsHost {
			targets = append(targets, c)
		}
	}
	rm.mu.Unlock()
	if len(targets) == 0 {
		return
	}
	payload, _ := json.Marshal(hubMessage{Type: "kicked", Payload: map[string]any{"reason": "host kicked"}})
	for _, c := range targets {
		_ = c.Conn.WriteMessage(websocket.TextMessage, payload)
		_ = c.Conn.Close()
	}
	h.broadcast(rm, hubMessage{
		Type: "member_left",
		Payload: map[string]any{"user_id": userID.Hex(), "kicked": true},
	}, nil)
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

// CloseRoom kicks every client and marks the room closed in Mongo.
func (h *WatchRoomHub) CloseRoom(rm *HubRoom, reason string) {
	rm.mu.Lock()
	if rm.closed {
		rm.mu.Unlock()
		return
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

	h.mu.Lock()
	delete(h.rooms, rm.ID)
	h.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.repo.CloseRoom(ctx, rm.ID)
	log.Printf("[hub] room=%s closed reason=%s", rm.ID.Hex(), reason)
}

// ── broadcast helpers ─────────────────────────────────────────────────────

func (h *WatchRoomHub) sendState(rm *HubRoom, c *HubClient) {
	rm.mu.Lock()
	state := hubMessage{
		Type: "state_sync",
		Payload: map[string]any{
			"position":    rm.Position,
			"is_playing":  rm.IsPlaying,
			"as_of_ms":    time.Now().UnixMilli(),
			"member_count": len(rm.clients),
		},
	}
	rm.mu.Unlock()
	h.sendTo(c, state)
}

func (h *WatchRoomHub) broadcastState(rm *HubRoom, except *HubClient) {
	rm.mu.Lock()
	state := hubMessage{
		Type: "state_sync",
		Payload: map[string]any{
			"position":   rm.Position,
			"is_playing": rm.IsPlaying,
			"as_of_ms":   time.Now().UnixMilli(),
		},
	}
	rm.mu.Unlock()
	h.broadcast(rm, state, except)
}

func (h *WatchRoomHub) broadcast(rm *HubRoom, msg hubMessage, except *HubClient) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	for c := range rm.clients {
		if c == except {
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

func (h *WatchRoomHub) writePump(c *HubClient) {
	defer c.Conn.Close()
	for msg := range c.send {
		_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
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
				h.broadcastState(rm, nil)
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

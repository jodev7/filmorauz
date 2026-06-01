package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// WatchRoom is a synchronized-playback room (Phase 1 + 2).
// The host (owner) drives the timeline; guests can only adjust their own
// audio volume. Chat + emoji live in room_messages.
type WatchRoom struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	OwnerID         primitive.ObjectID `bson:"owner_id" json:"owner_id"`
	OwnerName       string             `bson:"owner_name,omitempty" json:"owner_name,omitempty"`
	OwnerAvatar     string             `bson:"owner_avatar,omitempty" json:"owner_avatar,omitempty"`
	OwnerIsPremium  bool               `bson:"owner_is_premium,omitempty" json:"owner_is_premium,omitempty"`
	// ContentType:
	//   "movie"   — single-movie room. ContentID = movie ID.
	//   "episode" — single-episode room (legacy). ContentID = episode ID.
	//   "series"  — series room with episode switching + auto-advance.
	//               ContentID = series ID; CurrentEpisodeID tracks what's playing.
	ContentType     string             `bson:"content_type" json:"content_type"`
	ContentID       primitive.ObjectID `bson:"content_id" json:"content_id"`
	ContentTitle    string             `bson:"content_title,omitempty" json:"content_title,omitempty"`
	ContentPoster   string             `bson:"content_poster,omitempty" json:"content_poster,omitempty"`
	ContentSlug     string             `bson:"content_slug,omitempty" json:"content_slug,omitempty"`
	// SeriesID + SeasonID for episode/series rooms — lets the frontend build
	// the /watch URL without an extra round-trip.
	SeriesID         primitive.ObjectID `bson:"series_id,omitempty" json:"series_id,omitempty"`
	SeasonID         primitive.ObjectID `bson:"season_id,omitempty" json:"season_id,omitempty"`
	// CurrentEpisodeID is the episode currently playing in a series room.
	// Set on creation (first episode by default) and bumped by host when
	// they switch or when auto-advance fires.
	CurrentEpisodeID    primitive.ObjectID `bson:"current_episode_id,omitempty" json:"current_episode_id,omitempty"`
	CurrentEpisodeTitle string             `bson:"current_episode_title,omitempty" json:"current_episode_title,omitempty"`
	Visibility string            `bson:"visibility" json:"visibility"` // "public" | "private"
	MaxMembers int               `bson:"max_members" json:"max_members"`

	// Kind distinguishes admin-created "premiere" rooms from ordinary
	// user-hosted rooms. Premiere rooms are pinned at the top of the /rooms
	// page (like a film premiere) and bypass the free/premium member caps.
	//   "normal"   — user-hosted room (default).
	//   "premiere" — admin/superadmin-hosted featured room.
	Kind string `bson:"kind,omitempty" json:"kind,omitempty"`
	// IsFeatured pins the room to the top of the public rooms listing.
	IsFeatured bool `bson:"is_featured,omitempty" json:"is_featured,omitempty"`
	// PinPriority orders featured rooms among themselves — higher floats up.
	PinPriority int `bson:"pin_priority,omitempty" json:"pin_priority,omitempty"`
	// ScheduledStartAt drives the "premyera boshlanishiga ... qoldi"
	// countdown on the rooms page. Zero means "already live / no schedule".
	ScheduledStartAt time.Time `bson:"scheduled_start_at,omitempty" json:"scheduled_start_at,omitempty"`

	// Playback state — broadcast verbatim to guests over WS.
	PositionSeconds  float64   `bson:"position_seconds" json:"position_seconds"`
	IsPlaying        bool      `bson:"is_playing" json:"is_playing"`
	LastStateUpdate  time.Time `bson:"last_state_update" json:"last_state_update"`

	// Theme is a host-selectable colour for the room background.
	// Premium-only feature. Stored verbatim (CSS-safe strings) so the
	// frontend can drop it into a linear-gradient without further
	// processing. Empty values → frontend uses the default brand bg.
	Theme RoomTheme `bson:"theme,omitempty" json:"theme,omitempty"`

	Status    string    `bson:"status" json:"status"` // "active" | "closed"
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
	ClosedAt  *time.Time `bson:"closed_at,omitempty" json:"closed_at,omitempty"`
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at"` // automatic cleanup at this point
}

// RoomTheme is the host-chosen background gradient for the room page.
// Both fields are stored as the bare colour strings the frontend will
// drop into `linear-gradient(135deg, from, to)` — typically hex or
// rgb()/rgba(). Empty values mean "use the default theme".
type RoomTheme struct {
	From string `bson:"from,omitempty" json:"from,omitempty"`
	To   string `bson:"to,omitempty"   json:"to,omitempty"`
}

// WatchRoomInvite is a single shareable invite link for a room. One row per
// invite code so the same room can have multiple, each with its own TTL or
// max-uses budget.
type WatchRoomInvite struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	RoomID    primitive.ObjectID `bson:"room_id" json:"room_id"`
	Code      string             `bson:"code" json:"code"` // random short code
	CreatedBy primitive.ObjectID `bson:"created_by" json:"created_by"`
	// TargetUserID is set when an invite is addressed to a specific account
	// (the in-app "invite by user ID" flow). Zero for shareable-link invites.
	TargetUserID primitive.ObjectID `bson:"target_user_id,omitempty" json:"target_user_id,omitempty"`
	MaxUses   int                `bson:"max_uses" json:"max_uses"` // 0 = unlimited (within ExpiresAt)
	Uses      int                `bson:"uses" json:"uses"`
	ExpiresAt time.Time          `bson:"expires_at" json:"expires_at"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

// WatchRoomMessage is one chat/emoji entry inside a room. NOT persisted to
// Mongo — it's used as the payload shape for the hub's in-memory chat ring
// buffer and the WebSocket chat events. Chat history dies with the room.
type WatchRoomMessage struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	RoomID    primitive.ObjectID `bson:"room_id" json:"room_id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	UserName  string             `bson:"user_name,omitempty" json:"user_name,omitempty"`
	UserAvatar string            `bson:"user_avatar,omitempty" json:"user_avatar,omitempty"`
	Kind      string             `bson:"kind" json:"kind"` // "text" | "emoji" | "gif"
	Text      string             `bson:"text,omitempty" json:"text,omitempty"`
	Emoji     string             `bson:"emoji,omitempty" json:"emoji,omitempty"`
	GifURL    string             `bson:"gif_url,omitempty" json:"gif_url,omitempty"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

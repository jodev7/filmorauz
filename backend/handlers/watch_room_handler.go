package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Free-tier limits. Premium accounts bypass both.
const (
	freeRoomsPerDay  = 3
	freeMaxMembers   = 2
	premiumMaxMembers = 20
	roomTTL          = 12 * time.Hour
	inviteTTLFree    = 6 * time.Hour
	inviteTTLPremium = 24 * time.Hour
)

type WatchRoomHandler struct {
	repo                 *repositories.WatchRoomRepository
	userRepo             *repositories.UserRepository
	movieRepo            *repositories.MovieRepository
	seriesRepo           *repositories.SeriesRepository
	notificationService  *services.NotificationService
	hub                  *services.WatchRoomHub
	upgrader             websocket.Upgrader
}

func NewWatchRoomHandler(
	repo *repositories.WatchRoomRepository,
	userRepo *repositories.UserRepository,
	movieRepo *repositories.MovieRepository,
	seriesRepo *repositories.SeriesRepository,
	notificationService *services.NotificationService,
	hub *services.WatchRoomHub,
) *WatchRoomHandler {
	return &WatchRoomHandler{
		repo:                repo,
		userRepo:            userRepo,
		movieRepo:           movieRepo,
		seriesRepo:          seriesRepo,
		notificationService: notificationService,
		hub:                 hub,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}
}

// POST /api/rooms — create a new watch-room.
func (h *WatchRoomHandler) CreateRoom(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "auth required"})
		return
	}
	userIDHex, _ := userIDRaw.(string)
	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	var body struct {
		ContentType string `json:"content_type" binding:"required"` // movie | episode
		ContentID   string `json:"content_id" binding:"required"`
		Visibility  string `json:"visibility"` // public | private (default private)
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.ContentType != "movie" && body.ContentType != "episode" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content_type must be movie or episode"})
		return
	}
	if body.Visibility != "public" && body.Visibility != "private" {
		body.Visibility = "private"
	}
	contentID, err := primitive.ObjectIDFromHex(body.ContentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content_id"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := h.userRepo.FindByID(userID.Hex())
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if user.IsBannedActive() {
		c.JSON(http.StatusForbidden, gin.H{"error": "banned user cannot create rooms"})
		return
	}
	isPremium := user.IsPremiumActive()

	// Quota check (free only).
	if !isPremium {
		since := time.Now().Truncate(24 * time.Hour)
		n, err := h.repo.CountRoomsCreatedSince(ctx, userID, since)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "quota check failed"})
			return
		}
		if n >= freeRoomsPerDay {
			c.JSON(http.StatusForbidden, gin.H{
				"error":  "daily room limit reached",
				"detail": "free users can create 3 rooms per day — upgrade to premium for unlimited",
			})
			return
		}
	}

	// Resolve content meta (title/poster).
	title, poster, slug, seriesID, seasonID, err := h.resolveContent(body.ContentType, contentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content not found"})
		return
	}

	maxMembers := freeMaxMembers
	if isPremium {
		maxMembers = premiumMaxMembers
	}

	now := time.Now()
	room := &models.WatchRoom{
		OwnerID:         userID,
		OwnerName:       resolveDisplayName(user),
		OwnerAvatar:     resolveAvatarURL(user),
		OwnerIsPremium:  isPremium,
		ContentType:     body.ContentType,
		ContentID:       contentID,
		ContentTitle:    title,
		ContentPoster:   poster,
		ContentSlug:     slug,
		SeriesID:        seriesID,
		SeasonID:        seasonID,
		Visibility:      body.Visibility,
		MaxMembers:      maxMembers,
		PositionSeconds: 0,
		IsPlaying:       false,
		LastStateUpdate: now,
		Status:          "active",
		CreatedAt:       now,
		UpdatedAt:       now,
		ExpiresAt:       now.Add(roomTTL),
	}
	if err := h.repo.CreateRoom(ctx, room); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create room"})
		return
	}
	c.JSON(http.StatusCreated, room)
}

func (h *WatchRoomHandler) resolveContent(ct string, id primitive.ObjectID) (title, poster, slug string, seriesID, seasonID primitive.ObjectID, err error) {
	if ct == "movie" {
		m, mErr := h.movieRepo.FindByID(id)
		if mErr != nil || m == nil {
			err = mongo.ErrNoDocuments
			return
		}
		title = m.Title
		poster = m.PosterURL
		slug = m.Slug
		return
	}
	// episode
	ep, epErr := h.seriesRepo.GetEpisodeByID(id)
	if epErr != nil || ep == nil {
		err = mongo.ErrNoDocuments
		return
	}
	title = ep.Title
	poster = ep.ThumbnailURL
	seriesID = ep.SeriesID
	seasonID = ep.SeasonID
	if !ep.SeriesID.IsZero() {
		if s, sErr := h.seriesRepo.GetByID(ep.SeriesID); sErr == nil && s != nil {
			slug = s.Slug
		}
	}
	return
}

// GET /api/rooms/:id
func (h *WatchRoomHandler) GetRoom(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	room, err := h.repo.GetRoomByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}
	c.JSON(http.StatusOK, room)
}

// AdminListRooms returns every active room with its in-hub members.
// Used by the admin dashboard "Rooms" tab.
// GET /api/admin/rooms
func (h *WatchRoomHandler) AdminListRooms(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cursor, err := h.repo.RoomsCollection().Find(ctx, bson.M{"status": "active"})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		return
	}
	defer cursor.Close(ctx)
	var rooms []models.WatchRoom
	if err := cursor.All(ctx, &rooms); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decode failed"})
		return
	}
	out := make([]map[string]interface{}, 0, len(rooms))
	for i := range rooms {
		r := rooms[i]
		out = append(out, map[string]interface{}{
			"room":    r,
			"members": h.hub.SnapshotMembers(r.ID),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// POST /api/rooms/:id/kick — host kicks a member.
func (h *WatchRoomHandler) KickMember(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "auth required"})
		return
	}
	userIDHex, _ := userIDRaw.(string)
	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	target, err := primitive.ObjectIDFromHex(body.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	room, err := h.repo.GetRoomByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}
	if room.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only host can kick"})
		return
	}
	hubRoom, err := h.hub.GetOrLoadRoom(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hub error"})
		return
	}
	h.hub.KickUser(hubRoom, target)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/rooms/:id/invites — generate a shareable invite code (or address one to a user).
func (h *WatchRoomHandler) CreateInvite(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "auth required"})
		return
	}
	userIDHex, _ := userIDRaw.(string)
	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var body struct {
		TargetUserID string `json:"target_user_id"`
		MaxUses      int    `json:"max_uses"`
	}
	_ = c.ShouldBindJSON(&body)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	room, err := h.repo.GetRoomByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}
	if room.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only host can invite"})
		return
	}

	ttl := inviteTTLFree
	if room.OwnerIsPremium {
		ttl = inviteTTLPremium
	}
	code := randomCode(8)
	inv := &models.WatchRoomInvite{
		RoomID:    room.ID,
		Code:      code,
		CreatedBy: userID,
		MaxUses:   body.MaxUses,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}
	if body.TargetUserID != "" {
		if tu, err := primitive.ObjectIDFromHex(body.TargetUserID); err == nil {
			inv.TargetUserID = tu
		}
	}
	if err := h.repo.CreateInvite(ctx, inv); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invite"})
		return
	}

	// If an in-app target user was specified, push a notification.
	if !inv.TargetUserID.IsZero() && h.notificationService != nil {
		host, _ := h.userRepo.FindByID(userID.Hex())
		hostName := ""
		if host != nil {
			hostName = host.DisplayName
		}
		_ = h.notificationService.CreateNotification(ctx, &models.NotificationCreateRequest{
			UserID:    inv.TargetUserID,
			Type:      models.NotificationRoomInvite,
			Title:     "Watch-room invite",
			Message:   hostName + " invited you to watch " + room.ContentTitle,
			ActionURL: "/watch-room/" + room.ID.Hex() + "?invite=" + code,
			Data: map[string]interface{}{
				"room_id":      room.ID.Hex(),
				"invite_code":  code,
				"content_type": room.ContentType,
				"content_id":   room.ContentID.Hex(),
				"title":        room.ContentTitle,
				"poster":       room.ContentPoster,
			},
		})
	}

	c.JSON(http.StatusCreated, inv)
}

// GET /api/rooms/:id/messages — replay chat history.
func (h *WatchRoomHandler) ListMessages(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msgs, err := h.repo.ListRoomMessages(ctx, id, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": msgs})
}

// GET /ws/rooms/:id — WebSocket upgrade. Auth via token query string
// (cookies/headers don't survive the cross-origin browser → ws:// hop the
// same way they do for fetch).
func (h *WatchRoomHandler) WebSocket(c *gin.Context, validateToken func(string) (string, bool)) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}
	userIDHex, ok := validateToken(token)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad user"})
		return
	}
	roomID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad room id"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	room, err := h.repo.GetRoomByID(ctx, roomID)
	if err != nil || room.Status != "active" {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not available"})
		return
	}

	user, err := h.userRepo.FindByID(userID.Hex())
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if user.IsBannedActive() {
		c.JSON(http.StatusForbidden, gin.H{"error": "banned user"})
		return
	}

	// Authorization: owner always allowed; public rooms anyone can enter;
	// private rooms require a valid (non-expired, target-or-anyone) invite.
	if room.OwnerID != userID && room.Visibility == "private" {
		inviteCode := strings.TrimSpace(c.Query("invite"))
		if inviteCode == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "invite required"})
			return
		}
		inv, err := h.repo.GetInviteByCode(ctx, inviteCode)
		if err != nil || inv.RoomID != roomID {
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid invite"})
			return
		}
		if time.Now().After(inv.ExpiresAt) {
			c.JSON(http.StatusForbidden, gin.H{"error": "invite expired"})
			return
		}
		if !inv.TargetUserID.IsZero() && inv.TargetUserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "invite addressed to another user"})
			return
		}
		if inv.MaxUses > 0 && inv.Uses >= inv.MaxUses {
			c.JSON(http.StatusForbidden, gin.H{"error": "invite at max uses"})
			return
		}
		_ = h.repo.IncrementInviteUses(ctx, inviteCode)
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[ws] upgrade failed: %v", err)
		return
	}

	hubRoom, err := h.hub.GetOrLoadRoom(ctx, roomID)
	if err != nil {
		_ = conn.Close()
		return
	}
	client := &services.HubClient{
		UserID:     userID,
		UserName:   resolveDisplayName(user),
		UserAvatar: resolveAvatarURL(user),
		IsHost:     room.OwnerID == userID,
		Conn:       conn,
	}
	if err := h.hub.AddClient(hubRoom, client); err != nil {
		_ = conn.WriteJSON(map[string]string{"type": "error", "error": err.Error()})
		_ = conn.Close()
		return
	}
	defer h.hub.RemoveClient(hubRoom, client)

	// Read pump (blocks here until connection closes).
	conn.SetReadLimit(1 << 16)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		h.hub.HandleCommand(hubRoom, client, msg)
	}
}

// SearchUsersForInvite finds users by query (display name / username
// substring, or an exact 24-hex ObjectID). Used by the in-room invite UI to
// let the host pick a registered user before sending the invite.
// GET /api/rooms/users/search?q=...
func (h *WatchRoomHandler) SearchUsersForInvite(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"items": []any{}})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type item struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Avatar      string `json:"avatar"`
		TelegramID  int64  `json:"telegram_id,omitempty"`
	}
	out := make([]item, 0)

	// Exact ObjectID lookup.
	if oid, err := primitive.ObjectIDFromHex(q); err == nil {
		u, _ := h.userRepo.FindByID(oid.Hex())
		if u != nil && !u.IsBannedActive() {
			out = append(out, item{
				ID:          u.ID.Hex(),
				DisplayName: resolveDisplayName(u),
				Avatar:      resolveAvatarURL(u),
				TelegramID:  u.TelegramID,
			})
			c.JSON(http.StatusOK, gin.H{"items": out})
			return
		}
	}

	// Exact Telegram-ID lookup — users typically copy this from Telegram.
	if tid, err := strconv.ParseInt(q, 10, 64); err == nil && tid > 0 {
		u, _ := h.userRepo.FindByTelegramID(tid)
		if u != nil && !u.IsBannedActive() {
			out = append(out, item{
				ID:          u.ID.Hex(),
				DisplayName: resolveDisplayName(u),
				Avatar:      resolveAvatarURL(u),
				TelegramID:  u.TelegramID,
			})
			c.JSON(http.StatusOK, gin.H{"items": out})
			return
		}
	}

	users, err := h.userRepo.SearchByDisplayName(ctx, q, 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		return
	}
	for i := range users {
		u := users[i]
		out = append(out, item{
			ID:          u.ID.Hex(),
			DisplayName: resolveDisplayName(&u),
			Avatar:      resolveAvatarURL(&u),
			TelegramID:  u.TelegramID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// resolveDisplayName picks the best non-empty label for a user. The hub
// previously fed bare user.DisplayName into UserName, which is often empty
// for Telegram-onboarded accounts where only first_name is set; the
// frontend then rendered "Foydalanuvchi" for everyone.
func resolveDisplayName(u *models.User) string {
	if u == nil {
		return "Foydalanuvchi"
	}
	if s := strings.TrimSpace(u.DisplayName); s != "" && s != "." && s != "-" {
		return s
	}
	if s := strings.TrimSpace(u.FirstName); s != "" && s != "." && s != "-" {
		return s
	}
	if s := strings.TrimSpace(u.TelegramUser); s != "" {
		return s
	}
	return "Foydalanuvchi"
}

func resolveAvatarURL(u *models.User) string {
	if u == nil {
		return ""
	}
	if u.ProfileImageURL != "" {
		return u.ProfileImageURL
	}
	return u.PhotoURL
}

// ── helpers ────────────────────────────────────────────────────────────────

func randomCode(n int) string {
	b := make([]byte, n/2+1)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// Suppress unused-import lint when bson is not referenced after refactors.
var _ = bson.M{}

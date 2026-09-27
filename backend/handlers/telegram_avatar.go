package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TelegramAvatarImporter copies a user's Telegram profile photo into our
// storage and uses it as their profile picture until they upload their own.
// Runs on login (new users / users without a picture) and as a background
// backfill for existing users.

const (
	tgAvatarOK      = "ok"
	tgAvatarNone    = "none" // no photo, or hidden by the user's privacy settings
	tgAvatarError   = "error"
	tgAvatarMaxSize = 5 << 20
)

type TelegramAvatarImporter struct {
	users    *mongo.Collection
	upload   *UploadHandler
	botToken string
	apiBase  string
	http     *http.Client
	running  sync.Mutex
}

// NewTelegramAvatarImporter returns nil when there is no bot token.
func NewTelegramAvatarImporter(db *mongo.Database, upload *UploadHandler, botToken string) *TelegramAvatarImporter {
	if strings.TrimSpace(botToken) == "" || upload == nil {
		return nil
	}
	return &TelegramAvatarImporter{
		users:    db.Collection("users"),
		upload:   upload,
		botToken: strings.TrimSpace(botToken),
		apiBase:  "https://api.telegram.org",
		http:     &http.Client{Timeout: 20 * time.Second},
	}
}

// noCustomPicture matches users who never set a profile picture.
var noCustomPicture = bson.M{"$or": []bson.M{
	{"profile_image_url": bson.M{"$exists": false}},
	{"profile_image_url": ""},
	{"profile_image_url": nil},
}}

// errTG hides the bot token that Go's *url.Error would print.
func (t *TelegramAvatarImporter) errTG(op string, err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	msg := strings.ReplaceAll(err.Error(), t.botToken, "<token>")
	return fmt.Errorf("telegram %s: %s", op, msg)
}

func (t *TelegramAvatarImporter) call(ctx context.Context, method string, params url.Values, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.apiBase+"/bot"+t.botToken+"/"+method+"?"+params.Encode(), nil)
	if err != nil {
		return t.errTG(method, err)
	}
	resp, err := t.http.Do(req)
	if err != nil {
		return t.errTG(method, err)
	}
	defer resp.Body.Close()
	var env struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
		ErrorCode   int             `json:"error_code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return t.errTG(method, err)
	}
	if !env.OK {
		return fmt.Errorf("telegram %s: %d %s", method, env.ErrorCode, env.Description)
	}
	return json.Unmarshal(env.Result, out)
}

type tgPhotoSize struct {
	FileID string `json:"file_id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// pickAvatarSize takes the smallest size at least 320px wide (Telegram
// sends 160/320/640), else the largest one.
func pickAvatarSize(sizes []tgPhotoSize) *tgPhotoSize {
	if len(sizes) == 0 {
		return nil
	}
	var best *tgPhotoSize
	for i := range sizes {
		s := &sizes[i]
		if s.Width >= 320 && (best == nil || best.Width < 320 || s.Width < best.Width) {
			best = s
		}
	}
	if best != nil && best.Width >= 320 {
		return best
	}
	best = &sizes[0]
	for i := range sizes {
		if sizes[i].Width > best.Width {
			best = &sizes[i]
		}
	}
	return best
}

// fetchPhoto downloads the user's current Telegram profile photo; nil data
// with no error means the user has none (or hides it).
func (t *TelegramAvatarImporter) fetchPhoto(ctx context.Context, telegramID int64) ([]byte, string, error) {
	var photos struct {
		TotalCount int             `json:"total_count"`
		Photos     [][]tgPhotoSize `json:"photos"`
	}
	if err := t.call(ctx, "getUserProfilePhotos", url.Values{"user_id": {fmt.Sprint(telegramID)}, "limit": {"1"}}, &photos); err != nil {
		return nil, "", err
	}
	if len(photos.Photos) == 0 {
		return nil, "", nil
	}
	size := pickAvatarSize(photos.Photos[0])
	if size == nil {
		return nil, "", nil
	}
	var file struct {
		FilePath string `json:"file_path"`
	}
	if err := t.call(ctx, "getFile", url.Values{"file_id": {size.FileID}}, &file); err != nil {
		return nil, "", err
	}
	if file.FilePath == "" {
		return nil, "", fmt.Errorf("telegram getFile: empty file_path")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.apiBase+"/file/bot"+t.botToken+"/"+file.FilePath, nil)
	if err != nil {
		return nil, "", t.errTG("download", err)
	}
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, "", t.errTG("download", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("telegram download: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, tgAvatarMaxSize+1))
	if err != nil {
		return nil, "", t.errTG("download", err)
	}
	if len(data) > tgAvatarMaxSize {
		return nil, "", fmt.Errorf("telegram photo too large")
	}
	ct := http.DetectContentType(data)
	if !allowedImageTypes[ct] {
		return nil, "", fmt.Errorf("telegram photo: unexpected type %s", ct)
	}
	return data, ct, nil
}

// store saves the image like a normal profile upload (webp, local in DEV,
// B2 in PROD) and returns its public URL.
func (t *TelegramAvatarImporter) store(userID primitive.ObjectID, data []byte, contentType string) (string, error) {
	data, contentType, err := maybeConvertImageToWebP(data, contentType)
	if err != nil {
		return "", err
	}
	key := buildProfileImageObjectKey(userID.Hex(), "telegram.jpg", contentType)
	if t.upload.config.IsDev {
		return t.upload.saveLocal(bytes.NewReader(data), filepath.Base(key))
	}
	if _, err := t.upload.uploadBytesToB2(key, data, contentType); err != nil {
		return "", err
	}
	return buildStoredMediaURL(t.upload.config, key, key, ""), nil
}

// Import fetches and stores one user's Telegram photo. With overwrite it
// replaces an existing picture (the user asked for it); otherwise it only
// fills an empty one. Returns the status and the new URL (if any).
func (t *TelegramAvatarImporter) Import(ctx context.Context, userID primitive.ObjectID, telegramID int64, overwrite bool) (string, string, error) {
	mark := func(status string) {
		_, _ = t.users.UpdateByID(ctx, userID, bson.M{"$set": bson.M{"tg_photo_status": status, "tg_photo_checked_at": time.Now()}})
	}
	data, ct, err := t.fetchPhoto(ctx, telegramID)
	if err != nil {
		mark(tgAvatarError)
		return tgAvatarError, "", err
	}
	if data == nil {
		mark(tgAvatarNone)
		return tgAvatarNone, "", nil
	}
	u, err := t.store(userID, data, ct)
	if err != nil {
		mark(tgAvatarError)
		return tgAvatarError, "", err
	}
	filter := bson.M{"_id": userID}
	if !overwrite {
		// Don't clobber a picture the user uploaded meanwhile.
		filter = bson.M{"$and": []bson.M{{"_id": userID}, noCustomPicture}}
	}
	if _, err := t.users.UpdateOne(ctx, filter, bson.M{"$set": bson.M{
		"profile_image_url":    u,
		"photo_url":            u,
		"profile_image_source": "telegram",
		"tg_photo_status":      tgAvatarOK,
		"tg_photo_checked_at":  time.Now(),
		"updated_at":           time.Now(),
	}}); err != nil {
		return tgAvatarError, "", err
	}
	return tgAvatarOK, u, nil
}

// ImportAsync is called after login for users without a picture.
func (t *TelegramAvatarImporter) ImportAsync(userID primitive.ObjectID, telegramID int64) {
	if t == nil || telegramID == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		status, _, err := t.Import(ctx, userID, telegramID, false)
		if err != nil {
			log.Printf("[TG_AVATAR] user=%s: %v", userID.Hex(), err)
		} else {
			log.Printf("[TG_AVATAR] user=%s status=%s", userID.Hex(), status)
		}
	}()
}

// pendingFilter: users with a Telegram account and no picture, not checked
// yet, or checked more than 30 days ago (they may have added a photo), or
// a transient error more than an hour ago.
func tgAvatarPendingFilter(now time.Time) bson.M {
	return bson.M{"$and": []bson.M{
		{"telegram_id": bson.M{"$gt": 0}},
		noCustomPicture,
		{"$or": []bson.M{
			{"tg_photo_checked_at": bson.M{"$exists": false}},
			{"tg_photo_status": tgAvatarNone, "tg_photo_checked_at": bson.M{"$lt": now.Add(-30 * 24 * time.Hour)}},
			{"tg_photo_status": tgAvatarError, "tg_photo_checked_at": bson.M{"$lt": now.Add(-time.Hour)}},
		}},
	}}
}

// Backfill imports photos for up to `limit` pending users.
func (t *TelegramAvatarImporter) Backfill(ctx context.Context, limit int) (imported, checked int, err error) {
	if !t.running.TryLock() {
		return 0, 0, nil
	}
	defer t.running.Unlock()
	cur, err := t.users.Find(ctx, tgAvatarPendingFilter(time.Now()),
		options.Find().SetProjection(bson.M{"_id": 1, "telegram_id": 1}).SetSort(bson.D{{Key: "last_login_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return 0, 0, err
	}
	var rows []struct {
		ID         primitive.ObjectID `bson:"_id"`
		TelegramID int64              `bson:"telegram_id"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return 0, 0, err
	}
	for _, r := range rows {
		if ctx.Err() != nil {
			return imported, checked, ctx.Err()
		}
		one, cancel := context.WithTimeout(ctx, 45*time.Second)
		status, _, err := t.Import(one, r.ID, r.TelegramID, false)
		cancel()
		checked++
		if err != nil {
			log.Printf("[TG_AVATAR] backfill user=%s: %v", r.ID.Hex(), err)
		} else if status == tgAvatarOK {
			imported++
		}
		time.Sleep(150 * time.Millisecond) // well under Telegram's ~30 req/s
	}
	return imported, checked, nil
}

// PendingCount is how many users still wait for a check.
func (t *TelegramAvatarImporter) PendingCount(ctx context.Context) int64 {
	n, _ := t.users.CountDocuments(ctx, tgAvatarPendingFilter(time.Now()))
	return n
}

// Start runs the backfill in the background: batches of 300 every 10
// minutes (first one two minutes after boot) until nobody is left.
func (t *TelegramAvatarImporter) Start(ctx context.Context) {
	if t == nil {
		log.Printf("[TG_AVATAR] disabled — TELEGRAM_BOT_TOKEN not set")
		return
	}
	go func() {
		timer := time.NewTimer(2 * time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			imported, checked, err := t.Backfill(ctx, 300)
			if err != nil {
				log.Printf("[TG_AVATAR] backfill: %v", err)
			} else if checked > 0 {
				log.Printf("[TG_AVATAR] backfill: %d checked, %d photos imported", checked, imported)
			}
			timer.Reset(10 * time.Minute)
		}
	}()
}

// ── HTTP ──

// UseTelegramPhoto POST /api/user/avatar/telegram — the user puts their
// current Telegram photo back as their profile picture.
func (t *TelegramAvatarImporter) UseTelegramPhoto(c *gin.Context) {
	if t == nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Telegram bot sozlanmagan"})
		return
	}
	uid, err := primitive.ObjectIDFromHex(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var u struct {
		TelegramID int64 `bson:"telegram_id"`
	}
	if err := t.users.FindOne(c.Request.Context(), bson.M{"_id": uid}, options.FindOne().SetProjection(bson.M{"telegram_id": 1})).Decode(&u); err != nil || u.TelegramID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Telegram hisobi topilmadi"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	status, imgURL, err := t.Import(ctx, uid, u.TelegramID, true)
	switch {
	case err != nil:
		log.Printf("[TG_AVATAR] manual user=%s: %v", uid.Hex(), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Telegram rasmini olib bo'lmadi"})
	case status == tgAvatarNone:
		c.JSON(http.StatusNotFound, gin.H{"error": "Telegram profilingizda rasm yo'q yoki u yashirin"})
	default:
		c.JSON(http.StatusOK, gin.H{"profile_image_url": imgURL})
	}
}

// BackfillNow POST /api/superadmin/users/telegram-avatars/backfill
func (t *TelegramAvatarImporter) BackfillNow(c *gin.Context) {
	if t == nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "TELEGRAM_BOT_TOKEN sozlanmagan"})
		return
	}
	pending := t.PendingCount(c.Request.Context())
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		for {
			imported, checked, err := t.Backfill(ctx, 300)
			log.Printf("[TG_AVATAR] manual backfill: %d checked, %d imported, err=%v", checked, imported, err)
			if err != nil || checked == 0 {
				return
			}
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"started": true, "pending": pending})
}

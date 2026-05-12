package repositories

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/filmorauz/backend/models"
)

type UserRepository struct {
	col *mongo.Collection
	db  *mongo.Database
}

func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{
		col: db.Collection("users"),
		db:  db,
	}
}

// EnsureIndexes creates necessary indexes for the users collection
func (r *UserRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "telegram_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "role", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

func (r *UserRepository) FindByTelegramID(telegramID int64) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := r.col.FindOne(ctx, bson.M{"telegram_id": telegramID}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByID(id string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var user models.User
	err = r.col.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByIDs(ids []primitive.ObjectID) ([]models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := r.col.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// UserStats represents public user statistics
type UserStats struct {
	CommentsCount  int64 `json:"comments_count"`
	RatingsCount   int64 `json:"ratings_count"`
	FavoritesCount int64 `json:"favorites_count"`
	WatchedCount   int64 `json:"watched_count"`
}

// PublicUserProfile represents public user info (no sensitive data)
type PublicUserProfile struct {
	ID               string               `json:"id"`
	DisplayName      string               `json:"display_name"`
	Username         string               `json:"username,omitempty"`
	AvatarURL        string               `json:"avatar_url,omitempty"`
	Role             string               `json:"role"`
	IsPremium        bool                 `json:"is_premium"`
	IsPremiumActive  bool                 `json:"is_premium_active"`
	PremiumExpiresAt *time.Time           `json:"premium_expires_at,omitempty"`
	ProfileStyle     *models.ProfileStyle `json:"profile_style,omitempty"`
	IsPrivate        bool                 `json:"is_private"`
	Ban              *models.BanInfo      `json:"ban,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	LastLoginAt      *time.Time           `json:"last_login_at,omitempty"`
	Stats            *UserStats           `json:"stats,omitempty"`
}

// GetPublicProfile returns public user profile (without sensitive data)
func (r *UserRepository) GetPublicProfile(userID primitive.ObjectID) (*PublicUserProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := r.col.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	// Get user stats
	stats, err := r.GetUserStatsByID(userID)
	if err != nil {
		// Don't fail the whole request if stats fail
		stats = nil
	}

	// Check privacy setting
	isPrivate := false
	if user.Privacy != nil {
		isPrivate = user.Privacy.IsPrivate
	}

	// Return only public fields
	return &PublicUserProfile{
		ID:               user.ID.Hex(),
		DisplayName:      getUserDisplayName(&user),
		Username:         user.TelegramUser,
		AvatarURL:        user.ProfileImageURL,
		Role:             user.Role,
		IsPremium:        user.IsPremium,
		IsPremiumActive:  user.IsPremiumActive(),
		PremiumExpiresAt: user.PremiumExpiresAt,
		ProfileStyle:     user.ProfileStyle,
		IsPrivate:        isPrivate,
		Ban:              user.Ban,
		CreatedAt:        user.CreatedAt,
		LastLoginAt:      &user.LastLoginAt,
		Stats:            stats,
	}, nil
}

func (r *UserRepository) Create(telegramID int64, firstName, lastName, username, photoURL, languageCode string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	user := models.User{
		TelegramID:   telegramID,
		FirstName:    firstName,
		LastName:     lastName,
		TelegramUser: username,
		PhotoURL:     photoURL,
		LanguageCode: languageCode,
		Role:         "user",
		AuthProvider: "telegram",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	result, err := r.col.InsertOne(ctx, user)
	if err != nil {
		log.Printf("[REPO] Create user error: %v", err)
		return nil, err
	}

	user.ID = result.InsertedID.(primitive.ObjectID)
	return &user, nil
}

// FindChatIDs returns up to limit valid Telegram chat_ids from non-banned users,
// ordered by most recently updated. Used for bot ad delivery.
func (r *UserRepository) FindChatIDs(limit int) ([]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"telegram_chat_id": bson.M{"$exists": true, "$ne": 0},
		"$or": []bson.M{
			{"ban": nil},
			{"ban.is_banned": bson.M{"$ne": true}},
		},
	}
	limitInt64 := int64(limit)
	cursor, err := r.col.Find(ctx, filter, &options.FindOptions{
		Sort:       bson.D{{Key: "updated_at", Value: -1}},
		Limit:      &limitInt64,
		Projection: bson.D{{Key: "telegram_chat_id", Value: 1}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []struct {
		TelegramChatID int64 `bson:"telegram_chat_id"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	chatIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.TelegramChatID != 0 {
			chatIDs = append(chatIDs, row.TelegramChatID)
		}
	}
	return chatIDs, nil
}

// validName returns true if the name is non-empty and not a placeholder like "." or "-"
func validName(s string) bool {
	v := strings.TrimSpace(s)
	return v != "" && v != "." && v != "-"
}

// UpsertTelegramUser saves or updates a user by telegram_id.
// Called when a user sends /start to the bot. Persists chat_id for later bot messaging.
func (r *UserRepository) UpsertTelegramUser(telegramID, chatID int64, username, firstName, lastName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	setFields := bson.M{
		"telegram_chat_id": chatID,
		"telegram_user":    username,
		"updated_at":       now,
	}
	// Only overwrite names if they carry real data
	if validName(firstName) {
		setFields["first_name"] = strings.TrimSpace(firstName)
	}
	if validName(lastName) {
		setFields["last_name"] = strings.TrimSpace(lastName)
	}

	result, err := r.col.UpdateOne(ctx,
		bson.M{"telegram_id": telegramID},
		bson.M{
			"$set": setFields,
			"$setOnInsert": bson.M{
				"telegram_id":   telegramID,
				"role":          "user",
				"auth_provider": "telegram",
				"is_active":     true,
				"created_at":    now,
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return err
	}
	if result.UpsertedCount > 0 {
		log.Printf("[USER] created bot user telegram_id=%d chat_id=%d", telegramID, chatID)
	} else {
		log.Printf("[USER] updated bot user telegram_id=%d chat_id=%d", telegramID, chatID)
	}
	return nil
}

// UpdateTelegramInfo refreshes Telegram profile fields on login.
// Only overwrites a field if the new value is valid (non-empty, not ".").
func (r *UserRepository) UpdateTelegramInfo(telegramID int64, username, firstName, lastName, photoURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setFields := bson.M{"updated_at": time.Now()}
	if username != "" {
		setFields["telegram_user"] = username
	}
	if validName(firstName) {
		setFields["first_name"] = strings.TrimSpace(firstName)
	}
	if validName(lastName) {
		setFields["last_name"] = strings.TrimSpace(lastName)
	}
	if photoURL != "" {
		setFields["photo_url"] = photoURL
	}

	_, err := r.col.UpdateOne(ctx,
		bson.M{"telegram_id": telegramID},
		bson.M{"$set": setFields},
	)
	return err
}

func (r *UserRepository) UpdateLastLogin(userID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"last_login_at": time.Now()}},
	)
	return err
}

func (r *UserRepository) UpdateSession(userID primitive.ObjectID, sessionToken string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Also set expiry 30 days from now
	expiry := time.Now().Add(30 * 24 * time.Hour)

	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"session_token":      sessionToken,
				"session_expires_at": expiry,
			},
		},
	)
	return err
}

func (r *UserRepository) FindBySessionToken(sessionToken string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := r.col.FindOne(ctx, bson.M{
		"session_token":      sessionToken,
		"session_expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) ClearSession(userID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$unset": bson.M{
				"session_token":      "",
				"session_expires_at": "",
			},
		},
	)
	return err
}

func (r *UserRepository) FindAll() ([]models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := r.col.Find(ctx, bson.M{}, &options.FindOptions{
		Sort: bson.D{{Key: "created_at", Value: -1}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (r *UserRepository) CountAll() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return r.col.CountDocuments(ctx, bson.M{})
}

func (r *UserRepository) CountThisMonth() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	startOfMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)

	return r.col.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": startOfMonth},
	})
}

// CountTotal returns total number of users
func (r *UserRepository) CountTotal() (int64, error) {
	return r.CountAll()
}

// CountPremium returns number of premium users (active premium)
func (r *UserRepository) CountPremium() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	count, err := r.col.CountDocuments(ctx, bson.M{
		"is_premium":         true,
		"premium_expires_at": bson.M{"$gt": now},
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountRegisteredToday returns count of users registered today
func (r *UserRepository) CountRegisteredToday() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	startOfDay := time.Now().Truncate(24 * time.Hour)

	return r.col.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": startOfDay},
	})
}

// CountRegisteredThisMonth returns count of users registered this month
func (r *UserRepository) CountRegisteredThisMonth() (int64, error) {
	return r.CountThisMonth()
}

// FindRecentUsers returns most recently created users
func (r *UserRepository) FindRecentUsers(limit int) ([]models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	limitInt64 := int64(limit)
	cursor, err := r.col.Find(ctx, bson.M{}, &options.FindOptions{
		Sort:  bson.D{{Key: "created_at", Value: -1}},
		Limit: &limitInt64,
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}

	return users, nil
}

// FindUsers returns users with pagination
func (r *UserRepository) FindUsers(page, limit int, search, role string) ([]models.User, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	skip := (page - 1) * limit

	// Build filter
	filter := bson.M{}
	if search != "" {
		// Normalize search: remove leading @ if present
		normalizedSearch := strings.TrimPrefix(search, "@")

		// Build OR conditions for multiple fields
		orConditions := []bson.M{
			{"first_name": bson.M{"$regex": normalizedSearch, "$options": "i"}},
			{"display_name": bson.M{"$regex": normalizedSearch, "$options": "i"}},
			{"telegram_user": bson.M{"$regex": normalizedSearch, "$options": "i"}},
		}

		// Try to parse as telegram_id (numeric)
		if telegramID, err := strconv.ParseInt(normalizedSearch, 10, 64); err == nil {
			orConditions = append(orConditions, bson.M{"telegram_id": telegramID})
		}

		// Try to parse as MongoDB ObjectID
		if objID, err := primitive.ObjectIDFromHex(normalizedSearch); err == nil {
			orConditions = append(orConditions, bson.M{"_id": objID})
		}

		filter["$or"] = orConditions
	}
	if role != "" {
		filter["role"] = role
	}

	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	limitInt64 := int64(limit)
	skipInt64 := int64(skip)
	cursor, err := r.col.Find(ctx, filter, &options.FindOptions{
		Sort:  bson.D{{Key: "created_at", Value: -1}},
		Skip:  &skipInt64,
		Limit: &limitInt64,
	})
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// FindBannedUsers returns users with ban records
func (r *UserRepository) FindBannedUsers(search, status string) ([]models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build filter for users with ban data
	filter := bson.M{
		"ban.is_banned": true,
	}

	// Apply status filter
	if status == "active" {
		// Active = has ban, and (permanent OR not expired)
		filter["$or"] = []bson.M{
			{"ban.banned_until": nil},
			{"ban.banned_until": bson.M{"$gt": time.Now()}},
		}
	} else if status == "expired" {
		// Expired = has ban that has passed
		filter["ban.banned_until"] = bson.M{"$lte": time.Now()}
		filter["ban.banned_until"] = bson.M{"$ne": nil}
	} else if status == "permanent" {
		// Permanent = has ban with no expiry
		filter["ban.banned_until"] = nil
	}

	// Build search filter
	if search != "" {
		normalizedSearch := strings.TrimPrefix(search, "@")
		orConditions := []bson.M{
			{"first_name": bson.M{"$regex": normalizedSearch, "$options": "i"}},
			{"display_name": bson.M{"$regex": normalizedSearch, "$options": "i"}},
			{"telegram_user": bson.M{"$regex": normalizedSearch, "$options": "i"}},
			{"ban.reason": bson.M{"$regex": normalizedSearch, "$options": "i"}},
			{"ban.banned_by_username": bson.M{"$regex": normalizedSearch, "$options": "i"}},
		}

		if telegramID, err := strconv.ParseInt(normalizedSearch, 10, 64); err == nil {
			orConditions = append(orConditions, bson.M{"telegram_id": telegramID})
		}

		if objID, err := primitive.ObjectIDFromHex(normalizedSearch); err == nil {
			orConditions = append(orConditions, bson.M{"_id": objID})
		}

		filter["$or"] = orConditions
	}

	// Use aggregate to sort properly (active first, then by banned_at desc)
	pipeline := []bson.M{
		{"$match": filter},
		{"$sort": bson.D{
			{Key: "ban.banned_until", Value: 1}, // nulls first (permanent), then soonest expiry
			{Key: "ban.banned_at", Value: -1},   // newest first
		}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}

	return users, nil
}

func (r *UserRepository) FindByHex(userHex string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userHex)
	if err != nil {
		return nil, err
	}

	var user models.User
	err = r.col.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) UpdateUserRoleByHex(userHex string, role string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userHex)
	if err != nil {
		return err
	}

	result, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"role":       role,
				"updated_at": time.Now(),
			},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// UpdateUserPremium updates a user's premium status
func (r *UserRepository) UpdateUserPremium(userHex string, isPremium bool, premiumExpiresAt *time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userHex)
	if err != nil {
		return err
	}

	update := bson.M{
		"$set": bson.M{
			"is_premium":         isPremium,
			"premium_expires_at": premiumExpiresAt,
			"updated_at":         time.Now(),
		},
	}

	// If setting premium to true and no expiry set, also set started_at
	if isPremium && premiumExpiresAt != nil {
		update["$set"].(bson.M)["premium_started_at"] = time.Now()
	}

	result, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		update,
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// UpdateUserBan updates a user's ban status
func (r *UserRepository) UpdateUserBan(userHex string, banInfo *models.BanInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userHex)
	if err != nil {
		return err
	}

	result, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"ban":        banInfo,
				"updated_at": time.Now(),
			},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// RemoveUserBan removes a user's ban
func (r *UserRepository) RemoveUserBan(userHex string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userHex)
	if err != nil {
		return err
	}

	result, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"ban":        nil,
				"updated_at": time.Now(),
			},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// CleanupExpiredPremium finds and deactivates expired premium subscriptions.
// Returns the number of users updated.
func (r *UserRepository) CleanupExpiredPremium() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()

	// Find users where isPremium=true AND premiumExpiresAt is not null AND premiumExpiresAt <= now
	filter := bson.M{
		"is_premium":         true,
		"premium_expires_at": bson.M{"$ne": nil, "$lte": now},
	}

	update := bson.M{
		"$set": bson.M{
			"is_premium":         false,
			"premium_expires_at": nil,
			"updated_at":         now,
		},
	}

	result, err := r.col.UpdateMany(ctx, filter, update)
	if err != nil {
		return 0, err
	}

	return result.ModifiedCount, nil
}

// GetUsersWithExpiringPremium returns users whose premium is about to expire within the specified days
func (r *UserRepository) GetUsersWithExpiringPremium(days int) ([]models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	expiryThreshold := now.Add(time.Duration(days) * 24 * time.Hour)

	// Find users where isPremium=true AND premiumExpiresAt is between now and threshold
	filter := bson.M{
		"is_premium": true,
		"premium_expires_at": bson.M{
			"$ne":  nil,
			"$gt":  now,
			"$lte": expiryThreshold,
		},
	}

	cursor, err := r.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}

	if users == nil {
		users = []models.User{}
	}

	return users, nil
}

// GetUsersWithExpiredPremium returns users whose premium just expired (within the last hour)
func (r *UserRepository) GetUsersWithExpiredPremium() ([]models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get users whose premium expired within the last hour
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	now := time.Now()

	// Find users where isPremium=false AND had premium that expired recently
	// We need to track this differently - for now, let's find users whose premium_expires_at
	// is between oneHourAgo and now, but is_premium is still true (before cleanup runs)
	// This won't work directly since cleanup sets is_premium to false

	// Alternative: We track users that need expiry notification separately
	// For simplicity, let's just query users who have is_premium=false but had a recent expiry
	filter := bson.M{
		"is_premium": false,
		"premium_expires_at": bson.M{
			"$ne":  nil,
			"$gt":  oneHourAgo,
			"$lte": now,
		},
	}

	cursor, err := r.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}

	if users == nil {
		users = []models.User{}
	}

	return users, nil
}

// UpdateProfileImage updates the user's first name and/or profile image URL
func (r *UserRepository) UpdateProfileImage(userIDHex string, firstName, profileImageURL string) error {
	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	if firstName != "" {
		update["$set"].(bson.M)["first_name"] = firstName
	}
	if profileImageURL != "" {
		update["$set"].(bson.M)["profile_image_url"] = profileImageURL
	}

	_, err = r.col.UpdateOne(ctx, bson.M{"_id": userID}, update)
	return err
}

// UpdateLanguageCode updates the user's preferred language code
func (r *UserRepository) UpdateLanguageCode(userIDHex string, languageCode string) error {
	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"language_code": languageCode,
			"updated_at":    time.Now(),
		},
	}

	_, err = r.col.UpdateOne(ctx, bson.M{"_id": userID}, update)
	return err
}

// GetUserStats returns user statistics (for admin dashboard)
func (r *UserRepository) GetUserStats() (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, err := r.col.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to count total users: %w", err)
	}

	startOfMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	thisMonth, err := r.col.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": startOfMonth},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to count this month users: %w", err)
	}

	return map[string]int64{
		"total":     total,
		"thisMonth": thisMonth,
	}, nil
}

// GetUserStatsByID returns stats for a specific user
func (r *UserRepository) GetUserStatsByID(userID primitive.ObjectID) (*UserStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get comments count
	commentsCount, err := r.db.Collection("movie_comments").CountDocuments(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to count comments: %w", err)
	}

	// Get ratings count
	ratingsCount, err := r.db.Collection("movie_ratings").CountDocuments(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to count ratings: %w", err)
	}

	// Get favorites count
	favoritesCount, err := r.db.Collection("favorites").CountDocuments(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to count favorites: %w", err)
	}

	// Get watched count (from watch history)
	watchedCount, err := r.db.Collection("watch_history").CountDocuments(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to count watched: %w", err)
	}

	return &UserStats{
		CommentsCount:  commentsCount,
		RatingsCount:   ratingsCount,
		FavoritesCount: favoritesCount,
		WatchedCount:   watchedCount,
	}, nil
}

// UpdateProfileStyle updates the user's profile style (premium only)
func (r *UserRepository) UpdateProfileStyle(userIDHex string, profileStyle *models.ProfileStyle) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		return err
	}

	// Validate profile style values
	if profileStyle != nil {
		// Validate frame
		if !containsString(profileStyle.Frame, models.ValidProfileStyles["frame"]) {
			return fmt.Errorf("invalid frame value: %s", profileStyle.Frame)
		}
		// Validate theme
		if !containsString(profileStyle.Theme, models.ValidProfileStyles["theme"]) {
			return fmt.Errorf("invalid theme value: %s", profileStyle.Theme)
		}
		// Validate gradient
		if !containsString(profileStyle.Gradient, models.ValidProfileStyles["gradient"]) {
			return fmt.Errorf("invalid gradient value: %s", profileStyle.Gradient)
		}
	}

	update := bson.M{
		"$set": bson.M{
			"profile_style": profileStyle,
			"updated_at":    time.Now(),
		},
	}

	result, err := r.col.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// UpdatePrivacy updates the user's privacy settings (premium only)
func (r *UserRepository) UpdatePrivacy(userIDHex string, privacy *models.PrivacySettings) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		return err
	}

	update := bson.M{
		"$set": bson.M{
			"privacy":    privacy,
			"updated_at": time.Now(),
		},
	}

	result, err := r.col.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// GetUserPremiumStatus returns whether a user is premium and active
func (r *UserRepository) GetUserPremiumStatus(userID primitive.ObjectID) (isPremium bool, isActive bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err = r.col.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return false, false, err
	}
	return user.IsPremium, user.IsPremiumActive(), nil
}

// TopUpWalletBalance adds amount to a user's wallet balance and returns the new balance.
func (r *UserRepository) TopUpWalletBalance(userHex string, amount float64) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userHex)
	if err != nil {
		return 0, err
	}

	var updated models.User
	err = r.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$inc": bson.M{"wallet_balance": amount},
			"$set": bson.M{"updated_at": time.Now()},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updated)
	if err == mongo.ErrNoDocuments {
		return 0, fmt.Errorf("user not found")
	}
	if err != nil {
		return 0, err
	}
	return updated.WalletBalance, nil
}

// DeductWalletAndActivatePremium atomically checks that the user's wallet balance
// is >= price, deducts price, and activates premium for durationDays.
// Returns "insufficient_balance" error if balance is too low.
func (r *UserRepository) DeductWalletAndActivatePremium(userHex string, price float64, durationDays int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(userHex)
	if err != nil {
		return err
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(durationDays) * 24 * time.Hour)

	// Atomic: only match if wallet_balance >= price
	result, err := r.col.UpdateOne(ctx,
		bson.M{
			"_id":            userID,
			"wallet_balance": bson.M{"$gte": price},
		},
		bson.M{
			"$inc": bson.M{"wallet_balance": -price},
			"$set": bson.M{
				"is_premium":         true,
				"premium_started_at": now,
				"premium_expires_at": expiresAt,
				"updated_at":         now,
			},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		// Distinguish "no such user" from "insufficient balance"
		user, findErr := r.FindByHex(userHex)
		if findErr != nil || user == nil {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("insufficient_balance")
	}
	return nil
}

// ActivateOrExtendPremium activates premium for the user, extending from the
// current premium_expires_at when an active subscription exists, or starting
// from now otherwise. Used by Telegram Stars purchases (no wallet deduction).
func (r *UserRepository) ActivateOrExtendPremium(userID primitive.ObjectID, durationDays int) (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := r.col.FindOne(ctx, bson.M{"_id": userID}).Decode(&user); err != nil {
		return time.Time{}, err
	}

	now := time.Now()
	base := now
	if user.IsPremium && user.PremiumExpiresAt != nil && user.PremiumExpiresAt.After(now) {
		base = *user.PremiumExpiresAt
	}
	expiresAt := base.Add(time.Duration(durationDays) * 24 * time.Hour)

	set := bson.M{
		"is_premium":         true,
		"premium_expires_at": expiresAt,
		"updated_at":         now,
	}
	if !user.IsPremium || user.PremiumStartedAt == nil {
		set["premium_started_at"] = now
	}
	if _, err := r.col.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$set": set}); err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}

// Helper function to check if a string is in a slice
func containsString(s string, list []string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

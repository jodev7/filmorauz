package repositories

import (
	"context"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CommentRepository struct {
	col         *mongo.Collection
	settingsCol *mongo.Collection
	userCol     *mongo.Collection
	movieCol    *mongo.Collection
	episodeCol  *mongo.Collection
}

func NewCommentRepository(db *mongo.Database) *CommentRepository {
	return &CommentRepository{
		col:         db.Collection("movie_comments"),
		settingsCol: db.Collection("comment_moderation_settings"),
		userCol:     db.Collection("users"),
		movieCol:    db.Collection("movies"),
		episodeCol:  db.Collection("episodes"),
	}
}

// EnsureIndexes creates required indexes
func (r *CommentRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		// Primary index for movie_id (backward compatibility)
		{
			Keys: bson.D{{Key: "movie_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
		// New index for target_type + target_id
		{
			Keys: bson.D{{Key: "target_type", Value: 1}, {Key: "target_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "parent_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create creates a new comment
func (r *CommentRepository) Create(comment *models.MovieComment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	comment.CreatedAt = time.Now()
	comment.UpdatedAt = time.Now()

	result, err := r.col.InsertOne(ctx, comment)
	if err != nil {
		return err
	}

	comment.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// Update updates an existing comment
func (r *CommentRepository) Update(comment *models.MovieComment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	comment.UpdatedAt = time.Now()

	filter := bson.M{"_id": comment.ID}
	update := bson.M{
		"$set": bson.M{
			"content":          comment.Content,
			"status":           comment.Status,
			"has_blocked_word": comment.HasBlockedWord,
			"has_link":         comment.HasLink,
			"updated_at":       comment.UpdatedAt,
		},
	}

	_, err := r.col.UpdateOne(ctx, filter, update)
	return err
}

// Delete deletes a comment by ID
func (r *CommentRepository) Delete(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": id}
	_, err := r.col.DeleteOne(ctx, filter)
	return err
}

// GetByID returns a comment by ID
func (r *CommentRepository) GetByID(id primitive.ObjectID) (*models.MovieComment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var comment models.MovieComment
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&comment)
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// GetByMovieID returns approved comments for a movie
func (r *CommentRepository) GetByMovieID(movieID primitive.ObjectID, limit, skip int) ([]models.CommentWithUser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get top-level approved comments - include comments with empty/missing status (backward compatibility)
	filter := bson.M{
		"movie_id": movieID,
		"$or": []bson.M{
			{"status": models.CommentStatusApproved},
			{"status": ""},
			{"status": bson.M{"$exists": false}},
		},
		"parent_id": nil,
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var comments []models.MovieComment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, err
	}

	// Enrich with user info
	result := make([]models.CommentWithUser, 0, len(comments))
	for _, c := range comments {
		user, _ := r.getUserByID(c.UserID)
		repliesCount, _ := r.CountReplies(c.ID)

		result = append(result, models.CommentWithUser{
			ID:                  c.ID,
			MovieID:             c.MovieID,
			UserID:              c.UserID,
			ParentID:            c.ParentID,
			Content:             c.Content,
			Status:              c.Status,
			HasBlockedWord:      c.HasBlockedWord,
			HasLink:             c.HasLink,
			CreatedAt:           c.CreatedAt,
			UpdatedAt:           c.UpdatedAt,
			IsPremiumUser:       c.IsPremiumUser,
			UserDisplayName:     getUserDisplayName(user),
			UserAvatarURL:       getUserAvatarURL(user),
			UserRole:            getUserRole(user),
			UserIsPremium:       getUserIsPremium(user),
			UserIsPremiumActive: getUserIsPremiumActive(user),
			RepliesCount:        repliesCount,
		})
	}

	return result, nil
}

// GetReplies returns approved replies for a parent comment
func (r *CommentRepository) GetReplies(parentID primitive.ObjectID) ([]models.CommentWithUser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Include comments with empty/missing status (backward compatibility)
	filter := bson.M{
		"parent_id": parentID,
		"$or": []bson.M{
			{"status": models.CommentStatusApproved},
			{"status": ""},
			{"status": bson.M{"$exists": false}},
		},
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var comments []models.MovieComment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, err
	}

	result := make([]models.CommentWithUser, 0, len(comments))
	for _, c := range comments {
		user, _ := r.getUserByID(c.UserID)
		result = append(result, models.CommentWithUser{
			ID:                  c.ID,
			MovieID:             c.MovieID,
			UserID:              c.UserID,
			ParentID:            c.ParentID,
			Content:             c.Content,
			Status:              c.Status,
			HasBlockedWord:      c.HasBlockedWord,
			HasLink:             c.HasLink,
			CreatedAt:           c.CreatedAt,
			UpdatedAt:           c.UpdatedAt,
			IsPremiumUser:       c.IsPremiumUser,
			UserDisplayName:     getUserDisplayName(user),
			UserAvatarURL:       getUserAvatarURL(user),
			UserRole:            getUserRole(user),
			UserIsPremium:       getUserIsPremium(user),
			UserIsPremiumActive: getUserIsPremiumActive(user),
			LikesCount:          c.LikesCount,
			LikedBy:             c.LikedBy,
		})
	}

	return result, nil
}

// CountReplies counts replies for a parent comment
func (r *CommentRepository) CountReplies(parentID primitive.ObjectID) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"parent_id": parentID,
		"status":    models.CommentStatusApproved,
	}

	count, err := r.col.CountDocuments(ctx, filter)
	return int(count), err
}

// commentLikedByViewer reports whether viewerID (if non-nil and non-zero) liked the comment.
func commentLikedByViewer(c *models.MovieComment, viewerID *primitive.ObjectID) bool {
	if viewerID == nil || viewerID.IsZero() {
		return false
	}
	for _, id := range c.LikedBy {
		if id == *viewerID {
			return true
		}
	}
	return false
}

// GetAllApprovedByMovieID returns all approved comments for a movie (no parent filter)
func (r *CommentRepository) GetAllApprovedByMovieID(movieID primitive.ObjectID, limit, skip int) ([]models.CommentWithUser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all approved comments for the movie (including nested replies)
	// Include comments with empty/missing status (backward compatibility)
	filter := bson.M{
		"movie_id": movieID,
		"$or": []bson.M{
			{"status": models.CommentStatusApproved},
			{"status": ""},
			{"status": bson.M{"$exists": false}},
		},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var comments []models.MovieComment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, err
	}

	// Enrich with user info
	result := make([]models.CommentWithUser, 0, len(comments))
	for _, c := range comments {
		user, _ := r.getUserByID(c.UserID)
		repliesCount, _ := r.CountReplies(c.ID)

		likesCount := c.LikesCount
		if likesCount == 0 && len(c.LikedBy) > 0 {
			likesCount = len(c.LikedBy)
		}

		result = append(result, models.CommentWithUser{
			ID:                  c.ID,
			MovieID:             c.MovieID,
			UserID:              c.UserID,
			ParentID:            c.ParentID,
			Content:             c.Content,
			Status:              c.Status,
			HasBlockedWord:      c.HasBlockedWord,
			HasLink:             c.HasLink,
			CreatedAt:           c.CreatedAt,
			UpdatedAt:           c.UpdatedAt,
			IsPremiumUser:       c.IsPremiumUser,
			UserDisplayName:     getUserDisplayName(user),
			UserAvatarURL:       getUserAvatarURL(user),
			UserRole:            getUserRole(user),
			UserIsPremium:       getUserIsPremium(user),
			UserIsPremiumActive: getUserIsPremiumActive(user),
			RepliesCount:        repliesCount,
			LikesCount:          likesCount,
			LikedBy:             c.LikedBy,
		})
	}

	return result, nil
}

// GetByStatus returns comments by status for admin
func (r *CommentRepository) GetByStatus(status string, page, limit int) ([]models.CommentWithUser, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{}
	if status != "" {
		filter["status"] = status
	}

	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	skip := (page - 1) * limit
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var comments []models.MovieComment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, 0, err
	}

	result := make([]models.CommentWithUser, 0, len(comments))
	for _, c := range comments {
		user, _ := r.getUserByID(c.UserID)
		movieTitle, _ := r.getMovieTitle(c.MovieID)
		result = append(result, models.CommentWithUser{
			ID:                  c.ID,
			MovieID:             c.MovieID,
			UserID:              c.UserID,
			ParentID:            c.ParentID,
			Content:             c.Content,
			Status:              c.Status,
			HasBlockedWord:      c.HasBlockedWord,
			HasLink:             c.HasLink,
			CreatedAt:           c.CreatedAt,
			UpdatedAt:           c.UpdatedAt,
			IsPremiumUser:       c.IsPremiumUser,
			UserDisplayName:     getUserDisplayName(user),
			UserAvatarURL:       getUserAvatarURL(user),
			UserRole:            getUserRole(user),
			UserIsPremium:       getUserIsPremium(user),
			UserIsPremiumActive: getUserIsPremiumActive(user),
			LikesCount:          c.LikesCount,
			LikedBy:             c.LikedBy,
		})
		_ = movieTitle // Could be added to response if needed
	}

	return result, total, nil
}

// UpdateStatus updates comment status
func (r *CommentRepository) UpdateStatus(id primitive.ObjectID, status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": id}
	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	_, err := r.col.UpdateOne(ctx, filter, update)
	return err
}

// GetModerationSettings returns moderation settings
func (r *CommentRepository) GetModerationSettings() (*models.CommentModerationSettings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var settings models.CommentModerationSettings
	err := r.settingsCol.FindOne(ctx, bson.M{"_id": "global"}).Decode(&settings)
	if err == mongo.ErrNoDocuments {
		// Create default settings
		defaultSettings := models.GetDefaultModerationSettings()
		_, err = r.settingsCol.InsertOne(ctx, defaultSettings)
		if err != nil {
			return nil, err
		}
		return defaultSettings, nil
	}
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

// UpdateModerationSettings updates moderation settings
func (r *CommentRepository) UpdateModerationSettings(settings *models.CommentModerationSettings) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if settings.ID == "" {
		settings.ID = "global"
	}
	if settings.MaxCommentLength <= 0 {
		settings.MaxCommentLength = 2000
	}
	if settings.MaxReplyDepth <= 0 {
		settings.MaxReplyDepth = 3
	}
	if settings.CommentCooldownSeconds <= 0 {
		settings.CommentCooldownSeconds = 30
	}
	if settings.MaxCommentsPerMinute <= 0 {
		settings.MaxCommentsPerMinute = 5
	}

	// Ensure banned words are lowercase
	for i, w := range settings.BannedWords {
		settings.BannedWords[i] = strings.ToLower(w)
	}

	if settings.DefaultSort == "" {
		settings.DefaultSort = "newest"
	}

	settings.UpdatedAt = time.Now()
	filter := bson.M{"_id": settings.ID}
	update := bson.M{
		"$set": bson.M{
			"comments_enabled":         settings.CommentsEnabled,
			"replies_enabled":          settings.RepliesEnabled,
			"block_links":              settings.BlockLinks,
			"max_comment_length":       settings.MaxCommentLength,
			"max_reply_depth":          settings.MaxReplyDepth,
			"require_moderation":       settings.RequireModeration,
			"banned_words":             settings.BannedWords,
			"auto_hide_banned_content": settings.AutoHideBannedContent,
			"comment_cooldown_seconds": settings.CommentCooldownSeconds,
			"max_comments_per_minute":  settings.MaxCommentsPerMinute,
			"default_sort":             settings.DefaultSort,
			"updated_at":               settings.UpdatedAt,
			"updated_by":               settings.UpdatedBy,
		},
	}

	_, err := r.settingsCol.UpdateOne(ctx, filter, update, upsertOpt)
	return err
}

// GetLastCommentByUser returns the most recent comment by user
func (r *CommentRepository) GetLastCommentByUser(userID primitive.ObjectID) (*models.MovieComment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var comment models.MovieComment
	err := r.col.FindOne(ctx, bson.M{"user_id": userID},
		options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})).Decode(&comment)
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// CountRecentCommentsByUser counts comments by user in the last minute
func (r *CommentRepository) CountRecentCommentsByUser(userID primitive.ObjectID, limit int) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	since := time.Now().Add(-time.Minute)
	count, err := r.col.CountDocuments(ctx, bson.M{
		"user_id":    userID,
		"created_at": bson.M{"$gte": since},
	})
	return count, err
}

// GetCommentDepth returns the depth of a comment (how many ancestors it has)
func (r *CommentRepository) GetCommentDepth(commentID primitive.ObjectID) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	depth := 0
	currentID := commentID

	for depth < 10 { // Safety limit
		var comment models.MovieComment
		err := r.col.FindOne(ctx, bson.M{"_id": currentID}).Decode(&comment)
		if err != nil || comment.ParentID == nil {
			break
		}
		currentID = *comment.ParentID
		depth++
	}

	return depth, nil
}

var upsertOpt = options.Update().SetUpsert(true)

// ToggleLike adds or removes a user's like on a comment.
// Returns (liked, likeCount, commentOwnerID, err). `liked` is true if the user now likes it.
func (r *CommentRepository) ToggleLike(commentID, userID primitive.ObjectID) (bool, int, primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var comment models.MovieComment
	if err := r.col.FindOne(ctx, bson.M{"_id": commentID}).Decode(&comment); err != nil {
		return false, 0, primitive.NilObjectID, err
	}

	already := false
	for _, id := range comment.LikedBy {
		if id == userID {
			already = true
			break
		}
	}

	var update bson.M
	if already {
		update = bson.M{
			"$pull": bson.M{"liked_by": userID},
			"$inc":  bson.M{"likes_count": -1},
		}
	} else {
		update = bson.M{
			"$addToSet": bson.M{"liked_by": userID},
			"$inc":      bson.M{"likes_count": 1},
		}
	}

	if _, err := r.col.UpdateOne(ctx, bson.M{"_id": commentID}, update); err != nil {
		return false, 0, primitive.NilObjectID, err
	}

	// Re-read for accurate count
	if err := r.col.FindOne(ctx, bson.M{"_id": commentID}).Decode(&comment); err != nil {
		return false, 0, primitive.NilObjectID, err
	}
	return !already, comment.LikesCount, comment.UserID, nil
}

// getUserByID retrieves user by ID
func (r *CommentRepository) getUserByID(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var user models.User
	err := r.userCol.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// getMovieTitle retrieves movie title by ID
func (r *CommentRepository) getMovieTitle(movieID primitive.ObjectID) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var movie models.Movie
	err := r.movieCol.FindOne(ctx, bson.M{"_id": movieID}).Decode(&movie)
	if err != nil {
		return "", err
	}
	return movie.Title, nil
}

// getUserDisplayName extracts display name from user
func getUserDisplayName(user *models.User) string {
	if user.DisplayName != "" {
		return user.DisplayName
	}
	if user.FirstName != "" {
		return user.FirstName
	}
	if user.TelegramUser != "" {
		return user.TelegramUser
	}
	return "Anonymous"
}

// getUserAvatarURL extracts avatar URL from user
func getUserAvatarURL(user *models.User) string {
	return user.ProfileImageURL
}

// getUserRole extracts role from user
func getUserRole(user *models.User) string {
	return user.Role
}

// getUserIsPremium extracts active premium status from user
func getUserIsPremium(user *models.User) bool {
	return user.IsPremiumActive()
}

// getUserIsPremiumActive extracts active premium status from user (same as getUserIsPremium but explicit)
func getUserIsPremiumActive(user *models.User) bool {
	return user.IsPremiumActive()
}

// GetEpisodeTitle retrieves episode title for admin display
func (r *CommentRepository) getEpisodeTitle(episodeID primitive.ObjectID) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var episode struct {
		Title string `bson:"title"`
	}

	err := r.episodeCol.FindOne(ctx, bson.M{"_id": episodeID}).Decode(&episode)
	if err != nil {
		return "", err
	}
	return episode.Title, nil
}

// GetByTarget returns approved comments for a target (movie or episode)
func (r *CommentRepository) GetByTarget(targetType models.CommentTargetType, targetID primitive.ObjectID, limit, skip int) ([]models.CommentWithUser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build filter for target type and ID
	filter := bson.M{
		"target_type": targetType,
		"target_id":   targetID,
		"$or": []bson.M{
			{"status": models.CommentStatusApproved},
			{"status": ""},
			{"status": bson.M{"$exists": false}},
		},
		"parent_id": nil,
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var comments []models.MovieComment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, err
	}

	// Enrich with user info
	result := make([]models.CommentWithUser, 0, len(comments))
	for _, c := range comments {
		user, _ := r.getUserByID(c.UserID)
		repliesCount, _ := r.CountReplies(c.ID)

		// Get target title
		targetTitle := ""
		if targetType == models.CommentTargetMovie {
			targetTitle, _ = r.getMovieTitle(targetID)
		} else if targetType == models.CommentTargetEpisode {
			targetTitle, _ = r.getEpisodeTitle(targetID)
		}

		result = append(result, models.CommentWithUser{
			ID:                  c.ID,
			TargetType:          c.TargetType,
			TargetID:            c.TargetID,
			UserID:              c.UserID,
			ParentID:            c.ParentID,
			Content:             c.Content,
			Status:              c.Status,
			HasBlockedWord:      c.HasBlockedWord,
			HasLink:             c.HasLink,
			CreatedAt:           c.CreatedAt,
			UpdatedAt:           c.UpdatedAt,
			IsPremiumUser:       c.IsPremiumUser,
			UserDisplayName:     getUserDisplayName(user),
			UserAvatarURL:       getUserAvatarURL(user),
			UserRole:            getUserRole(user),
			UserIsPremium:       getUserIsPremium(user),
			UserIsPremiumActive: getUserIsPremiumActive(user),
			TargetTitle:         targetTitle,
			RepliesCount:        repliesCount,
			LikesCount:          c.LikesCount,
			LikedBy:             c.LikedBy,
		})
	}

	return result, nil
}

// GetAllApprovedByTarget returns all approved comments for a target (including nested replies)
func (r *CommentRepository) GetAllApprovedByTarget(targetType models.CommentTargetType, targetID primitive.ObjectID, limit, skip int) ([]models.CommentWithUser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build filter for target type and ID
	filter := bson.M{
		"target_type": targetType,
		"target_id":   targetID,
		"$or": []bson.M{
			{"status": models.CommentStatusApproved},
			{"status": ""},
			{"status": bson.M{"$exists": false}},
		},
	}

	// Sort by premium status first (true before false), then by created_at descending
	opts := options.Find().
		SetSort(bson.D{
			{Key: "is_premium_user", Value: -1}, // premium users first (true > false)
			{Key: "created_at", Value: -1},      // newest first
		}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var comments []models.MovieComment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, err
	}

	// Enrich with user info
	result := make([]models.CommentWithUser, 0, len(comments))
	for _, c := range comments {
		user, _ := r.getUserByID(c.UserID)
		repliesCount, _ := r.CountReplies(c.ID)

		// Get target title
		targetTitle := ""
		if targetType == models.CommentTargetMovie {
			targetTitle, _ = r.getMovieTitle(targetID)
		} else if targetType == models.CommentTargetEpisode {
			targetTitle, _ = r.getEpisodeTitle(targetID)
		}

		likesCount := c.LikesCount
		if likesCount == 0 && len(c.LikedBy) > 0 {
			likesCount = len(c.LikedBy)
		}

		result = append(result, models.CommentWithUser{
			ID:                  c.ID,
			TargetType:          c.TargetType,
			TargetID:            c.TargetID,
			UserID:              c.UserID,
			ParentID:            c.ParentID,
			Content:             c.Content,
			Status:              c.Status,
			HasBlockedWord:      c.HasBlockedWord,
			HasLink:             c.HasLink,
			CreatedAt:           c.CreatedAt,
			UpdatedAt:           c.UpdatedAt,
			IsPremiumUser:       c.IsPremiumUser,
			UserDisplayName:     getUserDisplayName(user),
			UserAvatarURL:       getUserAvatarURL(user),
			UserRole:            getUserRole(user),
			UserIsPremium:       getUserIsPremium(user),
			UserIsPremiumActive: getUserIsPremiumActive(user),
			TargetTitle:         targetTitle,
			RepliesCount:        repliesCount,
			LikesCount:          likesCount,
			LikedBy:             c.LikedBy,
		})
	}

	return result, nil
}

// GetRepliesByTarget returns approved replies for a parent comment, filtered by target
func (r *CommentRepository) GetRepliesByTarget(parentID primitive.ObjectID, targetType models.CommentTargetType, targetID primitive.ObjectID) ([]models.CommentWithUser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Include comments with empty/missing status (backward compatibility)
	filter := bson.M{
		"parent_id":   parentID,
		"target_type": targetType,
		"target_id":   targetID,
		"$or": []bson.M{
			{"status": models.CommentStatusApproved},
			{"status": ""},
			{"status": bson.M{"$exists": false}},
		},
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var comments []models.MovieComment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, err
	}

	result := make([]models.CommentWithUser, 0, len(comments))
	for _, c := range comments {
		user, _ := r.getUserByID(c.UserID)
		result = append(result, models.CommentWithUser{
			ID:                  c.ID,
			TargetType:          c.TargetType,
			TargetID:            c.TargetID,
			UserID:              c.UserID,
			ParentID:            c.ParentID,
			Content:             c.Content,
			Status:              c.Status,
			HasBlockedWord:      c.HasBlockedWord,
			HasLink:             c.HasLink,
			CreatedAt:           c.CreatedAt,
			UpdatedAt:           c.UpdatedAt,
			IsPremiumUser:       c.IsPremiumUser,
			UserDisplayName:     getUserDisplayName(user),
			UserAvatarURL:       getUserAvatarURL(user),
			UserRole:            getUserRole(user),
			UserIsPremium:       getUserIsPremium(user),
			UserIsPremiumActive: getUserIsPremiumActive(user),
			LikesCount:          c.LikesCount,
			LikedBy:             c.LikedBy,
		})
	}

	return result, nil
}

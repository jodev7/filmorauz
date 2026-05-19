package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NotificationConfig holds configuration for the notification service
type NotificationConfig struct {
	BotToken        string
	BotUsername     string
	ChannelUsername string
}

// NotificationService handles notification creation and management
type NotificationService struct {
	notificationRepo *repositories.NotificationRepository
	userRepo         *repositories.UserRepository
	botToken         string
	botUsername      string
	channelUsername  string
}

// NewNotificationServiceWithConfig creates a new NotificationService with config
func NewNotificationServiceWithConfig(config NotificationConfig) (*NotificationService, error) {
	return &NotificationService{
		notificationRepo: nil, // Will be set when DB is initialized
		userRepo:         nil,
		botToken:         config.BotToken,
		botUsername:      config.BotUsername,
		channelUsername:  config.ChannelUsername,
	}, nil
}

// NewNotificationService creates a new NotificationService
func NewNotificationService(
	notificationRepo *repositories.NotificationRepository,
	userRepo *repositories.UserRepository,
) *NotificationService {
	return &NotificationService{
		notificationRepo: notificationRepo,
		userRepo:         userRepo,
	}
}

// SetRepositories sets the repositories (called after DB initialization)
func (s *NotificationService) SetRepositories(
	notificationRepo *repositories.NotificationRepository,
	userRepo *repositories.UserRepository,
) {
	s.notificationRepo = notificationRepo
	s.userRepo = userRepo
}

// SendMovieNotificationAsync sends a notification about a new movie
func (s *NotificationService) SendMovieNotificationAsync(movie interface{}) {
	// TODO: Implement batch notification to all users about new movie
	log.Printf("Movie notification: new movie added (async notification not yet implemented)")
}

// CreateNotification creates a new notification for a user
func (s *NotificationService) CreateNotification(ctx context.Context, req *models.NotificationCreateRequest) error {
	if s.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}

	notification := &models.Notification{
		UserID:    req.UserID,
		Type:      req.Type,
		Title:     req.Title,
		Message:   req.Message,
		ActionURL: req.ActionURL,
		Data:      req.Data,
	}

	return s.notificationRepo.Create(ctx, notification)
}

// NotifyPremiumActivated sends notification when premium is activated
func (s *NotificationService) NotifyPremiumActivated(ctx context.Context, userID primitive.ObjectID, expiresAt *time.Time) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	var message string
	if expiresAt != nil {
		message = fmt.Sprintf("Premium obunangiz %s gacha faol.", expiresAt.Format("02.01.2006"))
	} else {
		message = "Premium obunangiz muvaffaqiyatli faollashtirildi."
	}

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      models.NotificationPremiumActivated,
		Title:     "Premium faollashtirildi",
		Message:   message,
		ActionURL: "/premium",
	})
}

// NotifyPremiumExpiringSoon sends notification when premium is about to expire
func (s *NotificationService) NotifyPremiumExpiringSoon(ctx context.Context, userID primitive.ObjectID, daysUntilExpiry int) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	// Deduplication: don't send if we already sent one in the last 24 hours
	hasRecent, err := s.notificationRepo.CheckRecentNotification(ctx, userID, models.NotificationPremiumExpiringSoon, 24)
	if err != nil {
		return err
	}
	if hasRecent {
		return nil // Already notified recently
	}

	daysText := ""
	switch daysUntilExpiry {
	case 1:
		daysText = "1 kun"
	case 2, 3, 4:
		daysText = fmt.Sprintf("%d kun", daysUntilExpiry)
	default:
		daysText = fmt.Sprintf("%d kun", daysUntilExpiry)
	}

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      models.NotificationPremiumExpiringSoon,
		Title:     "Premium tugashiga oz qoldi",
		Message:   fmt.Sprintf("Premium obunangiz tugashiga %s qoldi.", daysText),
		ActionURL: "/premium",
		Data: map[string]interface{}{
			"days_until_expiry": daysUntilExpiry,
		},
	})
}

// NotifyPremiumExpired sends notification when premium expires
func (s *NotificationService) NotifyPremiumExpired(ctx context.Context, userID primitive.ObjectID) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      models.NotificationPremiumExpired,
		Title:     "Premium muddati tugadi",
		Message:   "Premium obunangiz muddati tugadi. Yangi premium obunani faollashtirish uchun premium sahifasiga kiring.",
		ActionURL: "/premium",
	})
}

// NotifyBanRemoved sends notification when a user is unbanned
func (s *NotificationService) NotifyBanRemoved(ctx context.Context, userID primitive.ObjectID) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      models.NotificationBanRemoved,
		Title:     "Ban bekor qilindi",
		Message:   "Hisobingizdagi ban bekor qilindi. Endi saytdan to'liq foydalanishingiz mumkin.",
		ActionURL: "/",
	})
}

// NotifyBanApplied sends notification when a user is banned
func (s *NotificationService) NotifyBanApplied(ctx context.Context, userID primitive.ObjectID, reason string, duration string, bannedByName string) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	// Build duration text
	var durationText string
	if duration == "permanent" {
		durationText = "doimiy"
	} else if duration != "" {
		durationText = duration
	} else {
		durationText = "muajjam"
	}

	message := fmt.Sprintf("Siz %s muddatga ban qilindingiz. Sabab: %s", durationText, reason)

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      models.NotificationBanApplied,
		Title:     "Siz ban qilindingiz",
		Message:   message,
		ActionURL: "/banned",
		Data: map[string]interface{}{
			"ban_reason":     reason,
			"ban_duration":   duration,
			"banned_by_name": bannedByName,
		},
	})
}

// NotifyAppealSubmitted sends notification when user submits an appeal (confirmation)
func (s *NotificationService) NotifyAppealSubmitted(ctx context.Context, userID primitive.ObjectID, appealID string) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      models.NotificationAppealSubmitted,
		Title:     "Apellyatsiya yuborildi",
		Message:   "Sizning apellyatsiyangiz muvaffaqiyatli yuborildi va ko'rib chiqish uchun tayyor.",
		ActionURL: "/banned",
		Data: map[string]interface{}{
			"appeal_id": appealID,
		},
	})
}

// NotifyAppealApproved sends notification when an appeal is approved
func (s *NotificationService) NotifyAppealApproved(ctx context.Context, userID primitive.ObjectID, appealID string) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      models.NotificationAppealApproved,
		Title:     "Apellyatsiya qabul qilindi",
		Message:   "Sizning apellyatsiyangiz qabul qilindi. Hisobingizdagi ban bekor qilindi.",
		ActionURL: "/",
		Data: map[string]interface{}{
			"appeal_id": appealID,
		},
	})
}

// NotifyAppealRejected sends notification when an appeal is rejected
func (s *NotificationService) NotifyAppealRejected(ctx context.Context, userID primitive.ObjectID, appealID string, adminNote string) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	message := "Sizning apellyatsiyangiz rad etildi."
	if adminNote != "" {
		message = fmt.Sprintf("Sizning apellyatsiyangiz rad etildi. Admin izohi: %s", adminNote)
	}

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      models.NotificationAppealRejected,
		Title:     "Apellyatsiya rad etildi",
		Message:   message,
		ActionURL: "/banned",
		Data: map[string]interface{}{
			"appeal_id":  appealID,
			"admin_note": adminNote,
		},
	})
}

// NotifyCommentReply sends notification when someone replies to a comment
func (s *NotificationService) NotifyCommentReply(ctx context.Context, commentOwnerID primitive.ObjectID, movieID string, movieSlug string, replierID primitive.ObjectID, commentID string, replyID string, replierName string, actionURL string) error {
	if s.notificationRepo == nil {
		return nil // Silently skip if not initialized
	}

	// Don't notify if replying to own comment (compare user IDs)
	if commentOwnerID == replierID {
		return nil // Don't notify about own replies
	}

	// Build action URL if not provided
	if actionURL == "" {
		if movieSlug != "" {
			actionURL = fmt.Sprintf("/movies/%s#comment-%s", movieSlug, commentID)
		} else {
			actionURL = fmt.Sprintf("/#comment-%s", commentID)
		}
	}

	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    commentOwnerID,
		Type:      models.NotificationCommentReply,
		Title:     "Izohingizga javob yozildi",
		Message:   fmt.Sprintf("%s sizning izohingizga javob yozdi.", replierName),
		ActionURL: actionURL,
		Data: map[string]interface{}{
			"movie_id":     movieID,
			"movie_slug":   movieSlug,
			"comment_id":   commentID,
			"reply_id":     replyID,
			"replier_id":   replierID.Hex(),
			"replier_name": replierName,
		},
	})
}

// NotifyCommentLike sends a notification to the comment owner when their comment is liked.
func (s *NotificationService) NotifyCommentLike(ctx context.Context, commentOwnerID, likerID primitive.ObjectID, likerName, movieID, movieSlug, commentID, actionURL string) error {
	if s.notificationRepo == nil {
		return nil
	}
	if commentOwnerID == likerID {
		return nil
	}
	return s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    commentOwnerID,
		Type:      models.NotificationCommentLike,
		Title:     "Izohingizga like bosildi",
		Message:   fmt.Sprintf("%s commentingizga like bosdi", likerName),
		ActionURL: actionURL,
		Data: map[string]interface{}{
			"liker_id":   likerID.Hex(),
			"liker_name": likerName,
			"comment_id": commentID,
			"movie_id":   movieID,
			"movie_slug": movieSlug,
		},
	})
}

// GetUserNotifications retrieves notifications for a user
func (s *NotificationService) GetUserNotifications(ctx context.Context, userID primitive.ObjectID, page, perPage int) (*models.NotificationResponse, error) {
	if s.notificationRepo == nil {
		return &models.NotificationResponse{
			Notifications: []models.Notification{},
			Total:         0,
			UnreadCount:   0,
		}, nil
	}

	notifications, total, err := s.notificationRepo.FindByUserIDPaginated(ctx, userID, page, perPage)
	if err != nil {
		return nil, err
	}

	unreadCount, err := s.notificationRepo.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &models.NotificationResponse{
		Notifications: notifications,
		Total:         total,
		UnreadCount:   unreadCount,
	}, nil
}

// GetUnreadCount returns the unread notification count for a user
func (s *NotificationService) GetUnreadCount(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	if s.notificationRepo == nil {
		return 0, nil
	}
	return s.notificationRepo.CountUnread(ctx, userID)
}

// MarkAsRead marks a notification as read
func (s *NotificationService) MarkAsRead(ctx context.Context, notificationID primitive.ObjectID, userID primitive.ObjectID) error {
	if s.notificationRepo == nil {
		return nil
	}
	return s.notificationRepo.MarkAsRead(ctx, notificationID, userID)
}

// DeleteNotification removes a notification entirely.
func (s *NotificationService) DeleteNotification(ctx context.Context, notificationID primitive.ObjectID, userID primitive.ObjectID) error {
	if s.notificationRepo == nil {
		return nil
	}
	return s.notificationRepo.DeleteByID(ctx, notificationID, userID)
}

// MarkAllAsRead marks all notifications as read for a user
func (s *NotificationService) MarkAllAsRead(ctx context.Context, userID primitive.ObjectID) error {
	if s.notificationRepo == nil {
		return nil
	}
	return s.notificationRepo.MarkAllAsRead(ctx, userID)
}

// MarkMultipleAsRead marks multiple notifications as read
func (s *NotificationService) MarkMultipleAsRead(ctx context.Context, ids []primitive.ObjectID, userID primitive.ObjectID) error {
	if s.notificationRepo == nil {
		return nil
	}
	return s.notificationRepo.MarkMultipleAsRead(ctx, ids, userID)
}

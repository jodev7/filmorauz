package services

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SuggestionService struct {
	suggestionRepo  *repositories.SuggestionRepository
	userRepo        *repositories.UserRepository
	notificationSvc *NotificationService
}

func NewSuggestionService(
	suggestionRepo *repositories.SuggestionRepository,
	userRepo *repositories.UserRepository,
	notificationSvc *NotificationService,
) *SuggestionService {
	return &SuggestionService{
		suggestionRepo:  suggestionRepo,
		userRepo:        userRepo,
		notificationSvc: notificationSvc,
	}
}

func (s *SuggestionService) CreateSuggestion(ctx context.Context, userID primitive.ObjectID, userName, userEmail string, input *models.SuggestionInput, imageURL, imageStorageKey, imageMimeType string, imageSize int64) (*models.Suggestion, error) {
	suggestion := &models.Suggestion{
		UserID:          userID,
		UserName:        userName,
		Type:            input.Type,
		Title:           html.EscapeString(strings.TrimSpace(input.Title)),
		Message:         html.EscapeString(strings.TrimSpace(input.Message)),
		SourceURL:       strings.TrimSpace(input.SourceURL),
		ImageURL:        imageURL,
		ImageStorageKey: imageStorageKey,
		ImageMimeType:   imageMimeType,
		ImageSize:       imageSize,
		Status:          models.SuggestionStatusPending,
	}

	if suggestion.SourceURL != "" && !isValidURL(suggestion.SourceURL) {
		return nil, fmt.Errorf("invalid source URL")
	}

	err := s.suggestionRepo.Create(suggestion)
	if err != nil {
		return nil, err
	}

	return suggestion, nil
}

func (s *SuggestionService) GetSuggestionsByUser(ctx context.Context, userID primitive.ObjectID, page, limit int) (*models.SuggestionListResponse, error) {
	suggestions, total, err := s.suggestionRepo.FindByUserID(userID, page, limit)
	if err != nil {
		return nil, err
	}

	return &models.SuggestionListResponse{
		Suggestions: suggestions,
		Total:       total,
		Page:        page,
		Limit:       limit,
	}, nil
}

func (s *SuggestionService) GetSuggestions(ctx context.Context, page, limit int, status *models.SuggestionStatus) (*models.SuggestionListResponse, error) {
	suggestions, total, err := s.suggestionRepo.List(page, limit, status)
	if err != nil {
		return nil, err
	}

	return &models.SuggestionListResponse{
		Suggestions: suggestions,
		Total:       total,
		Page:        page,
		Limit:       limit,
	}, nil
}

func (s *SuggestionService) GetSuggestionByID(ctx context.Context, id string) (*models.Suggestion, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	return s.suggestionRepo.FindByID(objID)
}

func (s *SuggestionService) UpdateSuggestionStatus(ctx context.Context, id string, status models.SuggestionStatus, adminMessage, reviewedBy string) (*models.Suggestion, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	err = s.suggestionRepo.UpdateStatus(objID, status, adminMessage, reviewedBy)
	if err != nil {
		return nil, err
	}

	suggestion, err := s.suggestionRepo.FindByID(objID)
	if err != nil {
		return nil, err
	}

	if s.notificationSvc != nil {
		s.sendStatusNotification(ctx, suggestion)
	}

	return suggestion, nil
}

func (s *SuggestionService) sendStatusNotification(ctx context.Context, suggestion *models.Suggestion) {
	var title, message string

	if suggestion.Status == models.SuggestionStatusAccepted {
		title = "Tavsiyangiz qabul qilindi"
		message = fmt.Sprintf("Sizning \"%s\" tavsiyangiz qabul qilindi.", suggestion.Title)
		if suggestion.AdminMessage != "" {
			message = fmt.Sprintf("Sizning \"%s\" tavsiyangiz qabul qilindi. Admin xabari: %s", suggestion.Title, suggestion.AdminMessage)
		}
	} else if suggestion.Status == models.SuggestionStatusRejected {
		title = "Tavsiyangiz rad etildi"
		message = fmt.Sprintf("Afsuski, \"%s\" tavsiyangiz rad etildi.", suggestion.Title)
		if suggestion.AdminMessage != "" {
			message = fmt.Sprintf("Afsuski, \"%s\" tavsiyangiz rad etildi. Admin xabari: %s", suggestion.Title, suggestion.AdminMessage)
		}
	}

	if title != "" && message != "" {
		_ = s.notificationSvc.CreateNotification(ctx, &models.NotificationCreateRequest{
			UserID:    suggestion.UserID,
			Type:      "SUGGESTION_STATUS",
			Title:     title,
			Message:   message,
			ActionURL: "/",
			Data: map[string]interface{}{
				"suggestion_id":    suggestion.ID.Hex(),
				"suggestion_title": suggestion.Title,
				"status":           string(suggestion.Status),
				"admin_message":    suggestion.AdminMessage,
			},
		})
	}
}

func (s *SuggestionService) GetStats(ctx context.Context) (map[string]int64, error) {
	return s.suggestionRepo.GetStats()
}

func isValidURL(url string) bool {
	if url == "" {
		return true
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}
	if len(url) < 10 || len(url) > 500 {
		return false
	}
	return true
}

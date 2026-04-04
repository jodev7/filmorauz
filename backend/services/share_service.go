package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
)

type ShareService struct {
	shareRepo  *repositories.ShareRepository
	movieRepo  *repositories.MovieRepository
	userRepo   *repositories.UserRepository
	domain     string
	codeLength int
}

func NewShareService(shareRepo *repositories.ShareRepository, movieRepo *repositories.MovieRepository, userRepo *repositories.UserRepository, domain string) *ShareService {
	return &ShareService{
		shareRepo:  shareRepo,
		movieRepo:  movieRepo,
		userRepo:   userRepo,
		domain:     domain,
		codeLength: 8,
	}
}

// CreateShare creates a new share for a movie
func (s *ShareService) CreateShare(movieID primitive.ObjectID, userID *primitive.ObjectID, source string) (*models.Share, string, error) {
	// Verify movie exists
	movie, err := s.movieRepo.FindByID(movieID)
	if err != nil {
		return nil, "", errors.New("movie not found")
	}
	if movie == nil {
		return nil, "", errors.New("movie not found")
	}

	// Generate unique share code
	code, err := s.generateShareCode()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate share code: %w", err)
	}

	// Create share document
	share := &models.Share{
		Code:    code,
		MovieID: movieID,
		Source:  source,
		Clicks:  0,
	}

	if userID != nil && !userID.IsZero() {
		share.CreatedByUserID = *userID
		share.CreatedByUserHex = userID.Hex()
	}

	// Save to database
	if err := s.shareRepo.Create(share); err != nil {
		return nil, "", fmt.Errorf("failed to create share: %w", err)
	}

	// Generate share URL
	shareURL := fmt.Sprintf("%s/movies/%s?src=share&share=%s", s.domain, movie.Slug, code)

	return share, shareURL, nil
}

// RecordShareOpen records when a share is opened
func (s *ShareService) RecordShareOpen(code string) error {
	// Find share by code
	share, err := s.shareRepo.FindByCode(code)
	if err != nil {
		return errors.New("share not found")
	}
	if share == nil {
		return errors.New("share not found")
	}

	// Increment clicks
	if err := s.shareRepo.IncrementClicks(code); err != nil {
		return fmt.Errorf("failed to record share open: %w", err)
	}

	return nil
}

// GetMovieShareStats returns share statistics for a movie
func (s *ShareService) GetMovieShareStats(movieID primitive.ObjectID) (*models.ShareStats, error) {
	return s.shareRepo.GetMovieShareStats(movieID)
}

// GetUserShareStats returns share statistics for a user
func (s *ShareService) GetUserShareStats(userID primitive.ObjectID) (*models.UserShareStats, error) {
	return s.shareRepo.GetUserShareStats(userID)
}

// GetAdminShareStats returns admin-level share statistics
func (s *ShareService) GetAdminShareStats() (*models.AdminShareStats, error) {
	return s.shareRepo.GetAdminShareStats()
}

// generateShareCode generates a unique short share code
func (s *ShareService) generateShareCode() (string, error) {
	// Try multiple times to generate a unique code
	for i := 0; i < 10; i++ {
		code, err := generateShareCodeOnly(s.codeLength)
		if err != nil {
			continue
		}

		// Check if code already exists
		existing, err := s.shareRepo.FindByCode(code)
		if err != nil {
			continue
		}
		if existing == nil {
			return code, nil
		}
	}

	return "", errors.New("failed to generate unique share code")
}

// generateShareCodeOnly generates a random alphanumeric code
func generateShareCodeOnly(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

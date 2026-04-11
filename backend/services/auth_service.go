package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo         *repositories.UserRepository
	authSessionRepo  *repositories.AuthSessionRepository
	jwtSecret        string
	botUsername      string
	sessionExpiresIn time.Duration
}

func NewAuthService(userRepo *repositories.UserRepository, authSessionRepo *repositories.AuthSessionRepository, jwtSecret string) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		authSessionRepo:  authSessionRepo,
		jwtSecret:        jwtSecret,
		botUsername:      "FilmoraUzBot",     // Default, can be overridden
		sessionExpiresIn: 7 * 24 * time.Hour, // 7 days for web sessions
	}
}

// SetBotUsername sets the Telegram bot username for deep links
func (s *AuthService) SetBotUsername(username string) {
	s.botUsername = username
}

// GenerateAuthSession creates a new Telegram auth session and returns the code and deep link
func (s *AuthService) GenerateAuthSession() (*models.AuthSession, error) {
	code, err := generateSecureCode(8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate code: %w", err)
	}

	session := &models.AuthSession{
		Code:      code,
		Status:    models.AuthSessionStatusPending,
		ExpiresAt: time.Now().Add(10 * time.Minute), // 10 minutes to complete
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.authSessionRepo.Create(session); err != nil {
		log.Printf("[AUTH SERVICE] ERROR: Failed to create auth session: %v", err)
		return nil, fmt.Errorf("failed to create auth session: %w", err)
	}

	log.Printf("[AUTH SERVICE] Created auth session: code=%s, expires_at=%v", code, session.ExpiresAt)
	return session, nil
}

// GetBotDeepLink returns the Telegram deep link for the given auth code
func (s *AuthService) GetBotDeepLink(code string) string {
	return fmt.Sprintf("https://t.me/%s?start=login_%s", s.botUsername, code)
}

// CompleteAuthSession completes a Telegram auth session
// This is called by the Telegram bot after user sends /start login_<code>
func (s *AuthService) CompleteAuthSession(req *models.AuthSessionRequest) (*models.AuthSession, *models.User, error) {
	log.Printf("[AUTH SERVICE] CompleteAuthSession called with code=%s, telegram_id=%d", req.Code, req.TelegramID)

	// Find the session
	session, err := s.authSessionRepo.FindByCode(req.Code)
	if err != nil {
		log.Printf("[AUTH SERVICE] ERROR: Session not found for code=%s: %v", req.Code, err)
		return nil, nil, errors.New("invalid auth code")
	}

	log.Printf("[AUTH SERVICE] Found session: code=%s, status=%s, expires_at=%v",
		session.Code, session.Status, session.ExpiresAt)

	// Check if already completed
	if session.Status == models.AuthSessionStatusCompleted {
		log.Printf("[AUTH SERVICE] ERROR: Session already completed: code=%s", req.Code)
		return session, nil, errors.New("auth code already used")
	}

	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		log.Printf("[AUTH SERVICE] ERROR: Session expired: code=%s, expired_at=%v", req.Code, session.ExpiresAt)
		_ = s.authSessionRepo.MarkAsExpired(req.Code)
		return nil, nil, errors.New("auth code expired")
	}

	log.Printf("[AUTH SERVICE] Session is valid, creating/updating user for telegram_id=%d", req.TelegramID)

	// Create or update user with Telegram data
	existingUser, dbErr := s.userRepo.FindByTelegramID(req.TelegramID)
	if dbErr != nil {
		log.Printf("[AUTH SERVICE] ERROR: DB error looking up user: %v", dbErr)
		return nil, nil, fmt.Errorf("failed to look up user: %w", dbErr)
	}

	var isNew bool
	var user *models.User

	if existingUser == nil {
		// User doesn't exist — create new one, sanitizing the name first
		firstName := sanitizeName(req.FirstName)
		user, dbErr = s.userRepo.Create(
			req.TelegramID,
			firstName,
			req.LastName,
			req.Username,
			req.PhotoURL,
			"", // languageCode not in request
		)
		if dbErr != nil {
			log.Printf("[AUTH SERVICE] ERROR: Failed to create user: %v", dbErr)
			return nil, nil, errors.New("failed to create user")
		}
		isNew = true
	} else {
		// User exists — refresh their Telegram info in case it changed
		user = existingUser
		isNew = false
		if err := s.userRepo.UpdateTelegramInfo(req.TelegramID, req.Username, req.FirstName, req.LastName, req.PhotoURL); err != nil {
			log.Printf("[AUTH SERVICE] WARN: Failed to update telegram info: %v", err)
			// Non-fatal: continue with existing user data
		}
	}

	log.Printf("[AUTH SERVICE] User created/updated: telegram_id=%d, user_id=%s, is_new=%v",
		req.TelegramID, user.ID.Hex(), isNew)

	// Mark session as completed
	if err := s.authSessionRepo.MarkAsCompleted(req.Code, user.ID, req.TelegramID); err != nil {
		log.Printf("[AUTH SERVICE] ERROR: Failed to mark session as completed: %v", err)
		return nil, nil, fmt.Errorf("failed to mark session as completed: %w", err)
	}

	log.Printf("[AUTH] Auth session completed for user %d (%s), is_new=%v", req.TelegramID, user.FirstName, isNew)

	// Update last login
	_ = s.userRepo.UpdateLastLogin(user.ID)

	return session, user, nil
}

// UpsertBotUser saves/updates a user from a bot /start interaction.
func (s *AuthService) UpsertBotUser(telegramID, chatID int64, username, firstName, lastName string) error {
	return s.userRepo.UpsertTelegramUser(telegramID, chatID, username, firstName, lastName)
}

// GetAuthSessionStatus returns the status of an auth session
func (s *AuthService) GetAuthSessionStatus(code string) (*models.AuthSession, error) {
	session, err := s.authSessionRepo.FindByCode(code)
	if err != nil {
		return nil, errors.New("invalid auth code")
	}

	// Check if expired (even if not marked yet)
	if time.Now().After(session.ExpiresAt) && session.Status == models.AuthSessionStatusPending {
		_ = s.authSessionRepo.MarkAsExpired(code)
		session.Status = models.AuthSessionStatusExpired
	}

	return session, nil
}

// GenerateWebToken generates a JWT token for the web session (after Telegram auth)
func (s *AuthService) GenerateWebToken(user *models.User) (string, error) {
	return s.generateToken(user)
}

// GetCurrentUser returns the current authenticated user from context
func (s *AuthService) GetCurrentUser(userID string) (*models.User, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	return user, nil
}

// UpdateProfile updates the user's profile (first name)
func (s *AuthService) UpdateProfile(userID string, firstName string) error {
	return s.userRepo.UpdateProfileImage(userID, firstName, "")
}

// UpdateLanguageCode updates the user's preferred language
func (s *AuthService) UpdateLanguageCode(userID string, languageCode string) error {
	return s.userRepo.UpdateLanguageCode(userID, languageCode)
}

// UpdateProfileStyle updates the user's profile style (premium only)
func (s *AuthService) UpdateProfileStyle(userID string, profileStyle *models.ProfileStyle) error {
	// First get the user to check premium status
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Only premium users can update profile style
	if !user.IsPremiumActive() {
		return fmt.Errorf("bu funksiya faqat premium foydalanuvchilar uchun")
	}

	return s.userRepo.UpdateProfileStyle(userID, profileStyle)
}

// UpdatePrivacy updates the user's privacy settings (premium only)
func (s *AuthService) UpdatePrivacy(userID string, privacy *models.PrivacySettings) error {
	// First get the user to check premium status
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Only premium users can update privacy settings
	if !user.IsPremiumActive() {
		return fmt.Errorf("bu funksiya faqat premium foydalanuvchilar uchun")
	}

	return s.userRepo.UpdatePrivacy(userID, privacy)
}

func (s *AuthService) generateToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id":       user.ID.Hex(),
		"telegram_id":   user.TelegramID,
		"role":          user.Role,
		"auth_provider": user.AuthProvider,
		"exp":           time.Now().Add(s.sessionExpiresIn).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

// ValidateToken parses and validates a JWT token
func (s *AuthService) ValidateToken(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// HashPassword creates a bcrypt hash
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// sanitizeName trims whitespace and rejects placeholder values like "."
// Returns the cleaned name or empty string if invalid.
func sanitizeName(name string) string {
	v := strings.TrimSpace(name)
	if v == "." || v == "-" {
		return ""
	}
	return v
}

// generateSecureCode generates a random alphanumeric code
func generateSecureCode(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

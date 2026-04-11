package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// sanitizeDisplayName returns the best non-empty, non-placeholder display name.
// Priority: firstName → username → "User"
func sanitizeDisplayName(firstName, username string) string {
	for _, candidate := range []string{firstName, username} {
		v := strings.TrimSpace(candidate)
		if v != "" && v != "." && v != "-" {
			return v
		}
	}
	return "User"
}

type AuthHandler struct {
	authService *services.AuthService
}

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// TelegramAuthStartRequest is the request for starting Telegram auth
type TelegramAuthStartRequest struct {
	// Optional: can pass device info or other metadata
	DeviceID string `json:"device_id"`
}

// TelegramAuthStartResponse is the response for starting Telegram auth
type TelegramAuthStartResponse struct {
	Code      string `json:"code"`
	BotURL    string `json:"bot_url"`
	ExpiresAt string `json:"expires_at"`
}

// RegisterBotUser godoc
// POST /api/auth/telegram/register
// Upserts a user record when they send /start to the bot.
func (h *AuthHandler) RegisterBotUser(c *gin.Context) {
	var req struct {
		TelegramID int64  `json:"telegram_id" binding:"required"`
		ChatID     int64  `json:"chat_id" binding:"required"`
		Username   string `json:"username"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authService.UpsertBotUser(req.TelegramID, req.ChatID, req.Username, req.FirstName, req.LastName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TelegramAuthStart godoc
// POST /api/auth/telegram/start
// Creates a new Telegram auth session and returns the deep link
func (h *AuthHandler) TelegramAuthStart(c *gin.Context) {
	session, err := h.authService.GenerateAuthSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create auth session"})
		return
	}

	botURL := h.authService.GetBotDeepLink(session.Code)

	c.JSON(http.StatusOK, TelegramAuthStartResponse{
		Code:      session.Code,
		BotURL:    botURL,
		ExpiresAt: session.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// TelegramAuthComplete godoc
// POST /api/auth/telegram/complete
// Completes a Telegram auth session (called by the bot)
func (h *AuthHandler) TelegramAuthComplete(c *gin.Context) {
	// Log incoming request
	var req models.AuthSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[AUTH HANDLER] Failed to bind request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[AUTH HANDLER] CompleteAuth request: code=%s, telegram_id=%d, username=%s",
		req.Code, req.TelegramID, req.Username)

	session, user, err := h.authService.CompleteAuthSession(&req)
	if err != nil {
		log.Printf("[AUTH HANDLER] CompleteAuth error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[AUTH HANDLER] Auth session completed: code=%s, status=%s, user_id=%s",
		session.Code, session.Status, user.ID.Hex())

	// Generate web token for the user
	token, err := h.authService.GenerateWebToken(user)
	if err != nil {
		log.Printf("[AUTH HANDLER] Failed to generate token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  session.Status,
		"user_id": user.ID.Hex(),
		"token":   token,
		"message": "Authentication successful",
	})
}

// TelegramAuthStatus godoc
// GET /api/auth/telegram/status/:code
// Returns the status of an auth session (polled by frontend)
func (h *AuthHandler) TelegramAuthStatus(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	session, err := h.authService.GetAuthSessionStatus(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"status":     session.Status,
		"code":       session.Code,
		"expires_at": session.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// If completed, include user info and token for frontend to use
	if session.Status == models.AuthSessionStatusCompleted && !session.UserID.IsZero() {
		user, err := h.authService.GetCurrentUser(session.UserID.Hex())
		if err == nil {
			token, tokenErr := h.authService.GenerateWebToken(user)
			if tokenErr == nil {
				userResponse := gin.H{
					"id":            user.ID.Hex(),
					"telegram_id":   user.TelegramID,
					"username":      user.TelegramUser,
					"first_name":    sanitizeDisplayName(user.FirstName, user.TelegramUser),
					"last_name":     user.LastName,
					"role":          user.Role,
					"auth_provider": user.AuthProvider,
				}

				// Include ban info if user is banned
				if user.Ban != nil && user.Ban.IsBanned {
					userResponse["ban"] = gin.H{
						"is_banned":          user.Ban.IsBanned,
						"reason":             user.Ban.Reason,
						"banned_at":          user.Ban.BannedAt,
						"banned_until":       user.Ban.BannedUntil,
						"banned_by_username": user.Ban.BannedByUsername,
					}
				}

				response["user"] = userResponse
				response["token"] = token
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// CurrentUser godoc
// GET /api/auth/me
// Returns the current authenticated user
func (h *AuthHandler) CurrentUser(c *gin.Context) {
	// Get user from context (set by middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"authenticated": false})
		return
	}

	user, err := h.authService.GetCurrentUser(userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"authenticated": false})
		return
	}

	// Prefer first_name; fall back to display_name (legacy), then username
	rawName := user.FirstName
	if rawName == "" {
		rawName = user.DisplayName
	}
	displayName := sanitizeDisplayName(rawName, user.TelegramUser)

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"user": gin.H{
			"id":                user.ID.Hex(),
			"telegram_id":       user.TelegramID,
			"username":          user.TelegramUser,
			"first_name":        displayName,
			"last_name":         user.LastName,
			"profile_image_url": user.ProfileImageURL,
			"role":              user.Role,
			"is_premium":        user.IsPremium,
			"is_premium_active": user.IsPremiumActive(),
			"premium_started_at": func() *string {
				if user.PremiumStartedAt != nil {
					t := user.PremiumStartedAt.Format(time.RFC3339)
					return &t
				}
				return nil
			}(),
			"premium_expires_at": func() *string {
				if user.PremiumExpiresAt != nil {
					t := user.PremiumExpiresAt.Format(time.RFC3339)
					return &t
				}
				return nil
			}(),
			"profile_style": user.ProfileStyle,
			"privacy":       user.Privacy,
			"auth_provider": user.AuthProvider,
			"created_at":    user.CreatedAt,
			"last_login_at": user.LastLoginAt,
			"ban": func() *gin.H {
				if user.Ban != nil && user.Ban.IsBanned {
					banInfo := gin.H{
						"is_banned": user.Ban.IsBanned,
						"reason":    user.Ban.Reason,
						"banned_at": func() *string {
							if user.Ban.BannedAt != nil {
								t := user.Ban.BannedAt.Format(time.RFC3339)
								return &t
							}
							return nil
						}(),
						"banned_until": func() *string {
							if user.Ban.BannedUntil != nil {
								t := user.Ban.BannedUntil.Format(time.RFC3339)
								return &t
							}
							return nil
						}(),
						"banned_by_username": user.Ban.BannedByUsername,
					}
					return &banInfo
				}
				return nil
			}(),
		},
	})
}

// UpdateProfile godoc
// PATCH /api/auth/me
// Updates the current user's profile (first name)
type UpdateProfileRequest struct {
	FirstName string `json:"first_name" binding:"required,max=100"`
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "first_name is required"})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	log.Printf("[AUTH HANDLER] UpdateProfile: userID=%s, first_name=%s", userID.(string), req.FirstName)

	err := h.authService.UpdateProfile(userID.(string), req.FirstName)
	if err != nil {
		log.Printf("[AUTH HANDLER] UpdateProfile error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "profile updated"})
}

// UpdateLanguageCode updates the current user's preferred language
type UpdateLanguageCodeRequest struct {
	LanguageCode string `json:"language_code" binding:"required,oneof=uz ru en"`
}

func (h *AuthHandler) UpdateLanguageCode(c *gin.Context) {
	var req UpdateLanguageCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "language_code is required and must be one of: uz, ru, en"})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	log.Printf("[AUTH HANDLER] UpdateLanguageCode: userID=%s, language_code=%s", userID.(string), req.LanguageCode)

	err := h.authService.UpdateLanguageCode(userID.(string), req.LanguageCode)
	if err != nil {
		log.Printf("[AUTH HANDLER] UpdateLanguageCode error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update language"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "language updated", "language_code": req.LanguageCode})
}

// UpdateProfileStyle godoc
// PATCH /api/auth/profile-style
// Updates the current user's profile style (premium only)
type UpdateProfileStyleRequest struct {
	Frame    string `json:"frame"`
	Theme    string `json:"theme"`
	Gradient string `json:"gradient"`
}

func (h *AuthHandler) UpdateProfileStyle(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdateProfileStyleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	log.Printf("[AUTH HANDLER] UpdateProfileStyle: userID=%s, frame=%s, theme=%s, gradient=%s",
		userID.(string), req.Frame, req.Theme, req.Gradient)

	err := h.authService.UpdateProfileStyle(userID.(string), &models.ProfileStyle{
		Frame:    req.Frame,
		Theme:    req.Theme,
		Gradient: req.Gradient,
	})
	if err != nil {
		log.Printf("[AUTH HANDLER] UpdateProfileStyle error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "profile style updated"})
}

// UpdatePrivacy godoc
// PATCH /api/auth/privacy
// Updates the current user's privacy settings (premium only)
type UpdatePrivacyRequest struct {
	IsPrivate bool `json:"is_private"`
}

func (h *AuthHandler) UpdatePrivacy(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdatePrivacyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	log.Printf("[AUTH HANDLER] UpdatePrivacy: userID=%s, is_private=%t", userID.(string), req.IsPrivate)

	err := h.authService.UpdatePrivacy(userID.(string), &models.PrivacySettings{
		IsPrivate: req.IsPrivate,
	})
	if err != nil {
		log.Printf("[AUTH HANDLER] UpdatePrivacy error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "privacy settings updated"})
}

// Logout godoc
// POST /api/auth/logout
// Logs out the current user
func (h *AuthHandler) Logout(c *gin.Context) {
	// Since we're using JWT tokens (stateless), logout is handled client-side
	// by removing the token from storage. We can still return a success response.
	c.JSON(http.StatusOK, gin.H{
		"message": "Logout successful",
	})
}

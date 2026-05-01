package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

type PremiumHandler struct {
	userRepo        *repositories.UserRepository
	paymentRepo     *repositories.PremiumPaymentRepository
	sessionRepo     *repositories.PremiumPurchaseSessionRepository
	botUsername     string
	sessionDuration time.Duration
}

func NewPremiumHandler(userRepo *repositories.UserRepository, paymentRepo *repositories.PremiumPaymentRepository, sessionRepo *repositories.PremiumPurchaseSessionRepository, botUsername string) *PremiumHandler {
	return &PremiumHandler{
		userRepo:        userRepo,
		paymentRepo:     paymentRepo,
		sessionRepo:     sessionRepo,
		botUsername:     botUsername,
		sessionDuration: 15 * time.Minute,
	}
}

// GetPackages GET /api/premium/packages
// Public list of Telegram Stars premium packages so the frontend can render cards.
func (h *PremiumHandler) GetPackages(c *gin.Context) {
	pkgs := services.PremiumPackages()
	out := make([]gin.H, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, gin.H{
			"id":              p.ID,
			"label":           p.Label,
			"duration_months": p.DurationMonths,
			"stars_price":     p.StarsPrice,
			"discount":        p.Discount,
		})
	}
	c.JSON(http.StatusOK, gin.H{"packages": out})
}

func generatePremiumSessionToken() (string, error) {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func (h *PremiumHandler) requireInternalToken(c *gin.Context) bool {
	expected := os.Getenv("BOT_INTERNAL_TOKEN")
	if expected == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "BOT_INTERNAL_TOKEN not configured"})
		return false
	}
	provided := c.GetHeader("X-Internal-Token")
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	return true
}

func (h *PremiumHandler) loadSessionUser(session *models.PremiumPurchaseSession) (*models.User, error) {
	user, err := h.userRepo.FindByID(session.UserID.Hex())
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user_not_found")
	}
	return user, nil
}

func (h *PremiumHandler) resolveValidatedSession(token string, telegramID int64) (*models.PremiumPurchaseSession, *models.User, services.PremiumPackage, string, error) {
	session, err := h.sessionRepo.FindByToken(token)
	if err != nil {
		return nil, nil, services.PremiumPackage{}, "", err
	}
	if session == nil {
		return nil, nil, services.PremiumPackage{}, "session_not_found", nil
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, nil, services.PremiumPackage{}, "session_expired", nil
	}
	if session.Status == models.PremiumPurchaseSessionStatusCompleted {
		return nil, nil, services.PremiumPackage{}, "session_completed", nil
	}

	user, err := h.loadSessionUser(session)
	if err != nil {
		if err.Error() == "user_not_found" {
			return nil, nil, services.PremiumPackage{}, "user_not_found", nil
		}
		return nil, nil, services.PremiumPackage{}, "", err
	}
	if user.TelegramID == 0 {
		return nil, nil, services.PremiumPackage{}, "user_not_linked", nil
	}
	if user.TelegramID != telegramID {
		return nil, nil, services.PremiumPackage{}, "telegram_mismatch", nil
	}

	pkg, ok := services.PremiumPackageByID(session.Package)
	if !ok {
		return nil, nil, services.PremiumPackage{}, "unknown_package", nil
	}
	return session, user, pkg, "", nil
}

// CreateStarsSession POST /api/premium/stars/session
// Auth required. Creates a short-lived Telegram Stars purchase session bound to the current website user.
func (h *PremiumHandler) CreateStarsSession(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		Package string `json:"package" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package is required"})
		return
	}

	pkg, ok := services.PremiumPackageByID(req.Package)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown package"})
		return
	}

	user, err := h.userRepo.FindByID(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user"})
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if user.TelegramID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "telegram_not_linked"})
		return
	}

	token, err := generatePremiumSessionToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	now := time.Now()
	session := &models.PremiumPurchaseSession{
		Token:      token,
		UserID:     user.ID,
		TelegramID: user.TelegramID,
		Package:    pkg.ID,
		Status:     models.PremiumPurchaseSessionStatusPending,
		ExpiresAt:  now.Add(h.sessionDuration),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := h.sessionRepo.Insert(session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":       session.Token,
		"package":     pkg.ID,
		"stars_price": pkg.StarsPrice,
		"expires_at":  session.ExpiresAt.Format(time.RFC3339),
		"bot_url":     "https://t.me/" + h.botUsername + "?start=premium_" + session.Token,
	})
}

// ValidateStarsSession POST /api/internal/premium/telegram-stars/session/validate
// Internal bot endpoint. Confirms that the Telegram account opening the bot matches the linked website user.
func (h *PremiumHandler) ValidateStarsSession(c *gin.Context) {
	if !h.requireInternalToken(c) {
		return
	}

	var req struct {
		Token      string `json:"token" binding:"required"`
		TelegramID int64  `json:"telegram_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, user, pkg, reason, err := h.resolveValidatedSession(req.Token, req.TelegramID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reason != "" {
		status := http.StatusBadRequest
		if reason == "session_not_found" || reason == "user_not_found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": reason})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"token":           session.Token,
		"user_id":         user.ID.Hex(),
		"telegram_id":     user.TelegramID,
		"package":         pkg.ID,
		"label":           pkg.Label,
		"duration_months": pkg.DurationMonths,
		"stars_price":     pkg.StarsPrice,
		"expires_at":      session.ExpiresAt.Format(time.RFC3339),
	})
}

// GrantTelegramStars POST /api/internal/premium/telegram-stars/grant
// Called by the bot after a successful_payment event.
// Auth: X-Internal-Token header must match BOT_INTERNAL_TOKEN env var.
// Idempotent: a duplicate telegram_payment_charge_id returns ok without re-extending premium.
func (h *PremiumHandler) GrantTelegramStars(c *gin.Context) {
	if !h.requireInternalToken(c) {
		return
	}

	var req struct {
		TelegramID              int64  `json:"telegram_id" binding:"required"`
		SessionToken            string `json:"session_token" binding:"required"`
		StarsAmount             int    `json:"stars_amount" binding:"required"`
		TelegramPaymentChargeID string `json:"telegram_payment_charge_id" binding:"required"`
		ProviderPaymentChargeID string `json:"provider_payment_charge_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Idempotency: if charge already processed, return success without changes.
	existing, err := h.paymentRepo.FindByChargeID(req.TelegramPaymentChargeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "already_processed": true})
		return
	}

	session, user, pkg, reason, err := h.resolveValidatedSession(req.SessionToken, req.TelegramID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reason != "" {
		status := http.StatusBadRequest
		if reason == "session_not_found" || reason == "user_not_found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": reason})
		return
	}
	if req.StarsAmount != pkg.StarsPrice {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stars_amount_mismatch"})
		return
	}

	payment := &models.TelegramStarsPayment{
		UserID:                  user.ID,
		TelegramID:              req.TelegramID,
		SessionToken:            req.SessionToken,
		Package:                 pkg.ID,
		StarsAmount:             req.StarsAmount,
		DurationMonths:          pkg.DurationMonths,
		TelegramPaymentChargeID: req.TelegramPaymentChargeID,
		ProviderPaymentChargeID: req.ProviderPaymentChargeID,
		Status:                  "succeeded",
	}
	if err := h.paymentRepo.Insert(payment); err != nil {
		// Race: another concurrent grant inserted the same charge_id first.
		if repositories.IsDuplicateKey(err) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "already_processed": true})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	expiresAt, err := h.userRepo.ActivateOrExtendPremium(user.ID, pkg.DurationDays)
	if err != nil {
		log.Printf("[PREMIUM] ActivateOrExtendPremium failed user=%s: %v", user.ID.Hex(), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.sessionRepo.MarkCompleted(session.Token, req.TelegramPaymentChargeID); err != nil {
		log.Printf("[PREMIUM] failed to mark session completed token=%s charge=%s: %v", session.Token, req.TelegramPaymentChargeID, err)
	}

	log.Printf("[PREMIUM] Stars grant ok user=%s package=%s expires=%s charge=%s",
		user.ID.Hex(), pkg.ID, expiresAt.Format("2006-01-02"), req.TelegramPaymentChargeID)

	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"user_id":         user.ID.Hex(),
		"premium_expires": expiresAt,
	})
}

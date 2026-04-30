package handlers

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

type PremiumHandler struct {
	userRepo    *repositories.UserRepository
	paymentRepo *repositories.PremiumPaymentRepository
}

func NewPremiumHandler(userRepo *repositories.UserRepository, paymentRepo *repositories.PremiumPaymentRepository) *PremiumHandler {
	return &PremiumHandler{userRepo: userRepo, paymentRepo: paymentRepo}
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

// GrantTelegramStars POST /api/internal/premium/telegram-stars/grant
// Called by the bot after a successful_payment event.
// Auth: X-Internal-Token header must match BOT_INTERNAL_TOKEN env var.
// Idempotent: a duplicate telegram_payment_charge_id returns ok without re-extending premium.
func (h *PremiumHandler) GrantTelegramStars(c *gin.Context) {
	expected := os.Getenv("BOT_INTERNAL_TOKEN")
	if expected == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "BOT_INTERNAL_TOKEN not configured"})
		return
	}
	provided := c.GetHeader("X-Internal-Token")
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		TelegramID              int64  `json:"telegram_id" binding:"required"`
		Package                 string `json:"package" binding:"required"`
		StarsAmount             int    `json:"stars_amount" binding:"required"`
		TelegramPaymentChargeID string `json:"telegram_payment_charge_id" binding:"required"`
		ProviderPaymentChargeID string `json:"provider_payment_charge_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pkg, ok := services.PremiumPackageByID(req.Package)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown package"})
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

	user, err := h.userRepo.FindByTelegramID(req.TelegramID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_linked"})
		return
	}

	payment := &models.TelegramStarsPayment{
		UserID:                  user.ID,
		TelegramID:              req.TelegramID,
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

	log.Printf("[PREMIUM] Stars grant ok user=%s package=%s expires=%s charge=%s",
		user.ID.Hex(), pkg.ID, expiresAt.Format("2006-01-02"), req.TelegramPaymentChargeID)

	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"user_id":         user.ID.Hex(),
		"premium_expires": expiresAt,
	})
}

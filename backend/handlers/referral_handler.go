package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// referralClaimWindow: only accounts created this recently can claim a code.
const referralClaimWindow = 72 * time.Hour

type ReferralHandler struct {
	repo    *repositories.ReferralRepository
	siteURL string
}

func NewReferralHandler(repo *repositories.ReferralRepository, siteURL string) *ReferralHandler {
	return &ReferralHandler{repo: repo, siteURL: strings.TrimRight(siteURL, "/")}
}

// GetMyReferral GET /api/user/referral — code, share link and stats.
func (h *ReferralHandler) GetMyReferral(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	code, err := h.repo.GetOrCreateCode(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get referral code"})
		return
	}
	stats, _ := h.repo.Stats(ctx, userID)
	base := h.siteURL
	if base == "" {
		base = "https://filmorauz.net"
	}
	c.JSON(http.StatusOK, gin.H{
		"code":            code,
		"link":            base + "/?ref=" + code,
		"invited":         stats.Invited,
		"rewarded":        stats.Rewarded,
		"reward_days":     stats.RewardDays,
		"days_per_friend": services.ReferralRewardDays(),
		"welcome_days":    services.ReferralWelcomeDays(),
	})
}

// ClaimReferral POST /api/user/referral/claim {code} — called by the site
// right after a NEW user's first login when they arrived via ?ref=CODE.
func (h *ReferralHandler) ClaimReferral(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	_, err := h.repo.Claim(ctx, userID, req.Code, referralClaimWindow)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"claimed": true})
	case errors.Is(err, repositories.ErrReferralInvalidCode), errors.Is(err, repositories.ErrReferralSelf),
		errors.Is(err, repositories.ErrReferralAlreadyTaken), errors.Is(err, repositories.ErrReferralTooLate):
		// Expected outcomes — 200 so the client simply forgets the code.
		c.JSON(http.StatusOK, gin.H{"claimed": false, "reason": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim referral"})
	}
}

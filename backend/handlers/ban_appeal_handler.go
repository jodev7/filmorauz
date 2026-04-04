package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/filmorauz/backend/middleware"
	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BanAppealHandler struct {
	banAppealRepo       *repositories.BanAppealRepository
	banHistoryRepo      *repositories.BanHistoryRepository
	userRepo            *repositories.UserRepository
	notificationService *services.NotificationService
}

func NewBanAppealHandler(
	banAppealRepo *repositories.BanAppealRepository,
	banHistoryRepo *repositories.BanHistoryRepository,
	userRepo *repositories.UserRepository,
	notificationService *services.NotificationService,
) *BanAppealHandler {
	return &BanAppealHandler{
		banAppealRepo:       banAppealRepo,
		banHistoryRepo:      banHistoryRepo,
		userRepo:            userRepo,
		notificationService: notificationService,
	}
}

// CreateAppeal handles POST /api/appeals - banned user submits an appeal
func (h *BanAppealHandler) CreateAppeal(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Foydalanuvchi avtorizatsiyadan o'tmagan"})
		return
	}

	userID := userIDStr.(string)

	// Check if user is banned
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Foydalanuvchi topilmadi"})
		return
	}

	if !user.IsBannedActive() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Siz ban emassiz, apellyatsiya yuborishga haqli emassiz"})
		return
	}

	// Check if user already has a pending appeal
	uid, _ := primitive.ObjectIDFromHex(userID)
	existingAppeal, err := h.banAppealRepo.FindPendingAppealByUserID(c.Request.Context(), uid)
	if err == nil && existingAppeal != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Sizning apellyatsiyangiz allaqachon ko'rib chiqilmoqda"})
		return
	}

	var req models.CreateBanAppealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Xabar maydoni talab qilinadi (kamida 10 belgi)"})
		return
	}

	// Get active ban info
	activeBan, _ := h.banHistoryRepo.FindActiveBanByUserID(userID)

	appeal := &models.BanAppeal{
		UserID:     uid,
		Username:   user.TelegramUser,
		TelegramID: fmt.Sprintf("%d", user.TelegramID),
		Message:    req.Message,
	}

	if activeBan != nil {
		appeal.BanHistoryID = activeBan.ID
		appeal.BanReason = activeBan.Reason
		appeal.BanBannedAt = activeBan.BannedAt.Format(time.RFC3339)
		if activeBan.BannedUntil != nil {
			appeal.BanBannedUntil = activeBan.BannedUntil.Format(time.RFC3339)
		}
		appeal.BanBannedByName = activeBan.BannedByUsername
	}

	if err := h.banAppealRepo.Create(c.Request.Context(), appeal); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Apellyatsiya yaratishda xatolik"})
		return
	}

	// Send notification to user confirming appeal submission
	go func() {
		if h.notificationService != nil {
			err := h.notificationService.NotifyAppealSubmitted(c.Request.Context(), uid, appeal.ID.Hex())
			if err != nil {
				log.Printf("[Notification] Failed to send appeal submitted notification: %v", err)
			} else {
				log.Printf("[Notification] Appeal submitted notification sent to user %s", userID)
			}
		}
	}()

	c.JSON(http.StatusCreated, gin.H{
		"message": "Apellyatsiya muvaffaqiyatli yuborildi",
		"appeal":  appeal,
	})
}

// GetMyAppeals handles GET /api/appeals/me - get current user's appeals
func (h *BanAppealHandler) GetMyAppeals(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Foydalanuvchi avtorizatsiyadan o'tmagan"})
		return
	}

	userID := userIDStr.(string)
	uid, _ := primitive.ObjectIDFromHex(userID)

	response, err := h.banAppealRepo.FindByUserID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Apellyatsiyalarni olishda xatolik"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetAppeals handles GET /api/admin/appeals - admin gets all appeals
func (h *BanAppealHandler) GetAppeals(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	status := c.DefaultQuery("status", "all")
	search := c.DefaultQuery("search", "")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 50 {
		perPage = 10
	}

	response, err := h.banAppealRepo.FindAll(c.Request.Context(), status, search, page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Apellyatsiyalarni olishda xatolik"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetAppealStats handles GET /api/admin/appeals/stats - get appeal statistics
func (h *BanAppealHandler) GetAppealStats(c *gin.Context) {
	stats, err := h.banAppealRepo.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Statistikani olishda xatolik"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// ReviewAppeal handles POST /api/admin/appeals/:id/review - admin reviews an appeal
func (h *BanAppealHandler) ReviewAppeal(c *gin.Context) {
	appealID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Noto'g'ri apellyatsiya ID"})
		return
	}

	adminIDStr, _ := c.Get("user_id")
	adminID := adminIDStr.(string)

	var req models.ReviewBanAppealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Noto'g'ri so'rov"})
		return
	}

	// Get the appeal
	appeal, err := h.banAppealRepo.FindByID(c.Request.Context(), appealID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Apellyatsiya topilmadi"})
		return
	}

	if appeal.IsProcessed() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bu apellyatsiya allaqachon ko'rib chiqilgan"})
		return
	}

	adminUID, _ := primitive.ObjectIDFromHex(adminID)

	// Get admin user from database to get display name
	adminUser, err := h.userRepo.FindByID(adminID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Admin foydalanuvchi topilmadi"})
		return
	}
	adminUsername := adminUser.DisplayName
	if adminUsername == "" {
		adminUsername = adminUser.FirstName
	}

	// Update appeal with review
	if err := h.banAppealRepo.UpdateReview(
		c.Request.Context(),
		appealID,
		&req,
		adminUID,
		adminUsername,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Apellyatsiyani yangilashda xatolik"})
		return
	}

	// If approved and unban is requested, unban the user
	if req.Action == "approve" && req.UnbanUser {
		// Check if target user is SuperAdmin - cannot unban SuperAdmin
		targetUser, targetErr := h.userRepo.FindByID(appeal.UserID.Hex())
		if targetErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Foydalanuvchini topishda xatolik"})
			return
		}
		if middleware.IsSuperAdmin(targetUser.Role) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "SuperAdminni bandan chiqarish mumkin emas",
				"message": "SuperAdmin hisobini o'zgartirish mumkin emas",
			})
			return
		}

		// Find and update the active ban in history
		activeBan, err := h.banHistoryRepo.FindActiveBanByUserID(appeal.UserID.Hex())
		if err == nil && activeBan != nil {
			h.banHistoryRepo.UpdateUnban(activeBan.ID.Hex(), adminUID, adminUsername)
		}

		// Remove ban from user
		if err := h.userRepo.RemoveUserBan(appeal.UserID.Hex()); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"message":  "Apellyatsiya tasdiqlandi, lekin foydalanuvchini bandan chiqarishda xatolik yuz berdi",
				"unbanned": false,
			})
			return
		}

		// Send notification to user that appeal was approved and they are unbanned
		go func() {
			if h.notificationService != nil {
				err := h.notificationService.NotifyAppealApproved(c.Request.Context(), appeal.UserID, appeal.ID.Hex())
				if err != nil {
					log.Printf("[Notification] Failed to send appeal approved notification: %v", err)
				} else {
					log.Printf("[Notification] Appeal approved notification sent to user %s", appeal.UserID.Hex())
				}
			}
		}()
	} else if req.Action == "reject" {
		// Send notification to user that appeal was rejected
		go func() {
			if h.notificationService != nil {
				adminNote := ""
				if req.AdminNote != "" {
					adminNote = req.AdminNote
				}
				err := h.notificationService.NotifyAppealRejected(c.Request.Context(), appeal.UserID, appeal.ID.Hex(), adminNote)
				if err != nil {
					log.Printf("[Notification] Failed to send appeal rejected notification: %v", err)
				} else {
					log.Printf("[Notification] Appeal rejected notification sent to user %s", appeal.UserID.Hex())
				}
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Apellyatsiya muvaffaqiyatli ko'rib chiqildi",
		"action":   req.Action,
		"unbanned": req.Action == "approve" && req.UnbanUser,
	})
}

// GetPendingCount handles GET /api/admin/appeals/pending-count - get count of pending appeals
func (h *BanAppealHandler) GetPendingCount(c *gin.Context) {
	count, err := h.banAppealRepo.GetPendingCount(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Xatolik"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

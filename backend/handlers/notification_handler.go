package handlers

import (
	"net/http"
	"strconv"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type NotificationHandler struct {
	notificationService *services.NotificationService
}

func NewNotificationHandler(notificationService *services.NotificationService) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
	}
}

// GetNotifications handles GET /api/notifications
func (h *NotificationHandler) GetNotifications(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Foydalanuvchi avtorizatsiyadan o'tmagan"})
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Noto'g'ri foydalanuvchi ID"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 50 {
		perPage = 20
	}

	response, err := h.notificationService.GetUserNotifications(c.Request.Context(), userID, page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Bildirishnomalarni olishda xatolik"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetUnreadCount handles GET /api/notifications/unread-count
func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Foydalanuvchi avtorizatsiyadan o'tmagan"})
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Noto'g'ri foydalanuvchi ID"})
		return
	}

	count, err := h.notificationService.GetUnreadCount(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Xatolik"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

// MarkAsRead handles PATCH /api/notifications/:id/read
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Foydalanuvchi avtorizatsiyadan o'tmagan"})
		return
	}

	notificationID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Noto'g'ri bildirishnoma ID"})
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Noto'g'ri foydalanuvchi ID"})
		return
	}

	if err := h.notificationService.MarkAsRead(c.Request.Context(), notificationID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Bildirishnomani o'qilgan deb belgilashda xatolik"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Bildirishnoma o'qilgan deb belgilandi"})
}

// MarkAllAsRead handles PATCH /api/notifications/read-all
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Foydalanuvchi avtorizatsiyadan o'tmagan"})
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Noto'g'ri foydalanuvchi ID"})
		return
	}

	if err := h.notificationService.MarkAllAsRead(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Bildirishnomalarni o'qilgan deb belgilashda xatolik"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Barcha bildirishnomalar o'qilgan deb belgilandi"})
}

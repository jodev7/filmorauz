package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

type DailyReportHandler struct {
	reporter *services.DailyReporter
}

func NewDailyReportHandler(reporter *services.DailyReporter) *DailyReportHandler {
	return &DailyReportHandler{reporter: reporter}
}

// Preview GET /api/superadmin/daily-report/preview — yesterday's report text
// and who would receive it, without sending.
func (h *DailyReportHandler) Preview(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	data, err := h.reporter.Collect(ctx, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hisobotni tayyorlab bo'lmadi"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"html":       data.Format(),
		"recipients": len(h.reporter.Recipients(ctx)),
	})
}

// SendNow POST /api/superadmin/daily-report/send — send yesterday's report
// immediately (independent of the daily schedule).
func (h *DailyReportHandler) SendNow(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	sent, total, _, err := h.reporter.Send(ctx, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hisobotni tayyorlab bo'lmadi"})
		return
	}
	if total == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Qabul qiluvchi yo'q: superadmin Telegram ID'si yoki DAILY_REPORT_CHAT_IDS kerak"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": sent, "recipients": total})
}

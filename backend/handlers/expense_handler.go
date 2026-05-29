package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ExpenseHandler serves the superadmin-only project-cost dashboard: manually
// entered expenses plus the automatically tracked Gemini clip-AI spend.
type ExpenseHandler struct {
	expenseRepo *repositories.ExpenseRepository
	aiUsageRepo *repositories.ClipAIUsageRepository
}

func NewExpenseHandler(expenseRepo *repositories.ExpenseRepository, aiUsageRepo *repositories.ClipAIUsageRepository) *ExpenseHandler {
	return &ExpenseHandler{expenseRepo: expenseRepo, aiUsageRepo: aiUsageRepo}
}

// Summary GET /api/superadmin/expenses
// Returns the full expense list plus a combined breakdown: manual categories,
// the clip-AI total (read from clip_ai_usage), and the grand total.
func (h *ExpenseHandler) Summary(c *gin.Context) {
	ctx := c.Request.Context()

	expenses, err := h.expenseRepo.List(ctx)
	if err != nil {
		log.Printf("[EXPENSES] list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load expenses"})
		return
	}

	cats, manualTotal, err := h.expenseRepo.TotalsByCategory(ctx)
	if err != nil {
		log.Printf("[EXPENSES] totals failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to total expenses"})
		return
	}

	// Clip AI spend is tracked automatically; fold its total into the summary.
	var aiCost float64
	if h.aiUsageRepo != nil {
		if totals, terr := h.aiUsageRepo.Totals(ctx); terr == nil && totals != nil {
			aiCost = totals.CostUSD
		} else if terr != nil {
			log.Printf("[EXPENSES] ai usage totals failed (treating as 0): %v", terr)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"expenses":     expenses,
		"categories":   cats,
		"manual_total": manualTotal,
		"ai_clip_cost": aiCost,
		"grand_total":  manualTotal + aiCost,
	})
}

// Create POST /api/superadmin/expenses
func (h *ExpenseHandler) Create(c *gin.Context) {
	var req struct {
		Category   string  `json:"category"`
		Title      string  `json:"title"`
		AmountUSD  float64 `json:"amount_usd"`
		Recurring  bool    `json:"recurring"`
		Note       string  `json:"note"`
		IncurredAt string  `json:"incurred_at"` // optional RFC3339; defaults to now
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Category = strings.TrimSpace(strings.ToLower(req.Category))
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if req.AmountUSD <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount_usd must be greater than 0"})
		return
	}
	if req.Category == "" {
		req.Category = "other"
	}

	incurred := time.Now()
	if strings.TrimSpace(req.IncurredAt) != "" {
		if t, perr := time.Parse(time.RFC3339, req.IncurredAt); perr == nil {
			incurred = t
		}
	}

	createdBy := ""
	if uid, ok := c.Get("user_id"); ok && uid != nil {
		createdBy, _ = uid.(string)
	}

	exp := &models.Expense{
		Category:   req.Category,
		Title:      req.Title,
		AmountUSD:  req.AmountUSD,
		Recurring:  req.Recurring,
		Note:       strings.TrimSpace(req.Note),
		IncurredAt: incurred,
		CreatedBy:  createdBy,
	}
	if err := h.expenseRepo.Create(c.Request.Context(), exp); err != nil {
		log.Printf("[EXPENSES] create failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save expense"})
		return
	}
	c.JSON(http.StatusCreated, exp)
}

// Delete DELETE /api/superadmin/expenses/:id
func (h *ExpenseHandler) Delete(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.expenseRepo.Delete(c.Request.Context(), id); err != nil {
		log.Printf("[EXPENSES] delete failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete expense"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

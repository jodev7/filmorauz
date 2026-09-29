package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdScheduleRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, field := range []string{"player_ad_interval_minutes", "player_ad_max_repeats"} {
		for _, value := range []string{"-1", "1441", "1.5"} {
			for _, method := range []string{http.MethodPost, http.MethodPut} {
				t.Run(method+"/"+field+"/"+value, func(t *testing.T) {
					h := &AdHandler{}
					router := gin.New()
					router.POST("/ads", h.AdminCreateAd)
					router.PUT("/ads/:id", h.AdminUpdateAd)
					url := "/ads"
					if method == http.MethodPut {
						url += "/507f1f77bcf86cd799439011"
					}
					body := fmt.Sprintf(`{"title":"Promo","target_url":"https://example.com","placements":["website"],%q:%s}`, field, value)
					req := httptest.NewRequest(method, url, strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					res := httptest.NewRecorder()
					router.ServeHTTP(res, req)
					if res.Code != http.StatusBadRequest {
						t.Fatalf("status = %d, want 400: %s", res.Code, res.Body.String())
					}
				})
			}
		}
	}
}

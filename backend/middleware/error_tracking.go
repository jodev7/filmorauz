package middleware

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
)

// ErrorTracking records panics (then re-panics so gin's Recovery still
// answers 500 as before) and every 5xx response, grouped per route, into
// the error_groups collection shown on /admin/errors.
func ErrorTracking(repo *repositories.ErrorRepository) gin.HandlerFunc {
	record := func(e repositories.ErrorEvent) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := repo.Record(ctx, e); err != nil {
				log.Printf("[ERRORS] record failed: %v", err)
			}
		}()
	}
	return func(c *gin.Context) {
		if repo == nil {
			c.Next()
			return
		}
		defer func() {
			if rec := recover(); rec != nil {
				route := c.FullPath()
				if route == "" {
					route = c.Request.URL.Path
				}
				record(repositories.ErrorEvent{
					Source:    "server",
					Kind:      "panic",
					Message:   fmt.Sprintf("panic: %v", rec),
					Stack:     string(debug.Stack()),
					URL:       c.Request.Method + " " + route,
					UserID:    toStringValue(c.Value("user_id")),
					UserAgent: c.Request.UserAgent(),
					Status:    500,
				})
				panic(rec)
			}
		}()
		c.Next()
		if status := c.Writer.Status(); status >= 500 {
			route := c.FullPath()
			if route == "" {
				return // unmatched paths aren't worth tracking
			}
			msg := fmt.Sprintf("HTTP %d %s %s", status, c.Request.Method, route)
			if len(c.Errors) > 0 {
				msg += ": " + c.Errors.Last().Error()
			}
			record(repositories.ErrorEvent{
				Source:    "server",
				Kind:      "http5xx",
				Message:   msg,
				URL:       c.Request.Method + " " + c.Request.URL.Path,
				UserID:    toStringValue(c.Value("user_id")),
				UserAgent: c.Request.UserAgent(),
				Status:    status,
			})
		}
	}
}

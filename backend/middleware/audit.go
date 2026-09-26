package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// maxAuditBody caps how much of a JSON request body is inspected/stored.
const maxAuditBody = 16 * 1024

// auditRedactKeys: any body key containing one of these (case-insensitive)
// is stored as "[redacted]".
var auditRedactKeys = []string{"password", "token", "secret", "api_key", "apikey", "authorization", "cookie", "session"}

func redactAuditValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			lk := strings.ToLower(k)
			redacted := false
			for _, needle := range auditRedactKeys {
				if strings.Contains(lk, needle) {
					redacted = true
					break
				}
			}
			if redacted {
				out[k] = "[redacted]"
			} else {
				out[k] = redactAuditValue(val)
			}
		}
		return out
	case []interface{}:
		// Long arrays (e.g. bulk ids) are summarised to keep entries small.
		if len(t) > 20 {
			return map[string]interface{}{"count": len(t), "first": redactAuditValue(t[0])}
		}
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = redactAuditValue(val)
		}
		return out
	case string:
		if len(t) > 500 {
			return t[:500] + "…"
		}
		return t
	default:
		return v
	}
}

// AuditLog records every mutating request (POST/PUT/PATCH/DELETE) that
// passed the admin auth middleware: who, which route and ids, a redacted
// copy of small JSON bodies, the response status and timing. Writes are
// asynchronous so they never slow the request down.
//
// Register it AFTER RequireAdmin/RequireSuperAdmin on a group so that
// user_id/role are already in the context and rejected requests are skipped.
func AuditLog(repo *repositories.AuditLogRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if repo == nil || method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		var body bson.M
		if strings.HasPrefix(c.ContentType(), "application/json") && c.Request.Body != nil &&
			c.Request.ContentLength > 0 && c.Request.ContentLength <= maxAuditBody {
			raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxAuditBody))
			c.Request.Body = io.NopCloser(bytes.NewReader(raw))
			if err == nil && len(raw) > 0 {
				var parsed map[string]interface{}
				if json.Unmarshal(raw, &parsed) == nil {
					if m, ok := redactAuditValue(parsed).(map[string]interface{}); ok {
						body = bson.M(m)
					}
				}
			}
		}

		start := time.Now()
		c.Next()

		params := make(map[string]string, len(c.Params))
		for _, p := range c.Params {
			params[p.Key] = p.Value
		}
		entry := &repositories.AuditLog{
			ActorID:    toStringValue(c.Value("user_id")),
			ActorRole:  toStringValue(c.Value("role")),
			Method:     method,
			Route:      c.FullPath(),
			Path:       c.Request.URL.Path,
			Params:     params,
			Body:       body,
			Status:     c.Writer.Status(),
			IP:         c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			DurationMS: time.Since(start).Milliseconds(),
			CreatedAt:  time.Now(),
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := repo.Insert(ctx, entry); err != nil {
				log.Printf("[AUDIT] insert failed for %s %s: %v", entry.Method, entry.Path, err)
			}
		}()
	}
}

func toStringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}

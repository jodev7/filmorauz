package middleware

import (
	"bytes"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// In-memory TTL response cache for hot, anonymous public GET endpoints
// (trending, featured, etc.). It snapshots the full JSON response and
// replays it for a short window so a burst of identical requests hits
// memory instead of MongoDB. Intentionally tiny and dependency-free —
// no Redis, no eviction beyond TTL expiry (the keyspace is bounded by the
// handful of routes we attach it to × their query variants).

type cachedResponse struct {
	status    int
	body      []byte
	expiresAt time.Time
}

var (
	respCache   = make(map[string]cachedResponse)
	respCacheMu sync.RWMutex
)

// cacheWriter tees the handler's response into a buffer so the middleware
// can store it after the handler returns, while still writing through to
// the real client.
type cacheWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w *cacheWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *cacheWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// CacheResponse caches successful (2xx) responses for the given TTL, keyed
// by full request URI. Only GET requests from anonymous callers are cached —
// an Authorization header bypasses the cache so per-user responses are never
// served to the wrong viewer.
func CacheResponse(ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != "GET" || c.GetHeader("Authorization") != "" {
			c.Next()
			return
		}
		key := c.Request.URL.RequestURI()

		respCacheMu.RLock()
		entry, ok := respCache[key]
		respCacheMu.RUnlock()
		if ok && time.Now().Before(entry.expiresAt) {
			c.Header("X-Cache", "HIT")
			c.Data(entry.status, "application/json; charset=utf-8", entry.body)
			c.Abort()
			return
		}

		writer := &cacheWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = writer
		c.Header("X-Cache", "MISS")
		c.Next()

		status := c.Writer.Status()
		if status >= 200 && status < 300 && writer.buf.Len() > 0 {
			respCacheMu.Lock()
			respCache[key] = cachedResponse{
				status:    status,
				body:      append([]byte(nil), writer.buf.Bytes()...),
				expiresAt: time.Now().Add(ttl),
			}
			respCacheMu.Unlock()
		}
	}
}

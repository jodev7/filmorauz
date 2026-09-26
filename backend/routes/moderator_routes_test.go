package routes

import (
	"testing"

	"github.com/filmorauz/backend/middleware"
	"github.com/gin-gonic/gin"
)

// Every entry on the moderator allowlist must match a route that is really
// registered — otherwise a renamed route silently locks moderators out (or
// a typo hides a missing permission).
func TestModeratorRoutesExist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Handlers are only referenced (method values), never called, so nil
	// receivers are fine for building the route table.
	Setup(r, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	registered := map[string]bool{}
	for _, rt := range r.Routes() {
		registered[rt.Method+" "+rt.Path] = true
	}
	for _, key := range middleware.ModeratorRoutes() {
		if !registered[key] {
			t.Errorf("moderator allowlist entry %q is not a registered route", key)
		}
	}
}

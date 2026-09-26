package middleware

import "strings"

// RoleModerator is a limited staff role: it can moderate the community
// (comments, ban appeals, suggestions, bans of regular users) but cannot
// touch content, ingestion, publishing, money or roles.
const RoleModerator = "moderator"

// IsModerator reports whether role is the moderator role (case-insensitive).
func IsModerator(role string) bool {
	return strings.ToLower(strings.TrimSpace(role)) == RoleModerator
}

// moderatorRoutes lists the admin API routes (method + gin route template)
// a moderator may call. Everything else under /api/admin and
// /api/v1/admin stays admin/superadmin-only.
var moderatorRoutes = map[string]bool{
	// Sidebar badges + command palette.
	"GET /api/admin/overview/badges": true,
	"GET /api/admin/search":          true,

	// Users: read + ban/unban (handlers additionally restrict moderators to
	// targets with the plain "user" role).
	"GET /api/admin/users":             true,
	"GET /api/admin/users/banned":      true,
	"GET /api/admin/users/ban-history": true,
	"POST /api/admin/users/:id/ban":    true,
	"DELETE /api/admin/users/:id/ban":  true,

	// Ban appeals.
	"GET /api/admin/appeals":               true,
	"GET /api/admin/appeals/stats":         true,
	"GET /api/admin/appeals/pending-count": true,
	"POST /api/admin/appeals/:id/review":   true,

	// Content suggestions from users.
	"GET /api/admin/suggestions":       true,
	"GET /api/admin/suggestions/stats": true,
	"GET /api/admin/suggestions/:id":   true,
	"PATCH /api/admin/suggestions/:id": true,
	"POST /api/admin/suggestions/:id/link": true,

	// Comment moderation (settings are read-only for moderators).
	"GET /api/v1/admin/comments":              true,
	"PATCH /api/v1/admin/comments/:id/status": true,
	"DELETE /api/v1/admin/comments/:id":       true,
	"GET /api/v1/admin/comment-settings":      true,
}

// ModeratorAllowed reports whether a moderator may call method+routePath,
// where routePath is gin's route template (c.FullPath()).
func ModeratorAllowed(method, routePath string) bool {
	return moderatorRoutes[method+" "+routePath]
}

// ModeratorRoutes returns a copy of the allowlist (used by tests).
func ModeratorRoutes() []string {
	out := make([]string, 0, len(moderatorRoutes))
	for k := range moderatorRoutes {
		out = append(out, k)
	}
	return out
}

package middleware

import (
	"strings"
	"testing"
)

func TestRedactAuditValue(t *testing.T) {
	in := map[string]interface{}{
		"reason":        "spam",
		"access_token":  "abc",
		"Password":      "x",
		"nested":        map[string]interface{}{"api_key": "k", "ok": 1.0},
		"ids":           make([]interface{}, 50),
		"long":          strings.Repeat("a", 600),
		"session_token": "s",
	}
	out := redactAuditValue(in).(map[string]interface{})

	for _, k := range []string{"access_token", "Password", "session_token"} {
		if out[k] != "[redacted]" {
			t.Errorf("%s not redacted: %v", k, out[k])
		}
	}
	if out["reason"] != "spam" {
		t.Errorf("reason changed: %v", out["reason"])
	}
	nested := out["nested"].(map[string]interface{})
	if nested["api_key"] != "[redacted]" || nested["ok"] != 1.0 {
		t.Errorf("nested redaction wrong: %v", nested)
	}
	if ids, ok := out["ids"].(map[string]interface{}); !ok || ids["count"] != 50 {
		t.Errorf("long array not summarised: %v", out["ids"])
	}
	if s := out["long"].(string); len(s) > 510 {
		t.Errorf("long string not truncated: %d", len(s))
	}
}

func TestModeratorAllowed(t *testing.T) {
	if !ModeratorAllowed("PATCH", "/api/v1/admin/comments/:id/status") {
		t.Error("moderator should moderate comments")
	}
	if !ModeratorAllowed("POST", "/api/admin/users/:id/ban") {
		t.Error("moderator should ban users")
	}
	for _, denied := range []string{
		"DELETE /api/admin/movies/:id",
		"PATCH /api/admin/users/:id/role/:role",
		"PATCH /api/admin/users/:id/premium",
		"PUT /api/v1/admin/comment-settings",
		"GET /api/admin/overview",
	} {
		parts := strings.SplitN(denied, " ", 2)
		if ModeratorAllowed(parts[0], parts[1]) {
			t.Errorf("moderator must NOT be allowed: %s", denied)
		}
	}
}

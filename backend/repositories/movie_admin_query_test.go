package repositories

import (
	"encoding/json"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func filterJSON(t *testing.T, f bson.M) string {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAdminMovieQueryFilter(t *testing.T) {
	if got := filterJSON(t, AdminMovieQuery{}.Filter()); got != "{}" {
		t.Errorf("empty query should match everything, got %s", got)
	}

	approved := filterJSON(t, AdminMovieQuery{Status: "approved"}.Filter())
	if !strings.Contains(approved, `"$exists":false`) {
		t.Errorf("approved must include legacy docs without approval_status: %s", approved)
	}

	if got := filterJSON(t, AdminMovieQuery{Status: "pending"}.Filter()); got != `{"approval_status":"pending"}` {
		t.Errorf("pending filter: %s", got)
	}
	if got := filterJSON(t, AdminMovieQuery{Status: "scheduled"}.Filter()); !strings.Contains(got, `"scheduled_publish_at"`) {
		t.Errorf("scheduled filter: %s", got)
	}

	combo := filterJSON(t, AdminMovieQuery{Status: "pending", Search: "a.b(", Premium: "free", Media: "missing"}.Filter())
	for _, want := range []string{`"$and"`, `"is_premium":{"$ne":true}`, `"master_playlist_url"`, `"approval_status":"pending"`, `a\\.b\\(`} {
		if !strings.Contains(combo, want) {
			t.Errorf("combined filter missing %s: %s", want, combo)
		}
	}
}

func TestAdminMovieQuerySort(t *testing.T) {
	if got := (AdminMovieQuery{}).sortSpec()[0].Key; got != "created_at" {
		t.Errorf("default sort = %s", got)
	}
	if got := (AdminMovieQuery{Sort: "schedule"}).sortSpec()[0].Key; got != "scheduled_publish_at" {
		t.Errorf("schedule sort = %s", got)
	}
}

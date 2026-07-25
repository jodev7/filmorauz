package services

import (
	"testing"
	"time"
)

func TestOnlineSessionsAndCounts(t *testing.T) {
	p := &PresenceService{
		sessions:   make(map[string]*presenceEntry),
		onlineTTL:  2 * time.Minute,
		cleanupInt: 30 * time.Second,
	}

	p.Touch("s1", "user-a", "1.1.1.1", "Chrome UA", nil)
	p.Touch("s2", "user-a", "2.2.2.2", "Safari UA", nil) // same user, second device
	p.Touch("s3", "", "3.3.3.3", "Firefox UA", nil)      // anonymous

	authed, anon, total := p.OnlineCounts()
	if authed != 1 || anon != 1 || total != 2 {
		t.Fatalf("OnlineCounts() = (%d,%d,%d), want (1,1,2)", authed, anon, total)
	}

	// Session list does NOT de-dupe: every device is its own row.
	sessions := p.OnlineSessions()
	if len(sessions) != 3 {
		t.Fatalf("OnlineSessions() len = %d, want 3", len(sessions))
	}

	// IP + UA are captured and refreshed.
	p.Touch("s1", "user-a", "9.9.9.9", "Edge UA", nil)
	for _, s := range p.OnlineSessions() {
		if s.SessionID == "s1" {
			if s.IP != "9.9.9.9" || s.UserAgent != "Edge UA" {
				t.Fatalf("s1 not refreshed: ip=%q ua=%q", s.IP, s.UserAgent)
			}
		}
	}
}

func TestTouchTracksWatchedContent(t *testing.T) {
	p := &PresenceService{
		sessions:   make(map[string]*presenceEntry),
		onlineTTL:  2 * time.Minute,
		cleanupInt: 30 * time.Second,
	}

	find := func(id string) *OnlineSession {
		for _, s := range p.OnlineSessions() {
			if s.SessionID == id {
				return &s
			}
		}
		return nil
	}

	p.Touch("s1", "user-a", "1.1.1.1", "Chrome UA", &SessionActivity{
		Type: "movie", ContentID: "m1", Title: "Interstellar", Slug: "interstellar",
	})
	s := find("s1")
	if s == nil || s.Activity == nil || s.Activity.Title != "Interstellar" {
		t.Fatalf("activity not recorded: %+v", s)
	}
	since := s.Activity.Since
	if since.IsZero() {
		t.Fatal("Since should be set on first report")
	}

	// Staying on the same content keeps the original start time.
	p.Touch("s1", "user-a", "1.1.1.1", "Chrome UA", &SessionActivity{
		Type: "movie", ContentID: "m1", Title: "Interstellar", Slug: "interstellar",
	})
	if got := find("s1").Activity.Since; !got.Equal(since) {
		t.Fatalf("Since = %v, want preserved %v", got, since)
	}

	// Switching content restarts it.
	p.Touch("s1", "user-a", "1.1.1.1", "Chrome UA", &SessionActivity{
		Type: "episode", ContentID: "e9", Title: "S01E02", Slug: "show-s1e2",
	})
	next := find("s1").Activity
	if next.ContentID != "e9" || next.Since.Equal(since) {
		t.Fatalf("switch not tracked: %+v", next)
	}

	// Leaving the watch page clears it.
	p.Touch("s1", "user-a", "1.1.1.1", "Chrome UA", nil)
	if a := find("s1").Activity; a != nil {
		t.Fatalf("activity should be cleared, got %+v", a)
	}
}

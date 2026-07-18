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

	p.Touch("s1", "user-a", "1.1.1.1", "Chrome UA")
	p.Touch("s2", "user-a", "2.2.2.2", "Safari UA") // same user, second device
	p.Touch("s3", "", "3.3.3.3", "Firefox UA")      // anonymous

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
	p.Touch("s1", "user-a", "9.9.9.9", "Edge UA")
	for _, s := range p.OnlineSessions() {
		if s.SessionID == "s1" {
			if s.IP != "9.9.9.9" || s.UserAgent != "Edge UA" {
				t.Fatalf("s1 not refreshed: ip=%q ua=%q", s.IP, s.UserAgent)
			}
		}
	}
}

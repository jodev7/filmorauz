package services

import (
	"sort"
	"sync"
	"time"
)

// PresenceService tracks online sessions (authenticated + anonymous) in memory.
// Sessions are kept alive by client heartbeats and expire after OnlineTTL.
type PresenceService struct {
	mu         sync.RWMutex
	sessions   map[string]*presenceEntry // key: sessionID
	onlineTTL  time.Duration
	cleanupInt time.Duration
}

type presenceEntry struct {
	UserID    string // empty for anonymous visitors
	IP        string
	UserAgent string
	FirstSeen time.Time
	LastSeen  time.Time
}

// OnlineSession is an immutable snapshot of a single live session, returned by
// OnlineSessions for the admin activity list.
type OnlineSession struct {
	SessionID string
	UserID    string // empty for anonymous
	IP        string
	UserAgent string
	FirstSeen time.Time
	LastSeen  time.Time
}

// NewPresenceService starts a background cleanup loop and returns the service.
func NewPresenceService() *PresenceService {
	p := &PresenceService{
		sessions:   make(map[string]*presenceEntry),
		onlineTTL:  2 * time.Minute,
		cleanupInt: 30 * time.Second,
	}
	go p.cleanupLoop()
	return p
}

// Touch records a heartbeat for a given session. userID may be empty for
// anonymous visitors. ip and userAgent are refreshed on every heartbeat so the
// admin list reflects the visitor's current network/device.
func (p *PresenceService) Touch(sessionID, userID, ip, userAgent string) {
	if sessionID == "" {
		return
	}
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.sessions[sessionID]; ok {
		entry.LastSeen = now
		if ip != "" {
			entry.IP = ip
		}
		if userAgent != "" {
			entry.UserAgent = userAgent
		}
		// Promote anonymous → authed if the user just logged in mid-session.
		if userID != "" && entry.UserID == "" {
			entry.UserID = userID
		}
		return
	}
	p.sessions[sessionID] = &presenceEntry{
		UserID:    userID,
		IP:        ip,
		UserAgent: userAgent,
		FirstSeen: now,
		LastSeen:  now,
	}
}

// OnlineCounts returns current online counts: authed, anonymous, total.
// Authed is de-duplicated by user_id (multiple sessions for one user count once).
func (p *PresenceService) OnlineCounts() (authed, anonymous, total int) {
	cutoff := time.Now().Add(-p.onlineTTL)
	p.mu.RLock()
	defer p.mu.RUnlock()
	userSet := make(map[string]struct{})
	for _, e := range p.sessions {
		if e.LastSeen.Before(cutoff) {
			continue
		}
		if e.UserID == "" {
			anonymous++
		} else {
			userSet[e.UserID] = struct{}{}
		}
	}
	authed = len(userSet)
	total = authed + anonymous
	return
}

// OnlineSessions returns a snapshot of all currently-live sessions, sorted by
// most-recently-seen first. Unlike OnlineCounts it does NOT de-duplicate authed
// users — each browser tab/device is its own row so admins can see every active
// session (with its own IP + device).
func (p *PresenceService) OnlineSessions() []OnlineSession {
	cutoff := time.Now().Add(-p.onlineTTL)
	p.mu.RLock()
	out := make([]OnlineSession, 0, len(p.sessions))
	for id, e := range p.sessions {
		if e.LastSeen.Before(cutoff) {
			continue
		}
		out = append(out, OnlineSession{
			SessionID: id,
			UserID:    e.UserID,
			IP:        e.IP,
			UserAgent: e.UserAgent,
			FirstSeen: e.FirstSeen,
			LastSeen:  e.LastSeen,
		})
	}
	p.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}

func (p *PresenceService) cleanupLoop() {
	ticker := time.NewTicker(p.cleanupInt)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-p.onlineTTL)
		p.mu.Lock()
		for id, e := range p.sessions {
			if e.LastSeen.Before(cutoff) {
				delete(p.sessions, id)
			}
		}
		p.mu.Unlock()
	}
}

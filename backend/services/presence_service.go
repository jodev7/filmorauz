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
	Activity  *SessionActivity // what the session is watching right now (nil = nothing)
	FirstSeen time.Time
	LastSeen  time.Time
}

// SessionActivity describes the content a session currently has open — the
// movie or episode watch page it is sitting on. Reported by the client with
// every heartbeat; absent on non-watch pages.
type SessionActivity struct {
	Type      string `json:"type"` // "movie" | "episode"
	ContentID string `json:"content_id"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	URL       string `json:"url"`
	Since     time.Time
}

// OnlineSession is an immutable snapshot of a single live session, returned by
// OnlineSessions for the admin activity list.
type OnlineSession struct {
	SessionID string
	UserID    string // empty for anonymous
	IP        string
	UserAgent string
	Activity  *SessionActivity
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
//
// activity is the content the session currently has open; a nil activity means
// the visitor is not on a watch page, and clears any previously reported one.
// Since is preserved while the session stays on the same content, so the admin
// list can show how long they have been watching it.
func (p *PresenceService) Touch(sessionID, userID, ip, userAgent string, activity *SessionActivity) {
	if sessionID == "" {
		return
	}
	now := time.Now()
	if activity != nil && activity.Since.IsZero() {
		activity.Since = now
	}
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
		if activity != nil && entry.Activity != nil && sameContent(entry.Activity, activity) {
			activity.Since = entry.Activity.Since
		}
		entry.Activity = activity
		return
	}
	p.sessions[sessionID] = &presenceEntry{
		UserID:    userID,
		IP:        ip,
		UserAgent: userAgent,
		Activity:  activity,
		FirstSeen: now,
		LastSeen:  now,
	}
}

// sameContent reports whether two activities point at the same movie/episode.
func sameContent(a, b *SessionActivity) bool {
	if a.ContentID != "" || b.ContentID != "" {
		return a.Type == b.Type && a.ContentID == b.ContentID
	}
	return a.Type == b.Type && a.Slug == b.Slug
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
			Activity:  e.Activity,
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

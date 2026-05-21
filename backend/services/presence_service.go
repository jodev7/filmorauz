package services

import (
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
	UserID   string // empty for anonymous visitors
	LastSeen time.Time
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

// Touch records a heartbeat for a given session. userID may be empty for anonymous.
func (p *PresenceService) Touch(sessionID, userID string) {
	if sessionID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.sessions[sessionID]; ok {
		entry.LastSeen = time.Now()
		// Promote anonymous → authed if the user just logged in mid-session.
		if userID != "" && entry.UserID == "" {
			entry.UserID = userID
		}
		return
	}
	p.sessions[sessionID] = &presenceEntry{
		UserID:   userID,
		LastSeen: time.Now(),
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

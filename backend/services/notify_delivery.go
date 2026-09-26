package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Per-user delivery settings (site / Telegram / web push per category) and
// web push fan-out. Wired with SetDelivery; without it everything behaves
// as before (site + Telegram, no push).

// SetDelivery enables per-user notification settings and web push.
func (s *NotificationService) SetDelivery(prefs *repositories.NotifyPrefsRepository, pusher *WebPusher) {
	s.prefsRepo = prefs
	s.pusher = pusher
}

// Pusher exposes the web push sender (nil when not configured).
func (s *NotificationService) Pusher() *WebPusher { return s.pusher }

// PrefsRepo exposes the settings repository (nil when not wired).
func (s *NotificationService) PrefsRepo() *repositories.NotifyPrefsRepository { return s.prefsRepo }

func (s *NotificationService) channelsFor(ctx context.Context, userID primitive.ObjectID, t models.NotificationType) repositories.ChannelPrefs {
	if s.prefsRepo == nil {
		return repositories.ChannelPrefs{Site: true, Telegram: true, Push: false}
	}
	p, err := s.prefsRepo.Get(ctx, userID)
	if err != nil {
		log.Printf("[NOTIFY] prefs for %s: %v (using defaults)", userID.Hex(), err)
	}
	return p.For(t)
}

// PushPayload is what the service worker receives.
type PushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

// PushToUser sends a payload to every browser the user enabled push on.
// Dead subscriptions are removed. Returns how many deliveries succeeded.
func (s *NotificationService) PushToUser(ctx context.Context, userID primitive.ObjectID, p PushPayload) int {
	if s.pusher == nil || s.prefsRepo == nil {
		return 0
	}
	subs, err := s.prefsRepo.Subscriptions(ctx, userID)
	if err != nil || len(subs) == 0 {
		return 0
	}
	if p.Icon == "" {
		p.Icon = "/icon-192.png"
	}
	if p.URL != "" {
		p.URL = s.absoluteURL(p.URL)
	}
	body, _ := json.Marshal(p)
	sent := 0
	for _, sub := range subs {
		var ps PushSubscription
		ps.Endpoint = sub.Endpoint
		ps.Keys.P256dh = sub.P256dh
		ps.Keys.Auth = sub.Auth
		err := s.pusher.Send(ps, body, 24*time.Hour)
		switch {
		case err == nil:
			sent++
		case errors.Is(err, ErrPushGone):
			_ = s.prefsRepo.DeleteEndpoint(ctx, sub.Endpoint)
		default:
			log.Printf("[PUSH] user %s: %v", userID.Hex(), err)
		}
	}
	return sent
}

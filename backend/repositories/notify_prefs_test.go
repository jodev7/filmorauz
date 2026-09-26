package repositories

import (
	"testing"

	"github.com/filmorauz/backend/models"
)

func TestNotifyPrefsNormalize(t *testing.T) {
	p := NotifyPrefs{
		models.NotifCatComments: {Site: false, Telegram: false, Push: true},
		models.NotifCatAccount:  {Site: false, Telegram: false, Push: false},
		"bogus":                 {Site: true},
	}.Normalize()
	if _, ok := p["bogus"]; ok {
		t.Fatal("unknown category kept")
	}
	if !p[models.NotifCatAccount].Site {
		t.Fatal("account notices must stay on the site")
	}
	if c := p.For(models.NotificationCommentLike); c.Site || c.Telegram || !c.Push {
		t.Fatalf("comment prefs not applied: %+v", c)
	}
	if c := p.For(models.NotificationNewEpisode); !c.Site || !c.Telegram || !c.Push {
		t.Fatalf("missing category should default on: %+v", c)
	}
}

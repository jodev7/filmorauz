package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/filmorauz/backend/handlers"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
)

type fakeLLMSSource struct{}

func (fakeLLMSSource) Titles(_ context.Context, kind string) ([]repositories.LLMSTitle, error) {
	if kind == "movie" {
		return []repositories.LLMSTitle{{Kind: "movie", Slug: "interstellar", Title: "Interstellar", Year: 2014}}, nil
	}
	return []repositories.LLMSTitle{{Kind: "series", Slug: "shogun", Title: "Shogun", Year: 2024}}, nil
}

func (f fakeLLMSSource) BySlug(ctx context.Context, kind, slug string) (*repositories.LLMSTitle, error) {
	list, _ := f.Titles(ctx, kind)
	for i := range list {
		if list[i].Slug == slug {
			return &list[i], nil
		}
	}
	return nil, nil
}

func (fakeLLMSSource) Version(context.Context) string { return "v1" }

// The frontend proxies /llms.txt to the backend; depending on whether the
// configured origin ends in /api the request lands on /llms.txt or
// /api/llms.txt — both must be served.
func TestLLMSRoutesServedAtRootAndUnderAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupExtras(r, ExtraDeps{
		Library: &handlers.LibraryHandler{}, Movies: &handlers.MovieHandler{}, Community: &handlers.CommunityHandler{},
		Referral: &handlers.ReferralHandler{}, Errors: &handlers.ErrorHandler{}, DailyReport: &handlers.DailyReportHandler{},
		History: &handlers.HistoryHandler{}, Lists: &handlers.UserListHandler{}, NotifyPrefs: &handlers.NotifySettingsHandler{},
		Credits: &handlers.CreditsHandler{}, Avatars: &handlers.TelegramAvatarImporter{},
		LLMS: handlers.NewLLMSHandler(fakeLLMSSource{}, "https://filmorauz.net"),
	})

	cases := map[string]string{
		"/llms.txt":                    "Interstellar",
		"/api/llms.txt":                "Interstellar",
		"/llms-full.txt":               "Shogun",
		"/api/llms-full.txt":           "Shogun",
		"/llms/movies/interstellar.md": "Interstellar",
		"/api/llms/series/shogun.md":   "Shogun",
	}
	for path, want := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
			t.Errorf("%s: got %d %q, want 200 containing %q", path, w.Code, w.Body.String()[:min(len(w.Body.String()), 80)], want)
		}
	}
}

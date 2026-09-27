package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
)

type fakeLLMS struct {
	movies, series []repositories.LLMSTitle
	version        string
	titleCalls     int
}

func (f *fakeLLMS) Titles(_ context.Context, kind string) ([]repositories.LLMSTitle, error) {
	f.titleCalls++
	if kind == "series" {
		return f.series, nil
	}
	return f.movies, nil
}
func (f *fakeLLMS) BySlug(_ context.Context, kind, slug string) (*repositories.LLMSTitle, error) {
	list := f.movies
	if kind == "series" {
		list = f.series
	}
	for _, t := range list {
		if t.Slug == slug {
			return &t, nil
		}
	}
	return nil, nil
}
func (f *fakeLLMS) Version(context.Context) string { return f.version }

func sampleLLMS() *fakeLLMS {
	return &fakeLLMS{
		version: "v1",
		movies: []repositories.LLMSTitle{{
			Kind: "movie", Slug: "qasoskorlar-final", Code: "101", Title: "Avengers: Endgame", TitleUz: "Qasoskorlar: Final", OriginalTitle: "Avengers: Endgame",
			DescriptionUz: "Qahramonlar koinotni qutqarish uchun\n oxirgi jangga chiqishadi.", Year: 2019, Duration: 181, Country: "AQSH",
			Genres: []string{"action", "drama"}, Cast: []string{"Robert Downey Jr.", "Chris Evans"}, Director: "Anthony Russo", RatingAvg: 4.6, RatingCount: 48,
		}},
		series: []repositories.LLMSTitle{{Kind: "series", Slug: "kalmar-oyini", TitleUz: "Kalmar o'yini [2021]", Year: 2021, Seasons: 2, Episodes: 16, IsPremium: true, Genres: []string{"thriller"}}},
	}
}

func TestLLMSIndexAndFull(t *testing.T) {
	f := sampleLLMS()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	idx := buildLLMSIndex("https://filmorauz.net", f.movies, f.series, now)
	for _, want := range []string{
		"# FilmoraUz\n\n> FilmoraUz",
		"Katalogda 1 ta kino va 1 ta serial bor",
		"- [Kinolar](https://filmorauz.net/movies):",
		"- [Qasoskorlar: Final (2019)](https://filmorauz.net/movies/qasoskorlar-final): Jangari, Drama · AQSH · 3 soat 1 daqiqa — Qahramonlar koinotni qutqarish uchun oxirgi jangga chiqishadi. ([md](https://filmorauz.net/llms/movies/qasoskorlar-final.md))",
		// brackets in titles must not break the link
		"- [Kalmar o'yini (2021) (2021)](https://filmorauz.net/series/kalmar-oyini): Triller · 2 fasl, 16 qism · Premium",
		"## Optional",
		"https://filmorauz.net/llms-full.txt",
	} {
		if !strings.Contains(idx, want) {
			t.Errorf("index missing %q\n---\n%s", want, idx)
		}
	}
	full := buildLLMSFull("https://filmorauz.net", f.movies, f.series, now)
	for _, want := range []string{"### Qasoskorlar: Final (2019)", "- Rollarda: Robert Downey Jr., Chris Evans", "- Rejissyor: Anthony Russo", "- Foydalanuvchilar bahosi: 4.6/5 (48 ta baho)", "- Kino kodi (Telegram bot uchun): 101", "- Fasllar: 2, qismlar: 16", "Premium obuna bilan"} {
		if !strings.Contains(full, want) {
			t.Errorf("full missing %q", want)
		}
	}
}

func TestLLMSSnapshotRebuildsOnlyWhenCatalogueChanges(t *testing.T) {
	f := sampleLLMS()
	h := NewLLMSHandler(f, "https://filmorauz.net/")
	ctx := context.Background()
	a, _, _ := h.snapshot(ctx, false)
	calls := f.titleCalls
	h.checkedAt = time.Time{} // pretend a minute passed
	b, _, _ := h.snapshot(ctx, false)
	if f.titleCalls != calls || a != b {
		t.Fatalf("unchanged version must reuse the cache (calls %d→%d)", calls, f.titleCalls)
	}
	// A new movie is published → version changes → rebuilt on next check.
	f.movies = append(f.movies, repositories.LLMSTitle{Kind: "movie", Slug: "yangi", TitleUz: "Yangi kino"})
	f.version = "v2"
	h.checkedAt = time.Time{}
	c, _, _ := h.snapshot(ctx, false)
	if !strings.Contains(c, "[Yangi kino](https://filmorauz.net/movies/yangi)") {
		t.Fatalf("new movie missing after version change:\n%s", c)
	}
}

func TestLLMSTitleRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewLLMSHandler(sampleLLMS(), "https://filmorauz.net")
	r.GET("/llms/movies/:file", h.GetMovie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/llms/movies/qasoskorlar-final.md", nil))
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "# Qasoskorlar: Final (2019)") || !strings.Contains(w.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("got %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/llms/movies/yoq.md", nil))
	if w.Code != 404 {
		t.Fatalf("missing slug should 404, got %d", w.Code)
	}
}

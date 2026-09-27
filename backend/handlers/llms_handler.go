package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
)

// llms.txt (https://llmstxt.org): a Markdown index of the site for AI
// assistants and crawlers, plus one Markdown page per movie/series.
// Rebuilt as soon as the catalogue changes (see LLMSRepository.Version).
//
//	/llms.txt                 site summary + every title with a one-line blurb
//	/llms-full.txt            every title with full details
//	/llms/movies/{slug}.md    one movie
//	/llms/series/{slug}.md    one series

type llmsSource interface {
	Titles(ctx context.Context, kind string) ([]repositories.LLMSTitle, error)
	BySlug(ctx context.Context, kind, slug string) (*repositories.LLMSTitle, error)
	Version(ctx context.Context) string
}

type LLMSHandler struct {
	repo llmsSource
	base string

	mu          sync.Mutex
	version     string
	checkedAt   time.Time
	builtAt     time.Time
	index, full string
	movies      int
	series      int
}

func NewLLMSHandler(repo llmsSource, baseSiteURL string) *LLMSHandler {
	base := strings.TrimRight(strings.TrimSpace(baseSiteURL), "/")
	if base == "" {
		base = "https://filmorauz.net"
	}
	return &LLMSHandler{repo: repo, base: base}
}

const (
	llmsVersionCheckEvery = time.Minute   // how often to ask Mongo whether anything changed
	llmsMaxAge            = 6 * time.Hour // rebuild at least this often anyway
	llmsBlurbLen          = 180
)

// snapshot returns the current files, rebuilding them when the catalogue
// changed. Concurrent requests share one rebuild.
func (h *LLMSHandler) snapshot(ctx context.Context, force bool) (index, full string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	fresh := h.index != "" && now.Sub(h.builtAt) < llmsMaxAge
	if fresh && !force && now.Sub(h.checkedAt) < llmsVersionCheckEvery {
		return h.index, h.full, nil
	}
	v := h.repo.Version(ctx)
	h.checkedAt = now
	if fresh && !force && v == h.version {
		return h.index, h.full, nil
	}
	movies, err := h.repo.Titles(ctx, "movie")
	if err != nil {
		if h.index != "" {
			return h.index, h.full, nil // serve the last good copy
		}
		return "", "", err
	}
	series, err := h.repo.Titles(ctx, "series")
	if err != nil {
		if h.index != "" {
			return h.index, h.full, nil
		}
		return "", "", err
	}
	h.index = buildLLMSIndex(h.base, movies, series, now)
	h.full = buildLLMSFull(h.base, movies, series, now)
	h.version, h.builtAt, h.movies, h.series = v, now, len(movies), len(series)
	return h.index, h.full, nil
}

func writeMarkdown(c *gin.Context, body string) {
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("X-Robots-Tag", "noindex") // for AI readers, not a search result page
	c.String(http.StatusOK, body)
}

// GetIndex GET /llms.txt
func (h *LLMSHandler) GetIndex(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	index, _, err := h.snapshot(ctx, false)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "llms.txt is temporarily unavailable")
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=300")
	c.String(http.StatusOK, index)
}

// GetFull GET /llms-full.txt
func (h *LLMSHandler) GetFull(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	_, full, err := h.snapshot(ctx, false)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "llms-full.txt is temporarily unavailable")
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=300")
	c.String(http.StatusOK, full)
}

func (h *LLMSHandler) title(c *gin.Context, kind string) {
	slug := strings.TrimSuffix(strings.TrimSpace(c.Param("file")), ".md")
	if slug == "" || len(slug) > 200 {
		c.String(http.StatusNotFound, "not found")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	t, err := h.repo.BySlug(ctx, kind, slug)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "temporarily unavailable")
		return
	}
	if t == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	writeMarkdown(c, buildLLMSTitlePage(h.base, *t))
}

// GetMovie GET /llms/movies/:file   (file = "{slug}.md")
func (h *LLMSHandler) GetMovie(c *gin.Context) { h.title(c, "movie") }

// GetSeries GET /llms/series/:file
func (h *LLMSHandler) GetSeries(c *gin.Context) { h.title(c, "series") }

// Status GET /api/admin/seo/llms — for the admin SEO page.
func (h *LLMSHandler) Status(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	force := c.Request.Method == http.MethodPost
	index, full, err := h.snapshot(ctx, force)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	c.JSON(http.StatusOK, gin.H{
		"movies":      h.movies,
		"series":      h.series,
		"built_at":    h.builtAt,
		"index_bytes": len(index),
		"full_bytes":  len(full),
		"index_url":   h.base + "/llms.txt",
		"full_url":    h.base + "/llms-full.txt",
	})
}

// ── Markdown builders (pure, tested) ──

func llmsGenres(t repositories.LLMSTitle) []string {
	if len(t.GenresUz) > 0 {
		return t.GenresUz
	}
	out := make([]string, 0, len(t.Genres))
	for _, g := range t.Genres {
		if l, ok := homepageGenreLabels[strings.ToLower(g)]; ok {
			out = append(out, l)
		} else if g != "" && g != "main" {
			out = append(out, strings.ToUpper(g[:1])+g[1:])
		}
	}
	return out
}

// oneLine collapses whitespace and cuts at a word boundary.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:—-") + "…"
}

// mdEscape keeps titles from breaking Markdown link syntax.
func mdEscape(s string) string {
	return strings.NewReplacer("[", "(", "]", ")", "\n", " ").Replace(strings.TrimSpace(s))
}

func llmsPath(t repositories.LLMSTitle) (page, md string) {
	if t.Kind == "series" {
		return "/series/" + t.Slug, "/llms/series/" + t.Slug + ".md"
	}
	return "/movies/" + t.Slug, "/llms/movies/" + t.Slug + ".md"
}

func llmsLabel(t repositories.LLMSTitle) string {
	label := mdEscape(t.DisplayTitle())
	if t.Year > 1900 {
		label += fmt.Sprintf(" (%d)", t.Year)
	}
	return label
}

// facts is the short "Drama, Triller · AQSH · 2 soat 10 daqiqa" line.
func llmsFacts(t repositories.LLMSTitle) string {
	var parts []string
	if g := llmsGenres(t); len(g) > 0 {
		parts = append(parts, strings.Join(g, ", "))
	}
	if c := strings.TrimSpace(t.Country); c != "" {
		parts = append(parts, c)
	}
	if t.Kind == "series" {
		if t.Seasons > 0 {
			parts = append(parts, fmt.Sprintf("%d fasl, %d qism", t.Seasons, t.Episodes))
		}
	} else if t.Duration > 0 {
		parts = append(parts, llmsDuration(t.Duration))
	}
	if t.IsPremium {
		parts = append(parts, "Premium")
	}
	return strings.Join(parts, " · ")
}

func llmsDuration(min int) string {
	if min >= 60 {
		if min%60 == 0 {
			return fmt.Sprintf("%d soat", min/60)
		}
		return fmt.Sprintf("%d soat %d daqiqa", min/60, min%60)
	}
	return fmt.Sprintf("%d daqiqa", min)
}

func llmsHeader(b *strings.Builder, base string, movies, series int, now time.Time) {
	b.WriteString("# FilmoraUz\n\n")
	b.WriteString("> FilmoraUz (" + base + ") — o'zbek tilidagi onlayn kinoteatr: kinolar, seriallar, multfilmlar, anime va doramalarni o'zbek tilida HD sifatda onlayn tomosha qilish mumkin. ")
	b.WriteString("Ro'yxatdan o'tish Telegram orqali; ko'p kontent bepul, ayrimlari Premium obuna bilan.\n\n")
	fmt.Fprintf(b, "Katalogda %d ta kino va %d ta serial bor. Ro'yxat avtomatik yangilanadi (oxirgi yangilanish: %s UTC).\n", movies, series, now.UTC().Format("2006-01-02 15:04"))
	b.WriteString("Har bir kino yoki serial sahifasining Markdown nusxasi `.md` havolada. Kino kodi (masalan, 101) Telegram botga yuborilsa, bot o'sha kinoni topib beradi.\n\n")
}

func buildLLMSIndex(base string, movies, series []repositories.LLMSTitle, now time.Time) string {
	var b strings.Builder
	llmsHeader(&b, base, len(movies), len(series), now)

	b.WriteString("## Asosiy sahifalar\n\n")
	for _, p := range [][3]string{
		{"Bosh sahifa", "/", "yangi, mashhur va tavsiya etilgan kinolar"},
		{"Kinolar", "/movies", "barcha kinolar: janr, yil, davlat va reyting bo'yicha filtr"},
		{"Seriallar", "/series", "barcha seriallar fasl va qismlari bilan"},
		{"Janrlar", "/genres", "janr bo'yicha kinolar"},
		{"To'plamlar", "/collections", "mavzuli kino to'plamlari"},
		{"Premium", "/premium", "reklamasiz tomosha, yuqori sifat va yangi kinolar"},
		{"Mualliflik huquqi", "/copyright", "kontent egalari uchun murojaat"},
	} {
		fmt.Fprintf(&b, "- [%s](%s%s): %s\n", p[0], base, p[1], p[2])
	}

	writeList := func(title string, list []repositories.LLMSTitle) {
		if len(list) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n## %s\n\n", title)
		for _, t := range list {
			page, md := llmsPath(t)
			line := fmt.Sprintf("- [%s](%s%s)", llmsLabel(t), base, page)
			var info []string
			if f := llmsFacts(t); f != "" {
				info = append(info, f)
			}
			if d := t.DisplayDescription(); d != "" {
				info = append(info, oneLine(d, llmsBlurbLen))
			}
			if len(info) > 0 {
				line += ": " + strings.Join(info, " — ")
			}
			line += fmt.Sprintf(" ([md](%s%s))", base, md)
			b.WriteString(line + "\n")
		}
	}
	writeList("Kinolar", movies)
	writeList("Seriallar", series)

	b.WriteString("\n## Optional\n\n")
	fmt.Fprintf(&b, "- [To'liq ma'lumot](%s/llms-full.txt): har bir kino va serialning to'liq tavsifi, aktyorlari va rejissyori\n", base)
	fmt.Fprintf(&b, "- [Sitemap](%s/sitemap.xml): qidiruv tizimlari uchun barcha sahifalar\n", base)
	return b.String()
}

func writeLLMSTitle(b *strings.Builder, base string, t repositories.LLMSTitle, level string) {
	page, _ := llmsPath(t)
	fmt.Fprintf(b, "%s %s\n\n", level, llmsLabel(t))
	kind := "Kino"
	if t.Kind == "series" {
		kind = "Serial"
	}
	fmt.Fprintf(b, "- Turi: %s\n", kind)
	fmt.Fprintf(b, "- Tomosha qilish: %s%s\n", base, page)
	if o := strings.TrimSpace(t.OriginalTitle); o != "" && !strings.EqualFold(o, t.DisplayTitle()) {
		fmt.Fprintf(b, "- Asl nomi: %s\n", o)
	}
	if t.Year > 1900 {
		fmt.Fprintf(b, "- Yil: %d\n", t.Year)
	}
	if g := llmsGenres(t); len(g) > 0 {
		fmt.Fprintf(b, "- Janr: %s\n", strings.Join(g, ", "))
	}
	if c := strings.TrimSpace(t.Country); c != "" {
		fmt.Fprintf(b, "- Davlat: %s\n", c)
	}
	if t.Kind == "series" {
		if t.Seasons > 0 {
			fmt.Fprintf(b, "- Fasllar: %d, qismlar: %d\n", t.Seasons, t.Episodes)
		}
	} else if t.Duration > 0 {
		fmt.Fprintf(b, "- Davomiyligi: %s\n", llmsDuration(t.Duration))
	}
	if d := strings.TrimSpace(t.Director); d != "" {
		label := "Rejissyor"
		if t.Kind == "series" {
			label = "Yaratuvchi"
		}
		fmt.Fprintf(b, "- %s: %s\n", label, d)
	}
	if len(t.Cast) > 0 {
		cast := t.Cast
		if len(cast) > 10 {
			cast = cast[:10]
		}
		fmt.Fprintf(b, "- Rollarda: %s\n", strings.Join(cast, ", "))
	}
	if q := strings.TrimSpace(t.Quality); q != "" {
		fmt.Fprintf(b, "- Sifat: %s\n", q)
	}
	if t.RatingCount > 0 {
		fmt.Fprintf(b, "- Foydalanuvchilar bahosi: %.1f/5 (%d ta baho)\n", t.RatingAvg, t.RatingCount)
	}
	if t.Code != "" && t.Kind == "movie" {
		fmt.Fprintf(b, "- Kino kodi (Telegram bot uchun): %s\n", t.Code)
	}
	access := "Bepul, o'zbek tilida"
	if t.IsPremium {
		access = "Premium obuna bilan, o'zbek tilida"
	}
	fmt.Fprintf(b, "- Kirish: %s\n", access)
	if d := t.DisplayDescription(); d != "" {
		b.WriteString("\n" + strings.TrimSpace(d) + "\n")
	}
	b.WriteString("\n")
}

func buildLLMSFull(base string, movies, series []repositories.LLMSTitle, now time.Time) string {
	var b strings.Builder
	llmsHeader(&b, base, len(movies), len(series), now)
	if len(movies) > 0 {
		b.WriteString("## Kinolar\n\n")
		for _, t := range movies {
			writeLLMSTitle(&b, base, t, "###")
		}
	}
	if len(series) > 0 {
		b.WriteString("## Seriallar\n\n")
		for _, t := range series {
			writeLLMSTitle(&b, base, t, "###")
		}
	}
	return b.String()
}

func buildLLMSTitlePage(base string, t repositories.LLMSTitle) string {
	var b strings.Builder
	writeLLMSTitle(&b, base, t, "#")
	fmt.Fprintf(&b, "---\n\nFilmoraUz — o'zbek tilidagi onlayn kinoteatr. Boshqa kinolar: %s/movies · Seriallar: %s/series · To'liq ro'yxat: %s/llms.txt\n", base, base, base)
	return b.String()
}

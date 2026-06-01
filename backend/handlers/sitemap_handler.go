package handlers

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SitemapHandler serves the sitemap index and all per-section sitemaps:
//
//	/sitemap.xml           — index of every sub-sitemap below
//	/sitemap-static.xml    — home + listing pages (movies, series, genres)
//	/sitemap-genres.xml    — one URL per genre slug
//	/sitemap-movies.xml    — every published movie detail page
//	/sitemap-series.xml    — every series + every season URL
//	/sitemap-episodes.xml  — every episode (SEO URL or /episode/:id fallback)
//	/sitemap-videos.xml    — Google Video sitemap (<video:video>) for movies
//
// Splitting like this keeps each file well below Google's 50k-URL / 50MB
// cap and lets Search Console report partial failures per section.
type SitemapHandler struct {
	movieRepo         *repositories.MovieRepository
	seriesRepo        *repositories.SeriesRepository
	clipRepo          *repositories.ClipRepository
	collectionService *services.CollectionService
	baseSiteURL       string
}

func NewSitemapHandler(movieRepo *repositories.MovieRepository, seriesRepo *repositories.SeriesRepository, clipRepo *repositories.ClipRepository, collectionService *services.CollectionService, baseSiteURL string) *SitemapHandler {
	trimmed := strings.TrimRight(strings.TrimSpace(baseSiteURL), "/")
	if trimmed == "" {
		trimmed = "https://filmorauz.net"
	}
	return &SitemapHandler{
		movieRepo:         movieRepo,
		seriesRepo:        seriesRepo,
		clipRepo:          clipRepo,
		collectionService: collectionService,
		baseSiteURL:       trimmed,
	}
}

// XML payload types -----------------------------------------------------

type sitemapURL struct {
	Loc        string  `xml:"loc"`
	LastMod    string  `xml:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	XmlnsVi string       `xml:"xmlns:video,attr,omitempty"`
	XmlnsIm string       `xml:"xmlns:image,attr,omitempty"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapIndexEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type sitemapIndex struct {
	XMLName  xml.Name            `xml:"sitemapindex"`
	Xmlns    string              `xml:"xmlns,attr"`
	Sitemaps []sitemapIndexEntry `xml:"sitemap"`
}

// Video sitemap types — separate struct so we don't ship empty <video>
// blocks on the regular sitemaps.
type videoSitemapURL struct {
	Loc   string         `xml:"loc"`
	Video *videoSitemapV `xml:"video:video,omitempty"`
}

type videoSitemapV struct {
	ThumbnailLoc    string  `xml:"video:thumbnail_loc"`
	Title           string  `xml:"video:title"`
	Description     string  `xml:"video:description"`
	PlayerLoc       string  `xml:"video:player_loc,omitempty"`
	ContentLoc      string  `xml:"video:content_loc,omitempty"`
	Duration        int     `xml:"video:duration,omitempty"`
	PublicationDate string  `xml:"video:publication_date,omitempty"`
	FamilyFriendly  string  `xml:"video:family_friendly,omitempty"`
	Rating          float64 `xml:"video:rating,omitempty"`
	Live            string  `xml:"video:live,omitempty"`
}

type videoURLSet struct {
	XMLName xml.Name          `xml:"urlset"`
	Xmlns   string             `xml:"xmlns,attr"`
	XmlnsVi string             `xml:"xmlns:video,attr"`
	URLs    []videoSitemapURL  `xml:"url"`
}

// Routes ----------------------------------------------------------------

// GetSitemapIndex serves /sitemap.xml — the index pointing at sub-sitemaps.
func (h *SitemapHandler) GetSitemapIndex(c *gin.Context) {
	now := time.Now().UTC().Format(time.RFC3339)
	index := sitemapIndex{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		Sitemaps: []sitemapIndexEntry{
			{Loc: h.baseSiteURL + "/sitemap-static.xml", LastMod: now},
			{Loc: h.baseSiteURL + "/sitemap-genres.xml", LastMod: now},
			{Loc: h.baseSiteURL + "/sitemap-movies.xml", LastMod: now},
			{Loc: h.baseSiteURL + "/sitemap-series.xml", LastMod: now},
			{Loc: h.baseSiteURL + "/sitemap-episodes.xml", LastMod: now},
			{Loc: h.baseSiteURL + "/sitemap-collections.xml", LastMod: now},
			{Loc: h.baseSiteURL + "/sitemap-videos.xml", LastMod: now},
		},
	}
	writeXML(c, index)
}

// GetSitemapStatic serves the home + listing pages.
func (h *SitemapHandler) GetSitemapStatic(c *gin.Context) {
	now := time.Now().UTC().Format(time.RFC3339)
	set := newURLSet()
	set.URLs = []sitemapURL{
		{Loc: h.baseSiteURL + "/", LastMod: now, ChangeFreq: "daily", Priority: 1.0},
		{Loc: h.baseSiteURL + "/movies", LastMod: now, ChangeFreq: "daily", Priority: 0.9},
		{Loc: h.baseSiteURL + "/series", LastMod: now, ChangeFreq: "daily", Priority: 0.9},
		{Loc: h.baseSiteURL + "/genres", LastMod: now, ChangeFreq: "weekly", Priority: 0.7},
		{Loc: h.baseSiteURL + "/collections", LastMod: now, ChangeFreq: "weekly", Priority: 0.7},
		{Loc: h.baseSiteURL + "/premium", LastMod: now, ChangeFreq: "monthly", Priority: 0.4},
	}
	writeXML(c, set)
}

// GetSitemapGenres serves one URL per known genre. Genres come from both
// movies and series so we can serve a unified /genres/<slug> page.
func (h *SitemapHandler) GetSitemapGenres(c *gin.Context) {
	movies, _ := h.movieRepo.ListPublishedForSitemap()
	seriesList, _ := h.seriesRepo.ListPublishedForSitemap()

	genres := collectGenres(movies, seriesList)
	now := time.Now().UTC().Format(time.RFC3339)
	set := newURLSet()
	for _, g := range genres {
		set.URLs = append(set.URLs, sitemapURL{
			Loc:        h.baseSiteURL + "/genres/" + g,
			LastMod:    now,
			ChangeFreq: "weekly",
			Priority:   0.7,
		})
	}
	writeXML(c, set)
}

// GetSitemapMovies serves every published movie detail page.
func (h *SitemapHandler) GetSitemapMovies(c *gin.Context) {
	movies, err := h.movieRepo.ListPublishedForSitemap()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load movies")
		return
	}
	set := newURLSet()
	for _, m := range movies {
		if strings.TrimSpace(m.Slug) == "" {
			continue
		}
		set.URLs = append(set.URLs, sitemapURL{
			Loc:        h.baseSiteURL + "/movies/" + m.Slug,
			LastMod:    formatTime(m.UpdatedAt),
			ChangeFreq: "weekly",
			Priority:   0.8,
		})
	}
	writeXML(c, set)
}

// GetSitemapSeries serves series + season URLs.
func (h *SitemapHandler) GetSitemapSeries(c *gin.Context) {
	seriesList, err := h.seriesRepo.ListPublishedForSitemap()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load series")
		return
	}
	ids := make([]primitive.ObjectID, 0, len(seriesList))
	for _, s := range seriesList {
		ids = append(ids, s.ID)
	}
	seasons, _ := h.seriesRepo.GetSeasonsBySeriesIDs(ids)
	seriesByID := make(map[primitive.ObjectID]repositories.SitemapSeriesRecord, len(seriesList))
	for _, s := range seriesList {
		seriesByID[s.ID] = s
	}

	set := newURLSet()
	for _, s := range seriesList {
		if strings.TrimSpace(s.Slug) == "" {
			continue
		}
		set.URLs = append(set.URLs, sitemapURL{
			Loc:        h.baseSiteURL + "/series/" + s.Slug,
			LastMod:    formatTime(s.UpdatedAt),
			ChangeFreq: "weekly",
			Priority:   0.8,
		})
	}
	for _, season := range seasons {
		series, ok := seriesByID[season.SeriesID]
		if !ok || strings.TrimSpace(series.Slug) == "" || season.SeasonNumber <= 0 {
			continue
		}
		set.URLs = append(set.URLs, sitemapURL{
			Loc:        fmt.Sprintf("%s/series/%s/season/%d", h.baseSiteURL, series.Slug, season.SeasonNumber),
			LastMod:    formatTime(season.UpdatedAt),
			ChangeFreq: "weekly",
			Priority:   0.7,
		})
	}
	writeXML(c, set)
}

// GetSitemapCollections serves published collection detail URLs.
func (h *SitemapHandler) GetSitemapCollections(c *gin.Context) {
	set := newURLSet()
	if h.collectionService != nil {
		if cols, err := h.collectionService.GetAll(c.Request.Context()); err == nil {
			for _, col := range cols {
				if !col.IsPublished || strings.TrimSpace(col.Slug) == "" {
					continue
				}
				set.URLs = append(set.URLs, sitemapURL{
					Loc:        h.baseSiteURL + "/collections/" + col.Slug,
					LastMod:    formatTime(col.UpdatedAt),
					ChangeFreq: "weekly",
					Priority:   0.6,
				})
			}
		} else {
			log.Printf("[sitemap-collections] load failed: %v", err)
		}
	}
	writeXML(c, set)
}

// GetSitemapEpisodes serves the canonical SEO episode URL when slug +
// season + episode are all known, falling back to /episode/:id otherwise.
func (h *SitemapHandler) GetSitemapEpisodes(c *gin.Context) {
	seriesList, err := h.seriesRepo.ListPublishedForSitemap()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load series")
		return
	}
	ids := make([]primitive.ObjectID, 0, len(seriesList))
	for _, s := range seriesList {
		ids = append(ids, s.ID)
	}
	seasons, _ := h.seriesRepo.GetSeasonsBySeriesIDs(ids)
	episodes, _ := h.seriesRepo.GetEpisodesBySeriesIDs(ids)

	seriesByID := make(map[primitive.ObjectID]repositories.SitemapSeriesRecord, len(seriesList))
	for _, s := range seriesList {
		seriesByID[s.ID] = s
	}
	seasonByID := make(map[primitive.ObjectID]repositories.SitemapSeasonRecord, len(seasons))
	for _, season := range seasons {
		seasonByID[season.ID] = season
	}

	set := newURLSet()
	for _, ep := range episodes {
		series, seriesOK := seriesByID[ep.SeriesID]
		season, seasonOK := seasonByID[ep.SeasonID]
		if !seriesOK || !seasonOK {
			continue
		}
		var loc string
		if strings.TrimSpace(series.Slug) != "" && season.SeasonNumber > 0 && ep.EpisodeNumber > 0 {
			loc = fmt.Sprintf("%s/series/%s/season/%d/episode/%d", h.baseSiteURL, series.Slug, season.SeasonNumber, ep.EpisodeNumber)
		} else if !ep.ID.IsZero() {
			loc = fmt.Sprintf("%s/episode/%s", h.baseSiteURL, ep.ID.Hex())
		} else {
			continue
		}
		set.URLs = append(set.URLs, sitemapURL{
			Loc:        loc,
			LastMod:    formatTime(ep.UpdatedAt),
			ChangeFreq: "weekly",
			Priority:   0.6,
		})
	}
	writeXML(c, set)
}

// GetSitemapVideos serves the Google Video sitemap. One <video:video>
// block per movie and one per episode (episode thumbnail falls back to
// the series poster when absent — Google rejects entries with no
// <video:thumbnail_loc>).
func (h *SitemapHandler) GetSitemapVideos(c *gin.Context) {
	movies, err := h.movieRepo.ListPublishedForSitemap()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load movies")
		return
	}
	set := videoURLSet{
		Xmlns:   "http://www.sitemaps.org/schemas/sitemap/0.9",
		XmlnsVi: "http://www.google.com/schemas/sitemap-video/1.1",
	}
	// Prefer a real MP4 clip for <video:content_loc> (Google wants raw video
	// bytes, and HLS .m3u8 is a playlist). We already generate ~15 vertical
	// MP4 clips per title — reuse the first as the indexable media file.
	var movieClipURLs, episodeClipURLs map[primitive.ObjectID]string
	if h.clipRepo != nil {
		movieClipURLs, _ = h.clipRepo.MovieClipURLs(c.Request.Context())
		episodeClipURLs, _ = h.clipRepo.EpisodeClipURLs(c.Request.Context())
	}
	for _, m := range movies {
		if strings.TrimSpace(m.Slug) == "" {
			continue
		}
		title := strings.TrimSpace(m.Title)
		if title == "" {
			title = m.Slug
		}
		desc := strings.TrimSpace(m.Description)
		if desc == "" {
			desc = title
		}
		if len(desc) > 2000 {
			desc = desc[:2000]
		}
		poster := strings.TrimSpace(m.PosterURL)
		if poster == "" {
			// Google requires a thumbnail — skip videos that don't have one.
			continue
		}
		// Google Video sitemaps require absolute URLs. Posters stored with
		// the relative "/images/..." path (DEV uploads or B2-key-only rows)
		// must be prefixed with the site URL or Search Console rejects the
		// entry with "Invalid URL, Missing thumbnail".
		poster = absoluteMediaURL(poster, h.baseSiteURL)
		pub := m.CreatedAt
		if pub.IsZero() {
			pub = m.UpdatedAt
		}
		loc := h.baseSiteURL + "/movies/" + m.Slug
		// <video:content_loc> must point at the actual media file, not the
		// HTML landing page (Google rejects entries where content_loc/player_loc
		// equals <loc>). Use the HLS master playlist (fallback: video_url).
		stream := absoluteMediaURL(strings.TrimSpace(firstNonEmpty(movieClipURLs[m.ID], m.MasterPlaylistURL, m.VideoURL)), h.baseSiteURL)
		if stream == "" || stream == loc {
			continue
		}
		set.URLs = append(set.URLs, videoSitemapURL{
			Loc: loc,
			Video: &videoSitemapV{
				ThumbnailLoc:    poster,
				Title:           title,
				Description:     desc,
				ContentLoc:      stream,
				Duration:        m.Duration * 60,
				PublicationDate: formatTime(pub),
				FamilyFriendly:  "yes",
				Live:            "no",
			},
		})
	}
	movieCount := len(set.URLs)

	// Episodes: emit a <video:video> per episode. Falls back to the series
	// poster when the episode has no thumbnail_url of its own — Google
	// rejects entries that are missing the <video:thumbnail_loc> tag.
	if seriesList, err := h.seriesRepo.ListPublishedForSitemap(); err == nil {
		ids := make([]primitive.ObjectID, 0, len(seriesList))
		seriesByID := make(map[primitive.ObjectID]repositories.SitemapSeriesRecord, len(seriesList))
		for _, s := range seriesList {
			ids = append(ids, s.ID)
			seriesByID[s.ID] = s
		}
		seasons, _ := h.seriesRepo.GetSeasonsBySeriesIDs(ids)
		seasonByID := make(map[primitive.ObjectID]repositories.SitemapSeasonRecord, len(seasons))
		for _, season := range seasons {
			seasonByID[season.ID] = season
		}
		episodes, _ := h.seriesRepo.GetEpisodesBySeriesIDs(ids)
		for _, ep := range episodes {
			series, sOK := seriesByID[ep.SeriesID]
			season, snOK := seasonByID[ep.SeasonID]
			if !sOK || !snOK {
				continue
			}
			// Episode thumbnails were populated by an older importer with a
			// dead-domain placeholder (https://uzmovi.tv/images/ogimage.png —
			// uzmovi.tv is offline, see worker/parser fix earlier today).
			// Google rejects sitemap entries whose <video:thumbnail_loc>
			// 404s, so treat any uzmovi.tv URL as empty and let the series
			// poster carry the entry.
			thumb := strings.TrimSpace(ep.ThumbnailURL)
			if thumb != "" && strings.Contains(strings.ToLower(thumb), "uzmovi.tv/") {
				thumb = ""
			}
			if thumb == "" {
				thumb = strings.TrimSpace(series.PosterURL)
			}
			if thumb == "" {
				continue
			}
			thumb = absoluteMediaURL(thumb, h.baseSiteURL)

			var loc string
			if strings.TrimSpace(series.Slug) != "" && season.SeasonNumber > 0 && ep.EpisodeNumber > 0 {
				loc = fmt.Sprintf("%s/series/%s/season/%d/episode/%d", h.baseSiteURL, series.Slug, season.SeasonNumber, ep.EpisodeNumber)
			} else if !ep.ID.IsZero() {
				loc = fmt.Sprintf("%s/episode/%s", h.baseSiteURL, ep.ID.Hex())
			} else {
				continue
			}
			// <video:content_loc> must be the real media file, not the landing
			// page. Skip episodes with no stream yet (still processing).
			stream := absoluteMediaURL(strings.TrimSpace(firstNonEmpty(episodeClipURLs[ep.ID], ep.MasterPlaylistURL, ep.VideoURL)), h.baseSiteURL)
			if stream == "" || stream == loc {
				continue
			}

			title := strings.TrimSpace(ep.Title)
			if title == "" {
				seriesTitle := strings.TrimSpace(series.Title)
				if seriesTitle == "" {
					seriesTitle = series.Slug
				}
				title = fmt.Sprintf("%s — S%dE%d", seriesTitle, season.SeasonNumber, ep.EpisodeNumber)
			}
			desc := strings.TrimSpace(series.Description)
			if desc == "" {
				desc = title
			}
			if len(desc) > 2000 {
				desc = desc[:2000]
			}
			pub := ep.CreatedAt
			if pub.IsZero() {
				pub = ep.UpdatedAt
			}
			set.URLs = append(set.URLs, videoSitemapURL{
				Loc: loc,
				Video: &videoSitemapV{
					ThumbnailLoc:    thumb,
					Title:           title,
					Description:     desc,
					ContentLoc:      stream,
					Duration:        ep.Duration * 60,
					PublicationDate: formatTime(pub),
					FamilyFriendly:  "yes",
					Live:            "no",
				},
			})
		}
	}

	log.Printf("[sitemap-videos] generated entries=%d (movies=%d episodes=%d)", len(set.URLs), movieCount, len(set.URLs)-movieCount)
	writeXML(c, set)
}

// GetSitemap is a backward-compatible alias for the legacy /sitemap.xml
// route that used to return one giant flat sitemap. It now serves the
// index instead.
func (h *SitemapHandler) GetSitemap(c *gin.Context) {
	h.GetSitemapIndex(c)
}

// GetRobotsTxt serves the canonical robots.txt with sitemap pointer.
func (h *SitemapHandler) GetRobotsTxt(c *gin.Context) {
	body := strings.Join([]string{
		"User-agent: *",
		"Allow: /",
		"Disallow: /admin",
		"Disallow: /api",
		"Disallow: /user",
		"Disallow: /profile",
		"Disallow: /login",
		"Disallow: /uploads",
		"Disallow: /notifications",
		"Disallow: /banned",
		"Disallow: /watch-room",
		"Disallow: /rooms",
		"",
		"User-agent: Googlebot",
		"Allow: /",
		"",
		"User-agent: Yandex",
		"Allow: /",
		fmt.Sprintf("Host: %s", strings.TrimPrefix(strings.TrimPrefix(h.baseSiteURL, "https://"), "http://")),
		"",
		"User-agent: Bingbot",
		"Allow: /",
		"",
		fmt.Sprintf("Sitemap: %s/sitemap.xml", h.baseSiteURL),
		"",
	}, "\n")
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=600")
	c.String(http.StatusOK, body)
}

// Helpers ---------------------------------------------------------------

func newURLSet() sitemapURLSet {
	return sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9"}
}

func writeXML(c *gin.Context, payload any) {
	xmlBytes, err := xml.MarshalIndent(payload, "", "  ")
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to render sitemap")
		return
	}
	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=300")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write([]byte(xml.Header))
	_, _ = c.Writer.Write(xmlBytes)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func collectGenres(movies []repositories.SitemapMovieRecord, seriesList []repositories.SitemapSeriesRecord) []string {
	set := make(map[string]struct{})
	for _, m := range movies {
		for _, g := range m.Genre {
			g = strings.ToLower(strings.TrimSpace(g))
			if g != "" {
				set[g] = struct{}{}
			}
		}
	}
	for _, s := range seriesList {
		for _, g := range s.Genre {
			g = strings.ToLower(strings.TrimSpace(g))
			if g != "" {
				set[g] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for g := range set {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}


// absoluteMediaURL turns a possibly-relative media path (e.g.
// "/images/posters/foo.webp" or "images/posters/foo.webp") into an
// absolute URL rooted at baseSiteURL. URLs that already start with
// "http://" / "https://" are returned unchanged.
func absoluteMediaURL(raw, base string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		return base + raw
	}
	return base + "/" + raw
}

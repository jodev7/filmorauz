package handlers

import (
	"encoding/xml"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SitemapHandler struct {
	movieRepo   *repositories.MovieRepository
	seriesRepo  *repositories.SeriesRepository
	baseSiteURL string
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapEntry struct {
	url        string
	lastMod    time.Time
	hasLastMod bool
}

func NewSitemapHandler(movieRepo *repositories.MovieRepository, seriesRepo *repositories.SeriesRepository, baseSiteURL string) *SitemapHandler {
	trimmed := strings.TrimRight(strings.TrimSpace(baseSiteURL), "/")
	if trimmed == "" {
		trimmed = "https://filmorauz.net"
	}
	return &SitemapHandler{
		movieRepo:   movieRepo,
		seriesRepo:  seriesRepo,
		baseSiteURL: trimmed,
	}
}

func (h *SitemapHandler) GetSitemap(c *gin.Context) {
	movies, err := h.movieRepo.ListPublishedForSitemap()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load movies")
		return
	}

	seriesList, err := h.seriesRepo.ListPublishedForSitemap()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load series")
		return
	}

	seriesIDs := make([]primitive.ObjectID, 0, len(seriesList))
	genreSet := make(map[string]struct{})
	mostRecentMovieUpdate := latestMovieTime(movies)
	mostRecentSeriesUpdate := latestSeriesTime(seriesList)

	for _, movie := range movies {
		for _, genre := range movie.Genre {
			if genre == "" {
				continue
			}
			genreSet[strings.ToLower(strings.TrimSpace(genre))] = struct{}{}
		}
	}

	for _, series := range seriesList {
		seriesIDs = append(seriesIDs, series.ID)
		for _, genre := range series.Genre {
			if genre == "" {
				continue
			}
			genreSet[strings.ToLower(strings.TrimSpace(genre))] = struct{}{}
		}
	}

	seasons, err := h.seriesRepo.GetSeasonsBySeriesIDs(seriesIDs)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load seasons")
		return
	}

	episodes, err := h.seriesRepo.GetEpisodesBySeriesIDs(seriesIDs)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load episodes")
		return
	}

	seriesByID := make(map[primitive.ObjectID]repositories.SitemapSeriesRecord, len(seriesList))
	for _, series := range seriesList {
		seriesByID[series.ID] = series
	}

	seasonByID := make(map[primitive.ObjectID]repositories.SitemapSeasonRecord, len(seasons))
	mostRecentSeasonUpdate := latestSeasonTime(seasons)
	for _, season := range seasons {
		seasonByID[season.ID] = season
	}

	genres := make([]string, 0, len(genreSet))
	for genre := range genreSet {
		genres = append(genres, genre)
	}
	sort.Strings(genres)

	rootLastMod := latestNonZero(mostRecentMovieUpdate, mostRecentSeriesUpdate, mostRecentSeasonUpdate)
	moviesLastMod := latestNonZero(mostRecentMovieUpdate, rootLastMod)
	seriesLastMod := latestNonZero(mostRecentSeriesUpdate, mostRecentSeasonUpdate, rootLastMod)
	genresLastMod := latestNonZero(rootLastMod, moviesLastMod, seriesLastMod)

	urls := make([]sitemapEntry, 0, 4+len(genres)+len(movies)+len(seriesList)+len(seasons)+len(episodes))
	seen := make(map[string]struct{})
	add := func(path string, updatedAt time.Time) {
		if strings.TrimSpace(path) == "" {
			return
		}
		fullURL := h.baseSiteURL + path
		if _, exists := seen[fullURL]; exists {
			return
		}
		seen[fullURL] = struct{}{}
		entry := sitemapEntry{url: fullURL}
		if !updatedAt.IsZero() {
			entry.lastMod = updatedAt.UTC()
			entry.hasLastMod = true
		}
		urls = append(urls, entry)
	}

	add("/", rootLastMod)
	add("/movies", moviesLastMod)
	add("/series", seriesLastMod)
	add("/genres", genresLastMod)

	for _, genre := range genres {
		add("/genres/"+genre, genresLastMod)
	}

	for _, movie := range movies {
		if strings.TrimSpace(movie.Slug) == "" {
			continue
		}
		add("/movies/"+movie.Slug, movie.UpdatedAt)
	}

	for _, series := range seriesList {
		if strings.TrimSpace(series.Slug) == "" {
			continue
		}
		add("/series/"+series.Slug, series.UpdatedAt)
	}

	for _, season := range seasons {
		series, ok := seriesByID[season.SeriesID]
		if !ok || strings.TrimSpace(series.Slug) == "" || season.SeasonNumber <= 0 {
			continue
		}
		add("/series/"+series.Slug+"/season/"+intToString(season.SeasonNumber), season.UpdatedAt)
	}

	episodeCount := 0
	for _, episode := range episodes {
		series, seriesOK := seriesByID[episode.SeriesID]
		season, seasonOK := seasonByID[episode.SeasonID]
		if !seriesOK || !seasonOK {
			continue
		}
		if episode.ID.IsZero() {
			continue
		}
		hasSeoRoute := strings.TrimSpace(series.Slug) != "" && season.SeasonNumber > 0 && episode.EpisodeNumber > 0
		if hasSeoRoute {
			continue
		}
		add("/episode/"+episode.ID.Hex(), episode.UpdatedAt)
		episodeCount++
	}

	response := sitemapURLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  make([]sitemapURL, 0, len(urls)),
	}
	for _, entry := range urls {
		item := sitemapURL{Loc: entry.url}
		if entry.hasLastMod {
			item.LastMod = entry.lastMod.Format(time.RFC3339)
		}
		response.URLs = append(response.URLs, item)
	}

	xmlBytes, err := xml.MarshalIndent(response, "", "  ")
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to render sitemap")
		return
	}

	log.Printf("[sitemap] generated movies=%d series=%d seasons=%d episodes=%d total=%d", len(movies), len(seriesList), len(seasons), episodeCount, len(response.URLs))

	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=300")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write([]byte(xml.Header))
	_, _ = c.Writer.Write(xmlBytes)
}

func latestMovieTime(items []repositories.SitemapMovieRecord) time.Time {
	var latest time.Time
	for _, item := range items {
		if item.UpdatedAt.After(latest) {
			latest = item.UpdatedAt
		}
	}
	return latest
}

func latestSeriesTime(items []repositories.SitemapSeriesRecord) time.Time {
	var latest time.Time
	for _, item := range items {
		if item.UpdatedAt.After(latest) {
			latest = item.UpdatedAt
		}
	}
	return latest
}

func latestSeasonTime(items []repositories.SitemapSeasonRecord) time.Time {
	var latest time.Time
	for _, item := range items {
		if item.UpdatedAt.After(latest) {
			latest = item.UpdatedAt
		}
	}
	return latest
}

func latestNonZero(values ...time.Time) time.Time {
	var latest time.Time
	for _, value := range values {
		if value.IsZero() {
			continue
		}
		if latest.IsZero() || value.After(latest) {
			latest = value
		}
	}
	return latest
}

func intToString(value int) string {
	return strconv.Itoa(value)
}

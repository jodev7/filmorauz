package handlers

import (
	"net/http"
	"sort"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

type HomepageHandler struct {
	movieService      *services.MovieService
	collectionService *services.CollectionService
	seriesService     *services.SeriesService
}

func NewHomepageHandler(movieService *services.MovieService, collectionService *services.CollectionService, seriesService *services.SeriesService) *HomepageHandler {
	return &HomepageHandler{
		movieService:      movieService,
		collectionService: collectionService,
		seriesService:     seriesService,
	}
}

func (h *HomepageHandler) GetHomepageData(c *gin.Context) {
	// Pull a larger recency pool than we render so genre rows have enough
	// distinct material to draw from after de-duplication.
	movies, _, err := h.movieService.ListMovies("", 1, 60)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch homepage movies"})
		return
	}

	trending, err := h.movieService.GetTrendingMovies("24h", 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch homepage trending"})
		return
	}

	featuredCollections, err := h.collectionService.GetFeatured(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch homepage collections"})
		return
	}

	seriesList, err := h.seriesService.ListSeries(10, 0, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch homepage series"})
		return
	}

	if movies == nil {
		movies = []models.Movie{}
	}
	for i := range movies {
		protectMovieMedia(&movies[i])
	}
	for i := range featuredCollections {
		featuredCollections[i].PosterURL = protectMediaURL(featuredCollections[i].PosterURL)
		for j := range featuredCollections[i].Movies {
			featuredCollections[i].Movies[j].PosterURL = protectMediaURL(featuredCollections[i].Movies[j].PosterURL)
		}
	}
	for i := range seriesList {
		protectSeriesMedia(&seriesList[i])
	}
	if featuredCollections == nil {
		featuredCollections = []models.CollectionWithMovies{}
	}
	if seriesList == nil {
		seriesList = []models.Series{}
	}

	genres := []gin.H{
		{"label": "Jangari", "slug": "action"},
		{"label": "Drama", "slug": "drama"},
		{"label": "Komediya", "slug": "comedy"},
		{"label": "Fantastika", "slug": "sci-fi"},
		{"label": "Triller", "slug": "thriller"},
		{"label": "Multifilmlar", "slug": "animation"},
		{"label": "Anime", "slug": "anime"},
		{"label": "Dorama", "slug": "dorama"},
		{"label": "Romantika", "slug": "romance"},
		{"label": "Dahshat", "slug": "horror"},
		{"label": "Klassika", "slug": "classic"},
	}

	trendingResponse := make([]gin.H, len(trending))
	for i, t := range trending {
		trendingResponse[i] = gin.H{
			"id":              t.Movie.ID.Hex(),
			"title":           t.Movie.Title,
			"slug":            t.Movie.Slug,
			"poster_url":      protectMediaURL(t.Movie.PosterURL),
			"backdrop_url":    protectMediaURL(t.Movie.BackdropURL),
			"year":            t.Movie.Year,
			"genre":           t.Movie.Genre,
			"views_in_period": t.ViewsInPeriod,
		}
	}

	hero := movies
	if len(hero) > 6 {
		hero = hero[:6]
	}

	newMovies := movies
	if len(newMovies) > 12 {
		newMovies = newMovies[:12]
	}

	// Cross-row de-duplication: with a small, heavily multi-genre catalogue,
	// every recency-ordered row otherwise echoes the same newest-import batch.
	// Track movies already shown in earlier rows so later rows surface fresh
	// titles. "Yangi filmlar" (newMovies) is the recency anchor, so seed the
	// used-set with it; hero is a banner and intentionally not excluded.
	used := make(map[string]bool)
	for i := range newMovies {
		used[newMovies[i].ID.Hex()] = true
	}

	// Top-rated row — ranked by audience rating, not recency. Exclude anything
	// already shown so it never repeats the "new movies" row.
	topRatedPool, err := h.movieService.ListTopRated(24)
	if err != nil {
		topRatedPool = []models.Movie{}
	}
	topRated := make([]models.Movie, 0, 12)
	for i := range topRatedPool {
		id := topRatedPool[i].ID.Hex()
		if used[id] {
			continue
		}
		protectMovieMedia(&topRatedPool[i])
		topRated = append(topRated, topRatedPool[i])
		used[id] = true
		if len(topRated) >= 12 {
			break
		}
	}

	// Genre discovery rows — each whitelisted genre is queried directly from the
	// DB so its carousel is filled with up to maxRowMovies titles, independent of
	// the recency pool or what earlier rows showed. Rows may share a title (a
	// movie tagged both comedy and action appears in both), which is intentional:
	// a full "Komediya" row matters more here than cross-row uniqueness.
	genreRows := h.buildGenreRows()

	premiumMovies := make([]models.Movie, 0, 12)
	for i := range movies {
		if movies[i].IsPremium {
			premiumMovies = append(premiumMovies, movies[i])
			if len(premiumMovies) >= 12 {
				break
			}
		}
	}

	seriesPreview := seriesList
	if len(seriesPreview) > 10 {
		seriesPreview = seriesPreview[:10]
	}

	heroResponse := make([]gin.H, len(hero))
	for i, movie := range hero {
		heroResponse[i] = gin.H{
			"id":           movie.ID.Hex(),
			"title":        movie.Title,
			"description":  movie.Description,
			"poster_url":   movie.PosterURL,
			"backdrop_url": movie.BackdropURL,
			"year":         movie.Year,
			"genre":        movie.Genre,
			"slug":         movie.Slug,
			"duration":     movie.Duration,
			"quality":      movie.Quality,
		}
	}

	seriesResponse := make([]gin.H, len(seriesPreview))
	for i, series := range seriesPreview {
		seriesResponse[i] = gin.H{
			"id":           series.ID.Hex(),
			"slug":         series.Slug,
			"title":        series.Title,
			"poster_url":   series.PosterURL,
			"year":         series.Year,
			"genre":        series.Genre,
			"rating_avg":   series.RatingAvg,
			"is_premium":   series.IsPremium,
			"is_completed": series.IsCompleted,
		}
	}

	// Weekly Top 10 — most-viewed titles across BOTH movies and series, ranked
	// purely by view count. Uses dedicated view-sorted queries (not the recency
	// pools) so the ranking is correct and series naturally mix in when they
	// out-rank movies. Each item carries a "type" for frontend routing.
	topMoviesPool, err := h.movieService.ListMostViewed(12)
	if err != nil {
		topMoviesPool = []models.Movie{}
	}
	topSeriesPool, err := h.seriesService.ListMostViewed(12)
	if err != nil {
		topSeriesPool = []models.Series{}
	}
	type rankedItem struct {
		card  gin.H
		views int64
	}
	ranked := make([]rankedItem, 0, len(topMoviesPool)+len(topSeriesPool))
	for i := range topMoviesPool {
		m := &topMoviesPool[i]
		ranked = append(ranked, rankedItem{
			views: m.Views,
			card: gin.H{
				"id":         m.ID.Hex(),
				"type":       "movie",
				"title":      m.Title,
				"slug":       m.Slug,
				"poster_url": protectMediaURL(m.PosterURL),
				"views":      m.Views,
			},
		})
	}
	for i := range topSeriesPool {
		s := &topSeriesPool[i]
		ranked = append(ranked, rankedItem{
			views: s.Views,
			card: gin.H{
				"id":         s.ID.Hex(),
				"type":       "series",
				"title":      s.Title,
				"slug":       s.Slug,
				"poster_url": protectMediaURL(s.PosterURL),
				"views":      s.Views,
			},
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].views > ranked[j].views
	})
	weeklyTop := make([]gin.H, 0, 10)
	for i := 0; i < len(ranked) && i < 10; i++ {
		weeklyTop = append(weeklyTop, ranked[i].card)
	}

	c.JSON(http.StatusOK, gin.H{
		"hero":                 heroResponse,
		"genres":               genres,
		"new_movies":           movieCards(newMovies),
		"trending":             trendingResponse,
		"premium_movies":       movieCards(premiumMovies),
		"top_rated":            movieCards(topRated),
		"genre_rows":           genreRows,
		"featured_collections": featuredCollections,
		"series":               seriesResponse,
		"weekly_top":           weeklyTop,
	})
}

// movieCards maps movies to the compact card shape the homepage carousels use.
func movieCards(list []models.Movie) []gin.H {
	out := make([]gin.H, len(list))
	for i, movie := range list {
		out[i] = gin.H{
			"id":           movie.ID.Hex(),
			"code":         movie.Code,
			"title":        movie.Title,
			"poster_url":   movie.PosterURL,
			"backdrop_url": movie.BackdropURL,
			"slug":         movie.Slug,
			"year":         movie.Year,
			"genre":        movie.Genre,
			"duration":     movie.Duration,
			"quality":      movie.Quality,
			"is_premium":   movie.IsPremium,
			"rating_avg":   movie.RatingAvg,
			"created_at":   movie.CreatedAt,
		}
	}
	return out
}

// homepageGenreLabels is the display-name whitelist for dynamic genre rows.
// Genres not listed here (and the junk "main" umbrella tag) are never rendered
// as their own row, so scraper artefacts don't leak onto the homepage.
var homepageGenreLabels = map[string]string{
	"action":    "Jangari",
	"drama":     "Drama",
	"comedy":    "Komediya",
	"thriller":  "Triller",
	"horror":    "Dahshat",
	"crime":     "Jinoyat",
	"fantasy":   "Fantastika",
	"animation": "Multfilmlar",
	"detective": "Detektiv",
	"romance":   "Romantika",
	"sci-fi":    "Ilmiy-fantastika",
	"family":    "Oilaviy",
	"history":   "Tarixiy",
	"western":   "Vestern",
	"anime":     "Anime",
}

// homepageGenreRowOrder is the priority order in which genre carousels are
// considered for the homepage. The first maxGenreRows genres that have at least
// minGenreRowSize published titles are rendered, so editorially important genres
// (comedy near the top) reliably surface.
var homepageGenreRowOrder = []string{
	"action", "comedy", "drama", "thriller", "horror", "crime",
	"fantasy", "sci-fi", "romance", "animation", "anime",
	"detective", "family", "history", "western",
}

// buildGenreRows emits one carousel per genre, querying each genre directly from
// the DB so every row is filled independently (up to maxRowMovies) rather than
// scavenged from a shared recency pool. Genres with fewer than minGenreRowSize
// titles are skipped; the first maxGenreRows qualifying genres are rendered.
func (h *HomepageHandler) buildGenreRows() []gin.H {
	const (
		minGenreRowSize = 4
		maxGenreRows    = 6
		maxRowMovies    = 12
	)

	rows := make([]gin.H, 0, maxGenreRows)
	for _, slug := range homepageGenreRowOrder {
		if len(rows) >= maxGenreRows {
			break
		}
		label, ok := homepageGenreLabels[slug]
		if !ok {
			continue
		}
		list, err := h.movieService.ListByGenre(slug, maxRowMovies)
		if err != nil || len(list) < minGenreRowSize {
			continue
		}
		for i := range list {
			protectMovieMedia(&list[i])
		}
		rows = append(rows, gin.H{
			"label":  label,
			"slug":   slug,
			"movies": movieCards(list),
		})
	}
	return rows
}

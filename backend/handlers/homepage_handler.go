package handlers

import (
	"net/http"
	"sort"
	"strings"

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

	// Genre discovery rows — instead of three fixed, overlapping genres, pick
	// the genres with the most still-unshown movies so each row is genuinely
	// distinct. Junk/umbrella genres are skipped, and any genre without enough
	// fresh titles is dropped rather than rendered half-empty.
	genreRows := buildGenreRows(movies, used)

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

// buildGenreRows picks the genres with the most still-unshown movies and emits
// one carousel per genre, drawing only titles not already used by earlier rows.
// Each emitted movie is marked used so genre rows never repeat each other or the
// "new movies" row. Genres below minGenreRowSize fresh titles are skipped.
func buildGenreRows(pool []models.Movie, used map[string]bool) []gin.H {
	const (
		minGenreRowSize = 4
		maxGenreRows    = 4
		maxRowMovies    = 12
	)

	// Group still-unshown pool movies by whitelisted genre.
	byGenre := make(map[string][]models.Movie)
	for i := range pool {
		if used[pool[i].ID.Hex()] {
			continue
		}
		for _, g := range pool[i].Genre {
			slug := strings.ToLower(strings.TrimSpace(g))
			if slug == "" {
				continue
			}
			if _, ok := homepageGenreLabels[slug]; !ok {
				continue
			}
			byGenre[slug] = append(byGenre[slug], pool[i])
		}
	}

	// Rank genres by how much fresh material they have; ties broken by slug so
	// the ordering is deterministic across requests.
	type genreCount struct {
		slug string
		n    int
	}
	order := make([]genreCount, 0, len(byGenre))
	for slug, list := range byGenre {
		order = append(order, genreCount{slug, len(list)})
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].n != order[j].n {
			return order[i].n > order[j].n
		}
		return order[i].slug < order[j].slug
	})

	rows := make([]gin.H, 0, maxGenreRows)
	for _, gc := range order {
		if len(rows) >= maxGenreRows {
			break
		}
		row := make([]models.Movie, 0, maxRowMovies)
		for _, m := range byGenre[gc.slug] {
			id := m.ID.Hex()
			if used[id] {
				continue
			}
			row = append(row, m)
			used[id] = true
			if len(row) >= maxRowMovies {
				break
			}
		}
		if len(row) < minGenreRowSize {
			// Not enough fresh titles once earlier rows claimed theirs — release
			// the few we reserved so a later genre can still use them.
			for i := range row {
				used[row[i].ID.Hex()] = false
			}
			continue
		}
		rows = append(rows, gin.H{
			"label":  homepageGenreLabels[gc.slug],
			"slug":   gc.slug,
			"movies": movieCards(row),
		})
	}
	return rows
}

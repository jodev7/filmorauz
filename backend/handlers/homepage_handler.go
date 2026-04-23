package handlers

import (
	"net/http"

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
	movies, _, err := h.movieService.ListMovies("", 1, 20)
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
		{"label": "Multfilm", "slug": "animation"},
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
			"poster_url":      t.Movie.PosterURL,
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

	featuredMovies := []models.Movie{}
	if len(movies) > 1 {
		featuredSubset := append([]models.Movie(nil), movies[1:]...)
		if len(featuredSubset) > 6 {
			featuredSubset = featuredSubset[:6]
		}
		featuredMovies = featuredSubset
	}

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

	toMovieCardResponse := func(list []models.Movie) []gin.H {
		out := make([]gin.H, len(list))
		for i, movie := range list {
			out[i] = gin.H{
				"id":         movie.ID.Hex(),
				"code":       movie.Code,
				"title":      movie.Title,
				"poster_url": movie.PosterURL,
				"slug":       movie.Slug,
				"year":       movie.Year,
				"genre":      movie.Genre,
				"duration":   movie.Duration,
				"quality":    movie.Quality,
				"is_premium": movie.IsPremium,
				"rating_avg": movie.RatingAvg,
				"created_at": movie.CreatedAt,
			}
		}
		return out
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
		"new_movies":           toMovieCardResponse(newMovies),
		"trending":             trendingResponse,
		"premium_movies":       toMovieCardResponse(premiumMovies),
		"featured_movies":      toMovieCardResponse(featuredMovies),
		"featured_collections": featuredCollections,
		"series":               seriesResponse,
	})
}

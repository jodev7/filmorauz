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

	// Top-rated row — genuinely distinct from "new movies" (which is just the
	// most recent uploads). Ranked by audience rating, not recency.
	topRated, err := h.movieService.ListTopRated(12)
	if err != nil {
		topRated = []models.Movie{}
	}
	for i := range topRated {
		protectMovieMedia(&topRated[i])
	}

	// Genre discovery rows — surface a few popular genres as their own carousels
	// so the homepage is more than recency-ordered lists. Empty genres are
	// skipped so we never render a heading with no cards.
	genreRowDefs := []struct{ label, slug string }{
		{"Jangari", "action"},
		{"Drama", "drama"},
		{"Komediya", "comedy"},
	}
	genreRows := make([]gin.H, 0, len(genreRowDefs))

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

	for _, def := range genreRowDefs {
		genreMovies, gErr := h.movieService.ListByGenre(def.slug, 12)
		if gErr != nil || len(genreMovies) == 0 {
			continue
		}
		for i := range genreMovies {
			protectMovieMedia(&genreMovies[i])
		}
		genreRows = append(genreRows, gin.H{
			"label":  def.label,
			"slug":   def.slug,
			"movies": toMovieCardResponse(genreMovies),
		})
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
		"top_rated":            toMovieCardResponse(topRated),
		"genre_rows":           genreRows,
		"featured_collections": featuredCollections,
		"series":               seriesResponse,
	})
}

package handlers

import (
	"github.com/filmorauz/backend/models"
	"github.com/gin-gonic/gin"
)

// Watching requires an account. Browse/detail endpoints stay public so SEO and
// the catalogue keep working, but the playback sources are held back until the
// request carries a session — otherwise a guest could lift the (already signed)
// media URL straight out of the JSON and skip the login gate entirely.

// isAuthedRequest reports whether the request carried a valid session. Routes
// must be wired with middleware.OptionalAuth for this to ever be true.
func isAuthedRequest(c *gin.Context) bool {
	return c.GetString("user_id") != ""
}

// stripMoviePlayback blanks every field a player could stream from.
func stripMoviePlayback(movie *models.Movie) {
	if movie == nil {
		return
	}
	movie.VideoURL = ""
	movie.MasterPlaylistURL = ""
	movie.EmbedURL = ""
}

// stripEpisodePlayback is stripMoviePlayback for episodes.
func stripEpisodePlayback(episode *models.Episode) {
	if episode == nil {
		return
	}
	episode.VideoURL = ""
	episode.MasterPlaylistURL = ""
	episode.EmbedURL = ""
}

// stripMoviePlaybackForGuests blanks playback sources unless the caller is
// logged in.
func stripMoviePlaybackForGuests(c *gin.Context, movie *models.Movie) {
	if !isAuthedRequest(c) {
		stripMoviePlayback(movie)
	}
}

// stripEpisodePlaybackForGuests blanks playback sources unless the caller is
// logged in.
func stripEpisodePlaybackForGuests(c *gin.Context, episode *models.Episode) {
	if !isAuthedRequest(c) {
		stripEpisodePlayback(episode)
	}
}

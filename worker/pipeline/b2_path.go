package pipeline

import (
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/filmorauz/worker/models"
)

// B2 root prefixes for video assets. New uploads must always go under one of
// these roots — never under `videos/movies/serials/...`. Old DB rows that
// still point at `videos/movies/serials/...` remain readable and deletable
// because cleanup is driven by the URL stored in the row, not by the new
// roots.
const (
	B2VideoRootMovies  = "videos/movies"
	B2VideoRootSerials = "videos/serials"
	B2VideoRootClips   = "videos/clips"
)

// ContentTypeMovie / ContentTypeEpisode / ContentTypeClip are the canonical
// labels logged on every path-building call.
const (
	ContentTypeMovie   = "movie"
	ContentTypeEpisode = "series_episode"
	ContentTypeClip    = "clip"
)

// B2VideoRoot returns the correct B2 prefix for a given ingestion job.
// Episode jobs go under `videos/serials/`; everything else goes under
// `videos/movies/`. Clips have their own dedicated builder.
func B2VideoRoot(job *models.IngestionJob) string {
	if job != nil && job.ContentType == "episode" {
		return B2VideoRootSerials
	}
	return B2VideoRootMovies
}

// JobContentTypeLabel returns the label used in [B2_PATH] logs.
func JobContentTypeLabel(job *models.IngestionJob) string {
	if job != nil && job.ContentType == "episode" {
		return ContentTypeEpisode
	}
	return ContentTypeMovie
}

// BuildVideoB2Path joins a B2 root with a folder + tail components into a
// clean forward-slash key. It also rejects the legacy nested form
// `videos/movies/serials/...` for new uploads — that layout is the bug we
// are fixing, and any caller that produces it has not been updated.
func BuildVideoB2Path(root, folder string, parts ...string) (string, error) {
	root = strings.Trim(root, "/")
	folder = strings.Trim(folder, "/")
	if root == "" {
		return "", fmt.Errorf("empty b2 root")
	}
	all := append([]string{root, folder}, parts...)
	joined := path.Join(all...)
	if root == B2VideoRootMovies && strings.HasPrefix(joined, B2VideoRootMovies+"/serials/") {
		return "", fmt.Errorf("rejected legacy path %q: series episodes must use %q root", joined, B2VideoRootSerials)
	}
	return joined, nil
}

// LogB2Path emits the standardized [B2_PATH] line.
func LogB2Path(contentType, b2Path string) {
	log.Printf("[B2_PATH] content_type=%s path=%s", contentType, b2Path)
}

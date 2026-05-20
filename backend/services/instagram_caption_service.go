package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	instagramClipCaptionLeadMovie  = "🎬 Kinoni profildagi bot orqali toping!"
	instagramClipCaptionLeadSeries = "📺 Serialni profildagi bot orqali toping!"
	instagramClipCaptionCodeMovie  = "🔢 Kino Kodi: %s"
	instagramClipCaptionCodeSeries = "🔢 Serial Kodi: %s"
)

// instagramHashtagsMovie / instagramHashtagsSeries are the discovery
// tags appended to every clip caption. Kept under 30 (Instagram's hard
// cap) and split across niche / broad / brand buckets so the post can
// surface on a mix of search and Explore feeds. Edit cautiously — IG
// shadow-bans repeated identical tag stacks, so periodic rotation is
// recommended.
const (
	instagramHashtagsMovie = "#uzbekistan #ozbekiston #filmorauz #filmorauznet\n" +
		"#kino #kinolar #topkinolar #yangikinolar\n" +
		"#hindkino #hollywoodkino #ozbektilida #tarjimakino\n" +
		"#film #movie #reels #fyp #foryou"

	instagramHashtagsSeries = "#uzbekistan #ozbekiston #filmorauz #filmorauznet\n" +
		"#serial #seriallar #topseriallar #tarjimaserial\n" +
		"#ozbektilida #serialkinolar\n" +
		"#series #reels #fyp #foryou"
)

// BuildInstagramClipCaption returns the short Instagram caption required
// for clip publishing. The caption shape depends on whether the clip
// belongs to a movie or a series — separate lead, code label, and
// hashtag stacks improve discoverability per content type.
func BuildInstagramClipCaption(code string, isSeries bool) string {
	lead := instagramClipCaptionLeadMovie
	codeLine := instagramClipCaptionCodeMovie
	tags := instagramHashtagsMovie
	if isSeries {
		lead = instagramClipCaptionLeadSeries
		codeLine = instagramClipCaptionCodeSeries
		tags = instagramHashtagsSeries
	}
	return fmt.Sprintf("%s\n%s\n\n%s", lead, fmt.Sprintf(codeLine, strings.TrimSpace(code)), tags)
}

// ResolveInstagramClipCode applies the code-selection rules for clip posts:
// movie clips use movie.code; series clips use series.code; if the clip-level
// code is missing we fall back to the parent series code when available.
func ResolveInstagramClipCode(ctx context.Context, clip *models.Clip, seriesRepo *repositories.SeriesRepository) string {
	if clip == nil {
		return ""
	}

	clipCode := strings.TrimSpace(clip.MovieCode)
	if !IsSeriesClip(clip) {
		return clipCode
	}
	if clipCode != "" {
		return clipCode
	}
	return lookupSeriesCode(ctx, seriesRepo, clip.SeriesID)
}

// ResolveInstagramCodeByClipID reloads the clip when needed so scheduled jobs
// and older records can still recover the correct code for serial clips.
func ResolveInstagramCodeByClipID(
	ctx context.Context,
	clipRepo *repositories.ClipRepository,
	seriesRepo *repositories.SeriesRepository,
	clipID primitive.ObjectID,
	fallbackCode string,
) string {
	if code := strings.TrimSpace(fallbackCode); code != "" {
		return code
	}
	if clipRepo == nil || clipID.IsZero() {
		return ""
	}
	clip, err := clipRepo.FindByID(ctx, clipID)
	if err != nil || clip == nil {
		return ""
	}
	return ResolveInstagramClipCode(ctx, clip, seriesRepo)
}

// IsSeriesClip reports whether the clip belongs to a series (any of:
// explicit content_kind="series", non-zero series_id, or non-zero
// episode_id). Exported so handlers can reuse the same rule when
// populating PublishJob.ContentKind at creation time.
func IsSeriesClip(clip *models.Clip) bool {
	kind := strings.ToLower(strings.TrimSpace(clip.ContentKind))
	return kind == "series" || !clip.SeriesID.IsZero() || !clip.EpisodeID.IsZero()
}

// IsSeriesContentKind reports whether the given kind string (as stored
// on PublishJob.ContentKind) represents a series clip. Empty/unknown
// strings default to false so legacy rows render as movie captions.
func IsSeriesContentKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), "series")
}

func lookupSeriesCode(ctx context.Context, seriesRepo *repositories.SeriesRepository, seriesID primitive.ObjectID) string {
	if seriesRepo == nil || seriesID.IsZero() {
		return ""
	}
	series, err := seriesRepo.GetByID(seriesID)
	if err != nil || series == nil {
		return ""
	}
	return strings.TrimSpace(series.Code)
}

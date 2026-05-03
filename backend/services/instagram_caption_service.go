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
	instagramClipCaptionLead = "🎬 Kinoni profildagi bot orqali toping!"
	instagramClipCaptionCode = "🔢 Kino Kodi: %s"
)

// BuildInstagramClipCaption returns the short Instagram caption required for
// clip publishing. It intentionally excludes any movie/series title.
func BuildInstagramClipCaption(code string) string {
	return fmt.Sprintf("%s\n%s", instagramClipCaptionLead, fmt.Sprintf(instagramClipCaptionCode, strings.TrimSpace(code)))
}

// ResolveInstagramClipCode applies the code-selection rules for clip posts:
// movie clips use movie.code; series clips use series.code; if the clip-level
// code is missing we fall back to the parent series code when available.
func ResolveInstagramClipCode(ctx context.Context, clip *models.Clip, seriesRepo *repositories.SeriesRepository) string {
	if clip == nil {
		return ""
	}

	clipCode := strings.TrimSpace(clip.MovieCode)
	if !isSeriesClip(clip) {
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

func isSeriesClip(clip *models.Clip) bool {
	kind := strings.ToLower(strings.TrimSpace(clip.ContentKind))
	return kind == "series" || !clip.SeriesID.IsZero() || !clip.EpisodeID.IsZero()
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

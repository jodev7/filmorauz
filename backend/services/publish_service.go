package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
)

// BuildYouTubeDescription returns the YouTube Shorts description.
// Title is already set as the video title, so it is omitted here.
func BuildYouTubeDescription(code string, isSeries bool) string {
	if isSeries {
		return fmt.Sprintf("Serialni profildagi bot orqali toping!\nSerial Kodi: %s", code)
	}
	return fmt.Sprintf("Kinoni profildagi bot orqali toping!\nKino Kodi: %s", code)
}

// ExecutePlatformUpload dispatches the upload to the correct platform service
// based on job.Platform. For Instagram, mediaID/postURL are populated when
// available (including via sidecar recovery). For other platforms they stay
// empty.
func ExecutePlatformUpload(parserURL string, job *models.PublishJob) (mediaID, postURL string, err error) {
	// Caption resolution: when the job carries an explicit override (used by
	// admin-curated Content folder clips that have no movie/series code),
	// publish exactly what the admin wrote. Otherwise fall back to the
	// movie/series caption template.
	captionFor := func() string {
		if strings.TrimSpace(job.CaptionOverride) != "" {
			return job.CaptionOverride
		}
		return BuildInstagramClipCaption(job.MovieCode, IsSeriesContentKind(job.ContentKind))
	}
	titleFor := func() string {
		if strings.TrimSpace(job.MovieTitle) != "" {
			return job.MovieTitle
		}
		return "Content"
	}

	switch job.Platform {
	case models.PublishPlatformInstagram:
		account := GetInstagramAccount(job.AccountName)
		if account == nil {
			return "", "", fmt.Errorf("instagram account not configured: %s", job.AccountName)
		}
		publishKey := "pj_" + job.ID.Hex()
		result, uerr := UploadReelToInstagram(parserURL, job.ClipURL, captionFor(), publishKey, account)
		if uerr != nil {
			return "", "", uerr
		}
		return result.MediaID, result.PostURL, nil
	case models.PublishPlatformYouTube:
		account := GetYouTubeAccount(job.AccountName)
		if account == nil {
			return "", "", fmt.Errorf("youtube account not configured: %s", job.AccountName)
		}
		desc := strings.TrimSpace(job.CaptionOverride)
		if desc == "" {
			desc = BuildYouTubeDescription(job.MovieCode, IsSeriesContentKind(job.ContentKind))
		}
		return "", "", UploadShortToYouTube(parserURL, job.ClipURL, titleFor(), desc, account)
	case models.PublishPlatformTikTok:
		account := GetTikTokAccount(job.AccountName)
		if account == nil {
			return "", "", fmt.Errorf("tiktok account not configured: %s", job.AccountName)
		}
		return "", "", UploadVideoToTikTok(parserURL, job.ClipURL, captionFor(), account)
	default:
		return "", "", fmt.Errorf("unknown platform: %s", job.Platform)
	}
}

func PopulateInstagramJobCode(
	ctx context.Context,
	job *models.PublishJob,
	clipRepo *repositories.ClipRepository,
	seriesRepo *repositories.SeriesRepository,
) {
	if job == nil {
		return
	}
	job.MovieCode = ResolveInstagramCodeByClipID(ctx, clipRepo, seriesRepo, job.ClipID, job.MovieCode)
}

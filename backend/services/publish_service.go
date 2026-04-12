package services

import (
	"fmt"

	"github.com/filmorauz/backend/models"
)

// BuildPublishCaption returns the standard caption used across all platforms.
func BuildPublishCaption(movieTitle, movieCode string) string {
	return fmt.Sprintf("%s\n\nKinoni profildagi bot orqali toping!\nKino Kodi: %s", movieTitle, movieCode)
}

// ExecutePlatformUpload dispatches the upload to the correct platform service
// based on job.Platform. Returns nil on success.
func ExecutePlatformUpload(parserURL string, job *models.PublishJob) error {
	caption := BuildPublishCaption(job.MovieTitle, job.MovieCode)
	switch job.Platform {
	case models.PublishPlatformInstagram:
		account := GetInstagramAccount(job.AccountName)
		if account == nil {
			return fmt.Errorf("instagram account not configured: %s", job.AccountName)
		}
		return UploadReelToInstagram(parserURL, job.ClipURL, caption, account)
	case models.PublishPlatformYouTube:
		account := GetYouTubeAccount(job.AccountName)
		if account == nil {
			return fmt.Errorf("youtube account not configured: %s", job.AccountName)
		}
		return UploadShortToYouTube(parserURL, job.ClipURL, job.MovieTitle, caption, account)
	case models.PublishPlatformTikTok:
		account := GetTikTokAccount(job.AccountName)
		if account == nil {
			return fmt.Errorf("tiktok account not configured: %s", job.AccountName)
		}
		return UploadVideoToTikTok(parserURL, job.ClipURL, caption, account)
	default:
		return fmt.Errorf("unknown platform: %s", job.Platform)
	}
}

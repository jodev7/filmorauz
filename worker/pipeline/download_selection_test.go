package pipeline

import (
	"testing"

	"github.com/filmorauz/worker/models"
)

func TestChooseDownloadCandidatesPrefersHighestQuality(t *testing.T) {
	meta := &models.ParsedMovieMetadata{
		VideoURLs: []models.VideoSource{
			{URL: "https://cdn.example.com/video-480.mp4", Quality: "480p", Type: "mp4"},
			{URL: "https://cdn.example.com/video-1080.m3u8", Quality: "1080p", Type: "m3u8"},
			{URL: "https://cdn.example.com/video-720.mp4", Quality: "720p", Type: "mp4"},
		},
	}
	candidates := chooseDownloadCandidates(meta, "")
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}
	if candidates[0].Quality != "1080p" {
		t.Fatalf("expected highest-quality candidate first, got %+v", candidates[0])
	}
}

func TestExpectedQualitySatisfied(t *testing.T) {
	if expectedQualitySatisfied("1080p", 850) {
		t.Fatalf("1080p should not be satisfied by 850px height")
	}
	if !expectedQualitySatisfied("1080p", 1080) {
		t.Fatalf("1080p should be satisfied by 1080px height")
	}
}

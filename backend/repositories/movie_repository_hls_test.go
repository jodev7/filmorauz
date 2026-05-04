package repositories

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestNormalizeMovieFromBSONReadsAvailableQualities(t *testing.T) {
	doc := bson.M{
		"code":                "0001",
		"slug":                "demo",
		"title":               "Demo",
		"available_qualities": []interface{}{"360p", "480p", "720p", "1080p"},
		"default_quality":     "1080p",
		"source_resolution":   "1920x1080",
		"master_playlist_url": "https://cdn.example.com/videos/demo/index.m3u8",
	}

	movie, err := normalizeMovieFromBSON(doc)
	if err != nil {
		t.Fatalf("normalizeMovieFromBSON returned error: %v", err)
	}

	wantQualities := []string{"360p", "480p", "720p", "1080p"}
	if !reflect.DeepEqual(movie.AvailableQualities, wantQualities) {
		t.Fatalf("AvailableQualities = %v, want %v", movie.AvailableQualities, wantQualities)
	}
	if !reflect.DeepEqual(movie.GeneratedQualities, wantQualities) {
		t.Fatalf("GeneratedQualities = %v, want %v", movie.GeneratedQualities, wantQualities)
	}
	if movie.DefaultQuality != "1080p" {
		t.Fatalf("DefaultQuality = %q, want 1080p", movie.DefaultQuality)
	}
	if movie.SourceResolution != "1920x1080" {
		t.Fatalf("SourceResolution = %q, want 1920x1080", movie.SourceResolution)
	}
}

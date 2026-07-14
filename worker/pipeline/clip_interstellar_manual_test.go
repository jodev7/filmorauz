package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/filmorauz/worker/storage"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestGenerateInterstellarClips is a manual, opt-in local harness that runs the
// REAL clip-generation code path (Gemini viral selection + logo/CTA overlay +
// burnt-in Uzbek subtitles) against parser/downloads/Interstellar.mp4 and
// writes the resulting clips to worker/uploads/movies/clips/interstellar/.
//
// It is gated behind RUN_INTERSTELLAR=1 so it never runs in normal `go test`.
// Requires: the parser running on PARSER_URL (default 127.0.0.1:8082) with a
// valid GEMINI_API_KEY, ffmpeg on PATH, and worker/docs/logo.png.
//
//	cd worker && RUN_INTERSTELLAR=1 go test ./pipeline/ \
//	    -run TestGenerateInterstellarClips -v -timeout 60m
func TestGenerateInterstellarClips(t *testing.T) {
	if os.Getenv("RUN_INTERSTELLAR") != "1" {
		t.Skip("set RUN_INTERSTELLAR=1 to run the manual Interstellar clip test")
	}

	// Resolve the input file (absolute) before we chdir.
	videoPath, err := filepath.Abs(filepath.Join("..", "..", "parser", "downloads", "Interstellar.mp4"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(videoPath); err != nil {
		t.Fatalf("input not found: %s (%v)", videoPath, err)
	}

	// The clip generator resolves docs/logo.png and uploads/ relative to the
	// working directory. The real worker runs from worker/, so chdir there.
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("chdir to worker root: %v", err)
	}

	parserURL := os.Getenv("PARSER_URL")
	if parserURL == "" {
		parserURL = "http://127.0.0.1:8082"
	}

	// Turn the new features ON for this run.
	os.Setenv("CLIP_AI_VIRAL", "1")
	os.Setenv("CLIP_SUBTITLES", "1")

	p := &Pipeline{
		config: Config{
			ParserURL: parserURL,
			StorageConfig: storage.Config{
				Mode:      "dev", // local output, no B2 upload
				BaseURL:   "http://localhost:8081",
				LocalPath: "./uploads",
			},
			DB: nil, // skips clip_ai_usage + clips persistence (guarded internally)
		},
	}

	target := clipTarget{
		Kind:          "movie",
		FilenameSlug:  "interstellar",
		FolderSubpath: "interstellar",
		DisplayLabel:  "Movie: Interstellar (code: 9999)",
		TopText:       "Kino kodi\\: 9999",
		MovieID:       primitive.NewObjectID(),
		MovieTitle:    "Interstellar",
		MovieSlug:     "interstellar",
		MovieCode:     "9999",
	}

	if err := p.generateClipsForTarget(context.Background(), target, videoPath); err != nil {
		t.Fatalf("generateClipsForTarget failed: %v", err)
	}

	outDir := filepath.Join("uploads", "movies", "clips", "interstellar")
	entries, _ := os.ReadDir(outDir)
	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".mp4" {
			count++
			t.Logf("clip: %s", filepath.Join(outDir, e.Name()))
		}
	}
	if count == 0 {
		t.Fatalf("no clips produced in %s", outDir)
	}
	t.Logf("✓ produced %d clips in %s", count, outDir)
}

# OpenAI Poster Generation - Implementation Guide

This document describes the OpenAI-powered poster generation pipeline that has been implemented in the FilmoraUz worker service.

## Overview

When a movie is imported, the worker can now generate a new branded movie poster using OpenAI image generation (DALL-E 3). The generated poster:
- Features a premium cinematic look
- Displays the Uzbek-localized movie title prominently
- Includes subtle FilmoraUz.net branding
- Preserves the original movie's tone and composition when possible

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Worker Service                               │
├─────────────────────────────────────────────────────────────────┤
│  Pipeline                                                         │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │ 1. Parse movie metadata                                     │ │
│  │ 2. Enrich with TMDB data                                    │ │
│  │ 3. Download video                                           │ │
│  │ 4. Process video (watermark removal, HLS)                   │ │
│  │ 5. Generate poster ← NEW: OpenAI integration               │ │
│  │ 6. Upload assets (dev: local, prod: B2)                     │ │
│  │ 7. Create movie in database                                │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │ OpenAI Client Service (services/openai_client.go)          │ │
│  │ - Dynamic prompt building based on metadata                 │ │
│  │ - Uzbek localization support                               │ │
│  │ - Genre-specific composition guidance                       │ │
│  │ - Branding instructions                                     │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │ Poster Generator (pipeline/poster_generator.go)             │ │
│  │ - OpenAI client integration                                 │ │
│  │ - Fallback to legacy AI endpoint                           │ │
│  │ - Fallback to original poster                              │ │
│  │ - Local/production storage handling                         │ │
│  └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Files Changed

| File | Description |
|------|-------------|
| `worker/services/openai_client.go` | **NEW** - OpenAI client service for image generation |
| `worker/pipeline/poster_generator.go` | Updated to integrate OpenAI client |
| `worker/pipeline/pipeline.go` | Updated to initialize OpenAI client |
| `worker/main.go` | Updated to pass OpenAI config to pipeline |
| `worker/.env` | Added OpenAI configuration variables |
| `worker/go.mod` | Added OpenAI SDK dependency |

## Configuration

### Environment Variables

Add these to `worker/.env`:

```bash
# ── OpenAI Configuration ─────────────────────────────────────
# Get your API key from: https://platform.openai.com/api-keys
OPENAI_API_KEY=sk-...

# Optional: Use a different base URL (for compatible APIs)
OPENAI_BASE_URL=

# Model to use (dall-e-3 for quality, dall-e-2 for speed)
OPENAI_MODEL=dall-e-3

# ── Legacy: Custom AI Endpoint (alternative to OpenAI) ──────────
# Leave empty to use OpenAI, or set to use a custom endpoint
AI_ENDPOINT=
```

### Pipeline Configuration

The pipeline accepts OpenAI configuration:

```go
pipeConfig := pipeline.Config{
    // ... other config ...
    OpenAIConfig: &services.OpenAIConfig{
        APIKey:      getEnv("OPENAI_API_KEY", ""),
        BaseURL:     getEnv("OPENAI_BASE_URL", ""),
        Model:       getEnv("OPENAI_MODEL", "dall-e-3"),
        Temperature: 0.7,
        Timeout:     120 * time.Second,
        TempDir:     getEnv("TEMP_DIR", "./tmp"),
    },
}
```

## Input/Output Flow

### Input to Poster Generation

```go
// Movie metadata passed to the poster generator
type EnrichedMetadata struct {
    Title          string   // Primary title
    OriginalTitle  string   // English/original title
    TitleUz        string   // Uzbek title (if available)
    Year           int      // Release year
    Genres         []string // Movie genres
    Description    string   // Synopsis/tagline
    PosterURL      string   // Original poster (TMDB preferred)
    BackdropURL    string   // Backdrop image
}
```

### Processing Flow

1. **Metadata Collection**: Collects movie title, genres, year, description, original poster
2. **Title Localization**: Prioritizes Uzbek title, falls back to mapped titles, then English
3. **Prompt Building**: Dynamically generates prompt based on:
   - Genre (action → dramatic lighting, horror → dark atmosphere)
   - Mood (from description)
   - Composition (characters in upper portion, action in foreground)
4. **Branding**: Adds FilmoraUz.net branding instructions
5. **Generation**: Calls OpenAI DALL-E 3 API
6. **Storage**: Saves to canonical folder (same as HLS files)

### Output

```go
// Poster generation result
type PosterResult struct {
    OriginalPosterURL  string // URL of original poster found
    GeneratedPosterURL string // URL of AI-generated poster
    PosterGenerated    bool   // Whether AI poster was generated
}
```

## Prompt Building

The prompt is dynamically built based on movie metadata:

```go
// Example prompt for "Forsaj 8" / "The Fate of the Furious"
prompt := `
Create a premium cinematic movie poster for "Forsaj 8" (2017).

Genre: high-octane action
Mood: dark, high-energy atmosphere with dramatic action sequences

Requirements:
- Central or off-center subject composition
- Character(s) prominently featured in upper portion
- Dramatic lighting with spotlight or rim lighting effects
- Action elements or vehicles in foreground

Visual Style:
- High-end streaming platform quality poster
- Dramatic cinematic composition with professional lighting
- Strong contrast and rich colors
- Clean, professional typography with large readable title
- Premium blockbuster aesthetic suitable for theatrical release

Branding Requirements:
- Add "FilmoraUz.net" branding subtly in the bottom-right corner
- Use clean spacing between branding elements
- Branding should be visible but not intrusive
- Style: 'Filmora' in white, 'Uz' in orange (#FF7A00), '.net' in white

Important:
- The title must be prominently displayed in Uzbek language
- Maintain facial accuracy and proper proportions
- Output must look like an official localized theatrical poster
`
```

## Fallback Behavior

If OpenAI poster generation fails, the system falls back in order:

1. **OpenAI API fails** → Try legacy AI endpoint (if configured)
2. **Legacy endpoint fails** → Use original poster from TMDB/enricher
3. **No poster available** → Log warning, continue without poster

```go
// In poster_generator.go
func (pg *PosterGenerator) generateAIPoster(...) {
    // Priority 1: OpenAI client (if configured)
    if pg.openAIClient != nil && pg.openAIClient.IsConfigured() {
        return pg.generateAIPosterWithOpenAI(ctx, metadata, canonicalFolder)
    }

    // Priority 2: Legacy AI endpoint (if configured)
    if pg.aiEndpoint != "" {
        return pg.generateAIPosterLegacy(ctx, metadata, canonicalFolder)
    }

    // No AI generation available
    return "", fmt.Errorf("no AI generation configured")
}
```

## Storage Behavior

### Development Mode

- Poster saved to: `worker/uploads/movies/<slug>/poster.jpg`
- Full URL: `http://localhost:8080/stream/<slug>/poster.jpg`

### Production Mode

- Poster uploaded to: B2 bucket `movies/<slug>/poster.jpg`
- CDN URL: `https://cdn.filmorauz.net/movies/<slug>/poster.jpg`

## Manual Test Checklist

### Pre-requisites

1. MongoDB running at `localhost:27017`
2. Backend running at `localhost:8080`
3. Parser running at `localhost:8082`
4. OpenAI API key set in `worker/.env`

### Test Steps

#### Test 1: Basic Import with OpenAI Poster Generation

```bash
# 1. Start the worker
cd worker
./filmora-worker

# 2. Check logs for OpenAI initialization
# Expected: "[OPENAI] OpenAI client initialized with model: dall-e-3"

# 3. Import a movie via admin panel or API
# POST /api/admin/ingestion/jobs with source=uzmovi&q=Forsaj

# 4. Monitor worker logs
# Expected sequence:
# - "[PIPELINE] ===== POSTER GENERATION START ====="
# - "[POSTER] OpenAI poster generation for: Forsaj 8"
# - "[OPENAI] Generating image with model: dall-e-3"
# - "[OPENAI] Image generated successfully"
# - "[POSTER] OpenAI poster generated: ./tmp/openai_gen_xxx.jpg"
```

#### Test 2: Poster Quality Check

```bash
# 1. Find the generated poster
# Dev mode: worker/uploads/movies/forsaj8_xxx/poster.jpg
# Prod mode: Check B2 bucket

# 2. Verify:
# - [ ] Poster has Uzbek title prominently displayed
# - [ ] Poster has cinematic/comic book quality
# - [ ] FilmoraUz.net branding visible in bottom-right
# - [ ] No faces/key elements covered by branding
```

#### Test 3: Fallback Behavior (No API Key)

```bash
# 1. Remove or empty OPENAI_API_KEY in .env
# 2. Restart worker
# 3. Import a movie
# 4. Check logs:
# - "[OPENAI] OpenAI not configured - API key not provided"
# - "[PIPELINE] OpenAI not configured - will use legacy endpoint or skip"
# - If poster exists from TMDB, it should be used
```

#### Test 4: OpenAI Generation Failure

```bash
# 1. Set an invalid API key
OPENAI_API_KEY=sk-invalid-key

# 2. Import a movie
# 3. Check logs for fallback:
# - "[OPENAI] OpenAI poster generation failed: ..."
# - "[POSTER] Using CLEAN original poster: https://image.tmdb.org/..."
# 4. Verify job completes with original poster
```

#### Test 5: Database Verification

```bash
# 1. After successful import, check MongoDB
db.movies.findOne({ title: "Forsaj 8" })

# 2. Verify:
# - poster_url field is set to the generated poster URL
# - poster_generated field is true
```

## Troubleshooting

### Common Issues

1. **"OpenAI client not configured"**
   - Check `OPENAI_API_KEY` is set in `.env`
   - Run `go mod tidy` to ensure SDK is installed
   - Restart worker after config changes

2. **"Image generation failed: ..."**
   - Check API key is valid
   - Check account has credits
   - Check rate limits

3. **"No poster available"**
   - TMDB poster might be blocked
   - Check network connectivity to TMDB
   - Check TMDB API key is valid

4. **"Failed to upload poster to B2"**
   - Check B2 credentials in `.env`
   - Check bucket name matches
   - Check B2 endpoint URL

## Cost Considerations

- DALL-E 3 (HD quality): ~$0.04-0.12 per image
- DALL-E 2: ~$0.02-0.04 per image
- Consider setting `OPENAI_MODEL=dall-e-2` for development/testing

## Security Notes

- Never commit API keys to version control
- Use environment variables or a secrets manager
- Rotate API keys periodically
- Monitor API usage for anomalies

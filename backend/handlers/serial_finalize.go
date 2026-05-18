package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// terminalEpisodeStatuses are the IngestionJob statuses that count as "done"
// for the purposes of deciding whether a parent serial can be finalized.
// Failed/cancelled episodes don't block finalization in lenient mode —
// their data simply isn't inserted into the series tree.
func isTerminalEpisodeStatus(s models.IngestionStatus) bool {
	switch s {
	case models.IngestionStatusCompleted,
		models.IngestionStatusFailed,
		models.IngestionStatusDownloadFailed:
		return true
	}
	return false
}

// FinalizeSerialReadyParents runs through every serial-parent ingestion job
// whose extraction finished but which hasn't been turned into real
// Series/Seasons/Episodes yet. For each, if every child episode job is in a
// terminal state, materialize the rows from the completed ones (lenient
// mode — failed children are skipped) and mark the parent completed.
//
// Called periodically from a background ticker in main.go so the lifecycle
// closes on its own without any inter-service callbacks.
func (h *IngestionHandler) FinalizeSerialReadyParents(ctx context.Context) (int, error) {
	cursor, err := h.jobRepo.GetCollection().Find(ctx, bson.M{
		"content_type": "serial_parent",
		"status":       models.IngestionStatusProcessing,
		"stage":        "waiting_for_episodes",
	}, options.Find().SetLimit(50))
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var parents []models.IngestionJob
	if err := cursor.All(ctx, &parents); err != nil {
		return 0, err
	}

	finalized := 0
	for i := range parents {
		parent := parents[i]
		if err := h.finalizeOneSerialParent(ctx, &parent); err != nil {
			log.Printf("[serial finalize] parent=%s err=%v", parent.ID.Hex(), err)
			continue
		}
		finalized++
	}
	return finalized, nil
}

func (h *IngestionHandler) finalizeOneSerialParent(ctx context.Context, parent *models.IngestionJob) error {
	parentID := parent.ID
	parentHex := parentID.Hex()

	// Pull every child job linked to this parent.
	cursor, err := h.jobRepo.GetCollection().Find(ctx, bson.M{
		"parent_job_id": parentID,
		"content_type":  "episode",
	})
	if err != nil {
		return fmt.Errorf("list children: %w", err)
	}
	defer cursor.Close(ctx)

	var children []models.IngestionJob
	if err := cursor.All(ctx, &children); err != nil {
		return fmt.Errorf("decode children: %w", err)
	}

	if len(children) == 0 {
		log.Printf("[serial finalize] parent=%s has no children yet — skipping", parentHex)
		return nil
	}

	completed := make([]models.IngestionJob, 0, len(children))
	pending := 0
	failed := 0
	for _, c := range children {
		if !isTerminalEpisodeStatus(c.Status) {
			pending++
			continue
		}
		if c.Status == models.IngestionStatusCompleted &&
			strings.TrimSpace(c.MasterPlaylistURL) != "" {
			completed = append(completed, c)
		} else {
			failed++
		}
	}

	if pending > 0 {
		log.Printf("[serial finalize] parent=%s still waiting: pending=%d completed=%d failed=%d",
			parentHex, pending, len(completed), failed)
		return nil
	}

	if len(completed) == 0 {
		// Every child failed — mark parent failed.
		log.Printf("[serial finalize] parent=%s all children failed (%d) — marking parent failed",
			parentHex, failed)
		return h.markParentFinalizedFailed(ctx, parentID, failed)
	}

	// Pull title/slug/poster/etc from the first completed child (they all
	// share the same series metadata).
	first := completed[0]
	title := strings.TrimSpace(parent.Title)
	if title == "" && first.Metadata != nil {
		title = strings.TrimSpace(first.Metadata.Title)
	}
	slug := strings.TrimSpace(first.SeriesSlug)
	if slug == "" {
		slug = slugifySerial(title)
	}

	// Build a synthetic payload from the parent (description/year/poster come
	// from the parent job that was created at import time).
	synthPayload := &parserSerialResponse{
		Description: "",
		Year:        0,
	}
	if parent.Metadata != nil {
		synthPayload.Description = ""
		synthPayload.Year = parent.Metadata.Year
		synthPayload.Poster = parent.Metadata.Poster
		synthPayload.Backdrop = parent.Metadata.Backdrop
	}

	series, err := h.upsertSeries(ctx, title, synthPayload)
	if err != nil {
		return fmt.Errorf("upsert series: %w", err)
	}

	seasonCache := map[int]*models.Season{}
	insertedEpisodes := 0
	for _, c := range completed {
		seasonNum := c.SeasonNumber
		if seasonNum <= 0 {
			seasonNum = 1
		}
		season, ok := seasonCache[seasonNum]
		if !ok {
			s, err := h.upsertSeason(ctx, series.ID, seasonNum)
			if err != nil {
				log.Printf("[serial finalize] parent=%s upsert season=%d: %v", parentHex, seasonNum, err)
				continue
			}
			season = s
			seasonCache[seasonNum] = season
		}

		ep := parserSerialEpisode{
			Season:     seasonNum,
			Episode:    c.EpisodeNumber,
			Title:      c.EpisodeTitle,
			EpisodeURL: c.DetailURL,
			VideoURL:   c.MasterPlaylistURL,
			Poster:     c.EpisodePosterURL,
		}
		episode, err := h.upsertEpisode(ctx, series.ID, season.ID, ep, series.PosterURL)
		if err != nil {
			log.Printf("[serial finalize] parent=%s upsert episode S%02dE%02d: %v",
				parentHex, seasonNum, c.EpisodeNumber, err)
			continue
		}

		// Persist the playback URL on the Episode row so the frontend can
		// stream it. CompleteEpisode handler already knows how to do this;
		// we replicate the minimum write inline to avoid the HTTP roundtrip.
		_, _ = h.seriesRepo.Collection().Database().
			Collection("episodes").
			UpdateByID(ctx, episode.ID, bson.M{
				"$set": bson.M{
					"video_url":  c.MasterPlaylistURL,
					"updated_at": time.Now(),
				},
			})

		// Backfill linkage on the IngestionJob so downstream tools can still
		// trace each job to its Series/Season/Episode rows.
		_, _ = h.jobRepo.GetCollection().UpdateByID(ctx, c.ID, bson.M{
			"$set": bson.M{
				"series_id":  series.ID,
				"season_id":  season.ID,
				"episode_id": episode.ID,
				"updated_at": time.Now(),
			},
		})

		insertedEpisodes++
	}

	// Mark parent completed with summary.
	now := time.Now()
	msg := fmt.Sprintf("Serial finalized: %d episodes inserted, %d failed/skipped", insertedEpisodes, failed)
	_, err = h.jobRepo.GetCollection().UpdateByID(ctx, parentID, bson.M{
		"$set": bson.M{
			"status":       models.IngestionStatusCompleted,
			"stage":        string(models.IngestionStatusCompleted),
			"progress":     100,
			"series_id":    series.ID,
			"series_slug":  series.Slug,
			"completed_at": now,
			"updated_at":   now,
			"message":      msg,
		},
	})
	if err != nil {
		return fmt.Errorf("update parent completed: %w", err)
	}
	log.Printf("[serial finalize] parent=%s done — series=%s episodes=%d failed=%d",
		parentHex, series.Slug, insertedEpisodes, failed)
	return nil
}

func (h *IngestionHandler) markParentFinalizedFailed(ctx context.Context, parentID primitive.ObjectID, failedCount int) error {
	now := time.Now()
	_, err := h.jobRepo.GetCollection().UpdateByID(ctx, parentID, bson.M{
		"$set": bson.M{
			"status":       models.IngestionStatusFailed,
			"stage":        "failed",
			"completed_at": now,
			"updated_at":   now,
			"message":      fmt.Sprintf("All %d episode jobs failed — series not created", failedCount),
			"error":        "all child episode jobs failed",
		},
	})
	return err
}

// Suppress "imported and not used" if go vet trims things later.
var _ = mongo.ErrNoDocuments

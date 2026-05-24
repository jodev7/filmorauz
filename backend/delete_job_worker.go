package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
)

// startDeleteJobWorker runs the background processor for content deletion.
//
// The admin "delete" button only enqueues a DeleteJob (status=queued); this
// loop executes it. Deletion is done in-process by the backend services, which
// already own B2 access, the clip repo and the Instagram-schedule repo — so the
// full cascade (B2 HLS folder, source video, poster, backdrop, clips, Instagram
// schedules, publish jobs, then the Mongo document) runs here with no
// cross-service HTTP hop. Progress is written back onto the DeleteJob via the
// tracker callback, so the admin UI can poll GET .../delete-jobs/:id and render
// a progress bar.
//
// Replaces the old worker-side DeletionWorker, which called the backend admin
// cascade endpoint over HTTP without an auth token and so was always rejected
// (401 for series, 404 for the unregistered movie route) — deletion never
// actually happened.
func startDeleteJobWorker(repo *repositories.DeleteJobRepository, movieService *services.MovieService, seriesService *services.SeriesService) {
	const (
		pollInterval = 10 * time.Second
		staleAfter   = 15 * time.Minute
	)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	log.Printf("[delete-worker] started (poll=%s, stale=%s)", pollInterval, staleAfter)

	for range ticker.C {
		processQueuedDeleteJobs(repo, movieService, seriesService, staleAfter)
	}
}

func processQueuedDeleteJobs(repo *repositories.DeleteJobRepository, movieService *services.MovieService, seriesService *services.SeriesService, staleAfter time.Duration) {
	ctx := context.Background()

	// Recover jobs wedged in "deleting" (e.g. a backend restart mid-delete) so a
	// crashed run never blocks future deletes of the same content forever.
	if n, err := repo.FailStaleJobs(ctx, staleAfter); err != nil {
		log.Printf("[delete-worker] FailStaleJobs error: %v", err)
	} else if n > 0 {
		log.Printf("[delete-worker] recovered %d stale deletion job(s)", n)
	}

	jobs, err := repo.FindQueued(ctx)
	if err != nil {
		log.Printf("[delete-worker] FindQueued error: %v", err)
		return
	}

	for _, job := range jobs {
		runDeleteJob(ctx, repo, movieService, seriesService, job)
	}
}

func runDeleteJob(ctx context.Context, repo *repositories.DeleteJobRepository, movieService *services.MovieService, seriesService *services.SeriesService, job *models.DeleteJob) {
	id := job.ContentID.Hex()
	log.Printf("[delete-worker] start job=%s type=%s content=%s title=%q", job.ID.Hex(), job.ContentType, id, job.Title)

	now := time.Now()
	job.Status = "deleting"
	job.CurrentStep = "Starting deletion"
	job.StartedAt = &now
	job.UpdatedAt = now
	if err := repo.Update(ctx, job); err != nil {
		log.Printf("[delete-worker] failed to mark job=%s deleting: %v", job.ID.Hex(), err)
		return
	}

	// Progress callback persists step/percent so the admin UI can poll it.
	track := func(step string, progress int) {
		job.CurrentStep = step
		job.Progress = progress
		job.UpdatedAt = time.Now()
		if err := repo.Update(ctx, job); err != nil {
			log.Printf("[delete-worker] progress update failed job=%s: %v", job.ID.Hex(), err)
		}
	}

	var runErr error
	switch job.ContentType {
	case "movie":
		_, runErr = movieService.DeleteMovieWithProgress(id, track)
	case "series":
		runErr = func() error {
			_, e := seriesService.DeleteSeriesWithProgress(job.ContentID, track)
			return e
		}()
	default:
		runErr = fmt.Errorf("unknown content_type %q", job.ContentType)
	}

	if runErr != nil {
		job.Status = "failed"
		job.Error = runErr.Error()
		job.UpdatedAt = time.Now()
		_ = repo.Update(ctx, job)
		log.Printf("[delete-worker] FAILED job=%s type=%s content=%s: %v", job.ID.Hex(), job.ContentType, id, runErr)
		return
	}

	completedAt := time.Now()
	job.Status = "completed"
	job.Progress = 100
	job.CurrentStep = "Completed"
	job.CompletedAt = &completedAt
	job.UpdatedAt = completedAt
	_ = repo.Update(ctx, job)
	log.Printf("[delete-worker] completed job=%s type=%s content=%s", job.ID.Hex(), job.ContentType, id)
}

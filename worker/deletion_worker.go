package worker

import (
	"context"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
)

type DeletionWorker struct {
	repo          *repositories.DeleteJobRepository
	movieService  *services.MovieService
	seriesService *services.SeriesService
}

func (w *DeletionWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processQueuedJobs(ctx)
		}
	}
}

func (w *DeletionWorker) processQueuedJobs(ctx context.Context) {
	// Find all queued jobs
	// Note: We need a method in repo to find all queued
	jobs, err := w.repo.FindQueued(ctx)
	if err != nil {
		log.Printf("[DeletionWorker] Error fetching queued jobs: %v", err)
		return
	}

	for _, job := range jobs {
		job.Status = "deleting"
		job.UpdatedAt = time.Now()
		w.repo.Update(ctx, job)

		var err error
		if job.ContentType == "movie" {
			_, err = w.movieService.DeleteMovie(job.ContentID.Hex())
		} else {
			_, err = w.seriesService.DeleteSeries(job.ContentID)
		}

		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
		} else {
			job.Status = "completed"
			job.Progress = 100
			completedAt := time.Now()
			job.CompletedAt = &completedAt
		}
		job.UpdatedAt = time.Now()
		w.repo.Update(ctx, job)
	}
}

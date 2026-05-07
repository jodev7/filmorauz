package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/filmorauz/worker/repositories"
)

type DeletionWorker struct {
	repo       *repositories.DeleteJobRepository
	backendURL string
}

func NewDeletionWorker(repo *repositories.DeleteJobRepository, backendURL string) *DeletionWorker {
	return &DeletionWorker{repo: repo, backendURL: backendURL}
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
	jobs, err := w.repo.FindQueued(ctx)
	if err != nil {
		log.Printf("[DeletionWorker] Error fetching queued jobs: %v", err)
		return
	}

	for _, job := range jobs {
		job.Status = "deleting"
		job.UpdatedAt = time.Now()
		w.repo.Update(ctx, job)

		// Trigger deletion via API to ensure backend handles DB cascade locally
		// safely without violating architectural boundaries.
		// Construct URL based on content type
		url := w.backendURL + "/api/admin/" + job.ContentType + "s/" + job.ContentID.Hex() + "/delete-cascade"
		
		req, _ := http.NewRequestWithContext(ctx, "DELETE", url, nil)
		client := &http.Client{}
		resp, err := client.Do(req)

		if err != nil || resp.StatusCode != http.StatusOK {
			job.Status = "failed"
			job.Error = "Cascade deletion trigger failed"
			if err != nil {
				job.Error = err.Error()
			}
		} else {
			job.Status = "completed"
			job.Progress = 100
			now := time.Now()
			job.CompletedAt = &now
		}
		job.UpdatedAt = time.Now()
		w.repo.Update(ctx, job)
	}
}

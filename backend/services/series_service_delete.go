package services

import (
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (s *SeriesService) DeleteSeriesWithProgress(id primitive.ObjectID, track ProgressTracker) (*SeriesDeleteResult, error) {
	series, err := s.seriesRepo.GetByID(id)
	if err != nil || series == nil {
		return nil, fmt.Errorf("series not found")
	}

	log.Printf("[SERIES DELETE] start — id=%s title=%q code=%s", id.Hex(), series.Title, series.Code)
	track("Initializing deletion", 5)

	result := &SeriesDeleteResult{
		SeriesID: id.Hex(),
		Title:    series.Title,
		B2:       NewB2DeleteSummary(),
	}

	track("Deleting DB records and media", 10)
	s.cleanupSeriesStorage(series, result, track)

	if len(result.B2.Errors) > 0 {
		result.Partial = true
	}

	track("Deleting DB document", 90)
	if delErr := s.seriesRepo.Delete(id); delErr != nil {
		log.Printf("[SERIES DELETE] FAILED repo.Delete id=%s: %v", id.Hex(), delErr)
		return result, delErr
	}

	track("Deletion completed", 100)
	log.Printf("[SERIES DELETE] done — id=%s removed from DB (b2_files_deleted=%d skipped=%d errors=%d)",
		id.Hex(), result.B2.FilesDeleted, len(result.B2.Skipped), len(result.B2.Errors))
	return result, nil
}

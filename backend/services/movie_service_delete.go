package services

import (
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ProgressTracker func(step string, progress int)

func (s *MovieService) DeleteMovieWithProgress(id string, track ProgressTracker) (*MovieDeleteResult, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid movie id")
	}

	movie, findErr := s.repo.FindByID(objID)
	if findErr != nil || movie == nil {
		return nil, fmt.Errorf("movie not found")
	}

	log.Printf("[MOVIE DELETE] start — id=%s title=%q code=%s", id, movie.Title, movie.Code)
	track("Initializing deletion", 5)

	result := &MovieDeleteResult{
		MovieID: id,
		Title:   movie.Title,
		B2:      NewB2DeleteSummary(),
	}

	track("Deleting DB records and media", 10)
	s.cleanupMovieStorage(objID, movie, result, track)

	if len(result.B2.Errors) > 0 {
		result.Partial = true
	}

	track("Deleting DB document", 90)
	if delErr := s.repo.Delete(id); delErr != nil {
		log.Printf("[MOVIE DELETE] FAILED repo.Delete id=%s: %v", id, delErr)
		return result, delErr
	}

	track("Deletion completed", 100)
	log.Printf("[MOVIE DELETE] done — id=%s removed from DB (b2_files_deleted=%d skipped=%d errors=%d)",
		id, result.B2.FilesDeleted, len(result.B2.Skipped), len(result.B2.Errors))
	return result, nil
}

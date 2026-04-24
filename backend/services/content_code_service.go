package services

import (
	"fmt"

	"github.com/filmorauz/backend/repositories"
)

const contentCodeMaxLimit = 999999

func getNextContentCode(movieRepo *repositories.MovieRepository, seriesRepo *repositories.SeriesRepository) (string, error) {
	movieHighest, err := movieRepo.FindHighestCode()
	if err != nil {
		return "", fmt.Errorf("find highest movie code: %w", err)
	}

	seriesHighest, err := seriesRepo.FindHighestCode()
	if err != nil {
		return "", fmt.Errorf("find highest series code: %w", err)
	}

	nextSeq := movieHighest
	if seriesHighest > nextSeq {
		nextSeq = seriesHighest
	}
	nextSeq++

	if nextSeq > contentCodeMaxLimit {
		return "", fmt.Errorf("content code limit exceeded: %d > %d", nextSeq, contentCodeMaxLimit)
	}

	return formatContentCode(nextSeq), nil
}

func formatContentCode(seq int64) string {
	switch {
	case seq <= 9999:
		return fmt.Sprintf("%04d", seq)
	case seq <= 99999:
		return fmt.Sprintf("%05d", seq)
	default:
		return fmt.Sprintf("%06d", seq)
	}
}

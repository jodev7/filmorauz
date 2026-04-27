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

	startSeq := movieHighest
	if seriesHighest > startSeq {
		startSeq = seriesHighest
	}

	for nextSeq := startSeq + 1; nextSeq <= contentCodeMaxLimit; nextSeq++ {
		code := formatContentCode(nextSeq)

		movieExists, err := movieRepo.CodeExists(code)
		if err != nil {
			return "", fmt.Errorf("check movie code exists %s: %w", code, err)
		}
		if movieExists {
			continue
		}

		seriesExists, err := seriesRepo.CodeExists(code)
		if err != nil {
			return "", fmt.Errorf("check series code exists %s: %w", code, err)
		}
		if seriesExists {
			continue
		}

		return code, nil
	}

	return "", fmt.Errorf("content code limit exceeded: %d", contentCodeMaxLimit)
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

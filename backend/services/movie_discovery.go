package services

import (
	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PersonCredits returns an actor's / director's published movies.
func (s *MovieService) PersonCredits(name string) (*repositories.PersonCredits, error) {
	return s.repo.FindPersonCredits(name, 120)
}

// RandomMovie picks a random playable movie (optionally of a genre).
func (s *MovieService) RandomMovie(genre string, exclude []primitive.ObjectID) (*models.Movie, error) {
	return s.repo.RandomMovie(genre, exclude)
}

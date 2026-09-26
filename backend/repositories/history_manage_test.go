package repositories

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestResumeTarget(t *testing.T) {
	ids := make([]primitive.ObjectID, 4)
	order := make([]orderedEpisode, 4)
	for i := range ids {
		ids[i] = primitive.NewObjectID()
		order[i] = orderedEpisode{ID: ids[i], SeasonNumber: 1 + i/2, EpisodeNumber: 1 + i%2}
	}
	prog := map[primitive.ObjectID]EpisodeProgress{
		ids[0]: {Completed: true, ProgressPercent: 100},
		ids[1]: {ProgressPercent: 40},
	}
	if r := resumeTarget(order, prog, ids[1]); r == nil || r.EpisodeID != ids[1].Hex() || r.Mode != "continue" {
		t.Fatalf("mid-episode should continue it: %+v", r)
	}
	// finished S1E1 last → next is S1E2
	if r := resumeTarget(order, prog, ids[0]); r == nil || r.EpisodeID != ids[1].Hex() || r.Mode != "next" {
		t.Fatalf("after finished episode should go next: %+v", r)
	}
	// skip already-finished episodes after the last one
	prog[ids[1]] = EpisodeProgress{Completed: true}
	prog[ids[2]] = EpisodeProgress{Completed: true}
	if r := resumeTarget(order, prog, ids[1]); r == nil || r.EpisodeID != ids[3].Hex() || r.SeasonNumber != 2 {
		t.Fatalf("should skip finished S2E1: %+v", r)
	}
	prog[ids[3]] = EpisodeProgress{Completed: true}
	if r := resumeTarget(order, prog, ids[3]); r != nil {
		t.Fatalf("all watched → nil, got %+v", r)
	}
	if r := resumeTarget(order, prog, primitive.NilObjectID); r != nil {
		t.Fatalf("nothing watched → nil")
	}
}

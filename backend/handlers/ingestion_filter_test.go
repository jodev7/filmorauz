package handlers

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestJobListFilterHidesClipOnlyForEveryStatus(t *testing.T) {
	for _, key := range []string{"all", "active", "pending", "processing", "failed", "stuck", "completed"} {
		f := jobListFilter(key, "uzmovi")
		ct, ok := f["content_type"].(bson.M)
		if !ok || ct["$ne"] != "clip_only" {
			t.Fatalf("%s: clip_only jobs must be excluded, got %v", key, f["content_type"])
		}
		if f["source"] != "uzmovi" {
			t.Fatalf("%s: source filter missing", key)
		}
	}
}

func TestStageBasedFiltersExcludeFinishedJobs(t *testing.T) {
	for _, key := range []string{"pending", "processing"} {
		or, ok := buildJobStatusFilter(key)["$or"].(bson.A)
		if !ok || len(or) != 2 {
			t.Fatalf("%s: unexpected filter %v", key, or)
		}
		stageBranch := or[1].(bson.M)
		if _, ok := stageBranch["status"]; !ok {
			t.Fatalf("%s: stage branch must also exclude finished statuses, got %v", key, stageBranch)
		}
	}
}

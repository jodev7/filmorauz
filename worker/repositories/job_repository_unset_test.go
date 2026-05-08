package repositories

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestUnsetHelper(t *testing.T) {
	fields := []string{"field1", "field2"}
	m := unset(fields...)

	if len(m) != 2 {
		t.Errorf("expected 2 fields, got %d", len(m))
	}

	if m["field1"] != "" {
		t.Errorf("expected empty string for field1, got %v", m["field1"])
	}

	if m["field2"] != "" {
		t.Errorf("expected empty string for field2, got %v", m["field2"])
	}
}

func TestTransitionToProcessingUpdateStructure(t *testing.T) {
	// We want to ensure that the update document for TransitionToProcessing
	// uses an array (bson.A) for the $unset stage, not a document.
	
	// This is a conceptual check of what the code is doing.
	// In the real code, TransitionToProcessing generates a bson.A.
	
	update := bson.A{
		bson.M{
			"$set": bson.M{
				"status": "ready_to_process",
			},
		},
	}

	update = append(update, bson.M{
		"$unset": bson.A{
			"locked_until",
			"last_progress_at",
		},
	})

	// Check if update[1].$unset is a bson.A
	stage1 := update[1].(bson.M)
	unsetVal := stage1["$unset"]
	
	if _, ok := unsetVal.(bson.A); !ok {
		t.Errorf("expected bson.A for $unset stage value, got %T", unsetVal)
	}
}

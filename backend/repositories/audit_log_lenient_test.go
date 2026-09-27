package repositories

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestLenientAuditView(t *testing.T) {
	id := primitive.NewObjectID()
	raw, _ := bson.Marshal(bson.M{
		"_id": id, "actor_id": "u1", "method": "POST", "route": "/api/admin/movies/:id",
		"status": int32(500), "duration_ms": int32(12), "created_at": time.Unix(1_700_000_000, 0),
		"params": bson.M{"id": "42", "n": int32(7)},
		"body":   bson.M{"title": "x"},
		"actor":  bson.M{"first_name": int32(5)}, // the strict decoder rejects this
	})
	var strict AuditLogView
	if bson.Unmarshal(raw, &strict) == nil {
		t.Fatal("expected strict decode to fail for this fixture")
	}
	v := lenientAuditView(raw)
	if v.ID != id || v.ActorID != "u1" || v.Status != 500 || v.DurationMS != 12 || v.Params["id"] != "42" || v.Params["n"] != "7" || v.Body["title"] != "x" {
		t.Fatalf("unexpected %+v", v)
	}
	if v.CreatedAt.Unix() != 1_700_000_000 {
		t.Fatalf("created_at %v", v.CreatedAt)
	}
}

package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The spoiler flag was saved but dropped from the list response, so the
// frontend never blurred spoiler comments.
func TestCommentToDTOKeepsSpoilerFlag(t *testing.T) {
	c := models.CommentWithUser{}
	c.ID = primitive.NewObjectID()
	c.UserID = primitive.NewObjectID()
	c.Content = "oxirida hamma o'ladi"
	c.IsSpoiler = true

	dto := commentToDTO(c)
	if !dto.IsSpoiler {
		t.Fatal("IsSpoiler lost in commentToDTO")
	}
	raw, _ := json.Marshal(dto)
	if !strings.Contains(string(raw), `"is_spoiler":true`) {
		t.Fatalf("is_spoiler missing from JSON: %s", raw)
	}
}

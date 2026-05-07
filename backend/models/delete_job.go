package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type DeleteJob struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ContentType   string             `bson:"content_type" json:"content_type"` // movie | series
	ContentID     primitive.ObjectID `bson:"content_id" json:"content_id"`
	Title         string             `bson:"title" json:"title"`
	Status        string             `bson:"status" json:"status"` // queued | deleting | completed | failed
	Progress      int                `bson:"progress" json:"progress"`
	CurrentStep   string             `bson:"current_step" json:"current_step"`
	DeletedCounts map[string]int     `bson:"deleted_counts" json:"deleted_counts"`
	Error         string             `bson:"error,omitempty" json:"error,omitempty"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time          `bson:"updated_at" json:"updated_at"`
	CompletedAt   *time.Time         `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
}

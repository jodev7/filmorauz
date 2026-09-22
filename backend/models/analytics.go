package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SearchEvent struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Query       string              `bson:"query" json:"query"`
	UserID      *primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`
	IP          string              `bson:"ip,omitempty" json:"ip,omitempty"`
	UserAgent   string              `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	ResultCount int                 `bson:"result_count" json:"result_count"`
	CreatedAt   time.Time           `bson:"created_at" json:"created_at"`
}

type ContentViewEvent struct {
	ID         primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	TargetType string              `bson:"target_type" json:"target_type"`
	TargetID   primitive.ObjectID  `bson:"target_id" json:"target_id"`
	UserID     *primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`
	IP         string              `bson:"ip,omitempty" json:"ip,omitempty"`
	UserAgent  string              `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	CreatedAt  time.Time           `bson:"created_at" json:"created_at"`
}

type PremiumFunnelEvent struct {
	ID         primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	EventType  string              `bson:"event_type" json:"event_type"`
	TargetType string              `bson:"target_type,omitempty" json:"target_type,omitempty"`
	TargetID   *primitive.ObjectID `bson:"target_id,omitempty" json:"target_id,omitempty"`
	UserID     primitive.ObjectID  `bson:"user_id" json:"user_id"`
	Package    string              `bson:"package,omitempty" json:"package,omitempty"`
	IP         string              `bson:"ip,omitempty" json:"ip,omitempty"`
	UserAgent  string              `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	CreatedAt  time.Time           `bson:"created_at" json:"created_at"`
}

type PlaybackReport struct {
	ID         primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	TargetType string              `bson:"target_type" json:"target_type"`
	TargetID   primitive.ObjectID  `bson:"target_id" json:"target_id"`
	UserID     *primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`
	Title      string              `bson:"title,omitempty" json:"title,omitempty"`
	URL        string              `bson:"url,omitempty" json:"url,omitempty"`
	Reason     string              `bson:"reason" json:"reason"`
	Note       string              `bson:"note,omitempty" json:"note,omitempty"`
	Status     string              `bson:"status" json:"status"`
	IP         string              `bson:"ip,omitempty" json:"ip,omitempty"`
	UserAgent  string              `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	CreatedAt  time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time           `bson:"updated_at" json:"updated_at"`
}

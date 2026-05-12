package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SuggestionType represents the type of suggestion
type SuggestionType string

const (
	SuggestionTypeMovie  SuggestionType = "movie"
	SuggestionTypeSeries SuggestionType = "series"
)

// SuggestionStatus represents the status of a suggestion
type SuggestionStatus string

const (
	SuggestionStatusPending  SuggestionStatus = "pending"
	SuggestionStatusAccepted SuggestionStatus = "accepted"
	SuggestionStatusRejected SuggestionStatus = "rejected"
)

// SuggestionUser represents minimal user info for suggestions
type SuggestionUser struct {
	ID               string `json:"id"`
	Username         string `json:"username,omitempty"`
	FullName         string `json:"full_name,omitempty"`
	TelegramUsername string `json:"telegram_username,omitempty"`
}

// Suggestion represents a movie/series suggestion from a user
type Suggestion struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID          primitive.ObjectID `bson:"user_id" json:"user_id"`
	UserName        string             `bson:"user_name" json:"user_name"`
	User            *SuggestionUser    `bson:"-" json:"user,omitempty"`
	Type            SuggestionType     `bson:"type" json:"type"`
	Title           string             `bson:"title" json:"title"`
	Message         string             `bson:"message" json:"message"`
	SourceURL       string             `bson:"source_url,omitempty" json:"source_url,omitempty"`
	ImageURL        string             `bson:"image_url,omitempty" json:"image_url,omitempty"`
	ImageStorageKey string             `bson:"image_storage_key,omitempty" json:"image_storage_key,omitempty"`
	ImageMimeType   string             `bson:"image_mime_type,omitempty" json:"image_mime_type,omitempty"`
	ImageSize       int64              `bson:"image_size,omitempty" json:"image_size,omitempty"`
	Status          SuggestionStatus   `bson:"status" json:"status"`
	AdminMessage    string             `bson:"admin_message,omitempty" json:"admin_message,omitempty"`
	ReviewedBy      string             `bson:"reviewed_by,omitempty" json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time         `bson:"reviewed_at,omitempty" json:"reviewed_at,omitempty"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
}

// SuggestionInput is used for creating a suggestion via JSON
type SuggestionInput struct {
	Type      SuggestionType `json:"type" binding:"required"`
	Title     string         `json:"title" binding:"required"`
	Message   string         `json:"message" binding:"required"`
	SourceURL string         `json:"source_url"`
	ImageURL  string         `json:"image_url"`
}

// SuggestionUpdateInput is used for admin updating a suggestion
type SuggestionUpdateInput struct {
	Status       SuggestionStatus `json:"status" binding:"required"`
	AdminMessage string           `json:"admin_message"`
}

// SuggestionListResponse represents the API response for listing suggestions
type SuggestionListResponse struct {
	Suggestions []Suggestion `json:"suggestions"`
	Total       int64        `json:"total"`
	Page        int          `json:"page"`
	Limit       int          `json:"limit"`
}

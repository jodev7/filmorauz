package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Announcement types.
const (
	// AnnouncementTypeModal is the default center-screen popup.
	AnnouncementTypeModal = "modal"
	// AnnouncementTypeAlert is a thin red banner pinned to the top of every
	// page (e.g. "bugun 19:00–20:00 texnik ishlar"). No link/body — just the
	// title text shown during the time window.
	AnnouncementTypeAlert = "alert"
)

// NormalizeAnnouncementType coerces an arbitrary string to a known type,
// defaulting to modal for empty/legacy values.
func NormalizeAnnouncementType(t string) string {
	if t == AnnouncementTypeAlert {
		return AnnouncementTypeAlert
	}
	return AnnouncementTypeModal
}

// Announcement is a site-wide notice shown to every user across every page
// while it is active and within its time window. Depending on Type it renders
// as a center modal or a thin top alert banner. Examples: scheduled
// maintenance notice, a new feature link, an emergency message.
type Announcement struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Type        string             `bson:"type,omitempty" json:"type"` // "modal" (default) | "alert"
	Title       string             `bson:"title" json:"title"`
	Body        string             `bson:"body" json:"body"`
	LinkURL     string             `bson:"link_url,omitempty" json:"link_url,omitempty"`
	LinkLabel   string             `bson:"link_label,omitempty" json:"link_label,omitempty"`
	StartsAt    time.Time          `bson:"starts_at" json:"starts_at"`
	EndsAt      time.Time          `bson:"ends_at" json:"ends_at"`
	Dismissible bool               `bson:"dismissible" json:"dismissible"`
	IsActive    bool               `bson:"is_active" json:"is_active"`
	Priority    int                `bson:"priority" json:"priority"`
	CreatedBy   primitive.ObjectID `bson:"created_by,omitempty" json:"created_by,omitempty"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`

	// Populated by the admin list handler — not persisted.
	CreatedByName string `bson:"-" json:"created_by_name,omitempty"`
}

// AnnouncementInput is used for create/update requests from the admin UI.
type AnnouncementInput struct {
	Type        string    `json:"type"` // "modal" (default) | "alert"
	Title       string    `json:"title" binding:"required"`
	Body        string    `json:"body"`
	LinkURL     string    `json:"link_url"`
	LinkLabel   string    `json:"link_label"`
	StartsAt    time.Time `json:"starts_at" binding:"required"`
	EndsAt      time.Time `json:"ends_at" binding:"required"`
	Dismissible bool      `json:"dismissible"`
	IsActive    bool      `json:"is_active"`
	Priority    int       `json:"priority"`
}

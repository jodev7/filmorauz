package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TelegramPost struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Text                string             `bson:"text" json:"text"`
	ImageURL            string             `bson:"image_url,omitempty" json:"image_url,omitempty"`
	SendToChannels      bool               `bson:"send_to_channels" json:"send_to_channels"`
	SendToBotUsers      bool               `bson:"send_to_bot_users" json:"send_to_bot_users"`
	ChannelsSentCount   int                `bson:"channels_sent_count" json:"channels_sent_count"`
	ChannelsFailedCount int                `bson:"channels_failed_count" json:"channels_failed_count"`
	BotSentCount        int                `bson:"bot_sent_count" json:"bot_sent_count"`
	BotFailedCount      int                `bson:"bot_failed_count" json:"bot_failed_count"`
	SentByUserID        primitive.ObjectID `bson:"sent_by_user_id" json:"sent_by_user_id"`
	SentByName          string             `bson:"sent_by_name" json:"sent_by_name"`
	SentAt              time.Time          `bson:"sent_at" json:"sent_at"`
	CreatedAt           time.Time          `bson:"created_at" json:"created_at"`
}

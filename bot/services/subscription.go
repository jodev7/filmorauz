package services

import (
	"fmt"
	"log"

	"github.com/filmorauz/bot/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// SubscriptionService handles channel subscription verification
type SubscriptionService struct {
	bot              *tgbotapi.BotAPI
	requiredChannels []models.RequiredChannel
}

// NewSubscriptionService creates a new subscription service
func NewSubscriptionService(bot *tgbotapi.BotAPI, channels []models.RequiredChannel) (*SubscriptionService, error) {
	log.Printf("[SUBSCRIPTION] Initializing with %d required channels", len(channels))

	for i, ch := range channels {
		log.Printf("[SUBSCRIPTION] Channel %d: %s (ID: %d, URL: %s)", i, ch.Title, ch.ID, ch.URL)
	}

	return &SubscriptionService{
		bot:              bot,
		requiredChannels: channels,
	}, nil
}

// RequiredChannelCount returns the number of required channels
func (s *SubscriptionService) RequiredChannelCount() int {
	return len(s.requiredChannels)
}

// GetRequiredChannels returns all required channels
func (s *SubscriptionService) GetRequiredChannels() []models.RequiredChannel {
	return s.requiredChannels
}

// GetMissingChannels returns only the channels the user is NOT subscribed to
func (s *SubscriptionService) GetMissingChannels(userID int64) ([]models.RequiredChannel, error) {
	var missing []models.RequiredChannel

	for _, channel := range s.requiredChannels {
		subscribed, err := s.checkChannelMembership(userID, channel.ID)
		if err != nil {
			// If error checking, treat as missing (deny by default)
			log.Printf("[SUBSCRIPTION] Error checking channel %s (ID: %d): %v - treating as missing",
				channel.Title, channel.ID, err)
			missing = append(missing, channel)
			continue
		}

		if !subscribed {
			log.Printf("[SUBSCRIPTION] User %d is NOT subscribed to %s (ID: %d)", userID, channel.Title, channel.ID)
			missing = append(missing, channel)
		} else {
			log.Printf("[SUBSCRIPTION] User %d IS subscribed to %s (ID: %d)", userID, channel.Title, channel.ID)
		}
	}

	return missing, nil
}

// CheckUserSubscriptions checks if user is subscribed to ALL required channels
// Returns SubscriptionStatus with missing channels info
func (s *SubscriptionService) CheckUserSubscriptions(userID int64) *models.SubscriptionStatus {
	// If no required channels, allow access
	if len(s.requiredChannels) == 0 {
		log.Printf("[SUBSCRIPTION] No required channels - allowing user %d", userID)
		return &models.SubscriptionStatus{
			IsSubscribed: true,
			MissingChans: nil,
		}
	}

	missing, err := s.GetMissingChannels(userID)
	if err != nil {
		log.Printf("[SUBSCRIPTION] Error checking subscriptions for user %d: %v", userID, err)
		// Deny on error
		return &models.SubscriptionStatus{
			IsSubscribed: false,
			MissingChans: s.requiredChannels,
		}
	}

	isSubscribed := len(missing) == 0
	return &models.SubscriptionStatus{
		IsSubscribed: isSubscribed,
		MissingChans: missing,
	}
}

// checkChannelMembership checks if user is subscribed to a specific channel
func (s *SubscriptionService) checkChannelMembership(userID int64, channelID int64) (bool, error) {
	config := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: channelID,
			UserID: userID,
		},
	}

	chatMember, err := s.bot.GetChatMember(config)
	if err != nil {
		log.Printf("[SUBSCRIPTION] getChatMember error for user %d, channel %d: %v", userID, channelID, err)
		return false, fmt.Errorf("getChatMember failed: %w", err)
	}

	// Check status - these mean user is subscribed
	switch chatMember.Status {
	case "member", "administrator", "creator":
		return true, nil
	case "left", "kicked":
		return false, nil
	default:
		// Unknown status - treat as not subscribed
		log.Printf("[SUBSCRIPTION] Unknown status '%s' for user %d in channel %d",
			chatMember.Status, userID, channelID)
		return false, nil
	}
}

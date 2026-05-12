package services

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// SendTextPostToChannel sends a text-only post to a Telegram channel
func (s *TelegramService) SendTextPostToChannel(channelTarget, text string) AdPostResult {
	if s.botToken == "" {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: "bot token not configured"}
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: err.Error()}
	}

	msg := tgbotapi.NewMessageToChannel(channelTarget, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = false

	sentMsg, sendErr := api.Send(msg)
	if sendErr != nil {
		log.Printf("[TELEGRAM POST] FAILED channel=%s error=%v", channelTarget, sendErr)
		if s.IsBlockedError(sendErr) {
			return AdPostResult{Target: channelTarget, Status: "blocked", Error: sendErr.Error(), Blocked: true}
		}
		return AdPostResult{Target: channelTarget, Status: "failed", Error: sendErr.Error()}
	}

	log.Printf("[TELEGRAM POST] ✓ channel=%s msg_id=%d", channelTarget, sentMsg.MessageID)
	return AdPostResult{Target: channelTarget, Status: "success", MessageID: sentMsg.MessageID}
}

// SendTextPostToBot sends a text-only post to a specific chat via the bot
func (s *TelegramService) SendTextPostToBot(chatID int64, text string) AdPostResult {
	target := fmt.Sprintf("bot:%d", chatID)
	if s.botToken == "" {
		return AdPostResult{Target: target, Status: "failed", Error: "bot token not configured"}
	}
	if chatID == 0 {
		return AdPostResult{Target: target, Status: "failed", Error: "chatID is 0"}
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: target, Status: "failed", Error: err.Error()}
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"

	sentMsg, sendErr := api.Send(msg)
	if sendErr != nil {
		log.Printf("[TELEGRAM POST] FAILED bot chat_id=%d error=%v", chatID, sendErr)
		return AdPostResult{Target: target, Status: "failed", Error: sendErr.Error()}
	}

	log.Printf("[TELEGRAM POST] ✓ bot chat_id=%d msg_id=%d", chatID, sentMsg.MessageID)
	return AdPostResult{Target: target, Status: "success", MessageID: sentMsg.MessageID}
}

// SendPostWithImageToChannel sends a photo with caption to a Telegram channel
func (s *TelegramService) SendPostWithImageToChannel(channelTarget, text, imageURL string) AdPostResult {
	if s.botToken == "" {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: "bot token not configured"}
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: err.Error()}
	}

	preparedMedia, preparedMode, prepErr := s.prepareTelegramPhoto(imageURL, "image_url")
	if prepErr != nil {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: prepErr.Error()}
	}

	log.Printf("[TELEGRAM POST] channel=%s method=sendPhoto mode=%s", channelTarget, preparedMode)
	msg := tgbotapi.NewPhotoToChannel(channelTarget, preparedMedia)
	msg.Caption = text
	msg.ParseMode = "HTML"

	sentMsg, sendErr := api.Send(msg)
	if sendErr != nil {
		log.Printf("[TELEGRAM POST] FAILED channel=%s error=%v", channelTarget, sendErr)
		if s.IsBlockedError(sendErr) {
			return AdPostResult{Target: channelTarget, Status: "blocked", Error: sendErr.Error(), Blocked: true}
		}
		return AdPostResult{Target: channelTarget, Status: "failed", Error: sendErr.Error()}
	}

	log.Printf("[TELEGRAM POST] ✓ channel=%s msg_id=%d", channelTarget, sentMsg.MessageID)
	return AdPostResult{Target: channelTarget, Status: "success", MessageID: sentMsg.MessageID}
}

// SendPostWithImageToBot sends a photo with caption to a specific chat via the bot
func (s *TelegramService) SendPostWithImageToBot(chatID int64, text, imageURL string) AdPostResult {
	target := fmt.Sprintf("bot:%d", chatID)
	if s.botToken == "" {
		return AdPostResult{Target: target, Status: "failed", Error: "bot token not configured"}
	}
	if chatID == 0 {
		return AdPostResult{Target: target, Status: "failed", Error: "chatID is 0"}
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: target, Status: "failed", Error: err.Error()}
	}

	preparedMedia, preparedMode, prepErr := s.prepareTelegramPhoto(imageURL, "image_url")
	if prepErr != nil {
		return AdPostResult{Target: target, Status: "failed", Error: prepErr.Error()}
	}

	log.Printf("[TELEGRAM POST] bot chat_id=%d method=sendPhoto mode=%s", chatID, preparedMode)
	msg := tgbotapi.NewPhoto(chatID, preparedMedia)
	msg.Caption = text
	msg.ParseMode = "HTML"

	sentMsg, sendErr := api.Send(msg)
	if sendErr != nil {
		log.Printf("[TELEGRAM POST] FAILED bot chat_id=%d error=%v", chatID, sendErr)
		return AdPostResult{Target: target, Status: "failed", Error: sendErr.Error()}
	}

	log.Printf("[TELEGRAM POST] ✓ bot chat_id=%d msg_id=%d", chatID, sentMsg.MessageID)
	return AdPostResult{Target: target, Status: "success", MessageID: sentMsg.MessageID}
}

// SendTextPostWithButtonToChannel sends a text post with an inline URL button to a channel.
func (s *TelegramService) SendTextPostWithButtonToChannel(channelTarget, text, buttonText, buttonURL string) AdPostResult {
	if s.botToken == "" {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: "bot token not configured"}
	}
	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: err.Error()}
	}
	msg := tgbotapi.NewMessageToChannel(channelTarget, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonURL(buttonText, buttonURL)),
	)
	sentMsg, sendErr := api.Send(msg)
	if sendErr != nil {
		if s.IsBlockedError(sendErr) {
			return AdPostResult{Target: channelTarget, Status: "blocked", Error: sendErr.Error(), Blocked: true}
		}
		return AdPostResult{Target: channelTarget, Status: "failed", Error: sendErr.Error()}
	}
	return AdPostResult{Target: channelTarget, Status: "success", MessageID: sentMsg.MessageID}
}

// SendPostWithImageAndButtonToChannel sends a photo with caption and inline button to a channel.
func (s *TelegramService) SendPostWithImageAndButtonToChannel(channelTarget, text, imageURL, buttonText, buttonURL string) AdPostResult {
	if s.botToken == "" {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: "bot token not configured"}
	}
	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: err.Error()}
	}
	preparedMedia, _, prepErr := s.prepareTelegramPhoto(imageURL, "image_url")
	if prepErr != nil {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: prepErr.Error()}
	}
	msg := tgbotapi.NewPhotoToChannel(channelTarget, preparedMedia)
	msg.Caption = text
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonURL(buttonText, buttonURL)),
	)
	sentMsg, sendErr := api.Send(msg)
	if sendErr != nil {
		if s.IsBlockedError(sendErr) {
			return AdPostResult{Target: channelTarget, Status: "blocked", Error: sendErr.Error(), Blocked: true}
		}
		return AdPostResult{Target: channelTarget, Status: "failed", Error: sendErr.Error()}
	}
	return AdPostResult{Target: channelTarget, Status: "success", MessageID: sentMsg.MessageID}
}

// SendTextPostWithButtonToBot sends a text post with an inline URL button to a bot chat.
func (s *TelegramService) SendTextPostWithButtonToBot(chatID int64, text, buttonText, buttonURL string) AdPostResult {
	target := fmt.Sprintf("bot:%d", chatID)
	if s.botToken == "" {
		return AdPostResult{Target: target, Status: "failed", Error: "bot token not configured"}
	}
	if chatID == 0 {
		return AdPostResult{Target: target, Status: "failed", Error: "chatID is 0"}
	}
	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: target, Status: "failed", Error: err.Error()}
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonURL(buttonText, buttonURL)),
	)
	sentMsg, sendErr := api.Send(msg)
	if sendErr != nil {
		if s.IsBlockedError(sendErr) {
			return AdPostResult{Target: target, Status: "blocked", Error: sendErr.Error(), Blocked: true}
		}
		return AdPostResult{Target: target, Status: "failed", Error: sendErr.Error()}
	}
	return AdPostResult{Target: target, Status: "success", MessageID: sentMsg.MessageID}
}

// SendPostWithImageAndButtonToBot sends a photo with caption and inline button to a bot chat.
func (s *TelegramService) SendPostWithImageAndButtonToBot(chatID int64, text, imageURL, buttonText, buttonURL string) AdPostResult {
	target := fmt.Sprintf("bot:%d", chatID)
	if s.botToken == "" {
		return AdPostResult{Target: target, Status: "failed", Error: "bot token not configured"}
	}
	if chatID == 0 {
		return AdPostResult{Target: target, Status: "failed", Error: "chatID is 0"}
	}
	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: target, Status: "failed", Error: err.Error()}
	}
	preparedMedia, _, prepErr := s.prepareTelegramPhoto(imageURL, "image_url")
	if prepErr != nil {
		return AdPostResult{Target: target, Status: "failed", Error: prepErr.Error()}
	}
	msg := tgbotapi.NewPhoto(chatID, preparedMedia)
	msg.Caption = text
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonURL(buttonText, buttonURL)),
	)
	sentMsg, sendErr := api.Send(msg)
	if sendErr != nil {
		if s.IsBlockedError(sendErr) {
			return AdPostResult{Target: target, Status: "blocked", Error: sendErr.Error(), Blocked: true}
		}
		return AdPostResult{Target: target, Status: "failed", Error: sendErr.Error()}
	}
	return AdPostResult{Target: target, Status: "success", MessageID: sentMsg.MessageID}
}

// ParseChannelsFromEnv parses TELEGRAM_CHANNELS env variable into channel list
func ParseChannelsFromEnv(channelsEnv string) []string {
	if channelsEnv == "" {
		return []string{}
	}
	parts := strings.Split(channelsEnv, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			if !strings.HasPrefix(trimmed, "@") {
				trimmed = "@" + trimmed
			}
			result = append(result, trimmed)
		}
	}
	return result
}

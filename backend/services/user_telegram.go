package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/filmorauz/backend/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Telegram direct messages to individual users (via the site bot), used for
// notifications that are worth leaving the site for: a new episode of a
// followed series, a suggestion that was added, etc.

var (
	userBotMu  sync.Mutex
	userBotAPI *tgbotapi.BotAPI
)

// SetSiteURL sets the public site URL used to build absolute links in
// Telegram messages.
func (s *NotificationService) SetSiteURL(url string) {
	s.siteURL = strings.TrimRight(strings.TrimSpace(url), "/")
}

func (s *NotificationService) botAPI() (*tgbotapi.BotAPI, error) {
	if s.botToken == "" {
		return nil, fmt.Errorf("bot token not configured")
	}
	userBotMu.Lock()
	defer userBotMu.Unlock()
	if userBotAPI != nil {
		return userBotAPI, nil
	}
	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return nil, err
	}
	userBotAPI = api
	return api, nil
}

func (s *NotificationService) absoluteURL(path string) string {
	if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	base := s.siteURL
	if base == "" {
		base = "https://filmorauz.net"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// SendTelegramToUser sends an HTML message with an optional link button to a
// user's private chat with the bot. Users who never started the bot (or
// blocked it) are skipped/logged — never an error for the caller's flow.
func (s *NotificationService) SendTelegramToUser(user *models.User, htmlText, buttonText, actionPath string) bool {
	if user == nil {
		return false
	}
	chatID := user.TelegramChatID
	if chatID == 0 {
		chatID = user.TelegramID
	}
	if chatID == 0 {
		return false
	}
	api, err := s.botAPI()
	if err != nil {
		return false
	}
	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	if buttonText != "" && actionPath != "" {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonURL(buttonText, s.absoluteURL(actionPath))),
		)
	}
	if _, err := api.Send(msg); err != nil {
		log.Printf("[NOTIFY] telegram to user %s failed: %v", user.ID.Hex(), err)
		return false
	}
	return true
}

// NotifyUserBoth creates an in-app notification and, when possible, sends the
// same news to the user's Telegram.
func (s *NotificationService) NotifyUserBoth(ctx context.Context, userID primitive.ObjectID, typ models.NotificationType, title, message, actionPath string, data map[string]interface{}, telegramHTML, buttonText string) {
	if err := s.CreateNotification(ctx, &models.NotificationCreateRequest{
		UserID:    userID,
		Type:      typ,
		Title:     title,
		Message:   message,
		ActionURL: actionPath,
		Data:      data,
	}); err != nil {
		log.Printf("[NOTIFY] in-app %s for %s failed: %v", typ, userID.Hex(), err)
	}
	if s.userRepo == nil || telegramHTML == "" {
		return
	}
	user, err := s.userRepo.FindByID(userID.Hex())
	if err != nil || user == nil {
		return
	}
	s.SendTelegramToUser(user, telegramHTML, buttonText, actionPath)
	// Stay well under Telegram's ~30 msg/s bot limit during fan-outs.
	time.Sleep(40 * time.Millisecond)
}

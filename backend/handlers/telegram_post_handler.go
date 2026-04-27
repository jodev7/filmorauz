package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/filmorauz/backend/config"
	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TelegramPostHandler struct {
	telegramService *services.TelegramService
	userRepo        *repositories.UserRepository
	postRepo        *repositories.TelegramPostRepository
}

func NewTelegramPostHandler(telegramService *services.TelegramService, userRepo *repositories.UserRepository, postRepo *repositories.TelegramPostRepository) *TelegramPostHandler {
	return &TelegramPostHandler{
		telegramService: telegramService,
		userRepo:        userRepo,
		postRepo:        postRepo,
	}
}

type TelegramPostRequest struct {
	Text             string `json:"text" binding:"required"`
	ImageURL         string `json:"image_url"`
	SendToChannels   bool   `json:"send_to_channels"`
	SendToBot        bool   `json:"send_to_bot"`
	InlineButtonText string `json:"inline_button_text"`
	InlineButtonURL  string `json:"inline_button_url"`
}

type TelegramPostResponse struct {
	ChannelsSent   int      `json:"channels_sent"`
	ChannelsFailed int      `json:"channels_failed"`
	BotSent        int      `json:"bot_sent"`
	BotFailed      int      `json:"bot_failed"`
	Errors         []string `json:"errors,omitempty"`
}

func (h *TelegramPostHandler) SendPost(c *gin.Context) {
	var req TelegramPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[TELEGRAM POST] Invalid request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}

	if !req.SendToChannels && !req.SendToBot {
		c.JSON(http.StatusBadRequest, gin.H{"error": "select at least one target: channels or bot"})
		return
	}

	req.ImageURL = normalizeTelegramPostImageURL(req.ImageURL)
	publicTelegramImageURL := absoluteTelegramPostImageURL(req.ImageURL)

	hasButton := strings.TrimSpace(req.InlineButtonText) != "" || strings.TrimSpace(req.InlineButtonURL) != ""
	if hasButton {
		if strings.TrimSpace(req.InlineButtonText) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "inline_button_text is required when using inline button"})
			return
		}
		if strings.TrimSpace(req.InlineButtonURL) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "inline_button_url is required when using inline button"})
			return
		}
		if !strings.HasPrefix(req.InlineButtonURL, "http://") && !strings.HasPrefix(req.InlineButtonURL, "https://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "inline_button_url must start with http:// or https://"})
			return
		}
	}

	response := &TelegramPostResponse{
		ChannelsSent:   0,
		ChannelsFailed: 0,
		BotSent:        0,
		BotFailed:      0,
		Errors:         []string{},
	}

	if h.telegramService == nil {
		response.Errors = append(response.Errors, "telegram service not configured")
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	channelsEnv := os.Getenv("TELEGRAM_CHANNELS")
	channelUsernames := parseChannelList(channelsEnv)

	btnText := strings.TrimSpace(req.InlineButtonText)
	btnURL := strings.TrimSpace(req.InlineButtonURL)

	if req.SendToChannels && len(channelUsernames) > 0 {
		for _, username := range channelUsernames {
			target := username
			if !strings.HasPrefix(target, "@") {
				target = "@" + target
			}

			var result services.AdPostResult
			switch {
			case publicTelegramImageURL != "" && hasButton:
				result = h.telegramService.SendPostWithImageAndButtonToChannel(target, req.Text, publicTelegramImageURL, btnText, btnURL)
			case publicTelegramImageURL != "":
				result = h.telegramService.SendPostWithImageToChannel(target, req.Text, publicTelegramImageURL)
			case hasButton:
				result = h.telegramService.SendTextPostWithButtonToChannel(target, req.Text, btnText, btnURL)
			default:
				result = h.telegramService.SendTextPostToChannel(target, req.Text)
			}

			if result.Status == "success" {
				response.ChannelsSent++
			} else {
				response.ChannelsFailed++
				if result.Error != "" {
					response.Errors = append(response.Errors, username+": "+result.Error)
				}
			}
		}
	}

	if req.SendToBot {
		chatIDs, err := h.userRepo.FindChatIDs(5000)
		if err != nil {
			response.Errors = append(response.Errors, "failed to load bot users")
		} else {
			for _, chatID := range chatIDs {
				var result services.AdPostResult
				switch {
				case publicTelegramImageURL != "" && hasButton:
					result = h.telegramService.SendPostWithImageAndButtonToBot(chatID, req.Text, publicTelegramImageURL, btnText, btnURL)
				case publicTelegramImageURL != "":
					result = h.telegramService.SendPostWithImageToBot(chatID, req.Text, publicTelegramImageURL)
				case hasButton:
					result = h.telegramService.SendTextPostWithButtonToBot(chatID, req.Text, btnText, btnURL)
				default:
					result = h.telegramService.SendTextPostToBot(chatID, req.Text)
				}

				if result.Status == "success" {
					response.BotSent++
				} else {
					response.BotFailed++
					if result.Error != "" && len(response.Errors) < 50 {
						response.Errors = append(response.Errors, fmt.Sprintf("bot:%d: %s", chatID, result.Error))
					}
				}
			}
		}
	}

	// Save post history
	var sentByUserID primitive.ObjectID
	sentByName := "Unknown"
	if uid, ok := c.Get("user_id"); ok {
		if uidStr, ok := uid.(string); ok {
			if oid, err := primitive.ObjectIDFromHex(uidStr); err == nil {
				sentByUserID = oid
				if user, err := h.userRepo.FindByID(uidStr); err == nil && user != nil {
					if user.DisplayName != "" {
						sentByName = user.DisplayName
					} else if user.TelegramUser != "" {
						sentByName = "@" + user.TelegramUser
					} else if user.FirstName != "" {
						sentByName = user.FirstName
					}
				}
			}
		}
	}

	historyPost := &models.TelegramPost{
		Text:                req.Text,
		ImageURL:            req.ImageURL,
		SendToChannels:      req.SendToChannels,
		SendToBotUsers:      req.SendToBot,
		InlineButtonText:    btnText,
		InlineButtonURL:     btnURL,
		ChannelsSentCount:   response.ChannelsSent,
		ChannelsFailedCount: response.ChannelsFailed,
		BotSentCount:        response.BotSent,
		BotFailedCount:      response.BotFailed,
		SentByUserID:        sentByUserID,
		SentByName:          sentByName,
	}
	if err := h.postRepo.Create(historyPost); err != nil {
		log.Printf("[TELEGRAM POST] failed to save history: %v", err)
	}

	c.JSON(http.StatusOK, response)
}

func parseChannelList(channelsEnv string) []string {
	if channelsEnv == "" {
		return []string{}
	}
	var result []string
	for _, ch := range strings.Split(channelsEnv, ",") {
		ch = strings.TrimSpace(ch)
		if ch != "" {
			result = append(result, ch)
		}
	}
	return result
}

func normalizeTelegramPostImageURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	value = stripMediaPrefix(value)
	if strings.HasPrefix(value, "/uploads/") {
		return value
	}

	mediaPath, _, ok := extractMediaPath(value)
	if !ok {
		return value
	}
	return mediaPath
}

func absoluteTelegramPostImageURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	value = stripMediaPrefix(value)
	cfg := config.Current()
	if cfg == nil {
		return value
	}
	if strings.HasPrefix(value, "/uploads/") {
		return strings.TrimSuffix(cfg.GetBaseURL(), "/") + strings.TrimPrefix(value, "/uploads")
	}
	if strings.HasPrefix(value, "/") {
		return cfg.GetCDNFileURL(strings.TrimPrefix(value, "/"))
	}
	return value
}

// stripMediaPrefix removes a leading "/media/" segment from a path-only URL.
// Used to clean legacy values that were saved with the deprecated protected-media prefix.
func stripMediaPrefix(value string) string {
	if strings.HasPrefix(value, "/media/") {
		return "/" + strings.TrimPrefix(value, "/media/")
	}
	return value
}

func (h *TelegramPostHandler) ListPosts(c *gin.Context) {
	posts, err := h.postRepo.List(50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"posts": posts})
}

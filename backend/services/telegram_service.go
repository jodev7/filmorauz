package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramChannel represents a Telegram channel for posting movies
type TelegramChannel struct {
	ID       int64    // Numeric chat ID (e.g., -1001234567890)
	Username string   // Channel username (without @)
	Title    string   // Display title
	Genres   []string // Genre keywords that route to this channel
}

// TelegramMovieData represents movie data for Telegram notification
type TelegramMovieData struct {
	Title       string   `json:"title"`
	Year        int      `json:"year"`
	Genres      []string `json:"genres"`
	GenresUz    []string `json:"genres_uz"`
	Code        string   `json:"code"`
	PosterURL   string   `json:"poster_url"`
	Quality     string   `json:"quality"`
	Duration    int      `json:"duration"`
	Description string   `json:"description"`
	MovieURL    string   `json:"movie_url"` // Full URL to watch the movie
	Slug        string   `json:"slug"`
}

// TelegramNotificationResult holds the result of a Telegram notification attempt
type TelegramNotificationResult struct {
	Success       bool     `json:"success"`
	ChannelPosted []string `json:"channel_posted"` // Channels that were successfully posted to
	ChannelFailed []string `json:"channel_failed"` // Channels that failed
	BotNotified   bool     `json:"bot_notified"`   // Whether admin bot notification was sent
	ErrorMessage  string   `json:"error_message,omitempty"`
}

// TelegramService handles Telegram notifications for new movies
type TelegramService struct {
	botToken        string
	botUsername     string
	channelUsername string
	adminTelegramID int64
	baseSiteURL     string
	channels        []TelegramChannel
	httpClient      *http.Client
}

// TelegramConfig holds configuration for Telegram service
type TelegramConfig struct {
	BotToken        string
	BotUsername     string
	ChannelUsername string
	AdminTelegramID int64
	BaseSiteURL     string
	MovieChannels   []TelegramChannel // Genre-routed channels
}

// NewTelegramService creates a new Telegram notification service
func NewTelegramService(config TelegramConfig) (*TelegramService, error) {
	// Set default channels if none configured
	channels := config.MovieChannels
	if len(channels) == 0 {
		// Default channels - these should be configured via environment
		channels = []TelegramChannel{
			{Genres: []string{"anime"}, Title: "Anime", Username: "anime_channel", ID: 0},
			{Genres: []string{"serial", "drama", "series"}, Title: "Serials", Username: "serials_channel", ID: 0},
			{Genres: []string{"main", "movie", "film"}, Title: "Movies", Username: "movie_channel", ID: 0},
		}
	}

	return &TelegramService{
		botToken:        config.BotToken,
		botUsername:     config.BotUsername,
		channelUsername: config.ChannelUsername,
		adminTelegramID: config.AdminTelegramID,
		baseSiteURL:     config.BaseSiteURL,
		channels:        channels,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// SendMovieNotification sends a movie notification to all appropriate channels
// Returns result with success/failure status for each channel
func (s *TelegramService) SendMovieNotification(movie *TelegramMovieData) *TelegramNotificationResult {
	result := &TelegramNotificationResult{
		Success:       true,
		ChannelPosted: []string{},
		ChannelFailed: []string{},
	}

	// Determine which channels to post to based on genres
	targetChannels := s.getTargetChannels(movie.Genres)
	if len(targetChannels) == 0 {
		// Default to main channel if no genre match
		for _, ch := range s.channels {
			if containsGenre(ch.Genres, "main") || containsGenre(ch.Genres, "movie") {
				targetChannels = append(targetChannels, ch)
			}
		}
	}

	log.Printf("[TELEGRAM] Sending notification for movie: %s (%d), genres: %v", movie.Title, movie.Year, movie.Genres)
	log.Printf("[TELEGRAM] Target channels: %d", len(targetChannels))

	// Post to each channel
	for _, channel := range targetChannels {
		if channel.ID == 0 {
			log.Printf("[TELEGRAM] Skipping channel %s - ID not configured", channel.Title)
			continue
		}

		success := s.postToChannel(movie, channel)
		if success {
			result.ChannelPosted = append(result.ChannelPosted, channel.Title)
			log.Printf("[TELEGRAM] ✓ Posted to channel: %s", channel.Title)
		} else {
			result.ChannelFailed = append(result.ChannelFailed, channel.Title)
			result.Success = false
			log.Printf("[TELEGRAM] ✗ Failed to post to channel: %s", channel.Title)
		}
	}

	// Also post to the main/default channel if no genre-specific posts succeeded
	if len(result.ChannelPosted) == 0 {
		log.Printf("[TELEGRAM] No genre-specific posts succeeded, trying default channel")
		if s.channelUsername != "" {
			// Try posting to the default channel by username
			success := s.postToDefaultChannel(movie)
			if success {
				result.ChannelPosted = append(result.ChannelPosted, "default")
				log.Printf("[TELEGRAM] ✓ Posted to default channel")
			} else {
				result.ChannelFailed = append(result.ChannelFailed, "default")
				result.Success = false
			}
		}
	}

	// Send admin notification to bot
	if s.adminTelegramID > 0 && s.botToken != "" {
		botSuccess := s.sendAdminNotification(movie)
		result.BotNotified = botSuccess
		if botSuccess {
			log.Printf("[TELEGRAM] ✓ Admin bot notification sent")
		} else {
			log.Printf("[TELEGRAM] ✗ Admin bot notification failed")
		}
	}

	return result
}

// getTargetChannels returns channels that match the given genres
func (s *TelegramService) getTargetChannels(genres []string) []TelegramChannel {
	var matching []TelegramChannel

	for _, channel := range s.channels {
		for _, genre := range genres {
			if containsGenre(channel.Genres, strings.ToLower(genre)) {
				matching = append(matching, channel)
				break
			}
		}
	}

	return matching
}

// containsGenre checks if a genre keyword is in the list
func containsGenre(genreList []string, genre string) bool {
	for _, g := range genreList {
		if strings.Contains(strings.ToLower(g), strings.ToLower(genre)) {
			return true
		}
	}
	return false
}

// postToChannel sends a movie to a specific Telegram channel
func (s *TelegramService) postToChannel(movie *TelegramMovieData, channel TelegramChannel) bool {
	if s.botToken == "" {
		log.Printf("[TELEGRAM] Cannot post to channel: bot token not configured")
		return false
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		log.Printf("[TELEGRAM] Failed to create bot API: %v", err)
		return false
	}

	// Build message
	caption := s.buildMovieCaption(movie)

	// Use poster if available
	posterURL := movie.PosterURL
	if posterURL == "" {
		// Try without poster
		msg := tgbotapi.NewMessage(channel.ID, caption)
		msg.ParseMode = "HTML"
		msg.DisableWebPagePreview = false

		_, err := api.Send(msg)
		if err != nil {
			log.Printf("[TELEGRAM] Failed to send message to channel %s: %v", channel.Title, err)
			return false
		}
		return true
	}

	// Send photo with caption
	photoMsg := tgbotapi.NewPhoto(channel.ID, tgbotapi.FileURL(posterURL))
	photoMsg.Caption = caption
	photoMsg.ParseMode = "HTML"

	_, err = api.Send(photoMsg)
	if err != nil {
		log.Printf("[TELEGRAM] Failed to send photo to channel %s: %v", channel.Title, err)
		// Fallback to text message
		msg := tgbotapi.NewMessage(channel.ID, caption)
		msg.ParseMode = "HTML"
		_, err := api.Send(msg)
		if err != nil {
			log.Printf("[TELEGRAM] Failed to send fallback message: %v", err)
			return false
		}
	}

	return true
}

// postToDefaultChannel posts to the default channel (using username)
func (s *TelegramService) postToDefaultChannel(movie *TelegramMovieData) bool {
	if s.botToken == "" || s.channelUsername == "" {
		return false
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return false
	}

	caption := s.buildMovieCaption(movie)
	posterURL := movie.PosterURL

	// Use the channel username directly - Telegram bot API accepts @username format
	channelIdentifier := "@" + s.channelUsername

	if posterURL != "" {
		photoMsg := tgbotapi.NewPhotoToChannel(channelIdentifier, tgbotapi.FileURL(posterURL))
		photoMsg.Caption = caption
		photoMsg.ParseMode = "HTML"
		_, err = api.Send(photoMsg)
	} else {
		msg := tgbotapi.NewMessageToChannel(channelIdentifier, caption)
		msg.ParseMode = "HTML"
		_, err = api.Send(msg)
	}

	if err != nil {
		log.Printf("[TELEGRAM] Failed to send to channel @%s: %v", s.channelUsername, err)
		return false
	}

	return true
}

// sendAdminNotification sends a notification to the admin via bot
func (s *TelegramService) sendAdminNotification(movie *TelegramMovieData) bool {
	if s.botToken == "" || s.adminTelegramID == 0 {
		return false
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return false
	}

	// Build admin notification message
	message := s.buildAdminNotification(movie)

	msg := tgbotapi.NewMessage(s.adminTelegramID, message)
	msg.ParseMode = "HTML"

	_, err = api.Send(msg)
	if err != nil {
		log.Printf("[TELEGRAM] Failed to send admin notification: %v", err)
		return false
	}

	return true
}

// buildMovieCaption builds the caption/text for a movie post
func (s *TelegramService) buildMovieCaption(movie *TelegramMovieData) string {
	var b strings.Builder

	// Title
	b.WriteString("🎬 <b>")
	b.WriteString(movie.Title)
	b.WriteString("</b>\n")

	// Year and Quality
	if movie.Year > 0 {
		b.WriteString(fmt.Sprintf("📅 Yili: %d\n", movie.Year))
	}
	if movie.Quality != "" {
		b.WriteString(fmt.Sprintf("🎞 Sifati: %s\n", movie.Quality))
	}
	if movie.Duration > 0 {
		b.WriteString(fmt.Sprintf("⏱ Davomiyligi: %d daqiqa\n", movie.Duration))
	}

	// Genre (prefer Uzbek genres if available)
	genres := movie.Genres
	if len(movie.GenresUz) > 0 {
		genres = movie.GenresUz
	}
	if len(genres) > 0 {
		b.WriteString(fmt.Sprintf("🎭 Janr: %s\n", strings.Join(genres, ", ")))
	}

	// Description (truncated)
	if movie.Description != "" {
		desc := movie.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		b.WriteString("\n")
		b.WriteString(desc)
	}

	// Movie code
	if movie.Code != "" {
		b.WriteString(fmt.Sprintf("\n\n🔢 Kod: <code>%s</code>", movie.Code))
	}

	// Watch link button
	if movie.MovieURL != "" {
		b.WriteString("\n\n👉 <a href=\"")
		b.WriteString(movie.MovieURL)
		b.WriteString("\">Filmni tomosha qilish</a>")
	}

	// Channel branding
	if s.channelUsername != "" {
		b.WriteString("\n\n📢 Bizning kanal: @")
		b.WriteString(s.channelUsername)
	}

	// Bot branding
	if s.botUsername != "" {
		b.WriteString("\n🤖 Bizning bot: @")
		b.WriteString(s.botUsername)
	}

	return b.String()
}

// buildAdminNotification builds the admin notification message
func (s *TelegramService) buildAdminNotification(movie *TelegramMovieData) string {
	var b strings.Builder

	b.WriteString("✅ <b>Yangi kino qo'shildi</b>\n\n")
	b.WriteString("🎬 <b>")
	b.WriteString(movie.Title)
	b.WriteString("</b>\n")

	if movie.Year > 0 {
		b.WriteString(fmt.Sprintf("📅 Yili: %d\n", movie.Year))
	}
	if movie.Code != "" {
		b.WriteString(fmt.Sprintf("🔢 Kod: %s\n", movie.Code))
	}
	if len(movie.Genres) > 0 {
		b.WriteString(fmt.Sprintf("🎭 Janr: %s\n", strings.Join(movie.Genres, ", ")))
	}

	if movie.MovieURL != "" {
		b.WriteString(fmt.Sprintf("\n🔗 Link: %s", movie.MovieURL))
	}

	return b.String()
}

// AdPostResult holds the result of posting a single ad to Telegram
type AdPostResult struct {
	Target    string `json:"target"`
	Status    string `json:"status"` // "success" | "failed"
	MessageID int    `json:"message_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// SendAdToChannel sends an ad to a specific Telegram channel (by @username or chat_id string)
func (s *TelegramService) SendAdToChannel(channelTarget, title, description, imageURL, targetURL, cta string) AdPostResult {
	if s.botToken == "" {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: "bot token not configured"}
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: err.Error()}
	}

	caption := buildAdCaption(title, description, targetURL, cta)

	// Inline keyboard with CTA button
	var keyboard *tgbotapi.InlineKeyboardMarkup
	if targetURL != "" {
		ctaText := cta
		if ctaText == "" {
			ctaText = "Ko'proq bilish ›"
		}
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(ctaText, targetURL),
			),
		)
		keyboard = &kb
	}

	var sentMsg tgbotapi.Message
	var sendErr error

	if imageURL != "" {
		msg := tgbotapi.NewPhotoToChannel(channelTarget, tgbotapi.FileURL(imageURL))
		msg.Caption = caption
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		sentMsg, sendErr = api.Send(msg)
		if sendErr != nil {
			// Fallback to text
			txtMsg := tgbotapi.NewMessageToChannel(channelTarget, caption)
			txtMsg.ParseMode = "HTML"
			if keyboard != nil {
				txtMsg.ReplyMarkup = keyboard
			}
			sentMsg, sendErr = api.Send(txtMsg)
		}
	} else {
		msg := tgbotapi.NewMessageToChannel(channelTarget, caption)
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		sentMsg, sendErr = api.Send(msg)
	}

	if sendErr != nil {
		log.Printf("[TELEGRAM AD] Failed to post ad to %s: %v", channelTarget, sendErr)
		return AdPostResult{Target: channelTarget, Status: "failed", Error: sendErr.Error()}
	}

	log.Printf("[TELEGRAM AD] ✓ Ad posted to %s (msg_id=%d)", channelTarget, sentMsg.MessageID)
	return AdPostResult{Target: channelTarget, Status: "success", MessageID: sentMsg.MessageID}
}

// SendAdToBot sends an ad to the admin/bot chat (for bot placement)
func (s *TelegramService) SendAdToBot(chatID int64, title, description, imageURL, targetURL, cta string) AdPostResult {
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

	caption := buildAdCaption(title, description, targetURL, cta)

	var keyboard *tgbotapi.InlineKeyboardMarkup
	if targetURL != "" {
		ctaText := cta
		if ctaText == "" {
			ctaText = "Ko'proq bilish ›"
		}
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(ctaText, targetURL),
			),
		)
		keyboard = &kb
	}

	var sentMsg tgbotapi.Message
	var sendErr error

	if imageURL != "" {
		msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(imageURL))
		msg.Caption = caption
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		sentMsg, sendErr = api.Send(msg)
		if sendErr != nil {
			txtMsg := tgbotapi.NewMessage(chatID, caption)
			txtMsg.ParseMode = "HTML"
			if keyboard != nil {
				txtMsg.ReplyMarkup = keyboard
			}
			sentMsg, sendErr = api.Send(txtMsg)
		}
	} else {
		msg := tgbotapi.NewMessage(chatID, caption)
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		sentMsg, sendErr = api.Send(msg)
	}

	if sendErr != nil {
		return AdPostResult{Target: target, Status: "failed", Error: sendErr.Error()}
	}
	return AdPostResult{Target: target, Status: "success", MessageID: sentMsg.MessageID}
}

// buildAdCaption creates a sponsored message caption for an ad
func buildAdCaption(title, description, targetURL, cta string) string {
	var b strings.Builder
	b.WriteString("📢 <b>Reklama</b>\n\n")
	b.WriteString("<b>")
	b.WriteString(title)
	b.WriteString("</b>")
	if description != "" {
		b.WriteString("\n")
		b.WriteString(description)
	}
	if targetURL != "" && cta == "" {
		b.WriteString("\n\n🔗 ")
		b.WriteString(targetURL)
	}
	return b.String()
}

// NotifyMovieCreated is called via HTTP from worker after successful movie creation
// This is the main entry point for the backend API
func (s *TelegramService) NotifyMovieCreated(movie *TelegramMovieData) *TelegramNotificationResult {
	return s.SendMovieNotification(movie)
}

// CallMovieNotification calls the backend API to send Telegram notification
// This is called by the worker
func CallMovieNotification(backendURL, botToken string, movie *TelegramMovieData) *TelegramNotificationResult {
	if backendURL == "" {
		log.Printf("[TELEGRAM WORKER] Backend URL not configured, skipping notification")
		return &TelegramNotificationResult{
			Success:      false,
			ErrorMessage: "backend URL not configured",
		}
	}

	url := fmt.Sprintf("%s/api/telegram/notify-movie", backendURL)

	payload, err := json.Marshal(movie)
	if err != nil {
		log.Printf("[TELEGRAM WORKER] Failed to marshal movie data: %v", err)
		return &TelegramNotificationResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to marshal: %v", err),
		}
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("[TELEGRAM WORKER] Failed to create request: %v", err)
		return &TelegramNotificationResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to create request: %v", err),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	if botToken != "" {
		req.Header.Set("X-Worker-Token", botToken)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[TELEGRAM WORKER] Failed to call backend: %v", err)
		return &TelegramNotificationResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to call backend: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[TELEGRAM WORKER] Backend returned status %d", resp.StatusCode)
		return &TelegramNotificationResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("backend returned status %d", resp.StatusCode),
		}
	}

	var result TelegramNotificationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[TELEGRAM WORKER] Failed to decode response: %v", err)
		return &TelegramNotificationResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to decode response: %v", err),
		}
	}

	return &result
}

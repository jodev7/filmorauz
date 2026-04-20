package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
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
	Country     string   `json:"country"`
	CountriesUz []string `json:"countries_uz"`
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
	botToken               string
	botUsername            string
	channelUsername        string
	serialsChannelUsername string
	adminTelegramID        int64
	baseSiteURL            string
	channels               []TelegramChannel
	extraChannels          []string // raw channel identifiers from TELEGRAM_CHANNELS env (e.g. "@filmorauznet_drama")
	httpClient             *http.Client
}

// TelegramConfig holds configuration for Telegram service
type TelegramConfig struct {
	BotToken               string
	BotUsername            string
	ChannelUsername        string
	SerialsChannelUsername string
	AdminTelegramID        int64
	BaseSiteURL            string
	MovieChannels          []TelegramChannel // Genre-routed channels
	ChannelsList           []string          // raw TELEGRAM_CHANNELS list — used for approval auto-posts
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
		botToken:               config.BotToken,
		botUsername:            config.BotUsername,
		channelUsername:        config.ChannelUsername,
		serialsChannelUsername: config.SerialsChannelUsername,
		adminTelegramID:        config.AdminTelegramID,
		baseSiteURL:            config.BaseSiteURL,
		channels:               channels,
		extraChannels:          config.ChannelsList,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// GetBaseSiteURL returns the configured base site URL.
func (s *TelegramService) GetBaseSiteURL() string {
	return s.baseSiteURL
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

// resolveTelegramMediaSource returns the appropriate RequestFileData for a media URL.
//
// DEV (localhost / 127.0.0.1 host, or a bare /uploads/… path):
//   - Maps the URL path to a local file on disk and sends via multipart upload.
//   - Returns an error if the file does not exist.
//
// PROD (any other public URL):
//   - Returns a FileURL so Telegram fetches the media directly.
func resolveTelegramMediaSource(mediaURL string) (tgbotapi.RequestFileData, string, error) {
	if mediaURL == "" {
		return nil, "", fmt.Errorf("empty media URL")
	}

	u, err := url.Parse(mediaURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid media URL %q: %w", mediaURL, err)
	}

	// Treat localhost, 127.0.0.1, and bare /uploads/… paths as local files.
	host := u.Hostname()
	isLocal := host == "localhost" || host == "127.0.0.1" ||
		(host == "" && strings.HasPrefix(u.Path, "/uploads/"))

	if isLocal {
		localPath := "." + u.Path // e.g. ./uploads/ads/images/file.jpg
		if _, statErr := os.Stat(localPath); statErr != nil {
			log.Printf("[TELEGRAM AD] file not found in uploads: %s", localPath)
			return nil, "", fmt.Errorf("file not found in uploads: %s", localPath)
		}
		log.Printf("[TELEGRAM AD] resolved local file: %s", localPath)
		return tgbotapi.FilePath(localPath), "local_file", nil
	}

	return tgbotapi.FileURL(mediaURL), "public_url", nil
}

// resolveMedia is kept as an internal alias so existing call-sites compile unchanged.
func resolveMedia(mediaURL string) (tgbotapi.RequestFileData, string, error) {
	return resolveTelegramMediaSource(mediaURL)
}

// AdPostResult holds the result of posting a single ad to Telegram
type AdPostResult struct {
	Target    string `json:"target"`
	Status    string `json:"status"` // "success" | "failed"
	MessageID int    `json:"message_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// buildAdKeyboard returns an inline keyboard with a CTA button, or nil if no targetURL.
func buildAdKeyboard(targetURL, cta string) *tgbotapi.InlineKeyboardMarkup {
	if targetURL == "" {
		return nil
	}
	ctaText := cta
	if ctaText == "" {
		ctaText = "Batafsil ›"
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(ctaText, targetURL),
		),
	)
	return &kb
}

// SendAdToChannel sends an ad to a specific Telegram channel.
// Requires imageURL or videoURL — rejects with missing_media_for_telegram if neither present.
func (s *TelegramService) SendAdToChannel(channelTarget, title, description, imageURL, videoURL, targetURL, cta string) AdPostResult {
	if s.botToken == "" {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: "bot token not configured"}
	}
	if imageURL == "" && videoURL == "" {
		log.Printf("[TELEGRAM AD] channel %s skipped: missing_media_for_telegram", channelTarget)
		return AdPostResult{Target: channelTarget, Status: "failed", Error: "missing_media_for_telegram"}
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: channelTarget, Status: "failed", Error: err.Error()}
	}

	caption := buildAdCaption(title, description, targetURL, cta)
	keyboard := buildAdKeyboard(targetURL, cta)

	var sentMsg tgbotapi.Message
	var sendErr error

	switch {
	case imageURL != "":
		media, mode, resolveErr := resolveMedia(imageURL)
		if resolveErr != nil {
			log.Printf("[TELEGRAM AD] FAILED channel=%s resolve error: %v", channelTarget, resolveErr)
			return AdPostResult{Target: channelTarget, Status: "failed", Error: resolveErr.Error()}
		}
		log.Printf("[TELEGRAM AD] channel=%s method=sendPhoto mode=%s", channelTarget, mode)
		msg := tgbotapi.NewPhotoToChannel(channelTarget, media)
		msg.Caption = caption
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		sentMsg, sendErr = api.Send(msg)

	case videoURL != "":
		media, mode, resolveErr := resolveMedia(videoURL)
		if resolveErr != nil {
			log.Printf("[TELEGRAM AD] FAILED channel=%s resolve error: %v", channelTarget, resolveErr)
			return AdPostResult{Target: channelTarget, Status: "failed", Error: resolveErr.Error()}
		}
		log.Printf("[TELEGRAM AD] channel=%s method=sendVideo mode=%s", channelTarget, mode)
		msg := tgbotapi.VideoConfig{
			BaseFile: tgbotapi.BaseFile{
				BaseChat: tgbotapi.BaseChat{ChannelUsername: channelTarget},
				File:     media,
			},
		}
		msg.Caption = caption
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		sentMsg, sendErr = api.Send(msg)
	}

	if sendErr != nil {
		log.Printf("[TELEGRAM AD] FAILED channel=%s error=%v", channelTarget, sendErr)
		return AdPostResult{Target: channelTarget, Status: "failed", Error: sendErr.Error()}
	}

	log.Printf("[TELEGRAM AD] ✓ channel=%s msg_id=%d", channelTarget, sentMsg.MessageID)
	return AdPostResult{Target: channelTarget, Status: "success", MessageID: sentMsg.MessageID}
}

// SendAdToBot sends an ad to a specific chat via the bot.
// Requires imageURL or videoURL — rejects with missing_media_for_telegram if neither present.
func (s *TelegramService) SendAdToBot(chatID int64, title, description, imageURL, videoURL, targetURL, cta string) AdPostResult {
	target := fmt.Sprintf("bot:%d", chatID)
	if s.botToken == "" {
		return AdPostResult{Target: target, Status: "failed", Error: "bot token not configured"}
	}
	if chatID == 0 {
		return AdPostResult{Target: target, Status: "failed", Error: "chatID is 0"}
	}
	if imageURL == "" && videoURL == "" {
		log.Printf("[TELEGRAM AD] bot chat_id=%d skipped: missing_media_for_telegram", chatID)
		return AdPostResult{Target: target, Status: "failed", Error: "missing_media_for_telegram"}
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		return AdPostResult{Target: target, Status: "failed", Error: err.Error()}
	}

	caption := buildAdCaption(title, description, targetURL, cta)
	keyboard := buildAdKeyboard(targetURL, cta)

	var sentMsg tgbotapi.Message
	var sendErr error

	switch {
	case imageURL != "":
		media, mode, resolveErr := resolveMedia(imageURL)
		if resolveErr != nil {
			log.Printf("[TELEGRAM AD] FAILED bot chat_id=%d resolve error: %v", chatID, resolveErr)
			return AdPostResult{Target: target, Status: "failed", Error: resolveErr.Error()}
		}
		log.Printf("[TELEGRAM AD] bot chat_id=%d method=sendPhoto mode=%s", chatID, mode)
		msg := tgbotapi.NewPhoto(chatID, media)
		msg.Caption = caption
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		sentMsg, sendErr = api.Send(msg)

	case videoURL != "":
		media, mode, resolveErr := resolveMedia(videoURL)
		if resolveErr != nil {
			log.Printf("[TELEGRAM AD] FAILED bot chat_id=%d resolve error: %v", chatID, resolveErr)
			return AdPostResult{Target: target, Status: "failed", Error: resolveErr.Error()}
		}
		log.Printf("[TELEGRAM AD] bot chat_id=%d method=sendVideo mode=%s", chatID, mode)
		msg := tgbotapi.NewVideo(chatID, media)
		msg.Caption = caption
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		sentMsg, sendErr = api.Send(msg)
	}

	if sendErr != nil {
		log.Printf("[TELEGRAM AD] FAILED bot chat_id=%d error=%v", chatID, sendErr)
		return AdPostResult{Target: target, Status: "failed", Error: sendErr.Error()}
	}
	log.Printf("[TELEGRAM AD] ✓ bot chat_id=%d msg_id=%d", chatID, sentMsg.MessageID)
	return AdPostResult{Target: target, Status: "success", MessageID: sentMsg.MessageID}
}

// buildAdCaption creates a sponsored message caption for an ad.
// title and description are HTML-escaped to prevent parse_mode errors.
func buildAdCaption(title, description, targetURL, cta string) string {
	var b strings.Builder
	b.WriteString("📢 <b>Reklama</b>\n\n")
	b.WriteString("<b>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</b>")
	if description != "" {
		b.WriteString("\n")
		b.WriteString(html.EscapeString(description))
	}
	if targetURL != "" && cta == "" {
		b.WriteString("\n\n🔗 ")
		b.WriteString(html.EscapeString(targetURL))
	}
	return b.String()
}

// NotifyMovieCreated is called via HTTP from worker after successful movie creation
// This is the main entry point for the backend API
func (s *TelegramService) NotifyMovieCreated(movie *TelegramMovieData) *TelegramNotificationResult {
	return s.SendMovieNotification(movie)
}

// PostContentApproval posts an approved movie or serial to Telegram.
// isSerial=true → posts only to the serials channel.
// isSerial=false → posts to the main movie channel + any genre-matched
// channels from TELEGRAM_CHANNELS. Returns the list of targets that
// received the message successfully — the caller persists this to skip
// re-posting on a duplicate approve click.
//
// Genre matching (movies only): a channel from TELEGRAM_CHANNELS is
// considered a genre channel if its identifier contains an underscore —
// the suffix after the last underscore is matched (substring, case-
// insensitive) against each of the movie's genres. Example:
//   movie genres "Drama"  +  channel "@filmorauznet_drama"   → match
//   movie genres "Anime"  +  channel "@filmorauznet_anime"   → match
//   channel "@filmorauznet" (no underscore) → always sent as the main channel
func (s *TelegramService) PostContentApproval(data *TelegramMovieData, isSerial bool) []string {
	if s.botToken == "" {
		log.Printf("[TELEGRAM APPROVE] bot token not configured, skipping")
		return nil
	}

	api, err := tgbotapi.NewBotAPI(s.botToken)
	if err != nil {
		log.Printf("[TELEGRAM APPROVE] failed to create bot API: %v", err)
		return nil
	}

	caption := s.buildApprovalCaption(data, isSerial)
	keyboard := buildAdKeyboard(data.MovieURL, "🎬 Saytda ko‘rish")

	var posted []string
	send := func(target string) {
		if target == "" {
			return
		}
		log.Printf("[TELEGRAM APPROVE] posting to %s", target)
		sendErr := s.sendApprovalToChannel(api, target, caption, data.PosterURL, keyboard)
		if sendErr != nil {
			log.Printf("[TELEGRAM APPROVE] FAILED target=%s err=%v", target, sendErr)
			return
		}
		log.Printf("[TELEGRAM APPROVE] ✓ target=%s", target)
		posted = append(posted, target)
	}

	if isSerial {
		if s.serialsChannelUsername != "" {
			send(ensureAtPrefix(s.serialsChannelUsername))
		} else {
			log.Printf("[TELEGRAM APPROVE] TELEGRAM_SERIALS_CHANNEL not configured, skipping serial post")
		}
		return posted
	}

	targets := s.resolveMovieApprovalTargets(data.Genres)
	if len(targets) == 0 {
		log.Printf("[TELEGRAM APPROVE] no channels resolved for movie %q — check TG_CHANNEL_USERNAME / TELEGRAM_CHANNELS", data.Title)
		return posted
	}
	log.Printf("[TELEGRAM APPROVE] movie=%q genres=%v targets=%v", data.Title, data.Genres, targets)
	for _, t := range targets {
		send(t)
	}
	return posted
}

// sendApprovalToChannel sends the rendered caption (with optional poster
// and inline button) to a single channel identifier. Falls back to a
// text-only message when the poster send fails.
func (s *TelegramService) sendApprovalToChannel(api *tgbotapi.BotAPI, target, caption, posterURL string, keyboard *tgbotapi.InlineKeyboardMarkup) error {
	if posterURL != "" {
		msg := tgbotapi.NewPhotoToChannel(target, tgbotapi.FileURL(posterURL))
		msg.Caption = caption
		msg.ParseMode = "HTML"
		if keyboard != nil {
			msg.ReplyMarkup = keyboard
		}
		if _, sendErr := api.Send(msg); sendErr == nil {
			return nil
		} else {
			log.Printf("[TELEGRAM APPROVE] photo send failed target=%s (%v), falling back to text", target, sendErr)
		}
	}
	txt := tgbotapi.NewMessageToChannel(target, caption)
	txt.ParseMode = "HTML"
	if keyboard != nil {
		txt.ReplyMarkup = keyboard
	}
	_, sendErr := api.Send(txt)
	return sendErr
}

// resolveMovieApprovalTargets returns de-duplicated channel identifiers
// (each "@username") to post a movie approval to: the main channel always,
// plus any TELEGRAM_CHANNELS entry whose trailing genre token matches one
// of the movie's genres. If TELEGRAM_CHANNELS has a bare entry (no
// underscore), it's treated as an additional main channel.
func (s *TelegramService) resolveMovieApprovalTargets(genres []string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(raw string) {
		t := ensureAtPrefix(strings.TrimSpace(raw))
		if t == "" || t == "@" {
			return
		}
		if _, ok := seen[t]; ok {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}

	if s.channelUsername != "" {
		add(s.channelUsername)
	}

	lowerGenres := make([]string, 0, len(genres))
	for _, g := range genres {
		lowerGenres = append(lowerGenres, strings.ToLower(strings.TrimSpace(g)))
	}

	for _, raw := range s.extraChannels {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		name := strings.TrimPrefix(raw, "@")
		idx := strings.LastIndex(name, "_")
		if idx == -1 {
			// Bare channel (no underscore) — treat as a main channel.
			add(raw)
			continue
		}
		suffix := strings.ToLower(name[idx+1:])
		for _, g := range lowerGenres {
			if g == "" {
				continue
			}
			if strings.Contains(g, suffix) || strings.Contains(suffix, g) {
				add(raw)
				break
			}
		}
	}
	return out
}

func ensureAtPrefix(ch string) string {
	ch = strings.TrimSpace(ch)
	if ch == "" {
		return ""
	}
	if strings.HasPrefix(ch, "@") {
		return ch
	}
	return "@" + ch
}

// buildApprovalCaption builds the caption for an approved content post.
// Format (movies):
//
//	🎬 Yangi kino platformaga qo‘shildi!
//
//	🔥 <b><title></b>
//
//	📅 Yil: <year>
//	🌍 Davlat: <country>
//	🎭 Janr: <genres>
//	🎞 Sifat: <quality>
//
//	📖 <i><description></i>
//
//	🆔 Kino kodi: <b><code></b>
//
//	🤖 Bizning bot: <b>@<botUsername></b>
//
//	━━━━━━━━━━━━━━━
//	🎥 <b>Saytda ko‘rish</b>
//
// Empty fields (country, genres, quality, description, code) are skipped.
// Serials use the "📺 Yangi serial" header and otherwise follow the same layout.
func (s *TelegramService) buildApprovalCaption(data *TelegramMovieData, isSerial bool) string {
	var b strings.Builder

	if isSerial {
		b.WriteString("📺 <b>Yangi serial platformaga qo‘shildi!</b>\n\n")
	} else {
		b.WriteString("🎬 <b>Yangi kino platformaga qo‘shildi!</b>\n\n")
	}

	if t := strings.TrimSpace(data.Title); t != "" {
		b.WriteString("🔥 <b>")
		b.WriteString(html.EscapeString(t))
		b.WriteString("</b>\n\n")
	}

	if data.Year > 0 {
		b.WriteString(fmt.Sprintf("📅 Yil: %d\n", data.Year))
	}

	// Country — prefer the Uzbek-localized list, fall back to the single
	// bson `country` string. Both empty → skip the line entirely.
	countries := data.CountriesUz
	if len(countries) == 0 {
		if c := strings.TrimSpace(data.Country); c != "" {
			countries = []string{c}
		}
	}
	if len(countries) > 0 {
		b.WriteString("🌍 Davlat: ")
		b.WriteString(html.EscapeString(strings.Join(countries, ", ")))
		b.WriteString("\n")
	}

	genres := data.GenresUz
	if len(genres) == 0 {
		genres = data.Genres
	}
	if len(genres) > 0 {
		b.WriteString("🎭 Janr: ")
		b.WriteString(html.EscapeString(strings.Join(genres, ", ")))
		b.WriteString("\n")
	}

	if q := strings.TrimSpace(data.Quality); q != "" {
		b.WriteString("🎞 Sifat: ")
		b.WriteString(html.EscapeString(q))
		b.WriteString("\n")
	}

	if desc := strings.TrimSpace(data.Description); desc != "" {
		if runes := []rune(desc); len(runes) > 200 {
			desc = string(runes[:200]) + "..."
		}
		b.WriteString("\n📖 <i>")
		b.WriteString(html.EscapeString(desc))
		b.WriteString("</i>\n")
	}

	if code := strings.TrimSpace(data.Code); code != "" {
		b.WriteString("\n🆔 Kino kodi: <b>")
		b.WriteString(html.EscapeString(code))
		b.WriteString("</b>\n")
	}

	// Bot mention — use configured TELEGRAM_BOT_USERNAME so the handle stays
	// in sync with whichever bot this deployment actually runs. Falls back to
	// the default @filmorauzbot if unconfigured.
	botName := strings.TrimPrefix(strings.TrimSpace(s.botUsername), "@")
	if botName == "" {
		botName = "filmorauzbot"
	}
	b.WriteString("\n🤖 Bizning bot: <b>@")
	b.WriteString(html.EscapeString(botName))
	b.WriteString("</b>\n")

	b.WriteString("\n━━━━━━━━━━━━━━━\n")
	b.WriteString("🎥 <b>Saytda ko‘rish</b>")

	return b.String()
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

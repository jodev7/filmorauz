package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/bot/config"
	"github.com/filmorauz/bot/keyboards"
	"github.com/filmorauz/bot/models"
	"github.com/filmorauz/bot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot represents the Telegram bot
type Bot struct {
	api                 *tgbotapi.BotAPI
	config              *config.Config
	subscriptionService *services.SubscriptionService
	authClient          *services.AuthClient
	httpClient          *http.Client
}

// MovieCodeResponse represents the response from backend API
type MovieCodeResponse struct {
	Found bool       `json:"found"`
	Movie *MovieInfo `json:"movie,omitempty"`
}

// MovieInfo represents movie info in API response
type MovieInfo struct {
	Title       string   `json:"title"`
	Code        string   `json:"code"`
	WebsiteURL  string   `json:"website_url"`
	PosterURL   string   `json:"poster_url"`
	Poster      string   `json:"poster"`
	PosterAlt   string   `json:"posterUrl"`
	BackdropURL string   `json:"backdrop_url"`
	Year        int      `json:"year"`
	Genre       []string `json:"genre"`
	Quality     string   `json:"quality"`
	Description string   `json:"description"`
	Duration    int      `json:"duration"`
}

func main() {
	cfg := config.Load()

	// Initialize bot
	bot, err := NewBot(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	log.Printf("Bot started as @%s", bot.api.Self.UserName)

	// Start polling for updates
	bot.startPolling()
}

// NewBot creates a new Bot instance
func NewBot(cfg *config.Config) (*Bot, error) {
	if cfg.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required in .env")
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	// Set debug mode in dev
	api.Debug = cfg.IsDev

	// Initialize subscription service
	subService, err := services.NewSubscriptionService(api, cfg.RequiredChannels)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription service: %w", err)
	}

	// Initialize auth client
	authClient := services.NewAuthClient(cfg.BackendBaseURL, api)

	return &Bot{
		api:                 api,
		config:              cfg,
		subscriptionService: subService,
		authClient:          authClient,
		httpClient:          &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// startPolling starts the bot's update polling
func (b *Bot) startPolling() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			b.handleCallback(update.CallbackQuery)
		}
	}
}

// handleMessage handles incoming messages
func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	// Check if message has text
	if msg.Text == "" {
		log.Printf("Received empty message")
		return
	}

	// Check if message has sender info
	if msg.From == nil {
		log.Printf("Received message without sender info: %s", msg.Text)
		return
	}

	log.Printf("Received message: '%s' from user %d", msg.Text, msg.From.ID)

	chatID := msg.Chat.ID
	userID := msg.From.ID

	// Normalize command - convert to lowercase for matching
	text := strings.ToLower(strings.TrimSpace(msg.Text))

	// Handle /start command (also matches /start@botname)
	if text == "/start" || strings.HasPrefix(text, "/start@") || strings.HasPrefix(text, "/start ") {
		log.Printf("Handling /start command for user %d", userID)

		// Check if this is a login deep link: /start login_XXXXXXXX
		if authCode := b.parseLoginCode(msg.Text); authCode != "" {
			log.Printf("Detected login deep link with code: %s", authCode)
			b.handleLogin(chatID, userID, msg.From, authCode)
			return
		}

		// Regular /start
		b.handleStart(chatID, userID, msg.From)
		return
	}

	// Handle /code command (also matches /code@botname)
	if text == "/code" || strings.HasPrefix(text, "/code@") || strings.HasPrefix(text, "/code ") {
		log.Printf("Handling /code command for user %d", userID)
		b.handleCode(chatID, userID, msg.Text)
		return
	}

	// Handle unknown commands
	log.Printf("Unknown command from user %d: %s", userID, msg.Text)
	b.sendMessage(chatID, "❌ Noto'g'ri buyruq. /start yoki /code buyrug'ini yuboring.")
}

// handleStart handles the /start command
func (b *Bot) handleStart(chatID int64, userID int64, from *tgbotapi.User) {
	// Save/update user in backend (best-effort, non-blocking)
	go func() {
		if err := b.authClient.RegisterBotUser(chatID, from); err != nil {
			log.Printf("[START] Failed to save user telegram_id=%d chat_id=%d: %v", from.ID, chatID, err)
		}
	}()
	log.Printf("[START] User %d sent /start", userID)
	log.Printf("[START] Required channels configured: %d", b.subscriptionService.RequiredChannelCount())

	// Check subscription FIRST, before sending welcome
	status := b.subscriptionService.CheckUserSubscriptions(userID)
	log.Printf("[START] Subscription check for user %d: subscribed=%v, missing=%d",
		userID, status.IsSubscribed, len(status.MissingChans))

	if !status.IsSubscribed {
		log.Printf("[START] User %d is missing %d channels - showing subscription gate",
			userID, len(status.MissingChans))

		// Send subscription required message FIRST (before welcome)
		subscriptionText := "🎬 FilmoraUz Rasmiy Botiga xush kelibsiz!\n\n❌ Botdan foydalanish uchun quyidagi kanallarga obuna bo'ling:"
		b.sendMessage(chatID, subscriptionText)

		// Send inline keyboard with ONLY missing channel links and check button
		if len(status.MissingChans) > 0 {
			keyboard := keyboards.BuildDynamicSubscriptionKeyboard(status.MissingChans)
			msg := tgbotapi.NewMessage(chatID, "Quyida siz hali a'zo bo'lmagan kanallar ko'rsatilgan.\nObuna bo'lgach, '✅ Tekshirish' tugmasini bosing:")
			msg.ReplyMarkup = keyboard
			b.api.Send(msg)
		}
		return
	}

	// User IS subscribed - show welcome and usage
	log.Printf("[START] User %d is subscribed - showing welcome", userID)
	welcomeText := "🎬 FilmoraUz Rasmiy Botiga xush kelibsiz!\n\nEndi kinolarni qidirish orqali filmlarni tomosha qilishingiz mumkin."
	b.sendMessage(chatID, welcomeText)

	time.Sleep(150 * time.Millisecond)

	usageText := `📌 Kino kodini yuboring:

<code>/code KINO_KODI</code>

💡 Masalan:
<code>/code 0001</code>

	Yoki filmorauz.net web sahifamiz orqali barcha kinolarni ko'rishingiz mumkin.`
	b.sendMessage(chatID, usageText)
}

// handleCode handles the /code command
func (b *Bot) handleCode(chatID int64, userID int64, text string) {
	log.Printf("User %d sent /code command: %s", userID, text)

	// Check subscription FIRST
	status := b.subscriptionService.CheckUserSubscriptions(userID)
	if !status.IsSubscribed {
		log.Printf("User %d is not subscribed, blocking /code usage", userID)
		b.sendSubscriptionRequestWithMissing(chatID, status.MissingChans)
		return
	}

	// Extract code from command - use the original text, not lowercase
	// Handle both "/code" and "/code @botname" formats
	// Also handle "/code" alone vs "/code ABC123"
	commandText := strings.TrimSpace(text)

	// Remove bot mention if present (e.g., /code@filmorauzbot ABC123 -> /code ABC123)
	if idx := strings.Index(commandText, " "); idx != -1 {
		commandText = commandText[idx+1:]
	} else {
		// No space found - user only typed /code or /code@botname
		b.sendMessage(chatID, keyboards.BuildCodeMissingMessage())
		return
	}

	code := strings.TrimSpace(commandText)

	// Validate code format: must be numeric only, 4-6 digits
	// Regex: ^\d{4,6}$
	if !isValidMovieCode(code) {
		log.Printf("User %d sent invalid code format: %s", userID, code)
		b.sendMessage(chatID, keyboards.BuildCodeInvalidMessage())
		return
	}

	log.Printf("User %d looking up movie code: %s", userID, code)
	b.lookupMovie(chatID, code)
}

// isValidMovieCode validates that the code is numeric with 4-6 digits
// Regex: ^\d{4,6}$
func isValidMovieCode(code string) bool {
	if len(code) < 4 || len(code) > 6 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// lookupMovie looks up a movie by code via backend API
func (b *Bot) lookupMovie(chatID int64, code string) {
	url := fmt.Sprintf("%s/api/public/movies/code/%s", b.config.BackendBaseURL, code)

	resp, err := b.httpClient.Get(url)
	if err != nil {
		log.Printf("Error calling backend API: %v", err)
		b.sendMessage(chatID, keyboards.BuildBackendErrorMessage())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Backend returned status: %d", resp.StatusCode)
		b.sendMessage(chatID, keyboards.BuildMovieNotFoundMessage())
		return
	}

	var movieResp MovieCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&movieResp); err != nil {
		log.Printf("Error decoding response: %v", err)
		b.sendMessage(chatID, keyboards.BuildErrorMessage())
		return
	}

	if !movieResp.Found || movieResp.Movie == nil {
		log.Printf("Movie not found for code: %s", code)
		b.sendMessage(chatID, keyboards.BuildMovieNotFoundMessage())
		return
	}

	movie := movieResp.Movie
	log.Printf("Movie found: %s, URL: %s", movie.Title, movie.WebsiteURL)

	// Get bot username from config
	botUsername := b.config.BotUsername

	// Build movie details text
	movieInfo := &keyboards.MovieInfo{
		Title:       movie.Title,
		Code:        movie.Code,
		WebsiteURL:  movie.WebsiteURL,
		Year:        movie.Year,
		Genre:       movie.Genre,
		Quality:     movie.Quality,
		Description: movie.Description,
		Duration:    movie.Duration,
	}
	detailsText := keyboards.BuildMovieDetailsText(movieInfo)

	posterURL := firstNonEmpty(movie.PosterURL, movie.PosterAlt, movie.Poster, movie.BackdropURL)
	log.Printf(
		"[MOVIE] title=%q poster_url=%q posterUrl=%q poster=%q backdrop_url=%q",
		movie.Title,
		movie.PosterURL,
		movie.PosterAlt,
		movie.Poster,
		movie.BackdropURL,
	)

	// Public URL? Try photo, but never let a broken/slow image hang /code.
	// Flow: normalize the poster URL → HEAD probe with 5s budget → bounded
	// Send. On any failure we fall through to a plain-text response carrying
	// the same caption and the inline watch button, so the user always gets
	// an answer.
	if keyboards.IsPublicURL(movie.WebsiteURL) {
		photoURL := b.resolvePosterURL(posterURL)
		log.Printf("[MOVIE] resolved photo url — title=%q photo_url=%q", movie.Title, photoURL)
		photoSent := false

		if photoURL != "" && b.isValidPosterURL(photoURL) {
			reachable, probeStatus, probeErr := b.isPosterReachable(photoURL)
			log.Printf(
				"[MOVIE] photo probe — title=%q photo_url=%s reachable=%t http_status=%d probe_error=%v",
				movie.Title, photoURL, reachable, probeStatus, probeErr,
			)

			log.Printf("[MOVIE] Sending photo with caption for: %s (photo_url=%s)", movie.Title, photoURL)
			if sendErr := b.sendMoviePhotoWithTimeout(chatID, photoURL, detailsText, movie.WebsiteURL); sendErr != nil {
				log.Printf("[MOVIE] photo send failed — title=%q photo_url=%s tg_error=%v", movie.Title, photoURL, sendErr)
			} else {
				photoSent = true
			}
		} else if posterURL != "" {
			log.Printf("[MOVIE] poster URL invalid or could not be resolved — title=%q raw=%s resolved=%q cdn_base=%q",
				movie.Title, posterURL, photoURL, b.config.CDNBaseURL)
		}

		if !photoSent {
			log.Printf("[MOVIE] falling back to text-only response for: %s", movie.Title)
			msgText := detailsText + "\n\n" + keyboards.BuildMovieFoundMessageWithBranding(movie.Title, botUsername)
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = "HTML"
			msg.ReplyMarkup = keyboards.BuildMovieFoundKeyboard(movie.WebsiteURL)
			if _, err := b.api.Send(msg); err != nil {
				log.Printf("[MOVIE] fallback text send failed — title=%q tg_error=%v", movie.Title, err)
			}
		}
	} else {
		// Dev/local mode - URL not publicly accessible
		log.Printf("[MOVIE] URL is LOCAL (dev) - showing raw URL: %s", movie.WebsiteURL)
		// Send plain text with raw URL visible (no button) and branding
		b.sendMessage(chatID, keyboards.BuildMovieFoundMessageWithBrandingAndURL(movie.Title, movie.WebsiteURL, botUsername))
	}
}

// legacyPosterPrefixRewrites maps the old DB-stored poster path prefixes onto
// their canonical /images/* layout in B2. These are applied to relative paths
// before resolving against the CDN base; legacy values like "posters/x.jpg"
// would otherwise 404 once the bucket switched layouts.
var legacyPosterPrefixRewrites = []struct{ from, to string }{
	{"posters/", "images/posters/"},
	{"backdrops/", "images/backdrops/"},
	{"profile/", "images/profile/"},
	{"avatars/", "images/profile/"},
}

// resolvePosterURL turns whatever the backend returned in poster_url into an
// absolute URL Telegram can fetch. Absolute http(s) URLs are returned as-is.
// Relative paths are resolved against CDNBaseURL after rewriting legacy
// prefixes. Returns "" when the input is empty or no CDN base is configured.
func (b *Bot) resolvePosterURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "/uploads/") || strings.HasPrefix(raw, "/uploads/") || strings.HasPrefix(raw, "uploads/") {
		return ""
	}

	base := b.telegramMediaBaseURL()
	if base == "" {
		return ""
	}

	if strings.HasPrefix(raw, "https://") {
		if strings.Contains(raw, "/media/images/") {
			if idx := strings.Index(raw, "/media/"); idx != -1 {
				return base + raw[idx+len("/media"):]
			}
			return raw
		}
		if idx := strings.Index(raw, "/file/filmorauznet/"); idx != -1 {
			path := raw[idx+len("/file/filmorauznet/"):]
			return b.resolvePosterURL(path)
		}
	}
	if strings.HasPrefix(raw, "http://") {
		if idx := strings.Index(raw, "/media/"); idx != -1 {
			return base + raw[idx+len("/media"):]
		}
		if idx := strings.Index(raw, "/file/filmorauznet/"); idx != -1 {
			path := raw[idx+len("/file/filmorauznet/"):]
			return b.resolvePosterURL(path)
		}
		return ""
	}

	path := strings.TrimPrefix(raw, "/")
	path = strings.TrimPrefix(path, "media/")
	for _, r := range legacyPosterPrefixRewrites {
		if strings.HasPrefix(path, r.from) {
			path = r.to + strings.TrimPrefix(path, r.from)
			break
		}
	}

	if strings.HasPrefix(path, "file/filmorauznet/") {
		path = strings.TrimPrefix(path, "file/filmorauznet/")
	}
	if strings.HasPrefix(path, "images/") {
		return base + "/" + path
	}
	if strings.HasPrefix(path, "media/images/") {
		return base + "/" + strings.TrimPrefix(path, "media/")
	}
	if strings.HasPrefix(path, "backdrops/") || strings.HasPrefix(path, "posters/") || strings.HasPrefix(path, "profile/") || strings.HasPrefix(path, "avatars/") {
		return b.resolvePosterURL(path)
	}
	return ""
}

func (b *Bot) isValidPosterURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	return raw != "" && strings.HasPrefix(raw, "https://")
}

func (b *Bot) telegramMediaBaseURL() string {
	cdn := strings.TrimSpace(b.config.CDNBaseURL)
	if cdn == "" {
		return "https://cdn.filmorauz.net/media"
	}
	cdn = strings.TrimRight(cdn, "/")
	if idx := strings.Index(cdn, "/file/filmorauznet"); idx != -1 {
		return cdn[:idx] + "/media"
	}
	if strings.Contains(cdn, "/media") {
		return strings.TrimSuffix(cdn, "/")
	}
	return "https://cdn.filmorauz.net/media"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

const (
	posterProbeTimeout = 5 * time.Second
	photoSendTimeout   = 15 * time.Second
)

// isPosterReachable performs a bounded HEAD (with a ranged-GET fallback for
// CDNs that reject HEAD) to confirm Telegram will be able to fetch the photo.
// Returns reachability plus the observed status and error so the caller can
// log them. The 5s budget keeps a broken image from blocking /code.
func (b *Bot) isPosterReachable(photoURL string) (bool, int, error) {
	client := &http.Client{Timeout: posterProbeTimeout}

	headReq, err := http.NewRequest(http.MethodHead, photoURL, nil)
	if err != nil {
		return false, 0, err
	}
	resp, err := client.Do(headReq)
	if err != nil {
		return false, 0, err
	}
	resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, resp.StatusCode, nil
	}

	// Some CDNs (and B2 in certain configs) reject HEAD with 405/501. Retry
	// with a 1-byte ranged GET so we don't pay for the full image.
	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotImplemented {
		return false, resp.StatusCode, nil
	}
	getReq, err := http.NewRequest(http.MethodGet, photoURL, nil)
	if err != nil {
		return false, resp.StatusCode, err
	}
	getReq.Header.Set("Range", "bytes=0-0")
	getResp, err := client.Do(getReq)
	if err != nil {
		return false, resp.StatusCode, err
	}
	defer getResp.Body.Close()
	if getResp.StatusCode >= 200 && getResp.StatusCode < 300 {
		return true, getResp.StatusCode, nil
	}
	return false, getResp.StatusCode, nil
}

// sendMoviePhotoWithTimeout runs the Telegram send-photo call in a bounded
// goroutine. tgbotapi v5 has no per-call timeout knob, so without this a
// hung Telegram-side fetch (e.g. unreachable CDN URL accepted by HEAD but
// stalled by Telegram's egress) would freeze /code indefinitely. On timeout
// the goroutine is left to drain on its own; the caller falls back to a
// text response so the user is never left waiting.
func (b *Bot) sendMoviePhotoWithTimeout(chatID int64, photoURL, caption, websiteURL string) error {
	photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(photoURL))
	photoMsg.Caption = caption
	photoMsg.ParseMode = "HTML"
	photoMsg.ReplyMarkup = keyboards.BuildMovieFoundKeyboard(websiteURL)

	errCh := make(chan error, 1)
	go func() {
		_, err := b.api.Send(photoMsg)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(photoSendTimeout):
		return fmt.Errorf("telegram send photo timed out after %s", photoSendTimeout)
	}
}

// sendSubscriptionRequest sends subscription request to user (all channels)
func (b *Bot) sendSubscriptionRequest(chatID int64) {
	b.sendMessage(chatID, keyboards.BuildSubscriptionMessage())

	// Get all required channels
	channels := b.subscriptionService.GetRequiredChannels()
	if len(channels) > 0 {
		keyboard := keyboards.BuildDynamicSubscriptionKeyboard(channels)
		msg := tgbotapi.NewMessage(chatID, "Kanallarga a'zo bo'ling va tekshiring:")
		msg.ReplyMarkup = keyboard
		b.api.Send(msg)
	}
}

// sendSubscriptionRequestWithMissing sends subscription request with only missing channels
func (b *Bot) sendSubscriptionRequestWithMissing(chatID int64, missingChannels []models.RequiredChannel) {
	b.sendMessage(chatID, keyboards.BuildSubscriptionMessage())

	// Send inline keyboard with ONLY missing channel links
	if len(missingChannels) > 0 {
		keyboard := keyboards.BuildDynamicSubscriptionKeyboard(missingChannels)
		msg := tgbotapi.NewMessage(chatID, "Quyida siz hali a'zo bo'lmagan kanallar ko'rsatilgan.\nObuna bo'lgach, '✅ Tekshirish' tugmasini bosing:")
		msg.ReplyMarkup = keyboard
		b.api.Send(msg)
	}
}

// handleCallback handles callback queries from inline buttons
func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	userID := callback.From.ID
	data := callback.Data

	log.Printf("Callback from user %d: %s", userID, data)

	// Answer callback immediately
	answer := tgbotapi.NewCallback(callback.ID, "")
	b.api.Send(answer)

	if data == "check_subscription" {
		b.handleCheckSubscription(chatID, userID)
	}
}

// handleCheckSubscription handles the "Tekshirish" button press
func (b *Bot) handleCheckSubscription(chatID int64, userID int64) {
	log.Printf("User %d pressed Tekshirish button", userID)

	status := b.subscriptionService.CheckUserSubscriptions(userID)
	if status.IsSubscribed {
		log.Printf("User %d is now subscribed, showing success and usage", userID)
		// Send success message
		b.sendMessage(chatID, keyboards.BuildSubscriptionSuccessMessage())

		// Then send usage instructions
		time.Sleep(200 * time.Millisecond) // Small delay
		usageText := `Kino linkini olish uchun quyidagicha yozing:

/code KINO_KODI

Masalan:
/code 0001`
		b.sendMessage(chatID, usageText)
	} else {
		log.Printf("User %d is still missing %d channels", userID, len(status.MissingChans))
		b.sendSubscriptionRequestWithMissing(chatID, status.MissingChans)
	}
}

// parseLoginCode parses the login code from /start command
// Returns empty string if not a login command
// Handles: /start login_XXXXXXXX, /start login_XXXXXXXX@botname, etc.
func (b *Bot) parseLoginCode(text string) string {
	// Normalize: remove bot mention if present and lowercase
	cleanText := strings.ToLower(strings.TrimSpace(text))

	// Remove bot mentions like @filmorauzbot
	if idx := strings.Index(cleanText, "@"); idx != -1 {
		cleanText = cleanText[:idx]
	}

	// Now check for /start login_ pattern
	const loginPrefix = "/start login_"
	if !strings.HasPrefix(cleanText, loginPrefix) {
		return ""
	}

	// Extract code after "/start login_"
	code := strings.TrimPrefix(cleanText, loginPrefix)

	// Validate: code should be alphanumeric and reasonable length (6-20 chars)
	code = strings.TrimSpace(code)
	if len(code) < 6 || len(code) > 20 {
		log.Printf("[AUTH] Invalid login code length: %d", len(code))
		return ""
	}

	// Check that code contains only alphanumeric characters
	for _, c := range code {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			log.Printf("[AUTH] Invalid login code format: contains non-alphanumeric char")
			return ""
		}
	}

	log.Printf("[AUTH] Parsed login code: %s", code)
	return strings.ToUpper(code)
}

// handleLogin handles the website login flow
// This is triggered by /start login_<AUTH_CODE>
func (b *Bot) handleLogin(chatID int64, userID int64, user *tgbotapi.User, authCode string) {
	log.Printf("[LOGIN] >>> Handling login for user %d with code %s", userID, authCode)

	// Save/update user in backend (best-effort, non-blocking)
	go func() {
		if err := b.authClient.RegisterBotUser(chatID, user); err != nil {
			log.Printf("[LOGIN] Failed to save user telegram_id=%d chat_id=%d: %v", user.ID, chatID, err)
		}
	}()

	// First, check subscription (required before login)
	log.Printf("[LOGIN] Checking subscription for user %d", userID)
	status := b.subscriptionService.CheckUserSubscriptions(userID)

	if !status.IsSubscribed {
		log.Printf("[LOGIN] User %d is not subscribed to all channels, blocking login", userID)
		// Send subscription requirement message
		b.sendMessage(chatID, keyboards.BuildAuthPendingMessage(b.getChannelTitles(status.MissingChans)))

		// Also show the subscription keyboard
		if len(status.MissingChans) > 0 {
			keyboard := keyboards.BuildDynamicSubscriptionKeyboard(status.MissingChans)
			msg := tgbotapi.NewMessage(chatID, "Quyida siz hali a'zo bo'lmagan kanallar ko'rsatilgan.\nObuna bo'lgach, qayta /start buyrug'ini yuboring:")
			msg.ReplyMarkup = keyboard
			b.api.Send(msg)
		}
		return
	}

	// User is subscribed - proceed with login
	log.Printf("[LOGIN] User %d is subscribed, calling backend to complete auth", userID)

	// Call backend to complete auth
	resp, err := b.authClient.CompleteAuthSession(authCode, user)
	if err != nil {
		errMsg := err.Error()
		log.Printf("[LOGIN] <<< Backend error for user %d: %s", userID, errMsg)

		// Check specific error types and show appropriate message
		switch {
		case strings.Contains(errMsg, "backendga ulanishda"):
			// Backend unreachable
			b.sendMessage(chatID, "❌ <b>Serverga ulanish mumkin emas.</b>\n\nIltimos, keyinroq qayta urinib ko'ring.")
		case strings.Contains(errMsg, "invalid auth code"):
			// Invalid code
			b.sendMessage(chatID, "❌ <b>Login havolasi noto'g'ri.</b>\n\nIltimos, saytdan yangi havola oling.")
		case strings.Contains(errMsg, "expired"):
			// Expired code
			b.sendMessage(chatID, "❌ <b>Login havolasi muddati tugagan.</b>\n\nIltimos, saytdan yangi havola oling.")
		case strings.Contains(errMsg, "already used"):
			// Already used
			b.sendMessage(chatID, "❌ <b>Login havolasi allaqachon ishlatilgan.</b>\n\nIltimos, saytdan yangi havola oling.")
		default:
			// Generic error
			b.sendMessage(chatID, keyboards.BuildAuthErrorMessage())
		}
		return
	}

	// Check for errors in response body
	if resp != nil && resp.Error != "" {
		log.Printf("[LOGIN] <<< Auth failed for user %d: %s", userID, resp.Error)

		// Check specific error types
		switch {
		case strings.Contains(resp.Error, "invalid auth code"):
			b.sendMessage(chatID, "❌ <b>Login havolasi noto'g'ri.</b>\n\nIltimos, saytdan yangi havola oling.")
		case strings.Contains(resp.Error, "expired"):
			b.sendMessage(chatID, "❌ <b>Login havolasi muddati tugagan.</b>\n\nIltimos, saytdan yangi havola oling.")
		case strings.Contains(resp.Error, "already used"):
			b.sendMessage(chatID, "❌ <b>Login havolasi allaqachon ishlatilgan.</b>\n\nIltimos, saytdan yangi havola oling.")
		default:
			b.sendMessage(chatID, keyboards.BuildAuthFailedMessage())
		}
		return
	}

	// Auth successful!
	log.Printf("[LOGIN] <<< Auth successful for user %d", userID)
	b.sendMessage(chatID, keyboards.BuildAuthSuccessMessage())
}

// getChannelTitles returns just the channel titles from RequiredChannel list
func (b *Bot) getChannelTitles(channels []models.RequiredChannel) []string {
	var titles []string
	for _, ch := range channels {
		titles = append(titles, ch.Title)
	}
	return titles
}

// sendMessage sends a text message
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML" // HTML mode supports <a href="url">text</a> inline links
	b.api.Send(msg)
}

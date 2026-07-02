package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/filmorauz/bot/config"
	"github.com/filmorauz/bot/keyboards"
	"github.com/filmorauz/bot/models"
	"github.com/filmorauz/bot/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	premiumExtendPrefix = "premium_extend:"
	premiumCancelPrefix = "premium_cancel:"

	pendingInvoiceTTL          = 10 * time.Minute
	pendingInvoiceCleanupEvery = 30 * time.Second
)

// pendingLogin tracks an in-progress login deep link so subscription-gated
// logins can complete auth after the user subscribes.
type pendingLogin struct {
	Code      string
	Username  string
	FirstName string
	LastName  string
}
type pendingInvoice struct {
	chatID    int64
	userID    int64
	msgIDs    []int
	createdAt time.Time
	resolved  bool // true once paid or expired — cleanup is idempotent
}

// Bot represents the Telegram bot
type Bot struct {
	api                 *tgbotapi.BotAPI
	config              *config.Config
	subscriptionService *services.SubscriptionService
	authClient          *services.AuthClient
	premiumClient       *services.PremiumClient
	httpClient          *http.Client

	pendingMu           sync.Mutex
	pendingInvoices     map[string]*pendingInvoice
	pendingLoginByUser  map[int64]*pendingLogin
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

func mustMarshalJSON(v interface{}) string {
	if v == nil {
		return "null"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("marshal_error=%v value=%#v", err, v)
	}
	return string(raw)
}

func formatPremiumDate(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.Format("2006-01-02")
}

type minimalStarsInvoiceConfig struct {
	tgbotapi.BaseChat
	Title         string
	Description   string
	Payload       string
	ProviderToken string
	Currency      string
	Prices        []tgbotapi.LabeledPrice
}

func (config minimalStarsInvoiceConfig) params() (tgbotapi.Params, error) {
	params := make(tgbotapi.Params)
	if err := params.AddFirstValid("chat_id", config.BaseChat.ChatID, config.BaseChat.ChannelUsername); err != nil {
		return params, err
	}
	params.AddNonZero("reply_to_message_id", config.BaseChat.ReplyToMessageID)
	params.AddBool("disable_notification", config.BaseChat.DisableNotification)
	params.AddBool("allow_sending_without_reply", config.BaseChat.AllowSendingWithoutReply)
	if err := params.AddInterface("reply_markup", config.BaseChat.ReplyMarkup); err != nil {
		return params, err
	}
	params["title"] = config.Title
	params["description"] = config.Description
	params["payload"] = config.Payload
	params["provider_token"] = config.ProviderToken
	params["currency"] = config.Currency
	if err := params.AddInterface("prices", config.Prices); err != nil {
		return params, err
	}
	return params, nil
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
	go bot.startPendingInvoiceJanitor()
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

	premiumClient := services.NewPremiumClient(cfg.BackendBaseURL, cfg.BotInternalToken)

	return &Bot{
		api:                 api,
		config:              cfg,
		subscriptionService: subService,
		authClient:          authClient,
		premiumClient:       premiumClient,
		httpClient:          &http.Client{Timeout: 10 * time.Second},
		pendingInvoices:     make(map[string]*pendingInvoice),
		pendingLoginByUser:  make(map[int64]*pendingLogin),
	}, nil
}

// startPolling starts the bot's update polling
func (b *Bot) startPolling() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.PreCheckoutQuery != nil {
			b.handlePreCheckout(update.PreCheckoutQuery)
			continue
		}
		if update.Message != nil {
			if update.Message.SuccessfulPayment != nil {
				b.handleSuccessfulPayment(update.Message)
				continue
			}
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

		// Check for premium deep link: /start premium_<token>
		if sessionToken := b.parsePremiumStart(msg.Text); sessionToken != "" {
			log.Printf("Detected premium deep link: token=%s", sessionToken)
			b.handlePremiumStart(chatID, userID, msg.From, sessionToken)
			return
		}

		// Check for movie deep link: /start movie_<code>
		// Lets Instagram captions link straight to the bot with the code
		// pre-filled, so IG visitors never need to learn /code syntax.
		if code := b.parseMovieStart(msg.Text); code != "" {
			log.Printf("Detected movie deep link: code=%s", code)
			// Register the user first (mirrors handleStart) so the
			// freshly-arrived IG visitor gets a row in the DB even if
			// they only ever look up movies via the deep link.
			go func() {
				if err := b.authClient.RegisterBotUser(chatID, msg.From); err != nil {
					log.Printf("[START movie_] register user failed telegram_id=%d: %v", msg.From.ID, err)
				}
			}()
			// Subscription gate must still apply.
			status := b.subscriptionService.CheckUserSubscriptions(userID)
			if !status.IsSubscribed {
				b.sendSubscriptionRequestWithMissing(chatID, status.MissingChans)
				return
			}
			b.lookupMovie(chatID, code)
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

	// User IS subscribed - show welcome and usage.
	// Two-message onboarding so first-time users (often arriving from
	// Instagram with no idea what to do) see an actionable CTA before
	// any instructions to read.
	log.Printf("[START] User %d is subscribed - showing welcome", userID)
	siteURL := b.config.SiteURL
	if siteURL == "" {
		siteURL = "https://filmorauz.net"
	}

	// 1. Short welcome + dual CTA (site / top movies).
	welcomeMsg := tgbotapi.NewMessage(chatID, keyboards.BuildWelcomeShortMessage())
	welcomeMsg.ParseMode = "HTML"
	welcomeMsg.ReplyMarkup = keyboards.BuildWelcomeKeyboard(siteURL)
	if _, err := b.api.Send(welcomeMsg); err != nil {
		log.Printf("[START] welcome short message failed user=%d: %v", userID, err)
	}

	time.Sleep(150 * time.Millisecond)

	// 2. Detailed usage walkthrough.
	b.sendMessage(chatID, keyboards.BuildWelcomeUsageMessage())
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
	// Append the "long-press to open in browser" hint so users on
	// Telegram's in-app browser know how to escape into Chrome/Safari.
	detailsText = detailsText + "\n\n" + keyboards.BrowserOpenHint

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

	base := b.telegramB2FileBaseURL()
	if base == "" {
		return ""
	}

	if strings.HasPrefix(raw, "https://") {
		if strings.Contains(raw, "/media/images/") {
			if idx := strings.Index(raw, "/media/images/"); idx != -1 {
				return cleanupPosterURL(base + "/" + raw[idx+len("/media/images/"):])
			}
			return raw
		}
		if idx := strings.Index(raw, "/file/filmorauznet/"); idx != -1 {
			path := raw[idx+len("/file/filmorauznet/"):]
			return b.resolvePosterURL(path)
		}
	}
	if strings.HasPrefix(raw, "http://") {
		if idx := strings.Index(raw, "/media/images/"); idx != -1 {
			return cleanupPosterURL(base + "/" + raw[idx+len("/media/images/"):])
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
		return cleanupPosterURL(base + "/" + path)
	}
	if strings.HasPrefix(path, "media/images/") {
		return cleanupPosterURL(base + "/" + strings.TrimPrefix(path, "media/images/"))
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

func (b *Bot) telegramB2FileBaseURL() string {
	cdn := strings.TrimSpace(b.config.CDNBaseURL)
	if cdn == "" {
		return "https://cdn.filmorauz.net/file/filmorauznet/images"
	}
	cdn = strings.TrimRight(cdn, "/")
	if idx := strings.Index(cdn, "/file/filmorauznet"); idx != -1 {
		return cdn[:idx] + "/file/filmorauznet/images"
	}
	if idx := strings.Index(cdn, "/media"); idx != -1 {
		return cdn[:idx] + "/file/filmorauznet/images"
	}
	return "https://cdn.filmorauz.net/file/filmorauznet/images"
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

func cleanupPosterURL(raw string) string {
	if strings.Contains(raw, "/media/images/") {
		raw = strings.Replace(raw, "/media/images/", "/file/filmorauznet/images/", 1)
	}
	if strings.Contains(raw, "/images/images/") {
		raw = strings.Replace(raw, "/images/images/", "/images/", 1)
	}
	return raw
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

	if data == "check_subscription" {
		b.answerCallback(callback.ID, "", false)
		b.handleCheckSubscription(chatID, userID)
		return
	}
	if strings.HasPrefix(data, premiumExtendPrefix) {
		b.answerCallback(callback.ID, "", false)
		token := strings.TrimSpace(strings.TrimPrefix(data, premiumExtendPrefix))
		b.handlePremiumInvoiceSend(chatID, userID, token)
		return
	}
	if strings.HasPrefix(data, premiumCancelPrefix) {
		token := strings.TrimSpace(strings.TrimPrefix(data, premiumCancelPrefix))
		if token != "" {
			if _, _, err := b.premiumClient.CancelStarsSession(services.CancelStarsSessionRequest{Token: token}); err != nil {
				log.Printf("[PREMIUM] cancel session failed token=%s user=%d err=%v", token, userID, err)
			}
		}
		b.answerCallback(callback.ID, "Bekor qilindi", false)
		b.sendMessage(chatID, "Premium xaridi bekor qilindi.")
		return
	}

	b.answerCallback(callback.ID, "", false)
}

// handleCheckSubscription handles the "Tekshirish" button press
func (b *Bot) handleCheckSubscription(chatID int64, userID int64) {
	log.Printf("User %d pressed Tekshirish button", userID)

	b.pendingMu.Lock()
	pending := b.pendingLoginByUser[userID]
	delete(b.pendingLoginByUser, userID)
	b.pendingMu.Unlock()

	status := b.subscriptionService.CheckUserSubscriptions(userID)
	if status.IsSubscribed {
		log.Printf("User %d is now subscribed, showing success and usage", userID)

		if pending != nil {
			log.Printf("[LOGIN] Completing pending auth for user %d with code %s", userID, pending.Code)
			user := &tgbotapi.User{
				ID:        userID,
				UserName:  pending.Username,
				FirstName: pending.FirstName,
				LastName:  pending.LastName,
			}
			resp, err := b.authClient.CompleteAuthSession(pending.Code, user)
			if err != nil {
				log.Printf("[LOGIN] Failed to complete pending auth for user %d: %v", userID, err)
				b.sendMessage(chatID, "❌ Avtorizatsiyani yakunlashda xatolik yuz berdi. Iltimos, saytdan qayta urinib ko'ring.")
			} else if resp != nil && resp.Error != "" {
				log.Printf("[LOGIN] Backend error completing pending auth for user %d: %s", userID, resp.Error)
				b.sendMessage(chatID, "❌ Avtorizatsiya amalga oshmadi. Iltimos, saytdan qayta urinib ko'ring.")
		} else {
			log.Printf("[LOGIN] Pending auth completed for user %d", userID)
			siteURL := b.config.SiteURL
			if siteURL == "" {
				siteURL = "https://filmorauz.net"
			}
			successMsg := tgbotapi.NewMessage(chatID, keyboards.BuildAuthSuccessMessage())
			successMsg.ParseMode = "HTML"
			successMsg.ReplyMarkup = keyboards.BuildAuthSuccessKeyboard(siteURL, pending.Code)
			b.api.Send(successMsg)
			b.sendMessage(chatID, keyboards.BuildSubscriptionSuccessMessage())
		}
		} else {
			b.sendMessage(chatID, keyboards.BuildSubscriptionSuccessMessage())
		}

		time.Sleep(200 * time.Millisecond)
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

// parseMovieStart parses a "/start movie_<code>" deep link payload.
// Returns the bare code (e.g. "0001") or "" when the input is anything
// else. The code is validated with the same regex /code uses, so this
// path can never trigger a backend lookup with garbage.
//
// Telegram deep-link payloads are alphanumeric + "_" + "-" only, so
// underscore is reserved as our namespace separator (movie_, login_,
// premium_). The "movie_" prefix wins because we register it first in
// the handler.
func (b *Bot) parseMovieStart(text string) string {
	cleanText := strings.ToLower(strings.TrimSpace(text))
	if idx := strings.Index(cleanText, "@"); idx != -1 {
		// "/start@filmorauzbot movie_0001" → "/start movie_0001"
		cleanText = cleanText[:idx] + cleanText[strings.Index(cleanText, " "):]
	}
	const moviePrefix = "/start movie_"
	if !strings.HasPrefix(cleanText, moviePrefix) {
		return ""
	}
	code := strings.TrimSpace(strings.TrimPrefix(cleanText, moviePrefix))
	if !isValidMovieCode(code) {
		log.Printf("[START movie_] invalid code %q", code)
		return ""
	}
	return code
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
		b.pendingMu.Lock()
		b.pendingLoginByUser[userID] = &pendingLogin{
			Code:      authCode,
			Username:  user.UserName,
			FirstName: user.FirstName,
			LastName:  user.LastName,
		}
		b.pendingMu.Unlock()
		b.sendMessage(chatID, keyboards.BuildAuthPendingMessage(b.getChannelTitles(status.MissingChans)))

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
	siteURL := b.config.SiteURL
	if siteURL == "" {
		siteURL = "https://filmorauz.net"
	}
	successMsg := tgbotapi.NewMessage(chatID, keyboards.BuildAuthSuccessMessage())
	successMsg.ParseMode = "HTML"
	successMsg.ReplyMarkup = keyboards.BuildAuthSuccessKeyboard(siteURL, authCode)
	if _, err := b.api.Send(successMsg); err != nil {
		log.Printf("[LOGIN] success message send failed user=%d: %v", userID, err)
	}
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

// sendMessageReturn sends a text message and returns the resulting message
// (or nil if Telegram rejected it). Used when we need the message ID later.
func (b *Bot) sendMessageReturn(chatID int64, text string) *tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	sent, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[PREMIUM] sendMessage failed chat=%d err=%v", chatID, err)
		return nil
	}
	return &sent
}

func (b *Bot) sendMessageWithURLButtonReturn(chatID int64, text, buttonText, url string) *tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(buttonText, url),
		),
	)
	sent, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[PREMIUM] sendMessageWithURLButton failed chat=%d err=%v", chatID, err)
		return nil
	}
	return &sent
}

// trackInvoiceMessage records a message ID under the given session token so the
// janitor / payment handler can later delete it. Safe to call with id<=0.
func (b *Bot) trackInvoiceMessage(token string, chatID, userID int64, messageID int) {
	if token == "" || messageID <= 0 {
		return
	}
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	pi, ok := b.pendingInvoices[token]
	if !ok {
		pi = &pendingInvoice{
			chatID:    chatID,
			userID:    userID,
			createdAt: time.Now(),
		}
		b.pendingInvoices[token] = pi
	}
	pi.msgIDs = append(pi.msgIDs, messageID)
}

// claimPendingInvoice atomically pulls a session out of the pending map so that
// only one caller cleans it up. Returns nil if it was already resolved/missing.
func (b *Bot) claimPendingInvoice(token string) *pendingInvoice {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	pi, ok := b.pendingInvoices[token]
	if !ok || pi.resolved {
		return nil
	}
	pi.resolved = true
	delete(b.pendingInvoices, token)
	return pi
}

// deleteInvoiceMessages removes the tracked invoice/info messages from the chat.
// Errors are logged but never block the caller — Telegram returns errors for
// already-deleted or too-old messages and we don't want to surface those.
func (b *Bot) deleteInvoiceMessages(pi *pendingInvoice) {
	if pi == nil {
		return
	}
	for _, id := range pi.msgIDs {
		if _, err := b.api.Request(tgbotapi.NewDeleteMessage(pi.chatID, id)); err != nil {
			log.Printf("[PREMIUM] deleteMessage chat=%d msg=%d failed: %v", pi.chatID, id, err)
		}
	}
}

// startPendingInvoiceJanitor expires Stars purchase sessions that the user
// never paid within pendingInvoiceTTL: deletes their invoice messages, cancels
// the backend session, and posts a single expiry notice.
func (b *Bot) startPendingInvoiceJanitor() {
	ticker := time.NewTicker(pendingInvoiceCleanupEvery)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		var expired []*pendingInvoice
		var expiredTokens []string
		b.pendingMu.Lock()
		for token, pi := range b.pendingInvoices {
			if pi.resolved {
				continue
			}
			if now.Sub(pi.createdAt) >= pendingInvoiceTTL {
				pi.resolved = true
				expired = append(expired, pi)
				expiredTokens = append(expiredTokens, token)
			}
		}
		for _, t := range expiredTokens {
			delete(b.pendingInvoices, t)
		}
		b.pendingMu.Unlock()

		for i, pi := range expired {
			token := expiredTokens[i]
			log.Printf("[PREMIUM] invoice session expired token=%s user=%d", token, pi.userID)
			b.deleteInvoiceMessages(pi)
			if b.config.BotInternalToken != "" {
				if _, _, err := b.premiumClient.CancelStarsSession(services.CancelStarsSessionRequest{Token: token}); err != nil {
					log.Printf("[PREMIUM] CancelStarsSession on expiry failed token=%s err=%v", token, err)
				}
			}
			b.sendMessage(pi.chatID, "To‘lov muddati tugadi. Qayta urinib ko‘ring.")
		}
	}
}

func (b *Bot) sendMessageWithURLButton(chatID int64, text, buttonText, url string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(buttonText, url),
		),
	)
	b.api.Send(msg)
}

func (b *Bot) sendPremiumAdminFallback(chatID int64, text string) {
	b.sendMessageWithURLButton(
		chatID,
		text,
		"Admin orqali premium olish",
		"https://t.me/filmorauznet?direct",
	)
}

func (b *Bot) answerCallback(callbackID, text string, alert bool) {
	answer := tgbotapi.NewCallback(callbackID, text)
	answer.ShowAlert = alert
	b.api.Send(answer)
}

func (b *Bot) sendPremiumExtendConfirmation(chatID int64, expiresAt, token string) {
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
		"Sizda Premium allaqachon faol. Tugash sanasi: %s. Yana uzaytirmoqchimisiz?",
		expiresAt,
	))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Ha, uzaytirish", premiumExtendPrefix+token),
			tgbotapi.NewInlineKeyboardButtonData("Bekor qilish", premiumCancelPrefix+token),
		),
	)
	b.api.Send(msg)
}

// parsePremiumStart returns the purchase session token from a "/start premium_<token>" command,
// or "" if the message is not a premium deep link.
func (b *Bot) parsePremiumStart(text string) string {
	clean := strings.ToLower(strings.TrimSpace(text))
	if idx := strings.Index(clean, "@"); idx != -1 {
		// strip bot mention before the space, e.g. "/start@bot premium_1m"
		space := strings.Index(clean, " ")
		if space != -1 && space > idx {
			clean = clean[:idx] + clean[space:]
		}
	}
	const prefix = "/start premium_"
	if !strings.HasPrefix(clean, prefix) {
		return ""
	}
	token := strings.TrimSpace(strings.TrimPrefix(clean, prefix))
	if len(token) < 12 || len(token) > 64 {
		return ""
	}
	for _, ch := range token {
		if (ch < 'a' || ch > 'f') && (ch < '0' || ch > '9') {
			return ""
		}
	}
	return token
}

// handlePremiumStart validates the purchase session against the current Telegram account and sends a Telegram Stars invoice.
func (b *Bot) handlePremiumStart(chatID int64, userID int64, from *tgbotapi.User, sessionToken string) {
	// Synchronously ensure the backend has a user record to link the payment
	// to. Telegram refuses sendInvoice for users who never opened the bot,
	// and a missing DB row also breaks our post-payment grant flow — so we
	// register first and surface the failure to the user.
	if from == nil {
		b.sendMessage(chatID, "❌ Avval botni /start bosib oching.")
		return
	}
	if err := b.authClient.RegisterBotUser(chatID, from); err != nil {
		log.Printf("[PREMIUM] RegisterBotUser failed telegram_id=%d: %v", from.ID, err)
		b.sendMessage(chatID, "❌ Avval botni /start bosib oching, so'ng qayta urinib ko'ring.")
		return
	}

	if b.config.BotInternalToken == "" {
		log.Printf("[PREMIUM] BOT_INTERNAL_TOKEN not configured; cannot validate session")
		b.sendMessage(chatID, "❌ Premium to'lovini tayyorlashda server sozlamasi xato. Keyinroq qayta urinib ko'ring.")
		return
	}
	sessionResp, status, err := b.premiumClient.ValidateStarsSession(services.ValidateStarsSessionRequest{
		Token:      sessionToken,
		TelegramID: userID,
	})
	if err != nil {
		log.Printf("[PREMIUM] session validate failed telegram_id=%d token=%s err=%v", userID, sessionToken, err)
		b.sendMessage(chatID, "❌ Premium sessiyasini tekshirishda xatolik. Saytdan qayta urinib ko'ring.")
		return
	}
	if status != http.StatusOK || sessionResp == nil || !sessionResp.OK {
		errCode := ""
		if sessionResp != nil {
			errCode = sessionResp.Error
		}
		log.Printf("[PREMIUM] session validate rejected telegram_id=%d token=%s status=%d error=%s", userID, sessionToken, status, errCode)
		switch errCode {
		case "telegram_mismatch":
			b.sendMessage(chatID, "❌ Bu Telegram akkaunt FilmoraUz profilingizga bog‘lanmagan. Iltimos saytga bog‘langan Telegram akkaunt bilan urinib ko‘ring.")
		case "session_expired":
			b.sendMessage(chatID, "❌ Premium sessiyasi muddati tugagan. Saytdan qayta premium tanlang.")
		case "session_paid":
			b.sendMessage(chatID, "❌ Bu premium havolasi allaqachon ishlatilgan. Saytdan yangi premium sessiyasi oching.")
		case "session_cancelled":
			b.sendMessage(chatID, "❌ Bu premium sessiyasi bekor qilingan. Saytdan qayta premium tanlang.")
		case "user_not_linked":
			b.sendMessage(chatID, "❌ Stars orqali premium olish uchun profilingizni Telegram bilan bog‘lang.")
		default:
			b.sendMessage(chatID, "❌ Premium havolasi yaroqsiz. Saytdan qayta urinib ko‘ring.")
		}
		return
	}

	if sessionResp.PremiumActive {
		expiresAt := "mavjud"
		if sessionResp.PremiumExpiresAt != nil {
			if formatted := formatPremiumDate(*sessionResp.PremiumExpiresAt); formatted != "" {
				expiresAt = formatted
			}
		}
		b.sendPremiumExtendConfirmation(chatID, expiresAt, sessionToken)
		return
	}

	b.handlePremiumInvoiceSend(chatID, userID, sessionToken)
}

func (b *Bot) handlePremiumInvoiceSend(chatID int64, userID int64, sessionToken string) {
	sessionResp, status, err := b.premiumClient.ValidateStarsSession(services.ValidateStarsSessionRequest{
		Token:      sessionToken,
		TelegramID: userID,
	})
	if err != nil {
		log.Printf("[PREMIUM] invoice validate failed telegram_id=%d token=%s err=%v", userID, sessionToken, err)
		b.sendMessage(chatID, "❌ Premium sessiyasini tekshirishda xatolik. Saytdan qayta urinib ko'ring.")
		return
	}
	if status != http.StatusOK || sessionResp == nil || !sessionResp.OK {
		errCode := ""
		if sessionResp != nil {
			errCode = sessionResp.Error
		}
		log.Printf("[PREMIUM] invoice validate rejected telegram_id=%d token=%s status=%d error=%s", userID, sessionToken, status, errCode)
		switch errCode {
		case "telegram_mismatch":
			b.sendMessage(chatID, "❌ Bu Telegram akkaunt FilmoraUz profilingizga bog‘lanmagan. Iltimos saytga bog‘langan Telegram akkaunt bilan urinib ko‘ring.")
		case "session_expired":
			b.sendMessage(chatID, "❌ Premium sessiyasi muddati tugagan. Saytdan qayta premium tanlang.")
		case "session_paid":
			b.sendMessage(chatID, "❌ Bu premium havolasi allaqachon ishlatilgan. Saytdan yangi premium sessiyasi oching.")
		case "session_cancelled":
			b.sendMessage(chatID, "❌ Bu premium sessiyasi bekor qilingan. Saytdan qayta premium tanlang.")
		default:
			b.sendMessage(chatID, "❌ Premium havolasi yaroqsiz. Saytdan qayta urinib ko‘ring.")
		}
		return
	}

	intro := fmt.Sprintf(
		"⭐ <b>FilmoraUz Premium — %s</b>\n\nNarx: <b>%d Stars</b>\nMuddat: <b>%d oy</b>\n\nTo'lovni amalga oshirish uchun pastdagi tugmani bosing.",
		sessionResp.Label, sessionResp.StarsPrice, sessionResp.DurationMonths,
	)
	if introMsg := b.sendMessageReturn(chatID, intro); introMsg != nil {
		b.trackInvoiceMessage(sessionToken, chatID, userID, introMsg.MessageID)
	}

	// Telegram Stars: empty provider token + currency "XTR".
	// For XTR, amount equals the raw star count (NOT multiplied by 100).
	payload := "premium_session:" + sessionToken
	amount := sessionResp.StarsPrice
	prices := []tgbotapi.LabeledPrice{
		{Label: "FilmoraUz Premium " + sessionResp.Label, Amount: amount},
	}
	invoice := minimalStarsInvoiceConfig{
		BaseChat: tgbotapi.BaseChat{ChatID: chatID},
		Title:    "FilmoraUz Premium " + sessionResp.Label,
		Description: fmt.Sprintf(
			"Premium obuna — %d oy. Reklamasiz tomosha, 1080p, premium kontent.",
			sessionResp.DurationMonths,
		),
		Payload:       payload,
		ProviderToken: "",
		Currency:      "XTR",
		Prices:        prices,
	}
	params, paramsErr := invoice.params()
	if paramsErr != nil {
		log.Printf("[PREMIUM] sendInvoice params build failed user=%d token=%s pkg=%s err=%v", userID, sessionToken, sessionResp.Package, paramsErr)
		b.sendPremiumAdminFallback(chatID, "❌ Invoice yuborishda xatolik. Agar Stars to‘lovi ishlamasa, admin orqali premium olishingiz mumkin.")
		return
	}
	requestJSON, _ := json.Marshal(params)
	pricesJSON, _ := json.Marshal(prices)
	log.Printf("[PREMIUM] sendInvoice user=%d token=%s pkg=%s payload=%q currency=%q provider_token=%q prices=%s prices_amount=%d stars=%d request_json=%s",
		userID, sessionToken, sessionResp.Package, payload, invoice.Currency, invoice.ProviderToken, string(pricesJSON), amount, sessionResp.StarsPrice, string(requestJSON))
	resp, err := b.api.MakeRequest("sendInvoice", params)
	if err != nil {
		responseBody := "null"
		if resp != nil {
			if raw, marshalErr := json.Marshal(resp); marshalErr == nil {
				responseBody = string(raw)
			} else {
				responseBody = fmt.Sprintf("marshal_error=%v resp=%#v", marshalErr, resp)
			}
		}

		if apiErr, ok := err.(*tgbotapi.Error); ok {
			log.Printf("[PREMIUM] sendInvoice FAILED user=%d token=%s pkg=%s payload=%q currency=%q prices_amount=%d code=%d description=%q request_json=%s response_body=%s response_parameters=%+v api_error=%#v",
				userID, sessionToken, sessionResp.Package, payload, invoice.Currency, amount, apiErr.Code, apiErr.Message, string(requestJSON), responseBody, apiErr.ResponseParameters, apiErr)
		} else {
			log.Printf("[PREMIUM] sendInvoice FAILED user=%d token=%s pkg=%s payload=%q currency=%q prices_amount=%d request_json=%s response_body=%s err=%T %#v",
				userID, sessionToken, sessionResp.Package, payload, invoice.Currency, amount, string(requestJSON), responseBody, err, err)
		}
		b.sendPremiumAdminFallback(
			chatID,
			"❌ Invoice yuborishda xatolik. Iltimos, avval /start bosing va qayta urinib ko'ring.\n\nAgar Stars to‘lovi ishlamasa, admin orqali premium olishingiz mumkin.",
		)
		return
	}
	log.Printf("[PREMIUM] sendInvoice OK user=%d token=%s pkg=%s request_json=%s response_body=%s", userID, sessionToken, sessionResp.Package, string(requestJSON), mustMarshalJSON(resp))

	// Capture the invoice message_id from sendInvoice response so we can delete
	// the "Pay 100 Stars" button after success or expiry.
	if resp != nil && len(resp.Result) > 0 {
		var invoiceMsg tgbotapi.Message
		if err := json.Unmarshal(resp.Result, &invoiceMsg); err == nil && invoiceMsg.MessageID > 0 {
			b.trackInvoiceMessage(sessionToken, chatID, userID, invoiceMsg.MessageID)
		} else if err != nil {
			log.Printf("[PREMIUM] invoice message_id parse failed token=%s err=%v", sessionToken, err)
		}
	}

	if fallbackMsg := b.sendMessageWithURLButtonReturn(chatID,
		"ℹ️ Telegram Stars to'lovi Telegram mobil ilovasida yaxshiroq ishlaydi.\n\n"+
			"Agar checkout ochilib, lekin to'lov ishlamasa, Telegram Stars to‘lovi hozircha sizning hisobingizda ishlamadi.\n\n"+
			"To‘lov amalga oshmasa, premium faollashtirilmaydi.\n\n"+
			"Agar Stars to‘lovi ishlamasa, admin orqali premium olishingiz mumkin.",
		"Admin orqali premium olish",
		"https://t.me/filmorauznet?direct",
	); fallbackMsg != nil {
		b.trackInvoiceMessage(sessionToken, chatID, userID, fallbackMsg.MessageID)
	}
}

// handlePreCheckout approves all incoming pre-checkout queries.
// Telegram Stars payloads we issue are validated again on successful_payment;
// rejecting at pre-checkout based on user state would block legitimate payers,
// so we accept and verify the payload server-side after charge.
func (b *Bot) handlePreCheckout(q *tgbotapi.PreCheckoutQuery) {
	log.Printf("[PREMIUM] PRE_CHECKOUT_QUERY user=%d id=%s payload=%s currency=%s amount=%d",
		q.From.ID, q.ID, q.InvoicePayload, q.Currency, q.TotalAmount)
	cfg := tgbotapi.PreCheckoutConfig{
		PreCheckoutQueryID: q.ID,
		OK:                 true,
	}
	if _, err := b.api.Request(cfg); err != nil {
		log.Printf("[PREMIUM] PRE_CHECKOUT_QUERY answer failed user=%d id=%s: %v", q.From.ID, q.ID, err)
	}
}

// handleSuccessfulPayment grants premium server-side after a Telegram Stars charge.
// We trust only the SuccessfulPayment event; the deep link alone never grants premium.
func (b *Bot) handleSuccessfulPayment(msg *tgbotapi.Message) {
	sp := msg.SuccessfulPayment
	if sp == nil || msg.From == nil {
		return
	}
	chatID := msg.Chat.ID
	userID := msg.From.ID

	log.Printf("[PREMIUM] SUCCESSFUL_PAYMENT user=%d currency=%s amount=%d charge=%s provider_charge=%s payload=%s",
		userID, sp.Currency, sp.TotalAmount, sp.TelegramPaymentChargeID, sp.ProviderPaymentChargeID, sp.InvoicePayload)

	// Validate payload: premium_session:<token>
	parts := strings.Split(sp.InvoicePayload, ":")
	if len(parts) != 2 || parts[0] != "premium_session" {
		log.Printf("[PREMIUM] Invalid payload format: %q", sp.InvoicePayload)
		b.sendMessage(chatID, "⚠️ To'lov qabul qilindi, lekin paketni aniqlab bo'lmadi. Admin bilan bog'laning.")
		return
	}
	sessionToken := parts[1]

	// Best-effort cleanup of the package info / invoice / fallback messages we
	// sent earlier for this session. Idempotent: claimPendingInvoice removes
	// the entry so a duplicate successful_payment cannot re-delete the success
	// message we are about to send.
	if pi := b.claimPendingInvoice(sessionToken); pi != nil {
		b.deleteInvoiceMessages(pi)
	}

	if sp.Currency != "XTR" {
		log.Printf("[PREMIUM] Unexpected currency: %s", sp.Currency)
		// Still attempt grant — backend is the source of truth — but log loudly.
	}

	if b.config.BotInternalToken == "" {
		log.Printf("[PREMIUM] BOT_INTERNAL_TOKEN not configured; cannot grant premium")
		b.sendMessage(chatID, "⚠️ To'lov qabul qilindi, lekin server sozlamasi noto'g'ri. Admin bilan bog'laning.")
		return
	}

	resp, status, err := b.premiumClient.GrantTelegramStars(services.GrantStarsRequest{
		TelegramID:              userID,
		SessionToken:            sessionToken,
		StarsAmount:             sp.TotalAmount,
		TelegramPaymentChargeID: sp.TelegramPaymentChargeID,
		ProviderPaymentChargeID: sp.ProviderPaymentChargeID,
	})
	if err != nil {
		log.Printf("[PREMIUM] Grant failed user=%d: %v", userID, err)
		b.sendPremiumAdminFallback(chatID, "⚠️ To'lov qabul qilindi, lekin server bilan bog'lanishda xatolik.\n\nAgar Stars to‘lovi ishlamasa, admin orqali premium olishingiz mumkin.")
		return
	}
	if status == http.StatusNotFound || (resp != nil && resp.Error == "user_not_linked") {
		b.sendMessage(chatID, fmt.Sprintf(
			"⚠️ Premium olish uchun avval FilmoraUz profilingizni Telegram bilan bog'lang.\n\nSayt: %s\n\nLogin qilgandan so'ng administratorga to'lov chekini yuboring — premium qo'lda faollashtiriladi.",
			b.config.SiteURL,
		))
		return
	}
	if resp != nil && resp.Error == "telegram_mismatch" {
		b.sendMessage(chatID, "❌ Bu Telegram akkaunt FilmoraUz profilingizga bog‘lanmagan. Iltimos saytga bog‘langan Telegram akkaunt bilan urinib ko‘ring.")
		return
	}
	if status != http.StatusOK || (resp != nil && !resp.OK) {
		errMsg := ""
		if resp != nil {
			errMsg = resp.Error
		}
		log.Printf("[PREMIUM] Grant non-OK status=%d err=%s", status, errMsg)
		if errMsg == "session_paid" {
			b.sendMessage(chatID, "ℹ️ Bu to‘lov sessiyasi allaqachon qayta ishlangan. Premium ikkinchi marta uzaytirilmadi.")
			return
		}
		b.sendPremiumAdminFallback(chatID, "⚠️ To'lov qabul qilindi, lekin premium faollashtirishda xatolik.\n\nAgar Stars to‘lovi ishlamasa, admin orqali premium olishingiz mumkin.")
		return
	}

	if resp != nil && resp.AlreadyProcessed {
		log.Printf("[PREMIUM] Duplicate charge ignored: %s", sp.TelegramPaymentChargeID)
		b.sendMessage(chatID, "ℹ️ Bu to‘lov allaqachon qayta ishlangan. Premium qayta uzaytirilmadi.")
		return
	}

	expiryText := ""
	if resp != nil && strings.TrimSpace(resp.PremiumExpires) != "" {
		if formatted := formatPremiumDate(resp.PremiumExpires); formatted != "" {
			expiryText = "\nTugash sanasi: " + formatted
		}
	}
	b.sendMessage(chatID, fmt.Sprintf("Premium faollashtirildi ✅%s", expiryText))
}

package keyboards

import (
	"fmt"
	"strings"

	"github.com/filmorauz/bot/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BuildSubscriptionKeyboard creates an inline keyboard for subscription (all channels)
func BuildSubscriptionKeyboard(channelURLs []string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Add channel buttons
	for _, url := range channelURLs {
		button := tgbotapi.NewInlineKeyboardButtonURL("📢 Kanal", url)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{button})
	}

	// Add check button
	checkButton := tgbotapi.NewInlineKeyboardButtonData("✅ Tekshirish", "check_subscription")
	rows = append(rows, []tgbotapi.InlineKeyboardButton{checkButton})

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// BuildDynamicSubscriptionKeyboard creates inline keyboard with ONLY missing channels
func BuildDynamicSubscriptionKeyboard(channels []models.RequiredChannel) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Add buttons for missing channels only
	for _, ch := range channels {
		title := ch.Title
		if title == "" {
			title = ch.Key + " kanal"
		}
		// Add emoji prefix
		buttonText := "📢 " + title
		button := tgbotapi.NewInlineKeyboardButtonURL(buttonText, ch.URL)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{button})
	}

	// Add check button
	checkButton := tgbotapi.NewInlineKeyboardButtonData("✅ Tekshirish", "check_subscription")
	rows = append(rows, []tgbotapi.InlineKeyboardButton{checkButton})

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// BuildSubscriptionMessage returns the subscription required message
func BuildSubscriptionMessage() string {
	return `❌ <b>Kanalga a'zo bo'lmagansiz!</b>

Botdan foydalanish uchun quyidagi kanallarga a'zo bo'ling:

Kanallarga a'zo bo'lgach, "✅ Tekshirish" tugmasini bosing.`
}

// BuildSubscriptionSuccessMessage returns the success message after verification
func BuildSubscriptionSuccessMessage() string {
	return `✅ <b>Tabriklaymiz!</b>

Endi botdan foydalanishingiz mumkin.

Kino kodini yuboring:
<code>/code KINO_KODI</code>`
}

// BuildCodeUsageMessage returns the code command usage example
func BuildCodeUsageMessage() string {
	return `🎬 <b>FilmoraUz Rasmiy Botiga xush kelibsiz!</b>

Kino linkini olish uchun quyidagicha yozing:

<code>/code KINO_KODI</code>

<b>Masalan:</b>
<code>/code 0001</code>`
}

// BuildCodeMissingMessage returns message when code is missing
func BuildCodeMissingMessage() string {
	return `⚠️ Kino kodini kiritmadingiz.

📌 To'g'ri foydalanish:
<code>/code 1234</code>

🎬 Ya'ni /code buyrug'idan keyin kino kodini yozing.`
}

// BuildCodeInvalidMessage returns message when code format is invalid
func BuildCodeInvalidMessage() string {
	return `❌ Noto'g'ri kod formati.

Kod 4-6 raqamdan iborat bo'lishi kerak.

<b>Masalan:</b> <code>/code 0005</code>`
}

// BuildMovieFoundMessage returns message when movie is found
// Simple message without link - inline keyboard button provides the clickable URL
func BuildMovieFoundMessage(title string) string {
	return "🎬 Kino topildi: " + title
}

// BuildMovieFoundKeyboard returns inline keyboard with URL button for movie
func BuildMovieFoundKeyboard(url string) tgbotapi.InlineKeyboardMarkup {
	button := tgbotapi.NewInlineKeyboardButtonURL("👉 Tomosha qilish!", url)
	row := []tgbotapi.InlineKeyboardButton{button}
	return tgbotapi.NewInlineKeyboardMarkup(row)
}

// BuildDevModeMessage returns message when URL is not publicly available
func BuildDevModeMessage(title string) string {
	return "🎬 Kino topildi: " + title + "\n\n⚠️ Hozircha bu kino ishlab chiqarish muhitida emas.\n\n💡 Tez orada filmorauz.net saytida jonli bo'ladi."
}

// BuildMovieFoundMessageWithRawURL returns message with visible raw URL (for dev/local)
func BuildMovieFoundMessageWithRawURL(title, url string) string {
	return "🎬 Kino topildi: " + title + "\n\n🔗 Link:\n" + url
}

// IsPublicURL checks if the URL is a valid public HTTP(S) URL
// Returns false for localhost, 127.0.0.1, 0.0.0.0, or non-HTTP URLs
func IsPublicURL(url string) bool {
	if url == "" {
		return false
	}

	// Check if URL starts with http:// or https://
	if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
		return false
	}

	// Check for local/dev hostnames
	localHosts := []string{
		"localhost",
		"127.0.0.1",
		"0.0.0.0",
		"::1",
		".local",
	}

	urlLower := strings.ToLower(url)

	for _, host := range localHosts {
		if strings.Contains(urlLower, host) {
			return false
		}
	}

	// Check for common dev domains
	devDomains := []string{
		".ngrok-free.app",
		".ngrok.io",
		".localtunnel.me",
		".playwright.dev",
		".cloudflareapps.com",
	}

	for _, domain := range devDomains {
		if strings.Contains(urlLower, domain) {
			// Allow ngrok and similar if explicitly marked as public
			// For now, we'll treat these as dev URLs
			return false
		}
	}

	return true
}

// BuildMovieNotFoundMessage returns message when movie is not found
func BuildMovieNotFoundMessage() string {
	return `❌ <b>Bunday kino kodi topilmadi.</b>

Kodini qayta tekshirib yuboring.

📌 Masalan:
<code>/code AB12CD</code>

💡 Yangi kino kodini olish uchun /start buyrug'ini yuboring.`
}

// BuildErrorMessage returns generic error message
func BuildErrorMessage() string {
	return `❌ <b>Xatolik yuz berdi.</b>

Iltimos, keyinroq urinib ko'ring.`
}

// BuildBackendErrorMessage returns backend connection error message
func BuildBackendErrorMessage() string {
	return `❌ <b>Server xatoligi.</b>

Iltimos, keyinroq urinib ko'ring.`
}

// BuildAuthSuccessMessage returns the success message after Telegram login
func BuildAuthSuccessMessage() string {
	return `✅ <b>Tizimga muvaffaqiyatli kirdingiz!</b>

🌐 Endi FilmoraUz saytiga qayting va foydalanishda davom eting.`
}

// BuildAuthFailedMessage returns the failure message for invalid/expired auth code
func BuildAuthFailedMessage() string {
	return `❌ <b>Login havolasi noto'g'ri yoki muddati tugagan.</b>

Saytdan qaytadan Telegram orqali kirishni bosing.`
}

// BuildAuthErrorMessage returns the error message when backend is unavailable
func BuildAuthErrorMessage() string {
	return `❌ <b>Xatolik yuz berdi.</b>

Qayta urinib ko'ring yoki saytdan qaytadan kirishga harakat qiling.`
}

// BuildAuthPendingMessage returns message when user needs to complete subscription first
func BuildAuthPendingMessage(missingChannels []string) string {
	var msg string
	if len(missingChannels) > 0 {
		msg = `❌ <b>Avval quyidagi kanallarga obuna bo'ling:</b>

`
		for _, ch := range missingChannels {
			msg += "📢 " + ch + "\n"
		}
		msg += `\nObuna bo'lgach, qayta /start buyrug'ini yuboring.`
	} else {
		msg = `❌ <b>Obuna tekshirilmadi.</b>

/start buyrug'ini yuboring va kanallarga obuna bo'ling.`
	}
	return msg
}

// BuildMovieFoundMessageWithBranding returns movie found message with bot branding
func BuildMovieFoundMessageWithBranding(title, botUsername string) string {
	branding := ""
	if botUsername != "" {
		branding = "\n\n🤖 Bizning bot: @" + botUsername
	}
	return "🎬 Kino topildi: " + title + branding
}

// BuildMovieFoundMessageWithBrandingAndURL returns movie found message with raw URL and bot branding
func BuildMovieFoundMessageWithBrandingAndURL(title, url, botUsername string) string {
	branding := ""
	if botUsername != "" {
		branding = "\n\n🤖 Bizning bot: @" + botUsername
	}
	return "🎬 Kino topildi: " + title + "\n\n🔗 Link:\n" + url + branding
}

// AddBrandingToCaption adds bot branding to the bottom of a caption
func AddBrandingToCaption(caption, botUsername string) string {
	if botUsername == "" {
		return caption
	}
	return caption + "\n\n🤖 Bizning bot: @" + botUsername
}

// AddChannelAndBotBranding adds channel and bot branding to the bottom of a message
func AddChannelAndBotBranding(text, channelUsername, botUsername string) string {
	branding := ""
	if channelUsername != "" {
		branding += "\n\n📢 Bizning kanal: @" + channelUsername
	}
	if botUsername != "" {
		branding += "\n🤖 Bizning bot: @" + botUsername
	}
	return text + branding
}

// MovieInfo minimal type for keyboard functions
type MovieInfo struct {
	Title       string
	Code        string
	WebsiteURL  string
	Year        int
	Genre       []string
	Quality     string
	Description string
	Duration    int
}

// BuildMovieDetailsText builds a detailed text message for movie lookup result
func BuildMovieDetailsText(movie *MovieInfo) string {
	var b strings.Builder

	// Title and code
	b.WriteString("🎬 <b>")
	b.WriteString(movie.Title)
	b.WriteString("</b>\n")

	// Year
	if movie.Year > 0 {
		b.WriteString(fmt.Sprintf("📅 Yili: %d\n", movie.Year))
	}

	// Quality
	if movie.Quality != "" {
		b.WriteString(fmt.Sprintf("🎞 Sifati: %s\n", movie.Quality))
	}

	// Duration
	if movie.Duration > 0 {
		b.WriteString(fmt.Sprintf("⏱ Davomiyligi: %d daqiqa\n", movie.Duration))
	}

	// Genre
	if len(movie.Genre) > 0 {
		b.WriteString(fmt.Sprintf("🎭 Janr: %s\n", strings.Join(movie.Genre, ", ")))
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

	return b.String()
}

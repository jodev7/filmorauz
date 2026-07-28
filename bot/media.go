package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/bot/keyboards"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// newMediaToken returns a short random token used to key a pending session
// and stay well under Telegram's 64-byte callback_data limit.
func newMediaToken() string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// handleMediaLink probes a superadmin-pasted URL and asks for a quality.
func (b *Bot) handleMediaLink(chatID, userID int64, mediaURL string) {
	log.Printf("[MEDIA] superadmin %d pasted link: %s", userID, mediaURL)
	statusMsg := b.sendMessageReturn(chatID, "🔍 Link tekshirilmoqda…")

	probe, err := b.mediaClient.Probe(mediaURL)
	if err != nil {
		log.Printf("[MEDIA] probe failed: %v", err)
		msg := "❌ Bu linkni yuklab bo'lmadi. Sayt qo'llab-quvvatlanmasligi yoki video mavjud emasligi mumkin."
		if statusMsg != nil {
			b.editText(chatID, statusMsg.MessageID, msg)
		} else {
			b.sendMessage(chatID, msg)
		}
		return
	}

	token := newMediaToken()
	promptID := 0
	if statusMsg != nil {
		promptID = statusMsg.MessageID
	}
	session := &pendingMediaSession{
		chatID:      chatID,
		userID:      userID,
		url:         mediaURL,
		title:       probe.Title,
		probe:       probe,
		promptMsgID: promptID,
		createdAt:   time.Now(),
	}
	b.mediaMu.Lock()
	b.pendingMedia[token] = session
	b.pruneMediaSessionsLocked()
	b.mediaMu.Unlock()

	header := fmt.Sprintf("🎬 <b>%s</b>\n\nSifatni tanlang:", htmlEscape(probe.Title))
	if len(probe.Qualities) == 0 {
		// No per-quality info — go straight to language/download at best.
		b.editText(chatID, promptID, "🎬 <b>"+htmlEscape(probe.Title)+"</b>")
		b.askMediaLangOrDownload(token, session)
		return
	}
	kb := keyboards.BuildMediaQualityKeyboard(token, probe.Qualities)
	b.editTextWithKeyboard(chatID, promptID, header, kb)
}

// handleMediaQualityPick records the chosen height and moves to language/download.
// payload is "<token>:<height>".
func (b *Bot) handleMediaQualityPick(callback *tgbotapi.CallbackQuery, payload string) {
	token, rest := splitToken(payload)
	session := b.getMediaSession(token)
	if session == nil {
		b.editText(callback.Message.Chat.ID, callback.Message.MessageID, "⌛️ Sessiya muddati tugagan. Linkni qayta yuboring.")
		return
	}
	if h, err := strconv.Atoi(rest); err == nil {
		session.height = h
	}
	b.askMediaLangOrDownload(token, session)
}

// handleMediaLangPick records the chosen audio language and starts the download.
// payload is "<token>:<langcode>" (empty langcode = default).
func (b *Bot) handleMediaLangPick(callback *tgbotapi.CallbackQuery, payload string) {
	token, rest := splitToken(payload)
	session := b.getMediaSession(token)
	if session == nil {
		b.editText(callback.Message.Chat.ID, callback.Message.MessageID, "⌛️ Sessiya muddati tugagan. Linkni qayta yuboring.")
		return
	}
	session.audioLang = rest
	b.startMediaDownload(token, session)
}

// askMediaLangOrDownload shows the audio-language keyboard when the source has
// more than one track; otherwise it starts the download immediately.
func (b *Bot) askMediaLangOrDownload(token string, s *pendingMediaSession) {
	langs := s.probe.AudioLangs
	if len(langs) > 1 {
		opts := make([]keyboards.MediaLangOption, 0, len(langs))
		for _, l := range langs {
			opts = append(opts, keyboards.MediaLangOption{Code: l.Code, Name: l.Name})
		}
		kb := keyboards.BuildMediaLangKeyboard(token, opts)
		b.editTextWithKeyboard(s.chatID, s.promptMsgID, "🔊 Audio tilni tanlang:", kb)
		return
	}
	b.startMediaDownload(token, s)
}

// startMediaDownload kicks off the parser download and the progress poller.
func (b *Bot) startMediaDownload(token string, s *pendingMediaSession) {
	// One-shot: remove the session so repeated button taps don't double-start.
	b.mediaMu.Lock()
	delete(b.pendingMedia, token)
	b.mediaMu.Unlock()

	b.editText(s.chatID, s.promptMsgID, "⏳ Yuklab olish boshlandi… 0%")

	jobID, err := b.mediaClient.StartDownload(s.url, s.height, s.audioLang)
	if err != nil {
		log.Printf("[MEDIA] start download failed: %v", err)
		b.editText(s.chatID, s.promptMsgID, "❌ Yuklashni boshlab bo'lmadi: "+htmlEscape(err.Error()))
		return
	}
	go b.runMediaDownloadAndDeliver(s, jobID)
}

// runMediaDownloadAndDeliver polls progress, then streams the file to Telegram.
func (b *Bot) runMediaDownloadAndDeliver(s *pendingMediaSession, jobID string) {
	lastShown := -1
	deadline := time.Now().Add(90 * time.Minute)

	for {
		if time.Now().After(deadline) {
			b.editText(s.chatID, s.promptMsgID, "❌ Yuklash juda uzoq davom etdi (timeout).")
			return
		}
		time.Sleep(3 * time.Second)

		st, err := b.mediaClient.Status(jobID)
		if err != nil {
			log.Printf("[MEDIA] status poll error job=%s: %v", jobID, err)
			continue
		}
		switch st.Status {
		case "downloading", "queued":
			pct := int(st.Percent)
			if pct != lastShown {
				lastShown = pct
				b.editText(s.chatID, s.promptMsgID, fmt.Sprintf("⏳ Yuklanmoqda… %d%%", pct))
			}
		case "completed":
			b.deliverMediaFile(s, jobID, st.FileSize)
			return
		case "failed":
			b.editText(s.chatID, s.promptMsgID, "❌ Yuklashda xatolik: "+htmlEscape(st.Error))
			return
		}
	}
}

// deliverMediaFile sends the downloaded file, or explains if it is too large.
func (b *Bot) deliverMediaFile(s *pendingMediaSession, jobID string, fileSize int64) {
	// Delete the parser-side source file once we're done with it — on every
	// exit path (delivered, failed, or too large) so downloads don't pile up on
	// the parser VPS. Best-effort.
	defer func() {
		if err := b.mediaClient.Cleanup(jobID); err != nil {
			log.Printf("[MEDIA] cleanup failed job=%s: %v", jobID, err)
		}
	}()

	if fileSize > b.config.MediaMaxUploadBytes {
		b.editText(s.chatID, s.promptMsgID, fmt.Sprintf(
			"⚠️ Fayl juda katta (%s). Telegram bot orqali yuborib bo'lmaydi.\n\n"+
				"Kattaroq fayllar uchun self-hosted Telegram Bot API server (TELEGRAM_BOT_API_URL) sozlang.",
			humanBytes(fileSize)))
		return
	}

	b.editText(s.chatID, s.promptMsgID, fmt.Sprintf("📤 Telegramga yuklanmoqda (%s)…", humanBytes(fileSize)))

	// Stream the parser's file into a local temp file so tgbotapi can upload
	// it from disk without buffering the whole movie in memory.
	reader, _, err := b.mediaClient.FileStream(jobID)
	if err != nil {
		log.Printf("[MEDIA] file stream failed job=%s: %v", jobID, err)
		b.editText(s.chatID, s.promptMsgID, "❌ Faylni olishda xatolik.")
		return
	}
	defer reader.Close()

	tmp, err := os.CreateTemp("", "media-*.mp4")
	if err != nil {
		log.Printf("[MEDIA] temp file create failed: %v", err)
		b.editText(s.chatID, s.promptMsgID, "❌ Vaqtinchalik fayl yaratib bo'lmadi.")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, reader); err != nil {
		tmp.Close()
		log.Printf("[MEDIA] temp file write failed: %v", err)
		b.editText(s.chatID, s.promptMsgID, "❌ Faylni saqlashda xatolik.")
		return
	}
	tmp.Close()

	video := tgbotapi.NewVideo(s.chatID, tgbotapi.FilePath(tmpPath))
	video.Caption = "🎬 " + s.title
	video.SupportsStreaming = true
	if _, err := b.api.Send(video); err != nil {
		// Fall back to a document (named after the title) if Telegram rejects
		// the video — e.g. codec/container it won't render inline.
		log.Printf("[MEDIA] send video failed, retrying as document: %v", err)
		doc := tgbotapi.NewDocument(s.chatID, tgbotapi.FileReader{
			Name:   sanitizeFileName(s.title) + ".mp4",
			Reader: mustOpen(tmpPath),
		})
		doc.Caption = "🎬 " + s.title
		if _, err2 := b.api.Send(doc); err2 != nil {
			log.Printf("[MEDIA] send document failed: %v", err2)
			b.editText(s.chatID, s.promptMsgID, "❌ Telegramga yuborib bo'lmadi: "+htmlEscape(err2.Error()))
			return
		}
	}
	b.editText(s.chatID, s.promptMsgID, "✅ Tayyor!")
}

// --- small helpers -------------------------------------------------------

func (b *Bot) getMediaSession(token string) *pendingMediaSession {
	b.mediaMu.Lock()
	defer b.mediaMu.Unlock()
	return b.pendingMedia[token]
}

// pruneMediaSessionsLocked drops expired sessions. Caller holds mediaMu.
func (b *Bot) pruneMediaSessionsLocked() {
	cutoff := time.Now().Add(-pendingMediaTTL)
	for k, v := range b.pendingMedia {
		if v.createdAt.Before(cutoff) {
			delete(b.pendingMedia, k)
		}
	}
}

// splitToken splits "<token>:<rest>" on the first colon.
func splitToken(payload string) (token, rest string) {
	if idx := strings.IndexByte(payload, ':'); idx >= 0 {
		return payload[:idx], payload[idx+1:]
	}
	return payload, ""
}

func (b *Bot) editText(chatID int64, messageID int, text string) {
	if messageID == 0 {
		b.sendMessage(chatID, text)
		return
	}
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "HTML"
	if _, err := b.api.Send(edit); err != nil {
		log.Printf("[MEDIA] editText failed chat=%d msg=%d: %v", chatID, messageID, err)
	}
}

func (b *Bot) editTextWithKeyboard(chatID int64, messageID int, text string, kb tgbotapi.InlineKeyboardMarkup) {
	if messageID == 0 {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "HTML"
		msg.ReplyMarkup = kb
		b.api.Send(msg)
		return
	}
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, kb)
	edit.ParseMode = "HTML"
	if _, err := b.api.Send(edit); err != nil {
		log.Printf("[MEDIA] editTextWithKeyboard failed chat=%d msg=%d: %v", chatID, messageID, err)
	}
}

// mustOpen opens a file for reading; on error returns an empty reader so the
// Send simply fails gracefully (already logged upstream).
func mustOpen(path string) io.Reader {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("[MEDIA] open temp for document failed: %v", err)
		return strings.NewReader("")
	}
	return f
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// sanitizeFileName keeps a filesystem/Telegram-friendly base name.
func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "video"
	}
	var out strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == ' ':
			out.WriteRune(r)
		default:
			out.WriteRune('_')
		}
	}
	name := strings.TrimSpace(out.String())
	if len(name) > 80 {
		name = name[:80]
	}
	if name == "" {
		return "video"
	}
	return name
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

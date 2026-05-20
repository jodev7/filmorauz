package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
)

// parserUploadClient has a bounded timeout so a stuck parser (e.g. CPU
// starved while the worker is transcoding on the same VPS) cannot hang
// backend goroutines indefinitely. Uploads can legitimately take minutes,
// hence 15m rather than a short admin-API timeout.
var parserUploadClient = &http.Client{Timeout: 15 * time.Minute}

// parserStatusClient is used for the short sidecar status lookup that we run
// after a transport-layer error from /instagram/upload. It must NOT share the
// 15-minute timeout above — we want a fast fail/recover loop here.
var parserStatusClient = &http.Client{Timeout: 8 * time.Second}

// InstagramUploadResult is returned on success — populated either from the
// parser response directly or recovered from the sidecar after a 504.
type InstagramUploadResult struct {
	MediaID  string
	PostURL  string
	Account  string
	Recovered bool // true when result came from sidecar after a transport error
}

// InstagramAccountFilter restricts which clips an account is intended
// for. Empty / zero fields disable the corresponding gate, so a fully
// empty filter (the default for legacy accounts) accepts everything.
// Kind is "movie" | "series" | "" (both).
type InstagramAccountFilter struct {
	Kind        string   `json:"kind,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	SeriesSlugs []string `json:"series_slugs,omitempty"`
	MovieSlugs  []string `json:"movie_slugs,omitempty"`
}

// IsEmpty reports whether the filter blocks nothing — used by the admin
// UI to render a generic "all content" label.
func (f InstagramAccountFilter) IsEmpty() bool {
	return f.Kind == "" && len(f.Genres) == 0 && len(f.SeriesSlugs) == 0 && len(f.MovieSlugs) == 0
}

type InstagramAccount struct {
	Name     string                 `json:"name"`
	Username string                 `json:"username"`
	Password string                 `json:"password"`
	Filter   InstagramAccountFilter `json:"filter,omitempty"`
}

// LoadInstagramAccounts parses INSTAGRAM_ACCOUNTS_JSON from env.
// Format: [{"name":"main","username":"...","password":"..."},...]
func LoadInstagramAccounts() []InstagramAccount {
	raw := os.Getenv("INSTAGRAM_ACCOUNTS_JSON")
	if raw == "" {
		return nil
	}
	var accounts []InstagramAccount
	if err := json.Unmarshal([]byte(raw), &accounts); err != nil {
		log.Printf("[Instagram] invalid INSTAGRAM_ACCOUNTS_JSON: %v", err)
		return nil
	}
	return accounts
}

// GetInstagramAccount returns the account with the given name, or nil.
func GetInstagramAccount(name string) *InstagramAccount {
	for _, a := range LoadInstagramAccounts() {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

// InstagramUploadError carries a classified failure from the Instagram upload pipeline.
type InstagramUploadError struct {
	ErrorType      string // challenge_required | checkpoint_required | session_expired | no_session | bad_credentials | rate_limited | network_error | publish_failed
	HumanMessage   string // Uzbek user-facing message
	ActionRequired string // what the admin should do to fix it
	RawError       string // original error string from parser/instagrapi
}

func (e *InstagramUploadError) Error() string { return e.RawError }

var errorTypeMessages = map[string]string{
	"challenge_required":  "Instagram tekshiruvi talab qilinadi (qayta login kerak)",
	"checkpoint_required": "Hisob tekshiruvi talab qilinadi",
	"session_expired":     "Sessiya muddati tugagan yoki bekor qilingan (token expired or invalid), qayta login kerak",
	"no_session":          "Sessiya fayli topilmadi",
	"bad_credentials":     "Login yoki parol noto'g'ri",
	"rate_limited":        "Juda ko'p so'rov yuborildi, keyinroq urinib ko'ring",
	"network_error":       "Parser servisiga ulanib bo'lmadi",
	"publish_failed":      "Yuklash muvaffaqiyatsiz tugadi",
}

var errorTypeActions = map[string]string{
	"challenge_required":  "ig_login.py orqali sessiyani yangilang",
	"checkpoint_required": "ig_login.py orqali sessiyani yangilang",
	"session_expired":     "ig_login.py orqali qayta login qiling",
	"no_session":          "ig_login.py orqali birinchi marta login qiling",
	"bad_credentials":     "Login va parolni tekshiring",
	"rate_limited":        "Bir necha soatdan keyin urinib ko'ring",
	"network_error":       "Parser servisini tekshiring",
	"publish_failed":      "Qayta urinib ko'ring",
}

func humanMessage(errorType string) string {
	if msg, ok := errorTypeMessages[errorType]; ok {
		return msg
	}
	return errorTypeMessages["publish_failed"]
}

func actionRequired(errorType string) string {
	if msg, ok := errorTypeActions[errorType]; ok {
		return msg
	}
	return errorTypeActions["publish_failed"]
}

func resolveInstagramClipURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	cfg := config.Current()
	cdnBase := ""
	if cfg != nil {
		cdnBase = strings.TrimSuffix(cfg.CDNBaseURL, "/")
	}
	if cdnBase == "" {
		return raw
	}

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		switch {
		case strings.Contains(parsed.Path, "/file/filmorauznet/"):
			return raw
		case strings.Contains(parsed.Path, "/media/"):
			mediaPath := parsed.Path[strings.Index(parsed.Path, "/media/")+len("/media"):]
			if strings.HasPrefix(mediaPath, "/videos/") {
				return cdnBase + mediaPath
			}
		case strings.HasPrefix(parsed.Path, "/videos/"):
			return cdnBase + parsed.Path
		}
		return raw
	}

	if strings.HasPrefix(raw, "/media/") {
		mediaPath := strings.TrimPrefix(raw, "/media")
		if strings.HasPrefix(mediaPath, "/videos/") {
			return cdnBase + mediaPath
		}
	}
	if strings.HasPrefix(raw, "/videos/") {
		return cdnBase + raw
	}
	if strings.HasPrefix(raw, "videos/") {
		return cdnBase + "/" + raw
	}

	return raw
}

// UploadReelToInstagram calls the parser service /instagram/upload endpoint.
// publishKey is opaque to the parser; we use it as a sidecar key so that, if
// the parser HTTP response is lost to a 504 / proxy timeout, we can recover
// the actual upload outcome via /instagram/upload/status?publish_key=...
// Returns (result, nil) on success, (nil, *InstagramUploadError) on failure.
func UploadReelToInstagram(parserURL, videoURL, caption, publishKey string, account *InstagramAccount) (*InstagramUploadResult, *InstagramUploadError) {
	resolvedVideoURL := resolveInstagramClipURL(videoURL)
	payload := map[string]string{
		"account_name": account.Name,
		"username":     account.Username,
		"password":     account.Password,
		"video_url":    resolvedVideoURL,
		"caption":      caption,
		"publish_key":  publishKey,
	}
	body, _ := json.Marshal(payload)

	endpoint := parserURL + "/instagram/upload?account=" + url.QueryEscape(account.Name)
	log.Printf("[instagram publish] start publish_key=%s account=%s raw_video_url=%s resolved_video_url=%s",
		publishKey, account.Name, videoURL, resolvedVideoURL)

	start := time.Now()
	resp, err := parserUploadClient.Post(endpoint, "application/json", bytes.NewReader(body))
	latency := time.Since(start)
	if err != nil {
		log.Printf("[instagram publish] transport error publish_key=%s account=%s latency=%s err=%v",
			publishKey, account.Name, latency, err)
		// Recovery path: the upload may have succeeded server-side. Poll the
		// sidecar status endpoint before reporting failure.
		if rec := pollInstagramSidecar(parserURL, publishKey); rec != nil {
			log.Printf("[instagram publish] timeout but previous success exists, keep completed publish_key=%s media_id=%s",
				publishKey, rec.MediaID)
			rec.Account = account.Name
			rec.Recovered = true
			return rec, nil
		}
		return nil, &InstagramUploadError{
			ErrorType:    "network_error",
			HumanMessage: humanMessage("network_error"),
			RawError:     fmt.Sprintf("parser unreachable: %v", err),
		}
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	bodyPreview := string(data)
	if len(bodyPreview) > 500 {
		bodyPreview = bodyPreview[:500] + "...(truncated)"
	}
	log.Printf("[instagram publish] parser response publish_key=%s account=%s http_status=%d latency=%s body=%s",
		publishKey, account.Name, resp.StatusCode, latency, bodyPreview)

	var result struct {
		Status         string `json:"status"`
		Error          string `json:"error"`
		ErrorType      string `json:"error_type"`
		ActionRequired string `json:"action_required"`
		MediaID        string `json:"media_id"`
		MediaCode      string `json:"media_code"`
		PostURL        string `json:"post_url"`
	}
	parseErr := json.Unmarshal(data, &result)

	// Whatever the HTTP status / decode outcome: if a sidecar success exists,
	// trust it. Covers the 504-from-proxy case where the body is HTML.
	if resp.StatusCode >= 500 || parseErr != nil || result.Status != "success" {
		if rec := pollInstagramSidecar(parserURL, publishKey); rec != nil {
			log.Printf("[instagram publish] non-success http=%d but sidecar success exists publish_key=%s media_id=%s",
				resp.StatusCode, publishKey, rec.MediaID)
			rec.Account = account.Name
			rec.Recovered = true
			return rec, nil
		}
	}

	if parseErr != nil {
		log.Printf("[instagram publish] failed reason=bad_parser_response publish_key=%s http=%d", publishKey, resp.StatusCode)
		return nil, &InstagramUploadError{
			ErrorType:      "publish_failed",
			HumanMessage:   humanMessage("publish_failed"),
			ActionRequired: actionRequired("publish_failed"),
			RawError:       fmt.Sprintf("bad response from parser (http=%d): %s", resp.StatusCode, data),
		}
	}
	if result.Status != "success" {
		errType := result.ErrorType
		if errType == "" {
			errType = "publish_failed"
		}
		action := result.ActionRequired
		if action == "" {
			action = actionRequired(errType)
		}
		log.Printf("[instagram publish] failed reason=%s publish_key=%s err=%s", errType, publishKey, result.Error)
		return nil, &InstagramUploadError{
			ErrorType:      errType,
			HumanMessage:   humanMessage(errType),
			ActionRequired: action,
			RawError:       result.Error,
		}
	}
	log.Printf("[instagram publish] success publish_key=%s media_id=%s", publishKey, result.MediaID)
	return &InstagramUploadResult{
		MediaID: result.MediaID,
		PostURL: result.PostURL,
		Account: account.Name,
	}, nil
}

// pollInstagramSidecar asks the parser whether a publish_key has a recorded
// success. The parser writes the sidecar inside instagrapi success, so even
// when the upload HTTP response is lost we can recover the media_id. We poll
// a few times because the sidecar may be written a few seconds after the
// transport timeout fires on the backend side.
func pollInstagramSidecar(parserURL, publishKey string) *InstagramUploadResult {
	if publishKey == "" || parserURL == "" {
		return nil
	}
	endpoint := strings.TrimRight(parserURL, "/") + "/instagram/upload/status?publish_key=" + url.QueryEscape(publishKey)
	deadline := time.Now().Add(45 * time.Second)
	delay := 3 * time.Second
	for attempt := 1; ; attempt++ {
		resp, err := parserStatusClient.Get(endpoint)
		if err == nil {
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var rec struct {
				Status   string `json:"status"`
				MediaID  string `json:"media_id"`
				PostURL  string `json:"post_url"`
				MediaCode string `json:"media_code"`
			}
			if json.Unmarshal(data, &rec) == nil && rec.Status == "success" && rec.MediaID != "" {
				return &InstagramUploadResult{MediaID: rec.MediaID, PostURL: rec.PostURL}
			}
		} else {
			log.Printf("[instagram publish] sidecar poll attempt=%d err=%v", attempt, err)
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(delay)
	}
}

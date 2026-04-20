package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// parserUploadClient has a bounded timeout so a stuck parser (e.g. CPU
// starved while the worker is transcoding on the same VPS) cannot hang
// backend goroutines indefinitely. Uploads can legitimately take minutes,
// hence 15m rather than a short admin-API timeout.
var parserUploadClient = &http.Client{Timeout: 15 * time.Minute}

type InstagramAccount struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
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

// UploadReelToInstagram calls the parser service /instagram/upload endpoint.
// Returns *InstagramUploadError on failure (with classified ErrorType) or nil on success.
func UploadReelToInstagram(parserURL, videoURL, caption string, account *InstagramAccount) error {
	payload := map[string]string{
		"account_name": account.Name,
		"username":     account.Username,
		"password":     account.Password,
		"video_url":    videoURL,
		"caption":      caption,
	}
	body, _ := json.Marshal(payload)

	endpoint := parserURL + "/instagram/upload"
	log.Printf("[Instagram] POST %s account=%s", endpoint, account.Name)

	start := time.Now()
	resp, err := parserUploadClient.Post(endpoint, "application/json", bytes.NewReader(body))
	latency := time.Since(start)
	if err != nil {
		log.Printf("[Instagram] parser request FAILED account=%s latency=%s err=%v",
			account.Name, latency, err)
		return &InstagramUploadError{
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
	log.Printf("[Instagram] parser response account=%s http_status=%d latency=%s body=%s",
		account.Name, resp.StatusCode, latency, bodyPreview)

	var result struct {
		Status         string `json:"status"`
		Error          string `json:"error"`
		ErrorType      string `json:"error_type"`
		ActionRequired string `json:"action_required"`
		MediaID        string `json:"media_id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return &InstagramUploadError{
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
		return &InstagramUploadError{
			ErrorType:      errType,
			HumanMessage:   humanMessage(errType),
			ActionRequired: action,
			RawError:       result.Error,
		}
	}
	return nil
}

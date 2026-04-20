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

// ttParserClient has a bounded timeout so a stuck parser (e.g. CPU
// starved while the worker is transcoding on the same VPS) cannot hang
// backend goroutines indefinitely.
var ttParserClient = &http.Client{Timeout: 15 * time.Minute}

// TikTokAccount holds credentials for a TikTok account.
// TIKTOK_ACCOUNTS_JSON env format:
//
//	[{"name":"main","token_file":"tt_sessions/main_token.json"}]
//
// The token_file is relative to the parser directory and is created by tt_login.py.
// It contains access_token, refresh_token, open_id, client_key, client_secret.
type TikTokAccount struct {
	Name      string `json:"name"`
	TokenFile string `json:"token_file"`
}

// LoadTikTokAccounts parses TIKTOK_ACCOUNTS_JSON from env.
func LoadTikTokAccounts() []TikTokAccount {
	raw := os.Getenv("TIKTOK_ACCOUNTS_JSON")
	if raw == "" {
		return nil
	}
	var accounts []TikTokAccount
	if err := json.Unmarshal([]byte(raw), &accounts); err != nil {
		log.Printf("[TikTok] invalid TIKTOK_ACCOUNTS_JSON: %v", err)
		return nil
	}
	return accounts
}

// GetTikTokAccount returns the account with the given name, or nil.
func GetTikTokAccount(name string) *TikTokAccount {
	for _, a := range LoadTikTokAccounts() {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

// UploadVideoToTikTok calls the parser service /tiktok/upload endpoint.
// Returns nil on success, error on failure.
func UploadVideoToTikTok(parserURL, videoURL, caption string, account *TikTokAccount) error {
	payload := map[string]string{
		"account_name": account.Name,
		"token_file":   account.TokenFile,
		"video_url":    videoURL,
		"caption":      caption,
	}
	body, _ := json.Marshal(payload)

	start := time.Now()
	resp, err := ttParserClient.Post(parserURL+"/tiktok/upload", "application/json", bytes.NewReader(body))
	latency := time.Since(start)
	if err != nil {
		log.Printf("[TikTok] parser request FAILED account=%s latency=%s err=%v",
			account.Name, latency, err)
		return fmt.Errorf("parser unreachable: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[TikTok] parser response account=%s http_status=%d latency=%s",
		account.Name, resp.StatusCode, latency)

	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Status  string `json:"status"`
		Error   string `json:"error"`
		VideoID string `json:"video_id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("bad response from parser: %s", data)
	}
	if result.Status != "success" {
		if result.Error != "" {
			return fmt.Errorf("tiktok upload failed: %s", result.Error)
		}
		return fmt.Errorf("tiktok upload failed (status=%s)", result.Status)
	}
	return nil
}

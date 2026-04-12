package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// YouTubeAccount holds credentials for a YouTube channel.
// YOUTUBE_ACCOUNTS_JSON env format:
//
//	[{"name":"main","token_file":"yt_sessions/main_token.json"}]
//
// The token_file is a path relative to the parser directory containing
// the OAuth2 token JSON produced by google-auth-oauthlib.
type YouTubeAccount struct {
	Name      string `json:"name"`
	TokenFile string `json:"token_file"`
}

// LoadYouTubeAccounts parses YOUTUBE_ACCOUNTS_JSON from env.
func LoadYouTubeAccounts() []YouTubeAccount {
	raw := os.Getenv("YOUTUBE_ACCOUNTS_JSON")
	if raw == "" {
		return nil
	}
	var accounts []YouTubeAccount
	if err := json.Unmarshal([]byte(raw), &accounts); err != nil {
		log.Printf("[YouTube] invalid YOUTUBE_ACCOUNTS_JSON: %v", err)
		return nil
	}
	return accounts
}

// GetYouTubeAccount returns the account with the given name, or nil.
func GetYouTubeAccount(name string) *YouTubeAccount {
	for _, a := range LoadYouTubeAccounts() {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

// UploadShortToYouTube calls the parser service /youtube/upload endpoint.
// Returns nil on success, error on failure.
func UploadShortToYouTube(parserURL, videoURL, title, description string, account *YouTubeAccount) error {
	payload := map[string]string{
		"account_name": account.Name,
		"token_file":   account.TokenFile,
		"video_url":    videoURL,
		"title":        title,
		"description":  description,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(parserURL+"/youtube/upload", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("parser unreachable: %w", err)
	}
	defer resp.Body.Close()

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
			return fmt.Errorf("youtube upload failed: %s", result.Error)
		}
		return fmt.Errorf("youtube upload failed (status=%s)", result.Status)
	}
	return nil
}

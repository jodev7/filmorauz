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

// UploadReelToInstagram calls the parser service /instagram/upload endpoint.
// The parser handles the actual instagrapi call so Go doesn't need Python deps.
func UploadReelToInstagram(parserURL, videoURL, caption string, account *InstagramAccount) error {
	payload := map[string]string{
		"account_name": account.Name,
		"username":     account.Username,
		"password":     account.Password,
		"video_url":    videoURL,
		"caption":      caption,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(parserURL+"/instagram/upload", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("parser unreachable: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("bad response from parser: %s", data)
	}
	if result.Status != "success" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

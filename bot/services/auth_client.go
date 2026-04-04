package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// AuthClient handles communication with backend auth endpoints
type AuthClient struct {
	baseURL    string
	httpClient *http.Client
	botAPI     *tgbotapi.BotAPI
}

// AuthCompleteRequest is the request body for completing auth
type AuthCompleteRequest struct {
	Code       string `json:"code"`
	TelegramID int64  `json:"telegram_id"`
	Username   string `json:"username"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	PhotoURL   string `json:"photo_url,omitempty"`
}

// AuthCompleteResponse is the response from auth completion endpoint
type AuthCompleteResponse struct {
	Status  string `json:"status"`
	UserID  string `json:"user_id,omitempty"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// NewAuthClient creates a new auth client
func NewAuthClient(baseURL string, botAPI *tgbotapi.BotAPI) *AuthClient {
	return &AuthClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		botAPI: botAPI,
	}
}

// CompleteAuthSession calls the backend to complete a Telegram auth session
func (c *AuthClient) CompleteAuthSession(authCode string, user *tgbotapi.User) (*AuthCompleteResponse, error) {
	// Extract username (without @)
	username := ""
	if user.UserName != "" {
		username = user.UserName
	}

	// Build request
	reqBody := AuthCompleteRequest{
		Code:       authCode,
		TelegramID: user.ID,
		Username:   username,
		FirstName:  user.FirstName,
		LastName:   user.LastName,
	}

	// Convert to JSON
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[AUTH CLIENT] Error marshaling request: %v", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make request
	url := fmt.Sprintf("%s/api/auth/telegram/complete", c.baseURL)
	log.Printf("[AUTH CLIENT] >>> POST %s", url)
	log.Printf("[AUTH CLIENT] >>> Request payload: %s", string(jsonBody))

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Printf("[AUTH CLIENT] Error creating request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[AUTH CLIENT] Error sending request: %v", err)
		return nil, fmt.Errorf("backendga ulanishda xatolik: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[AUTH CLIENT] Error reading response: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[AUTH CLIENT] <<< Response status: %d", resp.StatusCode)
	log.Printf("[AUTH CLIENT] <<< Response body: %s", string(body))

	// Parse response
	var authResp AuthCompleteResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		log.Printf("[AUTH CLIENT] Error parsing response: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for backend error in response body
	if authResp.Error != "" {
		log.Printf("[AUTH CLIENT] Backend returned error: %s", authResp.Error)
		return &authResp, fmt.Errorf(authResp.Error)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("backend returned status %d: %s", resp.StatusCode, authResp.Message)
		log.Printf("[AUTH CLIENT] %s", errMsg)
		return &authResp, fmt.Errorf(errMsg)
	}

	log.Printf("[AUTH CLIENT] Auth completed successfully for user %d", user.ID)
	return &authResp, nil
}

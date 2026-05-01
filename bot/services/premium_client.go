package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type PremiumClient struct {
	baseURL       string
	internalToken string
	httpClient    *http.Client
}

func NewPremiumClient(baseURL, internalToken string) *PremiumClient {
	return &PremiumClient{
		baseURL:       baseURL,
		internalToken: internalToken,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
	}
}

type GrantStarsRequest struct {
	TelegramID              int64  `json:"telegram_id"`
	SessionToken            string `json:"session_token"`
	StarsAmount             int    `json:"stars_amount"`
	TelegramPaymentChargeID string `json:"telegram_payment_charge_id"`
	ProviderPaymentChargeID string `json:"provider_payment_charge_id"`
}

type GrantStarsResponse struct {
	OK               bool   `json:"ok"`
	AlreadyProcessed bool   `json:"already_processed"`
	UserID           string `json:"user_id"`
	PremiumExpires   string `json:"premium_expires"`
	Error            string `json:"error"`
}

type ValidateStarsSessionRequest struct {
	Token      string `json:"token"`
	TelegramID int64  `json:"telegram_id"`
}

type ValidateStarsSessionResponse struct {
	OK             bool   `json:"ok"`
	Token          string `json:"token"`
	UserID         string `json:"user_id"`
	TelegramID     int64  `json:"telegram_id"`
	Package        string `json:"package"`
	Label          string `json:"label"`
	DurationMonths int    `json:"duration_months"`
	StarsPrice     int    `json:"stars_price"`
	ExpiresAt      string `json:"expires_at"`
	Error          string `json:"error"`
}

// GrantTelegramStars calls the backend to grant premium for a successful Stars
// payment. Returns the parsed response so the bot can show the right message
// (e.g. "user_not_linked" → ask the user to log in on the website first).
func (c *PremiumClient) GrantTelegramStars(req GrantStarsRequest) (*GrantStarsResponse, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, err
	}
	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/internal/premium/telegram-stars/grant", bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("backend request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var out GrantStarsResponse
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &out)
	}
	return &out, resp.StatusCode, nil
}

func (c *PremiumClient) ValidateStarsSession(req ValidateStarsSessionRequest) (*ValidateStarsSessionResponse, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, err
	}
	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/internal/premium/telegram-stars/session/validate", bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("backend request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var out ValidateStarsSessionResponse
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &out)
	}
	return &out, resp.StatusCode, nil
}

package seo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GoogleIndexingService submits URL_UPDATED / URL_DELETED notifications to
// the Google Indexing API. Google officially supports this API for
// JobPosting and BroadcastEvent schemas only — we use it as a best-effort
// signal alongside sitemap-resubmit. If a call fails it does not block.
type GoogleIndexingService struct {
	tokens *googleTokenSource
	client *http.Client
}

const googleIndexingEndpoint = "https://indexing.googleapis.com/v3/urlNotifications:publish"

func NewGoogleIndexingService(credentialsPath string) (*GoogleIndexingService, error) {
	ts, err := newGoogleTokenSource(credentialsPath, "https://www.googleapis.com/auth/indexing")
	if err != nil {
		return nil, err
	}
	return &GoogleIndexingService{
		tokens: ts,
		client: &http.Client{Timeout: 20 * time.Second},
	}, nil
}

// NotifyAction is the type of notification to send.
type NotifyAction string

const (
	NotifyUpdated NotifyAction = "URL_UPDATED"
	NotifyDeleted NotifyAction = "URL_DELETED"
)

func (g *GoogleIndexingService) Publish(target string, action NotifyAction) error {
	if g == nil {
		return errors.New("google indexing: service not configured")
	}
	token, err := g.tokens.Token()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"url":  target,
		"type": string(action),
	})
	req, err := http.NewRequest(http.MethodPost, googleIndexingEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("google indexing: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 300))
}

// PublishBatch sends notifications sequentially. Google does not provide a
// batch endpoint for this API; quota is 200 requests/day by default.
func (g *GoogleIndexingService) PublishBatch(urls []string, action NotifyAction) (int, error) {
	if g == nil {
		return 0, errors.New("google indexing: service not configured")
	}
	ok := 0
	var firstErr error
	for _, u := range urls {
		if err := g.Publish(u, action); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ok++
	}
	return ok, firstErr
}

func (g *GoogleIndexingService) ServiceAccountEmail() string {
	if g == nil || g.tokens == nil {
		return ""
	}
	return g.tokens.Email()
}

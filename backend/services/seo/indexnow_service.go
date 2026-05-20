package seo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// IndexNowService pings the IndexNow endpoint used by Bing, Yandex, Seznam,
// Naver and Yep. A single submission is broadcast to all participating
// search engines, so we only need to talk to one host.
type IndexNowService struct {
	key      string
	host     string
	endpoint string
	client   *http.Client
}

const defaultIndexNowEndpoint = "https://api.indexnow.org/IndexNow"

func NewIndexNowService(key, siteURL string) (*IndexNowService, error) {
	key = strings.TrimSpace(key)
	siteURL = strings.TrimSpace(siteURL)
	if key == "" {
		return nil, errors.New("indexnow: key is required")
	}
	if siteURL == "" {
		return nil, errors.New("indexnow: siteURL is required")
	}
	u, err := url.Parse(siteURL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("indexnow: invalid siteURL: %s", siteURL)
	}
	return &IndexNowService{
		key:      key,
		host:     u.Host,
		endpoint: defaultIndexNowEndpoint,
		client:   &http.Client{Timeout: 20 * time.Second},
	}, nil
}

// Submit sends the given URLs to IndexNow in a single batch. The IndexNow
// spec allows up to 10,000 URLs per request, but we cap conservatively.
func (s *IndexNowService) Submit(urls []string) error {
	if s == nil || len(urls) == 0 {
		return nil
	}
	if len(urls) == 1 {
		return s.submitSingle(urls[0])
	}

	const maxBatch = 5000
	for i := 0; i < len(urls); i += maxBatch {
		end := i + maxBatch
		if end > len(urls) {
			end = len(urls)
		}
		body, err := json.Marshal(map[string]any{
			"host":        s.host,
			"key":         s.key,
			"keyLocation": fmt.Sprintf("https://%s/%s.txt", s.host, s.key),
			"urlList":     urls[i:end],
		})
		if err != nil {
			return err
		}
		req, err := http.NewRequest(http.MethodPost, s.endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		// Accepted statuses per spec: 200, 202.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			return fmt.Errorf("indexnow: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 200))
		}
	}
	return nil
}

func (s *IndexNowService) submitSingle(target string) error {
	q := url.Values{}
	q.Set("url", target)
	q.Set("key", s.key)
	q.Set("keyLocation", fmt.Sprintf("https://%s/%s.txt", s.host, s.key))
	endpoint := s.endpoint + "?" + q.Encode()
	resp, err := s.client.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("indexnow: status=%d body=%s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

func (s *IndexNowService) Key() string  { return s.key }
func (s *IndexNowService) Host() string { return s.host }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

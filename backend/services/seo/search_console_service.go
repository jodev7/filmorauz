package seo

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SearchConsoleService re-submits sitemaps to Google Search Console. This
// is the safe, officially-supported way to tell Google "new URLs are
// available" and works for any schema (not just JobPosting).
//
// The service account used here must be added as an Owner of the property
// in Search Console — Restricted/Full role is not enough.
type SearchConsoleService struct {
	tokens  *googleTokenSource
	siteURL string
	client  *http.Client
}

// NewSearchConsoleService returns nil, nil when credentials or siteURL are
// missing — callers may treat that as "not configured" and continue.
func NewSearchConsoleService(credentialsPath, siteURL string) (*SearchConsoleService, error) {
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		return nil, errors.New("search console: GOOGLE_SEARCH_CONSOLE_SITE_URL is empty")
	}
	ts, err := newGoogleTokenSource(credentialsPath, "https://www.googleapis.com/auth/webmasters")
	if err != nil {
		return nil, err
	}
	return &SearchConsoleService{
		tokens:  ts,
		siteURL: siteURL,
		client:  &http.Client{Timeout: 20 * time.Second},
	}, nil
}

// SubmitSitemap notifies Search Console that the given sitemap URL has
// been updated. The endpoint is idempotent — calling it on every publish
// is fine.
func (s *SearchConsoleService) SubmitSitemap(sitemapURL string) error {
	if s == nil {
		return errors.New("search console: service not configured")
	}
	token, err := s.tokens.Token()
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf(
		"https://searchconsole.googleapis.com/webmasters/v3/sites/%s/sitemaps/%s",
		url.PathEscape(s.siteURL),
		url.PathEscape(sitemapURL),
	)
	req, err := http.NewRequest(http.MethodPut, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("search console: status=%d body=%s", resp.StatusCode, truncate(string(body), 300))
}

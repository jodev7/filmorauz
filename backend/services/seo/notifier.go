package seo

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// EventStatus is the outcome of a single submission to an external service.
type EventStatus string

const (
	StatusOK      EventStatus = "ok"
	StatusError   EventStatus = "error"
	StatusSkipped EventStatus = "skipped"
)

// SubmitEvent is a row in the seo_events Mongo collection. Each row
// represents one submission attempt to one provider (indexnow / google /
// search-console). The collection is used by the admin SEO dashboard.
type SubmitEvent struct {
	ID        any         `bson:"_id,omitempty"            json:"id,omitempty"`
	Provider  string      `bson:"provider"                 json:"provider"`
	Action    string      `bson:"action"                   json:"action"`
	URL       string      `bson:"url,omitempty"            json:"url,omitempty"`
	Sitemap   string      `bson:"sitemap,omitempty"        json:"sitemap,omitempty"`
	Status    EventStatus `bson:"status"                   json:"status"`
	Error     string      `bson:"error,omitempty"          json:"error,omitempty"`
	Count     int         `bson:"count,omitempty"          json:"count,omitempty"`
	CreatedAt time.Time   `bson:"created_at"               json:"created_at"`
}

// Config is the wiring container passed by main.go.
type Config struct {
	Enabled               bool
	SiteURL               string // e.g. https://filmorauz.net
	IndexNow              *IndexNowService
	GoogleIndexing        *GoogleIndexingService
	SearchConsole         *SearchConsoleService
	EventsCol             *mongo.Collection
	SitemapURLs           []string // e.g. ["/sitemap.xml"] — relative to SiteURL
	GoogleIndexingEnabled bool     // controls whether per-URL Google calls run
}

// Notifier is the single entry point used by movie/series services. All
// methods are safe to call when the Notifier is nil or disabled — they
// no-op silently. Submissions are dispatched on a background goroutine
// so callers never block on network IO.
type Notifier struct {
	cfg Config
	mu  sync.Mutex // serializes mongo writes to avoid burst contention
}

func NewNotifier(cfg Config) *Notifier {
	return &Notifier{cfg: cfg}
}

// Enabled reports whether at least one downstream provider is configured.
func (n *Notifier) Enabled() bool {
	if n == nil || !n.cfg.Enabled {
		return false
	}
	return n.cfg.IndexNow != nil || n.cfg.GoogleIndexing != nil || n.cfg.SearchConsole != nil
}

// NotifyURL signals one content URL was created or updated. Path is
// relative ("/movies/foo") and is joined to the configured site URL.
func (n *Notifier) NotifyURL(path string) {
	if !n.Enabled() {
		return
	}
	full := n.absolute(path)
	if full == "" {
		return
	}
	go n.dispatchOne(full, NotifyUpdated)
}

// NotifyURLs signals a batch of paths.
func (n *Notifier) NotifyURLs(paths []string) {
	if !n.Enabled() || len(paths) == 0 {
		return
	}
	urls := make([]string, 0, len(paths))
	for _, p := range paths {
		if u := n.absolute(p); u != "" {
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 {
		return
	}
	go n.dispatchBatch(urls, NotifyUpdated)
}

// NotifyDeleted signals a content URL was removed.
func (n *Notifier) NotifyDeleted(path string) {
	if !n.Enabled() {
		return
	}
	full := n.absolute(path)
	if full == "" {
		return
	}
	go n.dispatchOne(full, NotifyDeleted)
}

// NotifySitemap re-submits every configured sitemap to Search Console.
// Safe to call on every publish — the endpoint is idempotent.
func (n *Notifier) NotifySitemap() {
	if !n.Enabled() {
		return
	}
	go n.resubmitSitemaps()
}

func (n *Notifier) dispatchOne(absURL string, action NotifyAction) {
	if action == NotifyUpdated && n.cfg.IndexNow != nil {
		err := n.cfg.IndexNow.Submit([]string{absURL})
		n.logEvent("indexnow", string(action), absURL, "", err, 1)
	}
	if n.cfg.GoogleIndexingEnabled && n.cfg.GoogleIndexing != nil {
		err := n.cfg.GoogleIndexing.Publish(absURL, action)
		n.logEvent("google_indexing", string(action), absURL, "", err, 1)
	}
	if action == NotifyUpdated && n.cfg.SearchConsole != nil {
		n.resubmitSitemaps()
	}
}

func (n *Notifier) dispatchBatch(urls []string, action NotifyAction) {
	if action == NotifyUpdated && n.cfg.IndexNow != nil {
		err := n.cfg.IndexNow.Submit(urls)
		n.logEvent("indexnow", string(action), "", "", err, len(urls))
	}
	if n.cfg.GoogleIndexingEnabled && n.cfg.GoogleIndexing != nil {
		ok, err := n.cfg.GoogleIndexing.PublishBatch(urls, action)
		n.logEvent("google_indexing", string(action), "", "", err, ok)
	}
	if action == NotifyUpdated && n.cfg.SearchConsole != nil {
		n.resubmitSitemaps()
	}
}

func (n *Notifier) resubmitSitemaps() {
	if n.cfg.SearchConsole == nil {
		return
	}
	for _, sm := range n.cfg.SitemapURLs {
		absSM := n.absolute(sm)
		if absSM == "" {
			continue
		}
		err := n.cfg.SearchConsole.SubmitSitemap(absSM)
		n.logEvent("search_console", "SITEMAP", "", absSM, err, 1)
	}
}

func (n *Notifier) logEvent(provider, action, url, sitemap string, err error, count int) {
	status := StatusOK
	errMsg := ""
	if err != nil {
		status = StatusError
		errMsg = err.Error()
		log.Printf("[seo] provider=%s action=%s err=%v url=%s sitemap=%s", provider, action, err, url, sitemap)
	}
	if n.cfg.EventsCol == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evt := SubmitEvent{
		Provider:  provider,
		Action:    action,
		URL:       url,
		Sitemap:   sitemap,
		Status:    status,
		Error:     errMsg,
		Count:     count,
		CreatedAt: time.Now().UTC(),
	}
	if _, derr := n.cfg.EventsCol.InsertOne(ctx, evt); derr != nil {
		log.Printf("[seo] failed to log event: %v", derr)
	}
}

func (n *Notifier) absolute(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	base := strings.TrimRight(n.cfg.SiteURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if base == "" {
		return ""
	}
	return base + path
}

// RecentEvents returns the most recent N events from the events collection
// (newest first). Used by the admin dashboard.
func (n *Notifier) RecentEvents(limit int) ([]SubmitEvent, error) {
	if n == nil || n.cfg.EventsCol == nil {
		return nil, errors.New("seo: events collection not configured")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cursor, err := n.cfg.EventsCol.Find(ctx, bson.M{},
		mongoFindOpts(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var events []SubmitEvent
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// StatusSummary returns counters used by the admin dashboard.
type StatusSummary struct {
	Enabled            bool   `json:"enabled"`
	IndexNowConfigured bool   `json:"indexnow_configured"`
	GoogleConfigured   bool   `json:"google_indexing_configured"`
	SearchConsoleReady bool   `json:"search_console_configured"`
	IndexNowKey        string `json:"indexnow_key,omitempty"`
	SiteURL            string `json:"site_url"`
}

func (n *Notifier) Status() StatusSummary {
	s := StatusSummary{SiteURL: n.cfg.SiteURL}
	if n == nil {
		return s
	}
	s.Enabled = n.Enabled()
	if n.cfg.IndexNow != nil {
		s.IndexNowConfigured = true
		s.IndexNowKey = n.cfg.IndexNow.Key()
	}
	s.GoogleConfigured = n.cfg.GoogleIndexing != nil
	s.SearchConsoleReady = n.cfg.SearchConsole != nil
	return s
}

func (n *Notifier) SiteURL() string {
	if n == nil {
		return ""
	}
	return n.cfg.SiteURL
}

// Submit lets the admin dashboard or cron call the same code path.
func (n *Notifier) Submit(paths []string) error {
	if !n.Enabled() {
		return errors.New("seo: notifier disabled")
	}
	urls := make([]string, 0, len(paths))
	for _, p := range paths {
		if u := n.absolute(p); u != "" {
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 {
		return errors.New("seo: no valid URLs to submit")
	}
	n.dispatchBatch(urls, NotifyUpdated)
	return nil
}

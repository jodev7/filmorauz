// Package services — B2 cleanup helpers.
//
// B2CleanupService authenticates against Backblaze B2 and deletes files
// (by exact key or by prefix walk). Used by movie and series deletion to
// remove HLS folders, posters, backdrops, clips, and other assets that
// would otherwise orphan in the bucket after the DB record is removed.
//
// All delete operations are best-effort: a missing file is logged and
// skipped, a transient error is logged and reported back in the summary,
// and the overall caller continues so a single failure cannot abort the
// cascade and leave the system half-deleted.
//
// Safety model: deletion is only allowed under a fixed allowlist of root
// prefixes (see allowedB2DeleteRoots). Any attempt to delete an empty key,
// a bucket-wide prefix, or one of the bare allowlist roots themselves is
// rejected up front. This is the guard against the historical bug where
// a single movie delete walked "videos/movies/" and wiped the whole
// catalog.
package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// allowedB2DeleteRoots are the only prefixes under which keys may be
// deleted. A delete target must extend at least one character past one
// of these roots to be considered safe.
var allowedB2DeleteRoots = []string{
	"videos/movies/",
	"videos/series/",
	"videos/serials/",
	"videos/clips/",
	"images/posters/",
	"images/backdrops/",
	"images/series/",
	"images/episodes/",
	"content/",
}

// broadB2DeletePrefixes is an explicit blocklist of prefixes that must
// never be passed to a delete. Even if a future bug allowed one of these
// through the allowlist check, this second guard rejects them.
var broadB2DeletePrefixes = map[string]struct{}{
	"":                  {},
	"/":                 {},
	"videos":            {},
	"videos/":           {},
	"videos/movies":     {},
	"videos/movies/":    {},
	"videos/series":     {},
	"videos/series/":    {},
	"videos/serials":    {},
	"videos/serials/":   {},
	"videos/clips":      {},
	"videos/clips/":     {},
	"images":            {},
	"images/":           {},
	"images/posters":    {},
	"images/posters/":   {},
	"images/backdrops":  {},
	"images/backdrops/": {},
	"images/series":     {},
	"images/series/":    {},
	"images/episodes":   {},
	"images/episodes/":  {},
	"movies":            {},
	"movies/":           {},
	"clips":             {},
	"clips/":            {},
	"content":           {},
	"content/":          {},
}

// B2CleanupService deletes files from a Backblaze B2 bucket.
// Zero value is not usable — create via NewB2CleanupService.
type B2CleanupService struct {
	keyID      string
	appKey     string
	bucketName string
	cdnURL     string

	mu            sync.Mutex
	apiURL        string
	authToken     string
	bucketID      string
	authExpiresAt time.Time

	httpClient *http.Client
}

// B2CleanupConfig holds the credentials and identifiers needed to talk to B2.
type B2CleanupConfig struct {
	KeyID      string
	AppKey     string
	BucketName string
	CDNURL     string // optional — used only for URL-to-key extraction
}

// B2DeleteStats summarizes a bulk delete.
type B2DeleteStats struct {
	Deleted int
	Skipped int
	Failed  int
}

// B2DeleteSummary aggregates the result of a cascade delete that touches
// multiple keys and prefixes. It is JSON-serialized in the admin delete
// response so the UI can show partial-failure warnings.
type B2DeleteSummary struct {
	FilesDeleted    int      `json:"files_deleted"`
	PrefixesDeleted []string `json:"prefixes_deleted"`
	Skipped         []string `json:"skipped"`
	Errors          []string `json:"errors"`
}

// NewB2DeleteSummary returns an empty, ready-to-fill summary with the
// slice fields initialized so JSON serializes them as [] instead of null.
func NewB2DeleteSummary() *B2DeleteSummary {
	return &B2DeleteSummary{
		PrefixesDeleted: []string{},
		Skipped:         []string{},
		Errors:          []string{},
	}
}

// NewB2CleanupService returns a ready-to-use cleanup service, or nil if
// B2 credentials are not configured. Callers should nil-check the return
// value and treat nil as "skip B2 cleanup" (e.g. DEV mode with local FS).
func NewB2CleanupService(cfg B2CleanupConfig) *B2CleanupService {
	if cfg.KeyID == "" || cfg.AppKey == "" || cfg.BucketName == "" {
		log.Printf("[B2_CLEANUP] disabled: missing B2 credentials or bucket")
		return nil
	}
	return &B2CleanupService{
		keyID:      cfg.KeyID,
		appKey:     cfg.AppKey,
		bucketName: cfg.BucketName,
		cdnURL:     strings.TrimSuffix(cfg.CDNURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type b2AuthResp struct {
	AccountID          string `json:"accountId"`
	AuthorizationToken string `json:"authorizationToken"`
	APIURL             string `json:"apiUrl"`
	Allowed            struct {
		BucketID   string `json:"bucketId"`
		BucketName string `json:"bucketName"`
	} `json:"allowed"`
}

type b2ListFilesResp struct {
	Files []struct {
		FileID      string `json:"fileId"`
		FileName    string `json:"fileName"`
		Action      string `json:"action"` // "upload" | "hide" | "folder"
		ContentType string `json:"contentType"`
	} `json:"files"`
	NextFileName string `json:"nextFileName"`
}

func (s *B2CleanupService) authorize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.authToken != "" && time.Now().Before(s.authExpiresAt) {
		return nil
	}

	req, err := http.NewRequest("GET", "https://api.backblazeb2.com/b2api/v2/b2_authorize_account", nil)
	if err != nil {
		return fmt.Errorf("b2 authorize: build req: %w", err)
	}
	creds := base64.StdEncoding.EncodeToString([]byte(s.keyID + ":" + s.appKey))
	req.Header.Set("Authorization", "Basic "+creds)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("b2 authorize: http: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("b2 authorize status=%d body=%s", resp.StatusCode, string(body))
	}
	var out b2AuthResp
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("b2 authorize: decode: %w", err)
	}

	s.apiURL = out.APIURL
	s.authToken = out.AuthorizationToken
	s.authExpiresAt = time.Now().Add(20 * time.Hour) // B2 tokens last 24h

	// Resolve bucket ID.
	if out.Allowed.BucketID != "" && (out.Allowed.BucketName == "" || out.Allowed.BucketName == s.bucketName) {
		s.bucketID = out.Allowed.BucketID
		return nil
	}
	return s.resolveBucketIDLocked(out.AccountID)
}

// resolveBucketIDLocked assumes s.mu is held.
func (s *B2CleanupService) resolveBucketIDLocked(accountID string) error {
	payload, _ := json.Marshal(map[string]string{
		"accountId":  accountID,
		"bucketName": s.bucketName,
	})
	req, err := http.NewRequest("POST", s.apiURL+"/b2api/v2/b2_list_buckets", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", s.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("b2 list_buckets status=%d body=%s", resp.StatusCode, string(body))
	}
	var out struct {
		Buckets []struct {
			BucketID   string `json:"bucketId"`
			BucketName string `json:"bucketName"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("b2 list_buckets decode: %w", err)
	}
	for _, b := range out.Buckets {
		if b.BucketName == s.bucketName {
			s.bucketID = b.BucketID
			return nil
		}
	}
	return fmt.Errorf("b2 bucket %q not found", s.bucketName)
}

// listByPrefix lists all files with the given prefix. Handles pagination.
func (s *B2CleanupService) listByPrefix(prefix string) ([]b2ListFilesResp, error) {
	if err := s.authorize(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	apiURL := s.apiURL
	authToken := s.authToken
	bucketID := s.bucketID
	s.mu.Unlock()

	var pages []b2ListFilesResp
	startFileName := ""
	for {
		payload, _ := json.Marshal(map[string]interface{}{
			"bucketId":      bucketID,
			"prefix":        prefix,
			"maxFileCount":  1000,
			"startFileName": startFileName,
		})
		req, err := http.NewRequest("POST", apiURL+"/b2api/v2/b2_list_file_names", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", authToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("b2 list_file_names: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("b2 list_file_names status=%d body=%s", resp.StatusCode, string(body))
		}
		var page b2ListFilesResp
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("b2 list_file_names decode: %w", err)
		}
		pages = append(pages, page)
		if page.NextFileName == "" || len(page.Files) == 0 {
			break
		}
		startFileName = page.NextFileName
	}
	return pages, nil
}

// deleteFileVersion deletes a single file version. Missing files are logged
// and treated as success (they may have already been cleaned up).
func (s *B2CleanupService) deleteFileVersion(fileName, fileID string) error {
	if err := s.authorize(); err != nil {
		return err
	}
	s.mu.Lock()
	apiURL := s.apiURL
	authToken := s.authToken
	s.mu.Unlock()

	payload, _ := json.Marshal(map[string]string{
		"fileName": fileName,
		"fileId":   fileID,
	})
	req, err := http.NewRequest("POST", apiURL+"/b2api/v2/b2_delete_file_version", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("b2 delete_file_version status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

// DeleteByKey deletes every version of a single object. Returns (skipped=true)
// when the file does not exist. Never returns an error for "not found".
//
// This is the low-level primitive — callers that take user/DB-derived input
// should prefer SafeDeleteKey, which normalizes the input and applies the
// allowlist guard.
func (s *B2CleanupService) DeleteByKey(key string) (skipped bool, err error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return true, nil
	}
	pages, err := s.listByPrefix(key)
	if err != nil {
		return false, err
	}
	found := false
	for _, page := range pages {
		for _, f := range page.Files {
			if f.FileName != key {
				continue
			}
			found = true
			if delErr := s.deleteFileVersion(f.FileName, f.FileID); delErr != nil {
				log.Printf("[B2_CLEANUP] FAILED key=%s: %v", key, delErr)
				return false, delErr
			}
			log.Printf("[B2_CLEANUP] DELETED key=%s", key)
		}
	}
	if !found {
		log.Printf("[B2_CLEANUP] SKIP key=%s (not found)", key)
		return true, nil
	}
	return false, nil
}

// DeleteByPrefix deletes every file under the given prefix (recursive).
// Returns counts; per-file failures are logged but do not abort the walk.
// A missing prefix (0 matches) is logged and returned as {0, 0, 0} with no error.
//
// Low-level primitive; prefer SafeDeletePrefix for cascade flows.
func (s *B2CleanupService) DeleteByPrefix(prefix string) (B2DeleteStats, error) {
	stats := B2DeleteStats{}
	prefix = normalizeB2DeletePrefix(prefix)
	if prefix == "" {
		return stats, nil
	}
	if isUnsafeB2DeletePrefix(prefix) {
		return stats, fmt.Errorf("unsafe delete prefix")
	}
	pages, err := s.listByPrefix(prefix)
	if err != nil {
		return stats, err
	}
	total := 0
	for _, page := range pages {
		total += len(page.Files)
	}
	if total == 0 {
		log.Printf("[B2_CLEANUP] SKIP prefix=%s (no files found)", prefix)
		return stats, nil
	}
	log.Printf("[B2_CLEANUP] prefix=%s files=%d — starting delete", prefix, total)
	for _, page := range pages {
		for _, f := range page.Files {
			if f.Action == "folder" {
				continue
			}
			if delErr := s.deleteFileVersion(f.FileName, f.FileID); delErr != nil {
				log.Printf("[B2_CLEANUP] FAILED prefix=%s file=%s: %v", prefix, f.FileName, delErr)
				stats.Failed++
				continue
			}
			stats.Deleted++
			log.Printf("[B2_CLEANUP] DELETED file=%s", f.FileName)
		}
	}
	log.Printf("[B2_CLEANUP] prefix=%s done deleted=%d failed=%d", prefix, stats.Deleted, stats.Failed)
	return stats, nil
}

// SafeDeleteKey normalizes raw (a CDN URL, /media/<key>, /<key>, or bare
// key) and deletes the resulting object if and only if it falls under one
// of the allowlist roots. Updates summary in place; never returns an error
// (collected in summary.Errors / summary.Skipped instead).
//
// label is a short human-readable tag (e.g. "movie-poster") that is
// included in skip/error entries to make admin debugging easier.
func (s *B2CleanupService) SafeDeleteKey(raw, label string, summary *B2DeleteSummary) {
	if summary == nil {
		summary = NewB2DeleteSummary()
	}
	key := s.NormalizeKey(raw)
	if key == "" {
		summary.Skipped = append(summary.Skipped, fmt.Sprintf("%s: empty key (raw=%q)", label, raw))
		log.Printf("[B2_SAFE_DELETE] SKIP %s: empty key (raw=%q)", label, raw)
		return
	}
	if !IsSafeB2DeleteKey(key) {
		summary.Skipped = append(summary.Skipped, fmt.Sprintf("%s: unsafe key %q (raw=%q)", label, key, raw))
		log.Printf("[B2_SAFE_DELETE] SKIP %s: unsafe key=%q raw=%q", label, key, raw)
		return
	}
	log.Printf("[B2_SAFE_DELETE] %s deleting key=%s", label, key)
	skipped, err := s.DeleteByKey(key)
	if err != nil {
		summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s: %v", label, key, err))
		return
	}
	if skipped {
		summary.Skipped = append(summary.Skipped, fmt.Sprintf("%s: %s (not found)", label, key))
		return
	}
	summary.FilesDeleted++
}

// SafeDeletePrefix normalizes prefix and recursively deletes every file
// under it if and only if it is under one of the allowlist roots and
// extends at least one segment past that root.
//
// Updates summary in place; never returns an error. label is a short
// human-readable tag included in skip/error entries.
func (s *B2CleanupService) SafeDeletePrefix(rawPrefix, label string, summary *B2DeleteSummary) {
	if summary == nil {
		summary = NewB2DeleteSummary()
	}
	prefix := s.NormalizeKey(rawPrefix)
	prefix = normalizeB2DeletePrefix(prefix)
	if prefix == "" {
		summary.Skipped = append(summary.Skipped, fmt.Sprintf("%s: empty prefix (raw=%q)", label, rawPrefix))
		log.Printf("[B2_SAFE_DELETE] SKIP %s: empty prefix (raw=%q)", label, rawPrefix)
		return
	}
	if !IsSafeB2DeletePrefix(prefix) {
		summary.Skipped = append(summary.Skipped, fmt.Sprintf("%s: unsafe prefix %q (raw=%q)", label, prefix, rawPrefix))
		log.Printf("[B2_SAFE_DELETE] SKIP %s: unsafe prefix=%q raw=%q", label, prefix, rawPrefix)
		return
	}
	log.Printf("[B2_SAFE_DELETE] %s deleting prefix=%s", label, prefix)
	stats, err := s.DeleteByPrefix(prefix)
	if err != nil {
		summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s: %v", label, prefix, err))
		return
	}
	summary.FilesDeleted += stats.Deleted
	if stats.Failed > 0 {
		summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s: %d file delete(s) failed", label, prefix, stats.Failed))
	}
	if stats.Deleted == 0 && stats.Failed == 0 {
		summary.Skipped = append(summary.Skipped, fmt.Sprintf("%s: %s (no files found)", label, prefix))
	} else {
		summary.PrefixesDeleted = append(summary.PrefixesDeleted, prefix)
	}
}

// NormalizeKey extracts a B2 object key from any of the URL/path shapes
// the codebase passes around. Returns an empty string when the input is
// empty, "/", or otherwise cannot be mapped to a key.
//
// Handles:
//   - https://cdn.filmorauz.net/file/<bucket>/<key>
//   - https://f000.backblazeb2.com/file/<bucket>/<key>
//   - https://<any-host>/<anything>/<bucket>/<key>           (bucketName match)
//   - https://<host>/media/<key>                             (website media proxy)
//   - /file/<bucket>/<key>
//   - /media/<key>
//   - /<key>
//   - <bare key>
func (s *B2CleanupService) NormalizeKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/" {
		return ""
	}
	// Full URL — parse and walk the path.
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Path == "" {
			return ""
		}
		return s.keyFromPath(u.Path)
	}
	// Path or bare key.
	return s.keyFromPath(raw)
}

func (s *B2CleanupService) keyFromPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	p = strings.TrimPrefix(p, "/")
	// /media/<key> — website's local proxy path.
	if strings.HasPrefix(p, "media/") {
		return strings.TrimPrefix(p, "media/")
	}
	// /file/<bucket>/<key> — standard B2 download URL pattern.
	if strings.HasPrefix(p, "file/") {
		rest := strings.TrimPrefix(p, "file/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return ""
	}
	// CDN host serves the bucket somewhere mid-path.
	if s.bucketName != "" {
		needle := "/" + s.bucketName + "/"
		if idx := strings.Index("/"+p, needle); idx != -1 {
			return ("/" + p)[idx+len(needle):]
		}
	}
	// Otherwise treat the path as the key directly.
	return p
}

// ExtractKeyFromURL is retained for backward compatibility with existing
// callers (movie_service, series_service derivation helpers). New code
// should call NormalizeKey, which also handles /media/ paths.
func (s *B2CleanupService) ExtractKeyFromURL(raw string) string {
	return s.NormalizeKey(raw)
}

// DeriveVideoFolderPrefix extracts the `videos/<folder>/` prefix that holds
// master.m3u8 and quality subfolders, given any URL inside that folder.
// Returns empty when the URL does not map to a videos/* key.
func (s *B2CleanupService) DeriveVideoFolderPrefix(videoURL string) string {
	key := s.NormalizeKey(videoURL)
	if key == "" {
		return ""
	}
	if !strings.HasPrefix(key, "videos/") {
		return ""
	}
	parts := strings.Split(key, "/")
	if len(parts) < 3 {
		return ""
	}
	if parts[1] == "movies" || parts[1] == "serials" || parts[1] == "series" {
		if len(parts) < 4 {
			return ""
		}
		return strings.Join(parts[:3], "/") + "/"
	}
	return strings.Join(parts[:2], "/") + "/"
}

// IsSafeB2DeletePrefix reports whether prefix is under one of the
// allowlist roots and extends at least one character past that root.
//
// Exposed for tests and for service-layer pre-checks.
func IsSafeB2DeletePrefix(prefix string) bool {
	prefix = normalizeB2DeletePrefix(prefix)
	if prefix == "" {
		return false
	}
	if isUnsafeB2DeletePrefix(prefix) {
		return false
	}
	for _, root := range allowedB2DeleteRoots {
		if strings.HasPrefix(prefix, root) && len(prefix) > len(root) {
			return true
		}
	}
	return false
}

// IsSafeB2DeleteKey reports whether key is a single-file delete target
// that lives under one of the allowlist roots.
func IsSafeB2DeleteKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return false
	}
	// Must not be a bare directory listing (no filename).
	if strings.HasSuffix(key, "/") {
		return false
	}
	if _, blocked := broadB2DeletePrefixes[key]; blocked {
		return false
	}
	for _, root := range allowedB2DeleteRoots {
		if strings.HasPrefix(key, root) && len(key) > len(root) {
			return true
		}
	}
	return false
}

func normalizeB2DeletePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix == "" {
		return "/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func isUnsafeB2DeletePrefix(prefix string) bool {
	prefix = normalizeB2DeletePrefix(prefix)
	if _, blocked := broadB2DeletePrefixes[prefix]; blocked {
		return true
	}
	return false
}

// Package services — B2 cleanup helpers.
//
// B2CleanupService authenticates against Backblaze B2 and deletes files
// (by exact key or by prefix walk). Used by movie deletion to remove HLS
// folders, posters, backdrops, and clip assets that would otherwise
// orphan in the bucket after the DB record is removed.
//
// All delete operations are best-effort: a missing file is logged and
// skipped, a transient error is logged and returned per-item, and the
// overall caller should not abort the flow on a single failure.
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

var broadB2DeletePrefixes = map[string]struct{}{
	"":               {},
	"/":              {},
	"videos":         {},
	"videos/":        {},
	"videos/movies":  {},
	"videos/movies/": {},
	"movies":         {},
	"movies/":        {},
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
func (s *B2CleanupService) DeleteByKey(key string) (skipped bool, err error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return true, nil
	}
	// list-by-prefix with the exact key matches all versions (if any).
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

// ExtractKeyFromURL parses a CDN / B2 URL and returns the object key (the
// path after the bucket prefix). Returns an empty string when the input is
// empty, not a URL, or cannot be mapped to a key.
//
// Handles the following shapes:
//   - https://cdn.filmorauz.net/file/<bucket>/videos/<folder>/master.m3u8
//   - https://f000.backblazeb2.com/file/<bucket>/images/movie/posters/123.jpg
//   - https://<any>/<anything>/<bucket>/<key…>           (via bucketName match)
//   - <bare key>                                           (already a key)
func (s *B2CleanupService) ExtractKeyFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Bare key (no scheme) — return as-is.
	if !strings.Contains(raw, "://") {
		return strings.TrimPrefix(raw, "/")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" {
		return ""
	}
	p := strings.TrimPrefix(u.Path, "/")
	// Standard B2 pattern: file/<bucket>/<key>
	if strings.HasPrefix(p, "file/") {
		rest := strings.TrimPrefix(p, "file/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return ""
	}
	// Bucket name appears somewhere in path.
	if s.bucketName != "" {
		needle := "/" + s.bucketName + "/"
		if idx := strings.Index("/"+p, needle); idx != -1 {
			return ("/" + p)[idx+len(needle):]
		}
	}
	// CDN that serves the bucket at root (e.g. cdn.example.com/images/…).
	// We can't distinguish assets from garbage here; accept the path as the key.
	return p
}

// DeriveVideoFolderPrefix extracts the `videos/<folder>/` prefix that holds
// master.m3u8 and quality subfolders, given any URL inside that folder.
// Returns empty when the URL does not map to a videos/* key.
func (s *B2CleanupService) DeriveVideoFolderPrefix(videoURL string) string {
	key := s.ExtractKeyFromURL(videoURL)
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
	if parts[1] == "movies" || parts[1] == "serials" {
		if len(parts) < 4 {
			return ""
		}
		return strings.Join(parts[:3], "/") + "/"
	}
	return strings.Join(parts[:2], "/") + "/"
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

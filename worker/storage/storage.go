package storage

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CacheTTLConfig defines cache TTL values for different file types
type CacheTTLConfig struct {
	M3U8TTL  time.Duration // playlist TTL (low for updates)
	TSTTL    time.Duration // segment TTL (long for caching)
	ImageTTL time.Duration // image TTL (very long)
	OtherTTL time.Duration // default TTL
}

// DefaultCacheTTL returns default cache configuration
func DefaultCacheTTL() CacheTTLConfig {
	return CacheTTLConfig{
		M3U8TTL:  10 * time.Second,
		TSTTL:    24 * time.Hour,
		ImageTTL: 7 * 24 * time.Hour,
		OtherTTL: 1 * time.Hour,
	}
}

// getCacheHeaders returns appropriate Cache-Control header based on file extension
func getCacheHeaders(filename string, config CacheTTLConfig) string {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".m3u8", ".M3U8":
		// Playlists: short TTL to allow quick updates
		return fmt.Sprintf("public, max-age=%.0f", config.M3U8TTL.Seconds())
	case ".ts", ".TS":
		// Segments: long TTL, immutable (won't change)
		return fmt.Sprintf("public, max-age=%.0f, immutable", config.TSTTL.Seconds())
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		// Images: very long TTL
		return fmt.Sprintf("public, max-age=%.0f", config.ImageTTL.Seconds())
	default:
		// Default: moderate TTL
		return fmt.Sprintf("public, max-age=%.0f", config.OtherTTL.Seconds())
	}
}

func encodeB2InfoValue(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func sha1Hex(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}

// Storage interface defines methods for file storage
type Storage interface {
	Upload(localPath, remotePath string) (string, error)
	UploadData(filename string, data []byte, contentType string) (string, error)
	Download(remotePath, localPath string) error
	Delete(remotePath string) error
	GetURL(remotePath string) string
	GetFileSize(remotePath string) (int64, error)
	GetPresignedUploadURL(remotePath, contentType string) (string, string, error)
	ParallelUpload(jobs []struct {
		LocalPath  string
		RemotePath string
		Data       []byte
	}) ([]parallelUploadResult, error)
}

// Config holds storage configuration
type Config struct {
	Mode       string // "dev" or "prod"
	LocalPath  string // Local storage base path
	B2Bucket   string // Backblaze B2 bucket name
	B2Endpoint string // Backblaze endpoint
	B2KeyID    string // Backblaze key ID
	B2AppKey   string // Backblaze application key
	CDNBaseURL string // CDN base URL for B2
	BaseURL    string // Base URL for development mode (e.g., http://localhost:8080)
}

// NewStorage creates a new storage instance based on config
func NewStorage(config Config) (Storage, error) {
	switch config.Mode {
	case "prod":
		return NewB2Storage(config)
	case "dev":
		return NewLocalStorage(config.LocalPath)
	default:
		return NewLocalStorage("./uploads")
	}
}

// LocalStorage implements local file storage
type LocalStorage struct {
	basePath string
}

// NewLocalStorage creates a new local storage instance
func NewLocalStorage(basePath string) (*LocalStorage, error) {
	// Create base path if not exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create local storage path: %w", err)
	}

	return &LocalStorage{
		basePath: basePath,
	}, nil
}

// LocalStorageUploadURL returns the full URL for an uploaded file in development mode
// This should be set from pipeline config (BASE_URL)
var LocalStorageUploadURL string = "http://localhost:8080"

// SetUploadURL sets the base URL for local storage uploads
func SetUploadURL(url string) {
	LocalStorageUploadURL = url
}

// Upload copies a local file to storage
func (s *LocalStorage) Upload(localPath, remotePath string) (string, error) {
	// Read source file
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Create destination path
	destPath := filepath.Join(s.basePath, remotePath)
	destDir := filepath.Dir(destPath)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Return FULL URL with base URL prefix for development mode
	// This ensures database stores complete URLs, not relative paths
	fullURL := LocalStorageUploadURL + "/uploads/" + remotePath
	return fullURL, nil
}

// UploadData uploads raw data to storage
// NOTE: This is used for direct data upload
// Returns FULL URL with base URL prefix for development mode
func (s *LocalStorage) UploadData(filename string, data []byte, contentType string) (string, error) {
	// Create destination path
	destPath := filepath.Join(s.basePath, filename)
	destDir := filepath.Dir(destPath)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Return FULL URL with base URL prefix for development mode
	// This ensures database stores complete URLs, not relative paths
	fullURL := LocalStorageUploadURL + "/uploads/" + filename
	return fullURL, nil
}

// Download is not typically used for local storage
func (s *LocalStorage) Download(remotePath, localPath string) error {
	srcPath := filepath.Join(s.basePath, remotePath)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Create local path
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}

	return os.WriteFile(localPath, data, 0644)
}

// Delete removes a file from storage
func (s *LocalStorage) Delete(remotePath string) error {
	filePath := filepath.Join(s.basePath, remotePath)
	return os.Remove(filePath)
}

// GetURL returns the URL for a file
func (s *LocalStorage) GetURL(remotePath string) string {
	return fmt.Sprintf("/uploads/%s", remotePath)
}

// GetFileSize returns the size of a file
func (s *LocalStorage) GetFileSize(remotePath string) (int64, error) {
	filePath := filepath.Join(s.basePath, remotePath)
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// GetPresignedUploadURL returns URL and auth token for direct upload
// For local storage, returns the upload endpoint URL
func (s *LocalStorage) GetPresignedUploadURL(remotePath, contentType string) (string, string, error) {
	uploadURL := LocalStorageUploadURL + "/uploads/" + remotePath
	return uploadURL, "", nil
}

func (s *LocalStorage) ParallelUpload(jobs []struct {
	LocalPath  string
	RemotePath string
	Data       []byte
}) ([]parallelUploadResult, error) {
	startTime := time.Now()
	totalJobs := len(jobs)
	totalData := 0

	log.Printf("[LOCAL] Starting parallel upload: %d files", totalJobs)

	for _, j := range jobs {
		destPath := filepath.Join(s.basePath, j.RemotePath)
		destDir := filepath.Dir(destPath)

		if err := os.MkdirAll(destDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}

		if err := os.WriteFile(destPath, j.Data, 0644); err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
		totalData += len(j.Data)
	}

	results := make([]parallelUploadResult, totalJobs)
	for i, j := range jobs {
		results[i] = parallelUploadResult{
			RemotePath: j.RemotePath,
			URL:        LocalStorageUploadURL + "/uploads/" + j.RemotePath,
			Err:        nil,
		}
	}

	elapsed := time.Since(startTime)
	speed := float64(totalData) / 1024 / 1024 / elapsed.Seconds()

	log.Printf("[LOCAL] Parallel upload complete: %d files, %.2f MB in %.2fs (%.2f MB/s)",
		totalJobs, float64(totalData)/1024/1024, elapsed.Seconds(), speed)

	return results, nil
}

// B2Storage implements Backblaze B2 cloud storage using the native B2 API.
type B2Storage struct {
	bucket string
	keyID  string
	appKey string
	cdnURL string

	// Cache configuration for CDN headers
	cacheTTL CacheTTLConfig

	mu          sync.Mutex
	httpClient  *http.Client
	authToken   string
	apiURL      string
	downloadURL string
	bucketID    string
	authExpiry  time.Time
}

// b2AuthResponse is the response from b2_authorize_account
type b2AuthResponse struct {
	AccountID          string `json:"accountId"`
	AuthorizationToken string `json:"authorizationToken"`
	APIURL             string `json:"apiUrl"`
	DownloadURL        string `json:"downloadUrl"`
	Allowed            struct {
		BucketID   string `json:"bucketId"`
		BucketName string `json:"bucketName"`
	} `json:"allowed"`
}

// b2UploadURLResponse is the response from b2_get_upload_url
type b2UploadURLResponse struct {
	BucketID           string `json:"bucketId"`
	UploadURL          string `json:"uploadUrl"`
	AuthorizationToken string `json:"authorizationToken"`
}

type uploadJob struct {
	localPath   string
	remotePath  string
	data        []byte
	contentType string
	result      chan string
	err         chan error
}

var (
	DefaultPoolSize  = 15
	DefaultBatchSize = 20
	MaxRetries       = 3
	RetryDelayMs     = 500
)

// NewB2Storage creates a new B2 storage instance
func NewB2Storage(config Config) (*B2Storage, error) {
	if config.B2KeyID == "" || config.B2AppKey == "" {
		return nil, fmt.Errorf("B2_KEY_ID and B2_APP_KEY are required for production storage")
	}
	if config.B2Bucket == "" {
		return nil, fmt.Errorf("B2_BUCKET is required for production storage")
	}
	return &B2Storage{
		bucket:     config.B2Bucket,
		keyID:      config.B2KeyID,
		appKey:     config.B2AppKey,
		cdnURL:     config.CDNBaseURL,
		cacheTTL:   DefaultCacheTTL(),
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}, nil
}

// authorize calls b2_authorize_account and caches the result for 23 hours.
func (s *B2Storage) authorize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.authToken != "" && time.Now().Before(s.authExpiry) {
		return nil
	}

	req, err := http.NewRequest("GET", "https://api.backblazeb2.com/b2api/v2/b2_authorize_account", nil)
	if err != nil {
		return fmt.Errorf("b2 authorize: build request: %w", err)
	}
	creds := base64.StdEncoding.EncodeToString([]byte(s.keyID + ":" + s.appKey))
	req.Header.Set("Authorization", "Basic "+creds)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("b2 authorize: request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("b2 authorize: status %d: %s", resp.StatusCode, body)
	}

	var auth b2AuthResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return fmt.Errorf("b2 authorize: decode: %w", err)
	}

	s.authToken = auth.AuthorizationToken
	s.apiURL = auth.APIURL
	s.downloadURL = auth.DownloadURL
	s.bucketID = auth.Allowed.BucketID
	s.authExpiry = time.Now().Add(23 * time.Hour)

	log.Printf("[B2] Authorized. apiURL=%s bucketID=%s", s.apiURL, s.bucketID)
	return nil
}

// getUploadURL calls b2_get_upload_url to obtain a fresh single-use upload URL.
// B2 upload URLs cannot be safely reused across concurrent goroutines (B2 rejects
// concurrent POSTs to the same upload URL), so we fetch a fresh one per call.
func (s *B2Storage) getUploadURL() (uploadURL, token string, err error) {
	s.mu.Lock()
	apiURL := s.apiURL
	authToken := s.authToken
	bucketID := s.bucketID
	s.mu.Unlock()

	payload, _ := json.Marshal(map[string]string{"bucketId": bucketID})
	req, err := http.NewRequest("POST", apiURL+"/b2api/v2/b2_get_upload_url", bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("b2 getUploadURL: build request: %w", err)
	}
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("b2 getUploadURL: request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("b2 getUploadURL: status %d: %s", resp.StatusCode, body)
	}

	var ur b2UploadURLResponse
	if err := json.Unmarshal(body, &ur); err != nil {
		return "", "", fmt.Errorf("b2 getUploadURL: decode: %w", err)
	}

	return ur.UploadURL, ur.AuthorizationToken, nil
}

// uploadBytes sends data bytes to B2 at remotePath.
func (s *B2Storage) uploadBytes(data []byte, remotePath, contentType string) (string, error) {
	// Get cache headers based on file type
	cacheControl := getCacheHeaders(remotePath, s.cacheTTL)

	log.Printf("[B2] Upload attempt: bucket=%s, key=%s, size=%d, contentType=%s", s.bucket, remotePath, len(data), contentType)
	log.Printf("[CDN] Cache-Control for %s: %s", filepath.Ext(remotePath), cacheControl)

	if err := s.authorize(); err != nil {
		log.Printf("[B2] Authorization failed: %v", err)
		return "", fmt.Errorf("b2 authorize: %w", err)
	}
	uploadURL, uploadToken, err := s.getUploadURL()
	if err != nil {
		log.Printf("[B2] Get upload URL failed: %v", err)
		return "", fmt.Errorf("b2 getUploadURL: %w", err)
	}

	sha1sum := sha1Hex(data)

	encodedCacheControl := encodeB2InfoValue(cacheControl)

	// B2 handles folder creation via path prefix automatically.
	// X-Bz-Info-* values must be URL-encoded; otherwise commas in Cache-Control
	// are rejected as bad percent-encoded strings by B2.
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("b2 upload: build request: %w", err)
	}
	req.Header.Set("Authorization", uploadToken)
	req.Header.Set("X-Bz-File-Name", remotePath) // raw path - B2 creates folders via prefix
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	req.Header.Set("X-Bz-Info-cache_control", encodedCacheControl) // Set CDN cache headers
	req.Header.Set("X-Bz-Content-Sha1", sha1sum)
	req.ContentLength = int64(len(data))

	log.Printf("[B2] Upload request target: url=%s", uploadURL)
	log.Printf("[B2] Upload request headers: X-Bz-File-Name=%q X-Bz-Content-Sha1=sha1:%s X-Bz-Info-cache_control=%q raw_cache_control=%q", remotePath, sha1sum, encodedCacheControl, cacheControl)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("[B2] Upload request failed: %v", err)
		return "", fmt.Errorf("b2 upload: request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("b2 upload %s: status %d: %s", remotePath, resp.StatusCode, body)
		log.Printf("[B2] Upload failed: key=%s, status=%d, response_body=%s", remotePath, resp.StatusCode, body)
		return "", fmt.Errorf("%s", errMsg)
	}

	log.Printf("[B2] Upload success: key=%s", remotePath)

	var fileURL string
	if s.cdnURL != "" {
		// Use CDN URL for faster delivery
		fileURL = s.cdnURL + "/file/filmorauznet/" + remotePath
		log.Printf("[CDN] Using CDN URL: %s", fileURL)
	} else {
		s.mu.Lock()
		dlURL := s.downloadURL
		bucket := s.bucket
		s.mu.Unlock()
		fileURL = dlURL + "/file/" + bucket + "/" + remotePath
		log.Printf("[CDN] Using direct B2 URL (no CDN configured): %s", fileURL)
	}
	log.Printf("[B2] Returning URL: %s", fileURL)
	return fileURL, nil
}

// Upload reads localPath and uploads it to B2 at remotePath.
func (s *B2Storage) Upload(localPath, remotePath string) (string, error) {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("b2 Upload: read file: %w", err)
	}
	contentType := detectContentType(remotePath)
	return s.uploadBytes(data, remotePath, contentType)
}

// UploadData uploads raw bytes to B2.
func (s *B2Storage) UploadData(filename string, data []byte, contentType string) (string, error) {
	if contentType == "" {
		contentType = detectContentType(filename)
	}
	return s.uploadBytes(data, filename, contentType)
}

type parallelUploadResult struct {
	RemotePath string
	URL        string
	Err        error
}

func (s *B2Storage) getUploadURLBatch(count int) ([]uploadURL, []string, error) {
	if err := s.authorize(); err != nil {
		return nil, nil, err
	}

	s.mu.Lock()
	apiURL := s.apiURL
	authToken := s.authToken
	bucketID := s.bucketID
	s.mu.Unlock()

	var urls []uploadURL
	var tokens []string

	for i := 0; i < count; i++ {
		payload, _ := json.Marshal(map[string]string{"bucketId": bucketID})
		req, err := http.NewRequest("POST", apiURL+"/b2api/v2/b2_get_upload_url", bytes.NewReader(payload))
		if err != nil {
			return nil, nil, fmt.Errorf("b2 getUploadURL batch: build request: %w", err)
		}
		req.Header.Set("Authorization", authToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("b2 getUploadURL batch: request: %w", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("b2 getUploadURL batch: status %d: %s", resp.StatusCode, body)
		}

		var ur b2UploadURLResponse
		if err := json.Unmarshal(body, &ur); err != nil {
			return nil, nil, fmt.Errorf("b2 getUploadURL batch: decode: %w", err)
		}

		urls = append(urls, uploadURL{url: ur.UploadURL, token: ur.AuthorizationToken})
		tokens = append(tokens, ur.AuthorizationToken)
	}

	return urls, tokens, nil
}

type uploadURL struct {
	url   string
	token string
}

func (s *B2Storage) uploadWithUrl(data []byte, uploadUrl, token, remotePath, contentType string) (string, error) {
	// Get cache headers based on file type
	cacheControl := getCacheHeaders(remotePath, s.cacheTTL)

	req, err := http.NewRequest("POST", uploadUrl, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("b2 upload: new request: %w", err)
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Bz-File-Name", remotePath)
	sha1sum := sha1Hex(data)
	req.Header.Set("X-Bz-Content-Sha1", sha1sum)
	encodedContentType := encodeB2InfoValue(contentType)
	req.Header.Set("X-Bz-Info-Content-Type", encodedContentType)
	encodedCacheControl := encodeB2InfoValue(cacheControl)
	req.Header.Set("X-Bz-Info-cache_control", encodedCacheControl) // CDN cache headers

	log.Printf("[B2] Parallel upload request target: url=%s", uploadUrl)
	log.Printf("[B2] Parallel upload request headers: X-Bz-File-Name=%q X-Bz-Content-Sha1=sha1:%s X-Bz-Info-Content-Type=%q X-Bz-Info-cache_control=%q raw_cache_control=%q", remotePath, sha1sum, encodedContentType, encodedCacheControl, cacheControl)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("b2 upload: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("b2 upload: status %d: %s", resp.StatusCode, body)
	}

	// Return full CDN URL
	var fileURL string
	if s.cdnURL != "" {
		fileURL = s.cdnURL + "/file/filmorauznet/" + remotePath
	} else {
		fileURL = remotePath
	}

	return fileURL, nil
}

func (s *B2Storage) ParallelUpload(jobs []struct {
	LocalPath  string
	RemotePath string
	Data       []byte
}) ([]parallelUploadResult, error) {
	startTime := time.Now()
	totalJobs := len(jobs)
	totalData := 0
	for _, j := range jobs {
		totalData += len(j.Data)
	}

	if totalJobs == 0 {
		return []parallelUploadResult{}, nil
	}

	log.Printf("[B2] Starting parallel upload: %d files, %.2f MB", totalJobs, float64(totalData)/1024/1024)

	batchSize := DefaultBatchSize
	results := make([]parallelUploadResult, totalJobs)

	var wg sync.WaitGroup
	jobChan := make(chan int, totalJobs)

	poolSize := DefaultPoolSize
	if poolSize > totalJobs {
		poolSize = totalJobs
	}

	uploadUrlChan := make(chan uploadURL, batchSize*2)
	urlFetcherDone := make(chan struct{})
	stopFetch := make(chan struct{}) // signals URL fetcher to stop

	// URL fetcher: fetch upload URLs in bounded batches until we have enough or are told to stop
	go func() {
		defer close(urlFetcherDone)
		totalFetched := 0
		for totalFetched < totalJobs+batchSize {
			// Check if we should stop early (workers finished)
			select {
			case <-stopFetch:
				log.Printf("[B2] URL fetcher stopped early: %d URLs fetched for %d jobs", totalFetched, totalJobs)
				return
			default:
			}

			urls, _, err := s.getUploadURLBatch(batchSize)
			if err != nil {
				log.Printf("[B2] ERROR: Failed to get upload URL batch: %v", err)
				return
			}
			for _, u := range urls {
				select {
				case uploadUrlChan <- u:
					totalFetched++
				case <-stopFetch:
					log.Printf("[B2] URL fetcher stopped during send: %d URLs fetched", totalFetched)
					return
				case <-time.After(5 * time.Second):
					// Channel full - workers may be slow; stop fetching
					log.Printf("[B2] Upload URL channel full after %d URLs, stopping fetch", totalFetched)
					return
				}
			}
		}
		log.Printf("[B2] URL fetcher done: fetched %d URLs for %d jobs", totalFetched, totalJobs)
	}()

	// Worker pool
	for i := 0; i < poolSize; i++ {
		wg.Add(1)
		go func(workerId int) {
			defer wg.Done()
			for idx := range jobChan {
				job := jobs[idx]
				contentType := detectContentType(job.RemotePath)

				var url string
				var err error
				maxRetries := MaxRetries

				for attempt := 0; attempt < maxRetries; attempt++ {
					select {
					case uploadUrl, ok := <-uploadUrlChan:
						if !ok {
							err = fmt.Errorf("upload URL channel closed")
							break
						}
						url, err = s.uploadWithUrl(job.Data, uploadUrl.url, uploadUrl.token, job.RemotePath, contentType)
					case <-time.After(30 * time.Second):
						err = fmt.Errorf("timeout waiting for upload URL")
						break
					}

					if err == nil {
						break
					}

					if attempt < maxRetries-1 {
						log.Printf("[B2] Retry %d/%d for %s: %v", attempt+1, maxRetries, job.RemotePath, err)
						time.Sleep(time.Duration(RetryDelayMs) * time.Millisecond)
					}
				}

				results[idx] = parallelUploadResult{
					RemotePath: job.RemotePath,
					URL:        url,
					Err:        err,
				}
			}
		}(i)
	}

	// Distribute jobs
	for i := 0; i < totalJobs; i++ {
		jobChan <- i
	}
	close(jobChan)
	wg.Wait() // all workers finished

	// Signal URL fetcher to stop and wait for it
	close(stopFetch)
	<-urlFetcherDone

	elapsed := time.Since(startTime)
	speed := float64(totalData) / 1024 / 1024 / elapsed.Seconds()

	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Err != nil {
			failCount++
			log.Printf("[B2] FAILED: %s - %v", r.RemotePath, r.Err)
		} else {
			successCount++
		}
	}

	log.Printf("[B2] Parallel upload complete: %d/%d succeeded, %.2f MB in %.2fs (%.2f MB/s)",
		successCount, totalJobs, float64(totalData)/1024/1024, elapsed.Seconds(), speed)

	return results, nil
}

// Download downloads a file from B2 to localPath.
func (s *B2Storage) Download(remotePath, localPath string) error {
	if err := s.authorize(); err != nil {
		return err
	}

	s.mu.Lock()
	dlURL := s.downloadURL
	bucket := s.bucket
	authToken := s.authToken
	s.mu.Unlock()

	fileURL := dlURL + "/file/" + bucket + "/" + remotePath
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		return fmt.Errorf("b2 Download: build request: %w", err)
	}
	req.Header.Set("Authorization", authToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("b2 Download: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("b2 Download %s: status %d: %s", remotePath, resp.StatusCode, body)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("b2 Download: mkdir: %w", err)
	}
	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("b2 Download: create file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// Delete deletes a file from B2 (lists versions then hides/deletes each).
func (s *B2Storage) Delete(remotePath string) error {
	// B2 delete is a two-step: list file versions then delete each.
	// For simplicity, hide the file (adds a delete marker).
	if err := s.authorize(); err != nil {
		return err
	}

	s.mu.Lock()
	apiURL := s.apiURL
	authToken := s.authToken
	bucketID := s.bucketID
	s.mu.Unlock()

	payload, _ := json.Marshal(map[string]string{
		"bucketId":      bucketID,
		"startFileName": remotePath,
		"maxFileCount":  "1",
	})
	req, err := http.NewRequest("POST", apiURL+"/b2api/v2/b2_list_file_names", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("b2 Delete list: %w", err)
	}
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("b2 Delete list request: %w", err)
	}
	defer resp.Body.Close()

	var listResp struct {
		Files []struct {
			FileID   string `json:"fileId"`
			FileName string `json:"fileName"`
		} `json:"files"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &listResp); err != nil || len(listResp.Files) == 0 {
		return nil // File not found — treat as success
	}

	for _, f := range listResp.Files {
		if f.FileName != remotePath {
			continue
		}
		delPayload, _ := json.Marshal(map[string]string{"fileId": f.FileID, "fileName": f.FileName})
		delReq, _ := http.NewRequest("POST", apiURL+"/b2api/v2/b2_delete_file_version", bytes.NewReader(delPayload))
		delReq.Header.Set("Authorization", authToken)
		delReq.Header.Set("Content-Type", "application/json")
		delResp, err := s.httpClient.Do(delReq)
		if err != nil {
			return fmt.Errorf("b2 Delete: %w", err)
		}
		delResp.Body.Close()
	}
	return nil
}

// GetURL returns the CDN (or B2 download) URL for a remote path.
func (s *B2Storage) GetURL(remotePath string) string {
	if s.cdnURL != "" {
		return s.cdnURL + "/file/filmorauznet/" + remotePath
	}
	s.mu.Lock()
	dlURL := s.downloadURL
	bucket := s.bucket
	s.mu.Unlock()
	if dlURL != "" {
		return dlURL + "/file/" + bucket + "/" + remotePath
	}
	return remotePath
}

// GetFileSize returns the size of a file stored in B2.
func (s *B2Storage) GetFileSize(remotePath string) (int64, error) {
	if err := s.authorize(); err != nil {
		return 0, err
	}

	s.mu.Lock()
	apiURL := s.apiURL
	authToken := s.authToken
	bucketID := s.bucketID
	s.mu.Unlock()

	payload, _ := json.Marshal(map[string]interface{}{
		"bucketId":      bucketID,
		"startFileName": remotePath,
		"maxFileCount":  1,
	})
	req, _ := http.NewRequest("POST", apiURL+"/b2api/v2/b2_list_file_names", bytes.NewReader(payload))
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("b2 GetFileSize: %w", err)
	}
	defer resp.Body.Close()

	var listResp struct {
		Files []struct {
			FileName      string `json:"fileName"`
			ContentLength int64  `json:"contentLength"`
		} `json:"files"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &listResp); err != nil {
		return 0, fmt.Errorf("b2 GetFileSize decode: %w", err)
	}
	for _, f := range listResp.Files {
		if f.FileName == remotePath {
			return f.ContentLength, nil
		}
	}
	return 0, fmt.Errorf("b2 GetFileSize: file not found: %s", remotePath)
}

// GetPresignedUploadURL returns URL and auth token for direct upload to B2
func (s *B2Storage) GetPresignedUploadURL(remotePath, contentType string) (string, string, error) {
	if err := s.authorize(); err != nil {
		return "", "", fmt.Errorf("b2 authorize: %w", err)
	}
	uploadURL, uploadToken, err := s.getUploadURL()
	if err != nil {
		return "", "", fmt.Errorf("b2 getUploadURL: %w", err)
	}
	return uploadURL, uploadToken, nil
}

// detectContentType returns a MIME type based on file extension.
func detectContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".m3u8"):
		return "application/x-mpegURL"
	case strings.HasSuffix(name, ".ts"):
		return "video/MP2T"
	case strings.HasSuffix(name, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	case strings.HasSuffix(name, ".vtt"):
		return "text/vtt"
	default:
		return "application/octet-stream"
	}
}

// Ensure ProcessingResult implements what we need
var _ Storage = (*LocalStorage)(nil)
var _ Storage = (*B2Storage)(nil)

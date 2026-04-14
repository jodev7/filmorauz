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

// Storage interface defines methods for file storage
type Storage interface {
	Upload(localPath, remotePath string) (string, error)
	UploadData(filename string, data []byte, contentType string) (string, error)
	Download(remotePath, localPath string) error
	Delete(remotePath string) error
	GetURL(remotePath string) string
	GetFileSize(remotePath string) (int64, error)
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

// B2Storage implements Backblaze B2 cloud storage using the native B2 API.
type B2Storage struct {
	bucket string
	keyID  string
	appKey string
	cdnURL string

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

// getUploadURL calls b2_get_upload_url to obtain a single-use upload URL.
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
	if err := s.authorize(); err != nil {
		return "", err
	}
	uploadURL, uploadToken, err := s.getUploadURL()
	if err != nil {
		return "", err
	}

	// Compute SHA1 of file content
	h := sha1.New()
	h.Write(data)
	sha1sum := hex.EncodeToString(h.Sum(nil))

	encodedPath := url.PathEscape(remotePath)
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("b2 upload: build request: %w", err)
	}
	req.Header.Set("Authorization", uploadToken)
	req.Header.Set("X-Bz-File-Name", encodedPath)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	req.Header.Set("X-Bz-Content-Sha1", sha1sum)
	req.ContentLength = int64(len(data))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("b2 upload: request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("b2 upload %s: status %d: %s", remotePath, resp.StatusCode, body)
	}

	var fileURL string
	if s.cdnURL != "" {
		fileURL = s.cdnURL + "/file/filmorauznet/" + remotePath
	} else {
		s.mu.Lock()
		dlURL := s.downloadURL
		bucket := s.bucket
		s.mu.Unlock()
		fileURL = dlURL + "/file/" + bucket + "/" + remotePath
	}
	log.Printf("[B2] Uploaded %s -> %s", remotePath, fileURL)
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
	default:
		return "application/octet-stream"
	}
}

// Ensure ProcessingResult implements what we need
var _ Storage = (*LocalStorage)(nil)
var _ Storage = (*B2Storage)(nil)

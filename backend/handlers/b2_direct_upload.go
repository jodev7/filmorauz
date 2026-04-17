package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxDirectImageUploadSize int64 = 20 * 1024 * 1024
	maxDirectVideoUploadSize int64 = 5 * 1024 * 1024 * 1024
)

type b2AuthorizeResponse struct {
	AccountID          string `json:"accountId"`
	AuthorizationToken string `json:"authorizationToken"`
	APIURL             string `json:"apiUrl"`
	Allowed            struct {
		BucketID   string `json:"bucketId"`
		BucketName string `json:"bucketName"`
	} `json:"allowed"`
}

type b2ListBucketsResponse struct {
	Buckets []struct {
		BucketID   string `json:"bucketId"`
		BucketName string `json:"bucketName"`
	} `json:"buckets"`
}

type b2UploadURLResponse struct {
	UploadURL          string `json:"uploadUrl"`
	AuthorizationToken string `json:"authorizationToken"`
}

type b2UploadCompleteRequest struct {
	FileKey     string `json:"fileKey"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"`
	Type        string `json:"type"`
	ContentType string `json:"contentType"`
}

func directUploadRules(fileType string) (prefix string, maxSize int64, allowedTypes map[string]bool, fallbackExt string, ok bool) {
	switch fileType {
	case "poster":
		return "posters", maxDirectImageUploadSize, allowedImageTypes, ".jpg", true
	case "backdrop":
		return "backdrops", maxDirectImageUploadSize, allowedImageTypes, ".jpg", true
	case "video":
		return "temp/videos", maxDirectVideoUploadSize, map[string]bool{
			"video/mp4":       true,
			"video/webm":      true,
			"video/ogg":       true,
			"video/quicktime": true,
		}, ".mp4", true
	default:
		return "", 0, nil, "", false
	}
}

func directUploadExtensionAllowed(fileType, filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch fileType {
	case "poster", "backdrop":
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp":
			return true
		}
	case "video":
		switch ext {
		case ".mp4", ".webm", ".ogg", ".mov":
			return true
		}
	}
	return false
}

func validateDirectUploadType(fileType, filename, contentType string, allowedTypes map[string]bool) error {
	if contentType != "" {
		if allowedTypes[contentType] {
			return nil
		}
		return fmt.Errorf("invalid file type")
	}
	if directUploadExtensionAllowed(fileType, filename) {
		return nil
	}
	return fmt.Errorf("invalid file extension")
}

func directUploadFileKey(fileType, originalFilename, contentType string) (string, error) {
	prefix, _, allowedTypes, fallbackExt, ok := directUploadRules(fileType)
	if !ok {
		return "", fmt.Errorf("invalid type: must be video, poster, or backdrop")
	}
	if err := validateDirectUploadType(fileType, originalFilename, contentType, allowedTypes); err != nil {
		return "", err
	}

	label := fileType
	ext := safeUploadExt(originalFilename, contentType, fallbackExt)
	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), label, ext)
	return strings.Trim(prefix, "/") + "/" + filename, nil
}

func (h *UploadHandler) authorizeB2() (*b2AuthorizeResponse, error) {
	if h.config.B2KeyID == "" || h.config.B2AppKey == "" {
		return nil, fmt.Errorf("B2_KEY_ID and B2_APP_KEY are required")
	}

	req, err := http.NewRequest("GET", "https://api.backblazeb2.com/b2api/v2/b2_authorize_account", nil)
	if err != nil {
		return nil, err
	}
	creds := base64.StdEncoding.EncodeToString([]byte(h.config.B2KeyID + ":" + h.config.B2AppKey))
	req.Header.Set("Authorization", "Basic "+creds)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("b2_authorize_account status=%d body=%s", resp.StatusCode, string(body))
	}

	var result b2AuthorizeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (h *UploadHandler) resolveB2BucketID(auth *b2AuthorizeResponse) (string, error) {
	if auth.Allowed.BucketID != "" {
		return auth.Allowed.BucketID, nil
	}
	if h.config.B2Bucket == "" {
		return "", fmt.Errorf("B2_BUCKET is required")
	}

	payload, _ := json.Marshal(map[string]string{
		"accountId":  auth.AccountID,
		"bucketName": h.config.B2Bucket,
	})
	req, err := http.NewRequest("POST", auth.APIURL+"/b2api/v2/b2_list_buckets", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", auth.AuthorizationToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("b2_list_buckets status=%d body=%s", resp.StatusCode, string(body))
	}

	var result b2ListBucketsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	for _, bucket := range result.Buckets {
		if bucket.BucketName == h.config.B2Bucket {
			return bucket.BucketID, nil
		}
	}
	return "", fmt.Errorf("bucket %q not found", h.config.B2Bucket)
}

func (h *UploadHandler) requestB2UploadURL(apiURL, authToken, bucketID string) (*b2UploadURLResponse, error) {
	payload, _ := json.Marshal(map[string]string{"bucketId": bucketID})
	req, err := http.NewRequest("POST", apiURL+"/b2api/v2/b2_get_upload_url", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("b2_get_upload_url status=%d body=%s", resp.StatusCode, string(body))
	}

	var result b2UploadURLResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetB2UploadURL returns a Backblaze B2 upload URL and token for browser-to-B2 uploads.
// GET /api/upload/b2-url?type=video|poster|backdrop&filename=movie.mp4&contentType=video/mp4&size=123
func (h *UploadHandler) GetB2UploadURL(c *gin.Context) {
	fileType := strings.TrimSpace(c.Query("type"))
	originalFilename := c.Query("filename")
	contentType := strings.TrimSpace(c.Query("contentType"))

	_, maxSize, allowedTypes, _, ok := directUploadRules(fileType)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type: must be video, poster, or backdrop"})
		return
	}
	if err := validateDirectUploadType(fileType, originalFilename, contentType, allowedTypes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if sizeParam := c.Query("size"); sizeParam != "" {
		size, err := strconv.ParseInt(sizeParam, 10, 64)
		if err != nil || size < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid size"})
			return
		}
		if size > maxSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("file too large (max %dMB)", maxSize/1024/1024)})
			return
		}
	}

	fileKey, err := directUploadFileKey(fileType, originalFilename, contentType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	auth, err := h.authorizeB2()
	if err != nil {
		log.Printf("[B2_DIRECT] authorize failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize B2 upload"})
		return
	}
	bucketID, err := h.resolveB2BucketID(auth)
	if err != nil {
		log.Printf("[B2_DIRECT] bucket resolve failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve B2 bucket"})
		return
	}
	upload, err := h.requestB2UploadURL(auth.APIURL, auth.AuthorizationToken, bucketID)
	if err != nil {
		log.Printf("[B2_DIRECT] get upload url failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get B2 upload URL"})
		return
	}

	cdnURL := h.config.GetCDNFileURL(fileKey)
	log.Printf("[B2_DIRECT] generated upload URL: fileKey=%s uploadUrl=%s", fileKey, upload.UploadURL)

	c.JSON(http.StatusOK, gin.H{
		"uploadUrl":          upload.UploadURL,
		"authorizationToken": upload.AuthorizationToken,
		"fileKey":            fileKey,
		"cdnUrl":             cdnURL,
		"upload_url":         upload.UploadURL,
		"auth_token":         upload.AuthorizationToken,
		"file_key":           fileKey,
		"cdn_url":            cdnURL,
	})
}

// CompleteB2Upload validates and records browser-to-B2 upload metadata.
// POST /api/upload/b2-complete
func (h *UploadHandler) CompleteB2Upload(c *gin.Context) {
	var input b2UploadCompleteRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	prefix, maxSize, allowedTypes, _, ok := directUploadRules(input.Type)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type: must be video, poster, or backdrop"})
		return
	}
	if input.FileKey == "" || strings.Contains(input.FileKey, "..") || filepath.IsAbs(input.FileKey) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fileKey"})
		return
	}
	if !strings.HasPrefix(input.FileKey, prefix+"/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fileKey does not match upload type"})
		return
	}
	if input.Size < 0 || input.Size > maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("file too large (max %dMB)", maxSize/1024/1024)})
		return
	}
	if err := validateDirectUploadType(input.Type, input.FileName, input.ContentType, allowedTypes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cdnURL := h.config.GetCDNFileURL(input.FileKey)
	log.Printf("[B2_DIRECT] upload completion: type=%s fileKey=%s fileName=%q size=%d contentType=%s url=%s",
		input.Type, input.FileKey, input.FileName, input.Size, input.ContentType, cdnURL)

	c.JSON(http.StatusOK, gin.H{
		"url":      cdnURL,
		"fileKey":  input.FileKey,
		"file_key": input.FileKey,
	})
}

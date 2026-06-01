package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// Backblaze B2 "large file" (multipart) upload broker.
//
// Large files (videos) are uploaded from the browser in parts so a single
// network blip doesn't restart a multi-GB upload from zero, and parts can be
// uploaded in parallel. The B2 master credentials never leave the server:
//   1. /upload/b2-large/start    → b2_start_large_file (returns fileId, partSize)
//   2. /upload/b2-large/part-url → b2_get_upload_part_url (per-thread upload URL)
//   3. browser POSTs each slice directly to B2 (b2_upload_part)
//   4. /upload/b2-large/finish   → b2_finish_large_file (with the SHA1 array)
//   5. /upload/b2-large/cancel   → b2_cancel_large_file (on abort/cleanup)

// defaultLargeFilePartSize is the part size we advertise to the browser when
// B2's recommendedPartSize is unset or smaller. 100MB keeps the part count low
// for multi-GB files (4GB ≈ 40 parts) while staying well under B2's 10k cap.
const defaultLargeFilePartSize int64 = 100 * 1024 * 1024

type b2StartLargeFileResponse struct {
	FileID string `json:"fileId"`
}

type b2GetPartURLResponse struct {
	UploadURL          string `json:"uploadUrl"`
	AuthorizationToken string `json:"authorizationToken"`
}

// startLargeFile / finishLargeFile / cancelLargeFile share this authorize +
// resolve-bucket prelude. Returns the account auth so callers can issue the
// follow-up B2 API call.
func (h *UploadHandler) authorizeForBucket() (*b2AuthorizeResponse, string, error) {
	auth, err := h.authorizeB2()
	if err != nil {
		return nil, "", err
	}
	bucketID, err := h.resolveB2BucketID(auth)
	if err != nil {
		return nil, "", err
	}
	return auth, bucketID, nil
}

func (h *UploadHandler) b2POST(apiURL, path, authToken string, payload any) ([]byte, int, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", apiURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// StartB2LargeFile begins a multipart upload and returns the fileId + the part
// size the browser should slice with.
// POST /api/upload/b2-large/start { type, filename, contentType, size }
func (h *UploadHandler) StartB2LargeFile(c *gin.Context) {
	var input struct {
		Type        string `json:"type"`
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Size        int64  `json:"size"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, maxSize, allowedTypes, _, ok := directUploadRules(input.Type)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type: must be video, poster, or backdrop"})
		return
	}
	if err := validateDirectUploadType(input.Type, input.Filename, input.ContentType, allowedTypes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Size < 0 || input.Size > maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("file too large (max %dMB)", maxSize/1024/1024)})
		return
	}

	fileKey, err := directUploadFileKey(input.Type, input.Filename, input.ContentType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	auth, bucketID, err := h.authorizeForBucket()
	if err != nil {
		log.Printf("[B2_LARGE] authorize failed: type=%s err=%v", input.Type, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize B2 upload"})
		return
	}

	contentType := input.ContentType
	if contentType == "" {
		contentType = "b2/x-auto"
	}
	respBody, status, err := h.b2POST(auth.APIURL, "/b2api/v2/b2_start_large_file", auth.AuthorizationToken, map[string]any{
		"bucketId":    bucketID,
		"fileName":    fileKey,
		"contentType": contentType,
	})
	if err != nil || status != http.StatusOK {
		log.Printf("[B2_LARGE] start_large_file failed status=%d err=%v body=%s", status, err, string(respBody))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start large file upload"})
		return
	}
	var started b2StartLargeFileResponse
	if err := json.Unmarshal(respBody, &started); err != nil || started.FileID == "" {
		log.Printf("[B2_LARGE] start_large_file decode failed err=%v body=%s", err, string(respBody))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid B2 start response"})
		return
	}

	partSize := auth.RecommendedPartSize
	if partSize < defaultLargeFilePartSize {
		partSize = defaultLargeFilePartSize
	}

	log.Printf("[B2_LARGE] started fileId=%s fileKey=%s partSize=%d", started.FileID, fileKey, partSize)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"fileId":   started.FileID,
		"fileKey":  fileKey,
		"partSize": partSize,
		"cdnUrl":   h.config.GetCDNFileURL(fileKey),
	})
}

// GetB2PartUploadURL returns an upload URL + token for one upload thread. A
// thread may upload many parts sequentially with the same URL, so the browser
// requests one per parallel worker.
// POST /api/upload/b2-large/part-url { fileId }
func (h *UploadHandler) GetB2PartUploadURL(c *gin.Context) {
	var input struct {
		FileID string `json:"fileId"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.FileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fileId is required"})
		return
	}

	auth, err := h.authorizeB2()
	if err != nil {
		log.Printf("[B2_LARGE] authorize (part-url) failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize B2 upload"})
		return
	}

	respBody, status, err := h.b2POST(auth.APIURL, "/b2api/v2/b2_get_upload_part_url", auth.AuthorizationToken, map[string]string{
		"fileId": input.FileID,
	})
	if err != nil || status != http.StatusOK {
		log.Printf("[B2_LARGE] get_upload_part_url failed status=%d err=%v body=%s", status, err, string(respBody))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get part upload URL"})
		return
	}
	var part b2GetPartURLResponse
	if err := json.Unmarshal(respBody, &part); err != nil || part.UploadURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid B2 part-url response"})
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"uploadUrl":          part.UploadURL,
		"authorizationToken": part.AuthorizationToken,
	})
}

// FinishB2LargeFile assembles the uploaded parts into the final file.
// POST /api/upload/b2-large/finish { fileId, fileKey, type, size, contentType, filename, partSha1Array[] }
func (h *UploadHandler) FinishB2LargeFile(c *gin.Context) {
	var input struct {
		FileID        string   `json:"fileId"`
		FileKey       string   `json:"fileKey"`
		Type          string   `json:"type"`
		Size          int64    `json:"size"`
		ContentType   string   `json:"contentType"`
		Filename      string   `json:"filename"`
		PartSha1Array []string `json:"partSha1Array"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.FileID == "" || len(input.PartSha1Array) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fileId and partSha1Array are required"})
		return
	}

	prefix, maxSize, allowedTypes, _, ok := directUploadRules(input.Type)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type"})
		return
	}
	if input.FileKey == "" || strings.Contains(input.FileKey, "..") || filepath.IsAbs(input.FileKey) || !strings.HasPrefix(input.FileKey, prefix+"/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fileKey"})
		return
	}
	if input.Size < 0 || input.Size > maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("file too large (max %dMB)", maxSize/1024/1024)})
		return
	}
	if err := validateDirectUploadType(input.Type, input.Filename, input.ContentType, allowedTypes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	auth, err := h.authorizeB2()
	if err != nil {
		log.Printf("[B2_LARGE] authorize (finish) failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize B2 upload"})
		return
	}

	respBody, status, err := h.b2POST(auth.APIURL, "/b2api/v2/b2_finish_large_file", auth.AuthorizationToken, map[string]any{
		"fileId":        input.FileID,
		"partSha1Array": input.PartSha1Array,
	})
	if err != nil || status != http.StatusOK {
		log.Printf("[B2_LARGE] finish_large_file failed status=%d err=%v body=%s", status, err, string(respBody))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finish large file upload"})
		return
	}

	cdnURL := h.config.GetCDNFileURL(input.FileKey)
	log.Printf("[B2_LARGE] finished fileId=%s fileKey=%s parts=%d size=%d url=%s", input.FileID, input.FileKey, len(input.PartSha1Array), input.Size, cdnURL)
	c.JSON(http.StatusOK, gin.H{
		"url":      cdnURL,
		"fileKey":  input.FileKey,
		"file_key": input.FileKey,
	})
}

// CancelB2LargeFile aborts an in-progress large file so B2 doesn't keep the
// orphaned parts. Called by the browser on upload failure / user abort.
// POST /api/upload/b2-large/cancel { fileId }
func (h *UploadHandler) CancelB2LargeFile(c *gin.Context) {
	var input struct {
		FileID string `json:"fileId"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.FileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fileId is required"})
		return
	}

	auth, err := h.authorizeB2()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize B2"})
		return
	}
	respBody, status, err := h.b2POST(auth.APIURL, "/b2api/v2/b2_cancel_large_file", auth.AuthorizationToken, map[string]string{
		"fileId": input.FileID,
	})
	if err != nil || status != http.StatusOK {
		log.Printf("[B2_LARGE] cancel_large_file failed status=%d err=%v body=%s", status, err, string(respBody))
		// Best-effort — report ok so the client doesn't loop on cleanup.
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}

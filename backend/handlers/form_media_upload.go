package handlers

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
	"github.com/gin-gonic/gin"
)

const (
	maxFormImageSize   int64 = 10 * 1024 * 1024
	maxFormGIFSize     int64 = 20 * 1024 * 1024
	maxFormAdVideoSize int64 = 50 * 1024 * 1024
)

var allowedFormImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

var allowedAdVideoTypes = map[string]bool{
	"video/mp4":       true,
	"video/webm":      true,
	"video/quicktime": true,
}

type uploadResponse struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Path    string `json:"path"`
}

type uploadSpec struct {
	FieldName    string
	Folder       string
	Label        string
	MaxSizeBytes int64
	AllowedTypes map[string]bool
	FallbackExt  string
	ReturnPath   string
}

func (h *UploadHandler) handleDirectUpload(c *gin.Context, spec uploadSpec) {
	file, header, err := c.Request.FormFile(spec.FieldName)
	if err != nil {
		c.JSON(400, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	data, contentType, err := readAndValidateUpload(file, spec.MaxSizeBytes, spec.AllowedTypes)
	if err != nil {
		status := 400
		if strings.Contains(err.Error(), "too large") {
			status = 413
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// Convert JPEG/PNG posters & backdrops to WebP before storage so only the
	// smaller .webp ever lands in B2 (the original is never uploaded). A
	// conversion failure is fatal — we do not silently store the original.
	if isWebPConvertibleImage(contentType) {
		webpData, convErr := convertToWebP(data)
		if convErr != nil {
			logSafeUploadError(spec.Label, header.Filename, convErr)
			c.JSON(500, gin.H{"error": "failed to convert image to webp"})
			return
		}
		data = webpData
		contentType = "image/webp"
	}

	objectKey := buildFolderObjectKey(spec.Folder, spec.Label, header.Filename, contentType, spec.FallbackExt)
	url, err := h.storeUploadedFile(objectKey, data, contentType)
	if err != nil {
		logSafeUploadError(spec.Label, objectKey, err)
		c.JSON(500, gin.H{"error": "failed to upload file"})
		return
	}

	c.JSON(200, uploadResponse{
		Success: true,
		URL:     buildStoredMediaURL(h.config, spec.ReturnPath, objectKey, url),
		Path:    objectKey,
	})
}

func readAndValidateUpload(file io.Reader, maxSize int64, allowedTypes map[string]bool) ([]byte, string, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file")
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("no file provided")
	}
	if int64(len(data)) > maxSize {
		return nil, "", fmt.Errorf("file too large (max %dMB)", maxSize/1024/1024)
	}

	contentType := strings.ToLower(strings.TrimSpace(httpDetectContentType(data)))
	if !allowedTypes[contentType] {
		return nil, "", fmt.Errorf("invalid file type")
	}
	return data, contentType, nil
}

func httpDetectContentType(data []byte) string {
	return strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
}

func buildFolderObjectKey(folder, label, originalFilename, contentType, fallbackExt string) string {
	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), label, safeUploadExt(originalFilename, contentType, fallbackExt))
	return strings.Trim(strings.TrimSpace(folder), "/") + "/" + filename
}

func (h *UploadHandler) storeUploadedFile(objectKey string, data []byte, contentType string) (string, error) {
	if h.config.IsDev {
		return h.saveBytesLocal(objectKey, data)
	}
	return h.uploadBytesToB2(objectKey, data, contentType)
}

func (h *UploadHandler) saveBytesLocal(objectKey string, data []byte) (string, error) {
	fullPath := filepath.Join(h.config.UploadsDir, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return strings.TrimSuffix(h.config.GetBaseURL(), "/") + "/" + strings.TrimPrefix(objectKey, "/"), nil
}

func (h *UploadHandler) uploadBytesToB2(objectKey string, fileBytes []byte, contentType string) (string, error) {
	auth, err := h.authorizeB2()
	if err != nil {
		return "", err
	}
	bucketID, err := h.resolveB2BucketID(auth)
	if err != nil {
		return "", err
	}
	uploadURL, err := h.requestB2UploadURL(auth.APIURL, auth.AuthorizationToken, bucketID)
	if err != nil {
		return "", err
	}

	sum := sha1.Sum(fileBytes)
	req, err := http.NewRequest(http.MethodPost, uploadURL.UploadURL, bytes.NewReader(fileBytes))
	if err != nil {
		return "", err
	}
	req.ContentLength = int64(len(fileBytes))
	req.Header.Set("Authorization", uploadURL.AuthorizationToken)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Bz-File-Name", url.PathEscape(objectKey))
	req.Header.Set("X-Bz-Content-Sha1", hex.EncodeToString(sum[:]))

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("b2 upload status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return h.config.GetCDNFileURL(objectKey), nil
}

func logSafeUploadError(label, objectKey string, err error) {
	log.Printf("[UPLOAD_DIRECT] %s upload failed: path=%q err=%v", label, objectKey, err)
}

func buildStoredMediaURL(cfg *config.Config, publicPath string, fallbackObjectKey string, devURL string) string {
	if cfg != nil && cfg.IsDev {
		return devURL
	}

	value := strings.TrimSpace(publicPath)
	if value != "" {
		if !strings.HasPrefix(value, "/") {
			return "/" + value
		}
		return value
	}

	key := strings.TrimSpace(fallbackObjectKey)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "/") {
		return key
	}
	return "/" + key
}

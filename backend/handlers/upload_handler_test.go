package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"regexp"
	"strings"
	"testing"

	"github.com/filmorauz/backend/config"
	"github.com/filmorauz/backend/models"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestUploadTempUploadsPosterBackdropDirectlyToB2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stubWebPEncoder(t)

	pngBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
		0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}

	for _, fileType := range []string{"poster", "backdrop"} {
		t.Run(fileType, func(t *testing.T) {
			var uploadedFileName string
			var b2 *httptest.Server
			b2 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/authorize":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"accountId":"acc","authorizationToken":"auth-token","apiUrl":"` + b2.URL + `"}`))
				case "/b2api/v2/b2_get_upload_url":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"uploadUrl":"` + b2.URL + `/upload","authorizationToken":"upload-token"}`))
				case "/upload":
					uploadedFileName = r.Header.Get("X-Bz-File-Name")
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"fileId":"4_z","fileName":"` + uploadedFileName + `"}`))
				default:
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
			}))
			defer b2.Close()

			handler := NewUploadHandler(nil, &config.Config{
				IsDev:       false,
				B2KeyID:     "key-id",
				B2AppKey:    "app-key",
				B2Bucket:    "filmorauznet",
				B2BucketID:  "bucket-123",
				B2PublicURL: "https://cdn.filmorauz.net/file/filmorauznet",
			})
			handler.httpClient = b2.Client()
			handler.b2AuthorizeURL = b2.URL + "/authorize"

			router := gin.New()
			router.POST("/api/admin/upload-temp", handler.UploadTemp)

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			fileHeader := make(textproto.MIMEHeader)
			fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="unsafe poster, 50%.png"`)
			fileHeader.Set("Content-Type", "image/png")
			part, err := writer.CreatePart(fileHeader)
			if err != nil {
				t.Fatalf("CreatePart: %v", err)
			}
			if _, err := part.Write(pngBytes); err != nil {
				t.Fatalf("write file: %v", err)
			}
			if err := writer.WriteField("type", fileType); err != nil {
				t.Fatalf("WriteField: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/admin/upload-temp", body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}

			var response map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			wantPrefix := "images/" + fileType + "s/"
			if !strings.HasPrefix(response["file_key"], wantPrefix) {
				t.Fatalf("file_key = %q, want prefix %q", response["file_key"], wantPrefix)
			}
			if !strings.Contains(uploadedFileName, strings.ReplaceAll(wantPrefix, "/", "%2F")) {
				t.Fatalf("uploaded file name = %q, want encoded prefix for %q", uploadedFileName, wantPrefix)
			}
		})
	}
}

func TestDirectUploadFileKeyUsesRequestedPrefixes(t *testing.T) {
	tests := []struct {
		name        string
		uploadType  string
		filename    string
		contentType string
		wantPattern string
	}{
		{
			name:        "video",
			uploadType:  "video",
			filename:    "Movie Final.mp4",
			contentType: "video/mp4",
			wantPattern: `^temp/videos/[0-9]+_video\.mp4$`,
		},
		{
			name:        "poster",
			uploadType:  "poster",
			filename:    "poster final.png",
			contentType: "image/png",
			wantPattern: `^images/posters/[0-9]+_poster\.png$`,
		},
		{
			name:        "backdrop",
			uploadType:  "backdrop",
			filename:    "backdrop final.webp",
			contentType: "image/webp",
			wantPattern: `^images/backdrops/[0-9]+_backdrop\.webp$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := directUploadFileKey(tt.uploadType, tt.filename, tt.contentType)
			if err != nil {
				t.Fatalf("directUploadFileKey returned error: %v", err)
			}
			if !regexp.MustCompile(tt.wantPattern).MatchString(key) {
				t.Fatalf("file key = %q, want pattern %s", key, tt.wantPattern)
			}
		})
	}
}

func TestDirectUploadFileKeyRejectsBadExtensionWithoutContentType(t *testing.T) {
	if _, err := directUploadFileKey("video", "movie.exe", ""); err == nil {
		t.Fatal("expected bad extension to be rejected")
	}
}

func TestCompleteB2UploadValidatesMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler(nil, &config.Config{
		IsDev: true,
		Port:  "8080",
	})

	router := gin.New()
	router.POST("/api/upload/b2-complete", handler.CompleteB2Upload)

	body := bytes.NewBufferString(`{"fileKey":"temp/videos/123_video.mp4","fileName":"movie.mp4","size":1234,"type":"video","contentType":"video/mp4"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/b2-complete", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["file_key"] != "temp/videos/123_video.mp4" {
		t.Fatalf("file_key = %q", response["file_key"])
	}
}

func TestCompleteB2UploadRejectsWrongPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler(nil, &config.Config{IsDev: true, Port: "8080"})
	router := gin.New()
	router.POST("/api/upload/b2-complete", handler.CompleteB2Upload)

	body := bytes.NewBufferString(`{"fileKey":"posters/123_poster.jpg","fileName":"movie.mp4","size":1234,"type":"video","contentType":"video/mp4"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/b2-complete", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUploadTempForwardsVideoAsTempMovieFileField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workerReceivedFile := false
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload-temp-movie" {
			t.Fatalf("worker path = %q, want %q", r.URL.Path, "/upload-temp-movie")
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("worker did not receive file field: %v", err)
		}
		defer file.Close()
		workerReceivedFile = true
		if header.Filename == "" {
			t.Fatal("worker received empty filename")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"url":"https://cdn.example/temp.mp4","temp_key":"temp/raw/test.mp4"}`))
	}))
	defer worker.Close()

	handler := NewUploadHandler(nil, &config.Config{
		IsDev:           false,
		WorkerUploadURL: worker.URL,
	})

	router := gin.New()
	router.POST("/api/admin/upload-temp", handler.UploadTemp)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="source.mp4"`)
	fileHeader.Set("Content-Type", "video/mp4")
	part, err := writer.CreatePart(fileHeader)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write([]byte("fake-mp4-data")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.WriteField("type", "video"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/upload-temp", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !workerReceivedFile {
		t.Fatal("worker did not receive file field")
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["file_key"] != "temp/raw/test.mp4" {
		t.Fatalf("file_key = %q, want temp/raw/test.mp4", response["file_key"])
	}
}

func TestBuildProfileImageObjectKeyUsesProfilePrefix(t *testing.T) {
	key := buildProfileImageObjectKey("507f1f77bcf86cd799439011", "avatar.png", "image/png")
	if !regexp.MustCompile(`^images/profile/507f1f77bcf86cd799439011_[0-9]+\.png$`).MatchString(key) {
		t.Fatalf("object key = %q", key)
	}
}

func TestUploadProfileImageRejectsSniffedNonImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler(nil, &config.Config{IsDev: false})
	router := gin.New()
	router.POST("/api/auth/upload/profile-image", func(c *gin.Context) {
		c.Set("user_id", "507f1f77bcf86cd799439011")
		handler.UploadProfileImage(c)
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", `form-data; name="image"; filename="avatar.png"`)
	fileHeader.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(fileHeader)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write([]byte("not really an image")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/upload/profile-image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid file type") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestUploadProfileImageUploadsDirectlyToB2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stubWebPEncoder(t)

	var uploadedFileName string
	var savedProfileURL string
	var b2 *httptest.Server
	b2 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorize":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accountId":"acc","authorizationToken":"auth-token","apiUrl":"` + b2.URL + `"}`))
		case "/b2api/v2/b2_get_upload_url":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"bucketId":"bucket-123"`) {
				t.Fatalf("unexpected bucket request body: %s", string(body))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uploadUrl":"` + b2.URL + `/upload","authorizationToken":"upload-token"}`))
		case "/upload":
			uploadedFileName = r.Header.Get("X-Bz-File-Name")
			if r.Header.Get("Authorization") != "upload-token" {
				t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
			}
			if !strings.HasPrefix(uploadedFileName, "images%2Fprofile%2F507f1f77bcf86cd799439011_") {
				t.Fatalf("uploaded file name = %q", uploadedFileName)
			}
			// PNG avatars are converted to WebP before upload.
			if r.Header.Get("Content-Type") != "image/webp" {
				t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
			}
			if !strings.HasSuffix(uploadedFileName, ".webp") {
				t.Fatalf("uploaded file name = %q, want .webp suffix", uploadedFileName)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"fileId":"4_z","fileName":"` + uploadedFileName + `"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer b2.Close()

	cfg := &config.Config{
		IsDev:       false,
		B2KeyID:     "key-id",
		B2AppKey:    "app-key",
		B2Bucket:    "filmorauznet",
		B2BucketID:  "bucket-123",
		B2PublicURL: "https://cdn.filmorauz.net/file/filmorauznet",
	}
	handler := NewUploadHandler(nil, cfg)
	handler.httpClient = b2.Client()
	handler.b2AuthorizeURL = b2.URL + "/authorize"
	handler.updateUserImage = func(userIDHex string, firstName string, profileImageURL string) error {
		if userIDHex != "507f1f77bcf86cd799439011" {
			t.Fatalf("userID = %q", userIDHex)
		}
		if profileImageURL == "" {
			t.Fatal("profileImageURL was empty")
		}
		savedProfileURL = profileImageURL
		return nil
	}
	handler.findUserByID = func(id string) (*models.User, error) {
		oid, _ := primitive.ObjectIDFromHex(id)
		return &models.User{
			ID:              oid,
			Role:            "user",
			AuthProvider:    "telegram",
			FirstName:       "Test",
			ProfileImageURL: savedProfileURL,
		}, nil
	}

	router := gin.New()
	router.POST("/api/auth/upload/profile-image", func(c *gin.Context) {
		c.Set("user_id", "507f1f77bcf86cd799439011")
		handler.UploadProfileImage(c)
	})

	// Minimal valid 1x1 PNG.
	pngBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
		0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", `form-data; name="image"; filename="avatar.png"`)
	fileHeader.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(fileHeader)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/upload/profile-image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	profileURL, _ := response["profile_image_url"].(string)
	if !strings.HasPrefix(profileURL, "/images/profile/507f1f77bcf86cd799439011_") {
		t.Fatalf("profile_image_url = %q", profileURL)
	}
}

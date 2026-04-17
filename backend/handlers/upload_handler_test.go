package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"regexp"
	"testing"

	"github.com/filmorauz/backend/config"
	"github.com/gin-gonic/gin"
)

func TestUploadTempForwardsPosterBackdropAsImageField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, fileType := range []string{"poster", "backdrop"} {
		t.Run(fileType, func(t *testing.T) {
			workerReceivedImage := false
			worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/upload-"+fileType {
					t.Fatalf("worker path = %q, want %q", r.URL.Path, "/upload-"+fileType)
				}
				file, header, err := r.FormFile("image")
				if err != nil {
					t.Fatalf("worker did not receive image field: %v", err)
				}
				defer file.Close()
				workerReceivedImage = true
				if header.Filename == "" {
					t.Fatal("worker received empty filename")
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"url":"https://cdn.example/test.png","file_key":"` + fileType + `s/test.png"}`))
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
			fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="unsafe poster, 50%.png"`)
			fileHeader.Set("Content-Type", "image/png")
			part, err := writer.CreatePart(fileHeader)
			if err != nil {
				t.Fatalf("CreatePart: %v", err)
			}
			if _, err := part.Write([]byte("fake-png-data")); err != nil {
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
			if !workerReceivedImage {
				t.Fatal("worker did not receive image field")
			}

			var response map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			wantKey := fileType + "s/test.png"
			if response["file_key"] != wantKey {
				t.Fatalf("file_key = %q, want %q", response["file_key"], wantKey)
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
			wantPattern: `^posters/[0-9]+_poster\.png$`,
		},
		{
			name:        "backdrop",
			uploadType:  "backdrop",
			filename:    "backdrop final.webp",
			contentType: "image/webp",
			wantPattern: `^backdrops/[0-9]+_backdrop\.webp$`,
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

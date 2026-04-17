package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
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
				_, _ = w.Write([]byte(`{"url":"https://cdn.example/test.png","temp_key":"temp/test.png"}`))
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
		})
	}
}

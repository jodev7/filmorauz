package handlers

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPickAvatarSize(t *testing.T) {
	got := pickAvatarSize([]tgPhotoSize{{FileID: "a", Width: 160}, {FileID: "b", Width: 320}, {FileID: "c", Width: 640}})
	if got == nil || got.FileID != "b" {
		t.Fatalf("want 320px, got %+v", got)
	}
	got = pickAvatarSize([]tgPhotoSize{{FileID: "a", Width: 100}, {FileID: "b", Width: 160}})
	if got == nil || got.FileID != "b" {
		t.Fatalf("want largest when all small, got %+v", got)
	}
	if pickAvatarSize(nil) != nil {
		t.Fatal("nil sizes")
	}
}

func testJPEG(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		img.Set(x, x, color.RGBA{255, 0, 0, 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTelegramAvatarFetchPhoto(t *testing.T) {
	const token = "123:SECRET"
	photo := testJPEG(t)
	var hidden bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bot" + token + "/getUserProfilePhotos":
			if hidden {
				_, _ = w.Write([]byte(`{"ok":true,"result":{"total_count":0,"photos":[]}}`))
				return
			}
			if r.URL.Query().Get("user_id") != "42" {
				t.Errorf("user_id = %q", r.URL.Query().Get("user_id"))
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"total_count":1,"photos":[[{"file_id":"s","width":160,"height":160},{"file_id":"m","width":320,"height":320},{"file_id":"l","width":640,"height":640}]]}}`))
		case "/bot" + token + "/getFile":
			if r.URL.Query().Get("file_id") != "m" {
				t.Errorf("file_id = %q", r.URL.Query().Get("file_id"))
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"profile_photos/file_1.jpg"}}`))
		case "/file/bot" + token + "/profile_photos/file_1.jpg":
			_, _ = w.Write(photo)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":404,"description":"Not Found"}`))
		}
	}))
	defer srv.Close()

	imp := &TelegramAvatarImporter{botToken: token, apiBase: srv.URL, http: srv.Client()}
	data, ct, err := imp.fetchPhoto(context.Background(), 42)
	if err != nil || ct != "image/jpeg" || !bytes.Equal(data, photo) {
		t.Fatalf("fetchPhoto: ct=%q len=%d err=%v", ct, len(data), err)
	}

	hidden = true
	data, _, err = imp.fetchPhoto(context.Background(), 42)
	if err != nil || data != nil {
		t.Fatalf("hidden photo should be (nil, nil), got len=%d err=%v", len(data), err)
	}

	// Transport errors must not leak the bot token.
	bad := &TelegramAvatarImporter{botToken: token, apiBase: "http://127.0.0.1:1", http: srv.Client()}
	if _, _, err := bad.fetchPhoto(context.Background(), 42); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error must exist and hide the token: %v", err)
	}
}

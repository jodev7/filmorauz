package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCleanSearchTitle(t *testing.T) {
	cases := map[string]string{
		"Qasoskorlar: Final (O'zbek tilida) HD 2019": "Qasoskorlar: Final",
		"Avengers Endgame 1080p uzbek tarjima":       "Avengers Endgame",
		"Squid Game 1-fasl barcha qismlar":           "Squid Game",
		"Titanic":                                    "Titanic",
	}
	for in, want := range cases {
		if got := cleanSearchTitle(in); got != want {
			t.Errorf("cleanSearchTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTMDBSearchAndCredits(t *testing.T) {
	var auth, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, key = r.Header.Get("Authorization"), r.URL.Query().Get("api_key")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			switch r.URL.Query().Get("query") {
			case "Avengers Endgame":
				// Wrong-year hit first: must be skipped.
				_, _ = w.Write([]byte(`{"results":[{"id":1,"release_date":"2012-04-25"},{"id":299534,"release_date":"2019-04-24"}]}`))
			default:
				_, _ = w.Write([]byte(`{"results":[]}`))
			}
		case "/movie/299534/credits":
			_, _ = w.Write([]byte(`{"cast":[{"id":3223,"name":"Robert Downey Jr.","character":"Tony Stark","profile_path":"/a.jpg"},{"id":16828,"name":"Chris Evans","character":"Steve Rogers","profile_path":null}],
				"crew":[{"id":9,"name":"Someone","job":"Producer"},{"id":19271,"name":"Anthony Russo","job":"Director","profile_path":"/r.jpg"}]}`))
		case "/tv/93405/aggregate_credits":
			_, _ = w.Write([]byte(`{"cast":[{"id":1,"name":"Lee Jung-jae","profile_path":"/l.jpg","roles":[{"character":"Seong Gi-hun"}]}]}`))
		case "/tv/93405":
			_, _ = w.Write([]byte(`{"created_by":[{"id":2,"name":"Hwang Dong-hyuk","profile_path":"/h.jpg"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewTMDBClient("k123", "")
	c.baseURL = srv.URL
	ctx := context.Background()

	id, err := c.Search(ctx, "movie", []string{"", "Avengers Endgame (O'zbek tilida) 1080p"}, 2019)
	if err != nil || id != 299534 {
		t.Fatalf("Search = %d, %v; want 299534", id, err)
	}
	if key != "k123" || auth != "" {
		t.Fatalf("api key auth not used: key=%q auth=%q", key, auth)
	}
	if id, _ := c.Search(ctx, "movie", []string{"Nothing here"}, 2019); id != 0 {
		t.Fatalf("expected no match, got %d", id)
	}

	cr, err := c.Credits(ctx, "movie", 299534)
	if err != nil {
		t.Fatal(err)
	}
	if len(cr.Cast) != 2 || cr.Cast[0].Name != "Robert Downey Jr." || cr.Cast[0].Character != "Tony Stark" ||
		cr.Cast[0].ProfileURL != tmdbProfileBase+"/a.jpg" || cr.Cast[1].ProfileURL != "" {
		t.Fatalf("bad cast: %+v", cr.Cast)
	}
	if cr.Director == nil || cr.Director.Name != "Anthony Russo" || cr.Director.ProfileURL != tmdbProfileBase+"/r.jpg" {
		t.Fatalf("bad director: %+v", cr.Director)
	}

	tv, err := c.Credits(ctx, "tv", 93405)
	if err != nil || len(tv.Cast) != 1 || tv.Cast[0].Character != "Seong Gi-hun" || tv.Director == nil || tv.Director.Name != "Hwang Dong-hyuk" {
		t.Fatalf("bad tv credits: %+v %v", tv, err)
	}

	// Bearer token wins over the api key.
	b := NewTMDBClient("k123", "tok")
	b.baseURL = srv.URL
	_, _ = b.Credits(ctx, "movie", 299534)
	if auth != "Bearer tok" || key != "" {
		t.Fatalf("bearer not used: auth=%q key=%q", auth, key)
	}
	if NewTMDBClient("", "") != nil {
		t.Fatal("client without credentials should be nil")
	}
}

func TestTMDBErrorsHideAPIKey(t *testing.T) {
	c := NewTMDBClient("secret-key-123", "")
	c.baseURL = "http://127.0.0.1:1" // nothing listens here
	_, err := c.Credits(context.Background(), "movie", 1)
	if err == nil || strings.Contains(err.Error(), "secret-key-123") {
		t.Fatalf("error must exist and not leak the key: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	c.baseURL = srv.URL
	if _, err := c.Credits(context.Background(), "movie", 1); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401 message, got %v", err)
	}
}

func TestTMDBBadTokenFallsBackToAPIKey(t *testing.T) {
	var bearerCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			bearerCalls++
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("api_key") != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"cast":[{"id":1,"name":"A"}],"crew":[]}`))
	}))
	defer srv.Close()
	c := NewTMDBClient("good", "bad-token")
	c.baseURL = srv.URL
	for i := 0; i < 2; i++ {
		cr, err := c.Credits(context.Background(), "movie", 1)
		if err != nil || len(cr.Cast) != 1 {
			t.Fatalf("call %d: %+v %v", i, cr, err)
		}
	}
	if bearerCalls != 1 {
		t.Fatalf("rejected token should be tried once, got %d", bearerCalls)
	}
}

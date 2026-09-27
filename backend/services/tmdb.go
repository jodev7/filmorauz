package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/filmorauz/backend/models"
)

// Minimal TMDB client for credits (cast, director/creator, photos).

const (
	tmdbAPIBase     = "https://api.themoviedb.org/3"
	tmdbProfileBase = "https://image.tmdb.org/t/p/w185"
	tmdbMaxCast     = 15
)

type TMDBClient struct {
	apiKey, token string
	baseURL       string
	http          *http.Client
	tokenRejected atomic.Bool
}

// NewTMDBClient returns nil when no credentials are configured.
func NewTMDBClient(apiKey, readToken string) *TMDBClient {
	if apiKey == "" && readToken == "" {
		return nil
	}
	return &TMDBClient{apiKey: apiKey, token: readToken, baseURL: tmdbAPIBase, http: &http.Client{Timeout: 12 * time.Second}}
}

func (c *TMDBClient) get(ctx context.Context, path string, q url.Values, out interface{}) error {
	token := c.token
	if c.tokenRejected.Load() {
		token = ""
	}
	err := c.getOnce(ctx, path, q, out, token)
	// A bad/revoked read token shouldn't break everything when the api key works.
	if errors.Is(err, errTMDBUnauthorized) && token != "" && c.apiKey != "" {
		if !c.tokenRejected.Swap(true) {
			log.Printf("[CREDITS] TMDB_READ_TOKEN rejected (401) — falling back to TMDB_API_KEY")
		}
		err = c.getOnce(ctx, path, q, out, "")
	}
	return err
}

func (c *TMDBClient) getOnce(ctx context.Context, path string, q url.Values, out interface{}, token string) error {
	if q == nil {
		q = url.Values{}
	} else {
		q = cloneValues(q)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		q.Set("api_key", c.apiKey)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error embeds the full URL, which contains api_key — drop it.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("TMDB'ga ulanib bo'lmadi (%s): %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errTMDBNotFound
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return errTMDBUnauthorized
		case http.StatusTooManyRequests:
			return fmt.Errorf("TMDB so'rovlar limiti (429) — birozdan keyin qayta urinib ko'ring")
		}
		return fmt.Errorf("TMDB %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

var (
	errTMDBNotFound     = fmt.Errorf("tmdb: not found")
	errTMDBUnauthorized = fmt.Errorf("TMDB kaliti noto'g'ri yoki bekor qilingan (401) — TMDB_API_KEY / TMDB_READ_TOKEN ni tekshiring")
)

func cloneValues(q url.Values) url.Values {
	out := make(url.Values, len(q))
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func tmdbProfileURL(path string) string {
	if path == "" {
		return ""
	}
	return tmdbProfileBase + path
}

var (
	reTitleBrackets = regexp.MustCompile(`[\(\[\{][^\)\]\}]*[\)\]\}]`)
	reTitleNoise    = regexp.MustCompile(`(?i)\b(o['‘’ʻ]?zbek(cha)?|uzbek(cha)?|tilida|tarjima(si)?|tas-?ix|premyera|kino|film|serial(i)?|barcha qismlar(i)?|hd|full ?hd|\d{3,4}p|4k|\d+[- ]?(qism|fasl|mavsum)(lar)?)\b`)
	reTitleYear     = regexp.MustCompile(`\b(19|20)\d{2}\b`)
)

// cleanSearchTitle strips site noise ("... (O'zbek tilida) HD 2019") so TMDB
// search can match the real title.
func cleanSearchTitle(t string) string {
	t = reTitleBrackets.ReplaceAllString(t, " ")
	t = reTitleNoise.ReplaceAllString(t, " ")
	t = reTitleYear.ReplaceAllString(t, " ")
	t = strings.Join(strings.Fields(t), " ")
	return strings.Trim(t, " -–—:|/,.")
}

type tmdbSearchHit struct {
	ID           int     `json:"id"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	Popularity   float64 `json:"popularity"`
}

func (h tmdbSearchHit) year() int {
	d := h.ReleaseDate
	if d == "" {
		d = h.FirstAirDate
	}
	if len(d) >= 4 {
		y, _ := strconv.Atoi(d[:4])
		return y
	}
	return 0
}

func yearClose(a, b int) bool {
	if a == 0 || b == 0 {
		return true
	}
	d := a - b
	return d >= -1 && d <= 1
}

// Search finds the TMDB id of a movie ("movie") or series ("tv"). Titles are
// tried in order; a known year must match within ±1. Returns 0 when nothing
// matches confidently.
func (c *TMDBClient) Search(ctx context.Context, kind string, titles []string, year int) (int, error) {
	yearParam := "year"
	if kind == "tv" {
		yearParam = "first_air_date_year"
	}
	seen := map[string]bool{}
	for _, raw := range titles {
		for _, t := range []string{strings.TrimSpace(raw), cleanSearchTitle(raw)} {
			key := strings.ToLower(t)
			if len([]rune(t)) < 2 || seen[key] {
				continue
			}
			seen[key] = true
			tries := []url.Values{{"query": {t}, "include_adult": {"false"}}}
			if year > 0 {
				tries = []url.Values{{"query": {t}, "include_adult": {"false"}, yearParam: {strconv.Itoa(year)}}, tries[0]}
			}
			for _, q := range tries {
				var res struct {
					Results []tmdbSearchHit `json:"results"`
				}
				if err := c.get(ctx, "/search/"+kind, q, &res); err != nil {
					return 0, err
				}
				for _, h := range res.Results {
					if yearClose(h.year(), year) {
						return h.ID, nil
					}
				}
			}
		}
	}
	return 0, nil
}

// TMDBCredits is the part of TMDB's credits we keep.
type TMDBCredits struct {
	TMDBID   int
	Cast     []models.CastMember
	Director *models.CastMember
}

type tmdbCastRaw struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
	Order       int    `json:"order"`
	Roles       []struct {
		Character string `json:"character"`
	} `json:"roles"` // tv aggregate_credits
}

type tmdbCrewRaw struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Job         string `json:"job"`
	ProfilePath string `json:"profile_path"`
	Jobs        []struct {
		Job string `json:"job"`
	} `json:"jobs"` // tv aggregate_credits
}

func castFromRaw(raw []tmdbCastRaw) []models.CastMember {
	out := make([]models.CastMember, 0, tmdbMaxCast)
	seen := map[int]bool{}
	for _, c := range raw {
		name := strings.TrimSpace(c.Name)
		if name == "" || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		ch := c.Character
		if ch == "" && len(c.Roles) > 0 {
			ch = c.Roles[0].Character
		}
		out = append(out, models.CastMember{Name: name, Character: strings.TrimSpace(ch), ProfileURL: tmdbProfileURL(c.ProfilePath), TMDBID: c.ID})
		if len(out) == tmdbMaxCast {
			break
		}
	}
	return out
}

// Credits loads the cast and the director (movie) / creator (tv).
func (c *TMDBClient) Credits(ctx context.Context, kind string, id int) (*TMDBCredits, error) {
	out := &TMDBCredits{TMDBID: id}
	if kind == "tv" {
		var agg struct {
			Cast []tmdbCastRaw `json:"cast"`
			Crew []tmdbCrewRaw `json:"crew"`
		}
		if err := c.get(ctx, fmt.Sprintf("/tv/%d/aggregate_credits", id), nil, &agg); err != nil {
			return nil, err
		}
		out.Cast = castFromRaw(agg.Cast)
		var show struct {
			CreatedBy []struct {
				ID          int    `json:"id"`
				Name        string `json:"name"`
				ProfilePath string `json:"profile_path"`
			} `json:"created_by"`
		}
		if err := c.get(ctx, fmt.Sprintf("/tv/%d", id), nil, &show); err == nil && len(show.CreatedBy) > 0 {
			cb := show.CreatedBy[0]
			out.Director = &models.CastMember{Name: cb.Name, ProfileURL: tmdbProfileURL(cb.ProfilePath), TMDBID: cb.ID}
		}
		return out, nil
	}
	var cr struct {
		Cast []tmdbCastRaw `json:"cast"`
		Crew []tmdbCrewRaw `json:"crew"`
	}
	if err := c.get(ctx, fmt.Sprintf("/movie/%d/credits", id), nil, &cr); err != nil {
		return nil, err
	}
	out.Cast = castFromRaw(cr.Cast)
	for _, p := range cr.Crew {
		if p.Job == "Director" && strings.TrimSpace(p.Name) != "" {
			out.Director = &models.CastMember{Name: strings.TrimSpace(p.Name), ProfileURL: tmdbProfileURL(p.ProfilePath), TMDBID: p.ID}
			break
		}
	}
	return out, nil
}

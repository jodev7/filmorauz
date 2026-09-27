package repositories

import (
	"context"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// "Yil yakuni" — a user's year on the site, computed from watch history.

var reviewZone = time.FixedZone("Asia/Tashkent", 5*60*60)

type YearTopTitle struct {
	TargetType string `json:"target_type"` // "movie" | "series"
	TargetID   string `json:"target_id"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	PosterURL  string `json:"poster_url"`
	Minutes    int64  `json:"minutes"`
}

type YearCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type YearReview struct {
	Year            int            `json:"year"`
	Complete        bool           `json:"complete"` // the year is over
	TotalMinutes    int64          `json:"total_minutes"`
	MoviesWatched   int            `json:"movies_watched"`
	EpisodesWatched int            `json:"episodes_watched"`
	SeriesWatched   int            `json:"series_watched"`
	Completed       int            `json:"completed"`
	ActiveDays      int            `json:"active_days"`
	TopGenres       []YearCount    `json:"top_genres"`
	Months          []int          `json:"months"`      // 12 buckets, titles watched per month
	TopMonth        int            `json:"top_month"`   // 1-12, 0 when empty
	TopWeekday      int            `json:"top_weekday"` // 0=Sunday .. 6, -1 when empty
	NightOwl        bool           `json:"night_owl"`   // most watching after 22:00
	TopTitles       []YearTopTitle `json:"top_titles"`
	Ratings         int64          `json:"ratings"`
	Reviews         int64          `json:"reviews"`
	Comments        int64          `json:"comments"`
}

type yearRecord struct {
	TargetType      string             `bson:"target_type"`
	TargetID        primitive.ObjectID `bson:"target_id"`
	MovieID         primitive.ObjectID `bson:"movie_id"`
	SeriesID        primitive.ObjectID `bson:"series_id"`
	LastPositionSec int64              `bson:"last_position_sec"`
	DurationSec     int64              `bson:"duration_sec"`
	Completed       bool               `bson:"completed"`
	ProgressPercent float64            `bson:"progress_percent"`
	LastWatchedAt   time.Time          `bson:"last_watched_at"`
}

// watchedSeconds is how long a record counts for: the full duration when
// finished, otherwise the last position (never above the duration).
func watchedSeconds(r yearRecord) int64 {
	if r.Completed || r.ProgressPercent >= 90 {
		if r.DurationSec > 0 {
			return r.DurationSec
		}
	}
	s := r.LastPositionSec
	if r.DurationSec > 0 && s > r.DurationSec {
		s = r.DurationSec
	}
	if s < 0 {
		return 0
	}
	return s
}

// topCounts sorts a counter map into the n largest entries (ties by key).
func topCounts(m map[string]int, n int) []YearCount {
	out := make([]YearCount, 0, len(m))
	for k, v := range m {
		out = append(out, YearCount{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// GetYearReview computes the review for `year` (Tashkent calendar).
func (r *WatchHistoryRepository) GetYearReview(userID primitive.ObjectID, year int, now time.Time) (*YearReview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	start := time.Date(year, 1, 1, 0, 0, 0, 0, reviewZone)
	end := start.AddDate(1, 0, 0)
	out := &YearReview{Year: year, Complete: !now.Before(end), Months: make([]int, 12), TopWeekday: -1, TopGenres: []YearCount{}, TopTitles: []YearTopTitle{}}

	cur, err := r.col.Find(ctx,
		bson.M{"user_id": userID, "last_watched_at": bson.M{"$gte": start, "$lt": end}},
		options.Find().SetProjection(bson.M{
			"target_type": 1, "target_id": 1, "movie_id": 1, "series_id": 1, "last_position_sec": 1,
			"duration_sec": 1, "completed": 1, "progress_percent": 1, "last_watched_at": 1,
		}).SetLimit(20000))
	if err != nil {
		return nil, err
	}
	var recs []yearRecord
	if err := cur.All(ctx, &recs); err != nil {
		return nil, err
	}

	var movieIDs, episodeIDs []primitive.ObjectID
	for i := range recs {
		if recs[i].TargetType == "" || recs[i].TargetType == "movie" {
			if recs[i].TargetID.IsZero() {
				recs[i].TargetID = recs[i].MovieID
			}
			recs[i].TargetType = "movie"
			movieIDs = append(movieIDs, recs[i].TargetID)
		} else if recs[i].TargetType == "episode" {
			episodeIDs = append(episodeIDs, recs[i].TargetID)
		}
	}

	type meta struct {
		Title, Slug, Poster string
		Genres              []string
	}
	loadMeta := func(colName string, ids []primitive.ObjectID) map[primitive.ObjectID]meta {
		m := map[primitive.ObjectID]meta{}
		if len(ids) == 0 {
			return m
		}
		c, err := r.col.Database().Collection(colName).Find(ctx, bson.M{"_id": bson.M{"$in": ids}},
			options.Find().SetProjection(bson.M{"title": 1, "title_uz": 1, "slug": 1, "poster_url": 1, "genre": 1}))
		if err != nil {
			return m
		}
		var docs []struct {
			ID      primitive.ObjectID `bson:"_id"`
			Title   string             `bson:"title"`
			TitleUz string             `bson:"title_uz"`
			Slug    string             `bson:"slug"`
			Poster  string             `bson:"poster_url"`
			Genre   []string           `bson:"genre"`
		}
		if c.All(ctx, &docs) == nil {
			for _, d := range docs {
				t := d.TitleUz
				if t == "" {
					t = d.Title
				}
				m[d.ID] = meta{Title: t, Slug: d.Slug, Poster: d.Poster, Genres: d.Genre}
			}
		}
		return m
	}

	// Episodes → their series.
	episodeSeries := map[primitive.ObjectID]primitive.ObjectID{}
	if len(episodeIDs) > 0 {
		if c, err := r.episodeCol.Find(ctx, bson.M{"_id": bson.M{"$in": episodeIDs}}, options.Find().SetProjection(bson.M{"series_id": 1})); err == nil {
			var eps []struct {
				ID       primitive.ObjectID `bson:"_id"`
				SeriesID primitive.ObjectID `bson:"series_id"`
			}
			if c.All(ctx, &eps) == nil {
				for _, e := range eps {
					episodeSeries[e.ID] = e.SeriesID
				}
			}
		}
	}
	var seriesIDs []primitive.ObjectID
	seenSeries := map[primitive.ObjectID]bool{}
	for _, sid := range episodeSeries {
		if !seenSeries[sid] {
			seenSeries[sid] = true
			seriesIDs = append(seriesIDs, sid)
		}
	}
	movies := loadMeta("movies", movieIDs)
	series := loadMeta("series", seriesIDs)

	genres := map[string]int{}
	titleSeconds := map[string]int64{}
	titleInfo := map[string]YearTopTitle{}
	days := map[string]bool{}
	weekdays := make([]int, 7)
	night, dayCount := 0, 0

	for _, rec := range recs {
		secs := watchedSeconds(rec)
		out.TotalMinutes += secs / 60
		if rec.Completed || rec.ProgressPercent >= 90 {
			out.Completed++
		}
		t := rec.LastWatchedAt.In(reviewZone)
		out.Months[int(t.Month())-1]++
		days[t.Format("2006-01-02")] = true
		weekdays[int(t.Weekday())]++
		if h := t.Hour(); h >= 22 || h < 4 {
			night++
		} else {
			dayCount++
		}

		var key string
		var info YearTopTitle
		var gs []string
		switch rec.TargetType {
		case "movie":
			out.MoviesWatched++
			m, ok := movies[rec.TargetID]
			if !ok {
				continue
			}
			key = "movie:" + rec.TargetID.Hex()
			info = YearTopTitle{TargetType: "movie", TargetID: rec.TargetID.Hex(), Title: m.Title, Slug: m.Slug, PosterURL: m.Poster}
			gs = m.Genres
		case "episode":
			out.EpisodesWatched++
			sid, ok := episodeSeries[rec.TargetID]
			if !ok {
				continue
			}
			s, ok := series[sid]
			if !ok {
				continue
			}
			key = "series:" + sid.Hex()
			info = YearTopTitle{TargetType: "series", TargetID: sid.Hex(), Title: s.Title, Slug: s.Slug, PosterURL: s.Poster}
			if _, counted := titleInfo[key]; !counted {
				gs = s.Genres // count a series' genres once
			}
		default:
			continue
		}
		titleSeconds[key] += secs
		titleInfo[key] = info
		for _, g := range gs {
			if g = strings.ToLower(strings.TrimSpace(g)); g != "" {
				genres[g]++
			}
		}
	}
	out.SeriesWatched = len(seriesIDs)
	out.ActiveDays = len(days)
	out.TopGenres = topCounts(genres, 5)
	out.NightOwl = night > dayCount && night >= 5

	best, bestN := 0, 0
	for i, n := range out.Months {
		if n > bestN {
			best, bestN = i+1, n
		}
	}
	out.TopMonth = best
	bestN = 0
	for i, n := range weekdays {
		if n > bestN {
			out.TopWeekday, bestN = i, n
		}
	}

	keys := make([]string, 0, len(titleSeconds))
	for k := range titleSeconds {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if titleSeconds[keys[i]] != titleSeconds[keys[j]] {
			return titleSeconds[keys[i]] > titleSeconds[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		if len(out.TopTitles) == 5 {
			break
		}
		t := titleInfo[k]
		t.Minutes = titleSeconds[k] / 60
		out.TopTitles = append(out.TopTitles, t)
	}

	db := r.col.Database()
	inYear := bson.M{"user_id": userID, "created_at": bson.M{"$gte": start, "$lt": end}}
	out.Ratings, _ = db.Collection("movie_ratings").CountDocuments(ctx, inYear)
	out.Reviews, _ = db.Collection("reviews").CountDocuments(ctx, inYear)
	out.Comments, _ = db.Collection("movie_comments").CountDocuments(ctx, inYear)
	return out, nil
}

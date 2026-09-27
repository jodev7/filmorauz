package services

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CreditsService pulls cast + photos from TMDB into movies and series:
// automatically in the background for anything not fetched yet, and on
// demand from the admin editors.

const (
	CreditsOK       = "ok"
	CreditsNotFound = "not_found"
	CreditsError    = "error"
)

var ErrTMDBDisabled = errors.New("TMDB sozlanmagan (TMDB_API_KEY)")
var ErrCreditsTargetNotFound = errors.New("not found")

type CreditsService struct {
	db      *mongo.Database
	tmdb    *TMDBClient
	running sync.Mutex
}

func NewCreditsService(db *mongo.Database, tmdb *TMDBClient) *CreditsService {
	return &CreditsService{db: db, tmdb: tmdb}
}

func (s *CreditsService) Enabled() bool { return s != nil && s.tmdb != nil }

// EnsureIndexes for the people collection and the backfill query.
func (s *CreditsService) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := s.db.Collection("people").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "tmdb_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "name_lower", Value: 1}}},
	}); err != nil {
		return err
	}
	for _, col := range []string{"movies", "series"} {
		if _, err := s.db.Collection(col).Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "credits_status", Value: 1}, {Key: "credits_fetched_at", Value: 1}},
		}); err != nil {
			return err
		}
	}
	return nil
}

// CreditsResult is what the admin endpoint returns.
type CreditsResult struct {
	Status      string              `json:"status"`
	TMDBID      int                 `json:"tmdb_id,omitempty"`
	Cast        []string            `json:"cast"`
	Director    string              `json:"director"`
	DirectorURL string              `json:"director_profile_url,omitempty"`
	CastDetails []models.CastMember `json:"cast_details"`
}

type creditsTarget struct {
	Title, TitleUz, OriginalTitle, Director string
	Year, TMDBID                            int
	Cast                                    []string
}

// Legacy documents store numbers as strings/doubles, so decode loosely.
func looseInt(v interface{}) int {
	switch n := v.(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

func looseString(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func targetFromDoc(doc bson.M) creditsTarget {
	t := creditsTarget{
		Title: looseString(doc["title"]), TitleUz: looseString(doc["title_uz"]), OriginalTitle: looseString(doc["original_title"]),
		Director: looseString(doc["director"]), Year: looseInt(doc["year"]), TMDBID: looseInt(doc["tmdb_id"]),
	}
	if arr, ok := doc["cast"].(bson.A); ok {
		for _, c := range arr {
			if s := looseString(c); s != "" {
				t.Cast = append(t.Cast, s)
			}
		}
	}
	return t
}

// FetchMovie / FetchSeries fetch and store credits for one title. With
// force, admin-entered cast/director are replaced by TMDB's; otherwise they
// are only filled when empty.
// tmdbID > 0 means the admin picked the TMDB title by hand: it is used as
// is (and replaces a wrong stored tmdb_id) instead of searching.
func (s *CreditsService) FetchMovie(ctx context.Context, id primitive.ObjectID, force bool, tmdbID int) (*CreditsResult, error) {
	return s.fetch(ctx, "movies", "movie", id, force, tmdbID)
}

func (s *CreditsService) FetchSeries(ctx context.Context, id primitive.ObjectID, force bool, tmdbID int) (*CreditsResult, error) {
	return s.fetch(ctx, "series", "tv", id, force, tmdbID)
}

// SearchTMDB lists candidates for the admin picker.
func (s *CreditsService) SearchTMDB(ctx context.Context, kind, query string, year int) ([]TMDBSearchItem, error) {
	if !s.Enabled() {
		return nil, ErrTMDBDisabled
	}
	return s.tmdb.SearchList(ctx, kind, query, year)
}

func (s *CreditsService) fetch(ctx context.Context, colName, kind string, id primitive.ObjectID, force bool, pickedID int) (*CreditsResult, error) {
	if !s.Enabled() {
		return nil, ErrTMDBDisabled
	}
	col := s.db.Collection(colName)
	var doc bson.M
	if err := col.FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetProjection(bson.M{
		"title": 1, "title_uz": 1, "original_title": 1, "year": 1, "tmdb_id": 1, "cast": 1, "director": 1,
	})).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrCreditsTargetNotFound
		}
		return nil, err
	}
	t := targetFromDoc(doc)
	now := time.Now()
	mark := func(status string) {
		_, _ = col.UpdateByID(ctx, id, bson.M{"$set": bson.M{"credits_status": status, "credits_fetched_at": now}})
	}

	tmdbID := t.TMDBID
	if pickedID > 0 {
		tmdbID = pickedID
	}
	if tmdbID == 0 {
		found, err := s.tmdb.Search(ctx, kind, []string{t.OriginalTitle, t.Title, t.TitleUz}, t.Year)
		if err != nil {
			mark(CreditsError)
			return nil, err
		}
		if found == 0 {
			mark(CreditsNotFound)
			return &CreditsResult{Status: CreditsNotFound, Cast: t.Cast, Director: t.Director, CastDetails: []models.CastMember{}}, nil
		}
		tmdbID = found
	}
	cr, err := s.tmdb.Credits(ctx, kind, tmdbID)
	if errors.Is(err, errTMDBNotFound) && tmdbID != 0 {
		mark(CreditsNotFound)
		return &CreditsResult{Status: CreditsNotFound, Cast: t.Cast, Director: t.Director, CastDetails: []models.CastMember{}}, nil
	}
	if err != nil {
		mark(CreditsError)
		return nil, err
	}

	set := bson.M{
		"cast_details":       cr.Cast,
		"credits_status":     CreditsOK,
		"credits_fetched_at": now,
		"tmdb_id":            tmdbID,
	}
	res := &CreditsResult{Status: CreditsOK, TMDBID: tmdbID, Cast: t.Cast, Director: t.Director, CastDetails: cr.Cast}
	if force || len(t.Cast) == 0 {
		names := make([]string, 0, len(cr.Cast))
		for _, m := range cr.Cast {
			names = append(names, m.Name)
		}
		if len(names) > 0 {
			set["cast"] = names
			res.Cast = names
		}
	}
	if cr.Director != nil {
		if force || strings.TrimSpace(t.Director) == "" {
			set["director"] = cr.Director.Name
			res.Director = cr.Director.Name
		}
		if strings.EqualFold(strings.TrimSpace(res.Director), cr.Director.Name) {
			set["director_profile_url"] = cr.Director.ProfileURL
			res.DirectorURL = cr.Director.ProfileURL
		}
	}
	if _, err := col.UpdateByID(ctx, id, bson.M{"$set": set}); err != nil {
		return nil, err
	}
	people := append([]models.CastMember{}, cr.Cast...)
	if cr.Director != nil {
		people = append(people, *cr.Director)
	}
	s.upsertPeople(ctx, people, cr.Director)
	return res, nil
}

func (s *CreditsService) upsertPeople(ctx context.Context, people []models.CastMember, director *models.CastMember) {
	if len(people) == 0 {
		return
	}
	models_ := make([]mongo.WriteModel, 0, len(people))
	for _, p := range people {
		if p.TMDBID == 0 || p.Name == "" {
			continue
		}
		dept := "Acting"
		if director != nil && p.TMDBID == director.TMDBID {
			dept = "Directing"
		}
		set := bson.M{"name": p.Name, "name_lower": strings.ToLower(strings.Join(strings.Fields(p.Name), " ")), "updated_at": time.Now()}
		if p.ProfileURL != "" {
			set["profile_url"] = p.ProfileURL
		}
		models_ = append(models_, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"tmdb_id": p.TMDBID}).
			SetUpdate(bson.M{"$set": set, "$setOnInsert": bson.M{"tmdb_id": p.TMDBID, "department": dept}}).
			SetUpsert(true))
	}
	if len(models_) == 0 {
		return
	}
	if _, err := s.db.Collection("people").BulkWrite(ctx, models_, options.BulkWrite().SetOrdered(false)); err != nil {
		log.Printf("[CREDITS] people upsert: %v", err)
	}
}

// pendingFilter selects titles to (re)try: never fetched, "not found" more
// than two weeks ago, or a transient error more than an hour ago.
func pendingFilter(now time.Time) bson.M {
	return bson.M{"$or": []bson.M{
		{"credits_fetched_at": bson.M{"$exists": false}},
		{"credits_status": CreditsNotFound, "credits_fetched_at": bson.M{"$lt": now.Add(-14 * 24 * time.Hour)}},
		{"credits_status": CreditsError, "credits_fetched_at": bson.M{"$lt": now.Add(-time.Hour)}},
	}}
}

// Backfill processes up to `limit` pending movies and series. Returns how
// many were updated with credits. Only one run at a time.
func (s *CreditsService) Backfill(ctx context.Context, limit int) (int, error) {
	if !s.Enabled() {
		return 0, ErrTMDBDisabled
	}
	if !s.running.TryLock() {
		return 0, nil
	}
	defer s.running.Unlock()
	done := 0
	for _, kind := range []struct {
		col   string
		fetch func(context.Context, primitive.ObjectID, bool, int) (*CreditsResult, error)
	}{{"movies", s.FetchMovie}, {"series", s.FetchSeries}} {
		cur, err := s.db.Collection(kind.col).Find(ctx, pendingFilter(time.Now()),
			options.Find().SetProjection(bson.M{"_id": 1}).SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
		if err != nil {
			return done, err
		}
		var ids []struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		err = cur.All(ctx, &ids)
		if err != nil {
			return done, err
		}
		for _, d := range ids {
			if ctx.Err() != nil {
				return done, ctx.Err()
			}
			one, cancel := context.WithTimeout(ctx, 30*time.Second)
			res, err := kind.fetch(one, d.ID, false, 0)
			cancel()
			if err != nil {
				log.Printf("[CREDITS] %s %s: %v", kind.col, d.ID.Hex(), err)
			} else if res.Status == CreditsOK {
				done++
			}
			time.Sleep(250 * time.Millisecond) // stay far below TMDB rate limits
		}
	}
	return done, nil
}

// Start runs Backfill every 10 minutes (first run a minute after boot).
func (s *CreditsService) Start(ctx context.Context) {
	if !s.Enabled() {
		log.Printf("[CREDITS] TMDB disabled — set TMDB_API_KEY or TMDB_READ_TOKEN to fetch cast automatically")
		return
	}
	go func() {
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			n, err := s.Backfill(ctx, 40)
			if err != nil {
				log.Printf("[CREDITS] backfill: %v", err)
			} else if n > 0 {
				log.Printf("[CREDITS] backfill: %d title(s) updated", n)
			}
			timer.Reset(10 * time.Minute)
		}
	}()
}

// PersonPhoto returns the stored photo for a name (case-insensitive).
func (s *CreditsService) PersonPhoto(ctx context.Context, name string) *models.Person {
	var p models.Person
	key := strings.ToLower(strings.Join(strings.Fields(name), " "))
	if err := s.db.Collection("people").FindOne(ctx, bson.M{"name_lower": key}, options.FindOne().SetSort(bson.D{{Key: "profile_url", Value: -1}})).Decode(&p); err != nil {
		return nil
	}
	return &p
}

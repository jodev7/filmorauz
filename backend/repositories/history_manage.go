package repositories

import (
	"context"
	"errors"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// User-facing watch history management: hide an item from "continue
// watching", mark something as watched, delete entries, and per-series
// episode progress.

// ErrHistoryTargetNotFound is returned when the movie/episode doesn't exist.
var ErrHistoryTargetNotFound = errors.New("target not found")

// ValidHistoryTarget reports whether t is a watch-history target type.
func ValidHistoryTarget(t string) bool { return t == "movie" || t == "episode" }

// HideFromContinue removes an item from the "continue watching" row without
// touching the history itself. Watching it again brings it back.
func (r *WatchHistoryRepository) HideFromContinue(userID primitive.ObjectID, targetType string, targetID primitive.ObjectID) error {
	return r.setHidden(userID, targetType, targetID, true)
}

// RestoreToContinue undoes HideFromContinue.
func (r *WatchHistoryRepository) RestoreToContinue(userID primitive.ObjectID, targetType string, targetID primitive.ObjectID) error {
	return r.setHidden(userID, targetType, targetID, false)
}

func (r *WatchHistoryRepository) setHidden(userID primitive.ObjectID, targetType string, targetID primitive.ObjectID, hidden bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.col.UpdateMany(ctx, r.watchLookupFilter(userID, targetID, targetType),
		bson.M{"$set": bson.M{"hidden_from_continue": hidden}})
	return err
}

// MarkWatched records a movie or episode as fully watched.
func (r *WatchHistoryRepository) MarkWatched(userID primitive.ObjectID, targetType string, targetID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var durationMin int
	var seriesID, seasonID, episodeID *primitive.ObjectID
	switch targetType {
	case "movie":
		var m struct {
			Duration int `bson:"duration"`
		}
		if err := r.movieCol.FindOne(ctx, bson.M{"_id": targetID}, options.FindOne().SetProjection(bson.M{"duration": 1})).Decode(&m); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return ErrHistoryTargetNotFound
			}
			return err
		}
		durationMin = m.Duration
	case "episode":
		var e struct {
			SeriesID primitive.ObjectID `bson:"series_id"`
			SeasonID primitive.ObjectID `bson:"season_id"`
			Duration int                `bson:"duration"`
		}
		if err := r.episodeCol.FindOne(ctx, bson.M{"_id": targetID}, options.FindOne().SetProjection(bson.M{"series_id": 1, "season_id": 1, "duration": 1})).Decode(&e); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return ErrHistoryTargetNotFound
			}
			return err
		}
		durationMin = e.Duration
		seriesID, seasonID, episodeID = &e.SeriesID, &e.SeasonID, &targetID
	default:
		return ErrHistoryTargetNotFound
	}

	durationSec := int64(durationMin) * 60
	if existing, err := r.GetProgress(userID, targetID, targetType); err == nil && existing != nil && existing.Duration > 0 {
		durationSec = existing.Duration
	}
	if durationSec <= 0 {
		durationSec = 1
	}
	return r.MarkComplete(userID, targetID, targetType, seriesID, seasonID, episodeID, durationSec)
}

// DeleteEntry removes one title from the user's history entirely.
func (r *WatchHistoryRepository) DeleteEntry(userID primitive.ObjectID, targetType string, targetID primitive.ObjectID) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := r.col.DeleteMany(ctx, r.watchLookupFilter(userID, targetID, targetType))
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// ClearHistory removes the user's whole watch history.
func (r *WatchHistoryRepository) ClearHistory(userID primitive.ObjectID) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := r.col.DeleteMany(ctx, bson.M{"user_id": userID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// EpisodeProgress is the user's state for one episode.
type EpisodeProgress struct {
	EpisodeID       string  `json:"episode_id"`
	SeasonNumber    int     `json:"season_number"`
	EpisodeNumber   int     `json:"episode_number"`
	ProgressPercent float64 `json:"progress_percent"`
	LastPositionSec int64   `json:"last_position_sec"`
	Completed       bool    `json:"completed"`
}

// SeriesResume tells the series page which episode the "continue" button
// should open.
type SeriesResume struct {
	EpisodeID       string  `json:"episode_id"`
	SeasonNumber    int     `json:"season_number"`
	EpisodeNumber   int     `json:"episode_number"`
	Mode            string  `json:"mode"` // "continue" (mid-episode) | "next" (after a finished one)
	ProgressPercent float64 `json:"progress_percent"`
}

type SeriesProgress struct {
	Episodes []EpisodeProgress `json:"episodes"`
	Watched  int               `json:"watched"`
	Total    int               `json:"total"`
	Resume   *SeriesResume     `json:"resume,omitempty"`
}

type orderedEpisode struct {
	ID            primitive.ObjectID
	SeasonNumber  int
	EpisodeNumber int
}

// resumeTarget picks the episode to open: the last watched one if it is
// unfinished, otherwise the episode after it. nil when everything is done or
// nothing was watched.
func resumeTarget(order []orderedEpisode, progress map[primitive.ObjectID]EpisodeProgress, lastID primitive.ObjectID) *SeriesResume {
	if lastID.IsZero() {
		return nil
	}
	for i, ep := range order {
		if ep.ID != lastID {
			continue
		}
		p := progress[ep.ID]
		if !p.Completed && p.ProgressPercent > 0 {
			return &SeriesResume{EpisodeID: ep.ID.Hex(), SeasonNumber: ep.SeasonNumber, EpisodeNumber: ep.EpisodeNumber, Mode: "continue", ProgressPercent: p.ProgressPercent}
		}
		for _, next := range order[i+1:] {
			if !progress[next.ID].Completed {
				return &SeriesResume{EpisodeID: next.ID.Hex(), SeasonNumber: next.SeasonNumber, EpisodeNumber: next.EpisodeNumber, Mode: "next", ProgressPercent: progress[next.ID].ProgressPercent}
			}
		}
		return nil
	}
	return nil
}

// GetSeriesProgress returns the user's per-episode progress for one series.
func (r *WatchHistoryRepository) GetSeriesProgress(userID, seriesID primitive.ObjectID) (*SeriesProgress, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := r.col.Database()
	seasonNumbers := map[primitive.ObjectID]int{}
	cur, err := db.Collection("seasons").Find(ctx, bson.M{"series_id": seriesID}, options.Find().SetProjection(bson.M{"season_number": 1}))
	if err != nil {
		return nil, err
	}
	var seasons []struct {
		ID           primitive.ObjectID `bson:"_id"`
		SeasonNumber int                `bson:"season_number"`
	}
	if err := cur.All(ctx, &seasons); err != nil {
		return nil, err
	}
	for _, s := range seasons {
		seasonNumbers[s.ID] = s.SeasonNumber
	}

	cur, err = r.episodeCol.Find(ctx, bson.M{"series_id": seriesID}, options.Find().SetProjection(bson.M{"season_id": 1, "episode_number": 1}))
	if err != nil {
		return nil, err
	}
	var eps []struct {
		ID            primitive.ObjectID `bson:"_id"`
		SeasonID      primitive.ObjectID `bson:"season_id"`
		EpisodeNumber int                `bson:"episode_number"`
	}
	if err := cur.All(ctx, &eps); err != nil {
		return nil, err
	}
	order := make([]orderedEpisode, 0, len(eps))
	ids := make([]primitive.ObjectID, 0, len(eps))
	for _, e := range eps {
		order = append(order, orderedEpisode{ID: e.ID, SeasonNumber: seasonNumbers[e.SeasonID], EpisodeNumber: e.EpisodeNumber})
		ids = append(ids, e.ID)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].SeasonNumber != order[j].SeasonNumber {
			return order[i].SeasonNumber < order[j].SeasonNumber
		}
		return order[i].EpisodeNumber < order[j].EpisodeNumber
	})

	out := &SeriesProgress{Episodes: []EpisodeProgress{}, Total: len(order)}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err = r.col.Find(ctx,
		bson.M{"user_id": userID, "target_type": "episode", "target_id": bson.M{"$in": ids}},
		options.Find().SetSort(bson.D{{Key: "last_watched_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		TargetID        primitive.ObjectID `bson:"target_id"`
		ProgressPercent float64            `bson:"progress_percent"`
		LastPositionSec int64              `bson:"last_position_sec"`
		Completed       bool               `bson:"completed"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	byID := map[primitive.ObjectID]orderedEpisode{}
	for _, e := range order {
		byID[e.ID] = e
	}
	progress := map[primitive.ObjectID]EpisodeProgress{}
	var lastID primitive.ObjectID
	for i, row := range rows {
		if i == 0 {
			lastID = row.TargetID // rows are sorted by last_watched_at desc
		}
		if _, dup := progress[row.TargetID]; dup {
			continue
		}
		e := byID[row.TargetID]
		p := EpisodeProgress{
			EpisodeID:       row.TargetID.Hex(),
			SeasonNumber:    e.SeasonNumber,
			EpisodeNumber:   e.EpisodeNumber,
			ProgressPercent: row.ProgressPercent,
			LastPositionSec: row.LastPositionSec,
			Completed:       row.Completed || row.ProgressPercent >= 90,
		}
		progress[row.TargetID] = p
		out.Episodes = append(out.Episodes, p)
		if p.Completed {
			out.Watched++
		}
	}
	out.Resume = resumeTarget(order, progress, lastID)
	return out, nil
}

package services

import (
	"context"
	"fmt"
	"html"
	"log"
	"sort"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EpisodeNotifier tells series subscribers about newly playable episodes.
//
// It works off the episodes collection itself instead of hooking every code
// path that can add a video (admin create/update, ingestion worker, bulk
// season edit): an episode that has a playable source and no
// `subscribers_notified_at` stamp is "new". Episodes are grouped per series
// and only picked up once they've been quiet for a couple of minutes, so a
// whole season imported at once becomes ONE message per subscriber.
type EpisodeNotifier struct {
	db            *mongo.Database
	library       *repositories.LibraryRepository
	notifications *NotificationService
}

func NewEpisodeNotifier(db *mongo.Database, library *repositories.LibraryRepository, notifications *NotificationService) *EpisodeNotifier {
	return &EpisodeNotifier{db: db, library: library, notifications: notifications}
}

const (
	episodeNotifyInterval = 3 * time.Minute
	episodeNotifyQuiet    = 2 * time.Minute
	episodeNotifyBaseline = "episode_notify_baseline_v1"
)

func playableEpisodeFilter() bson.M {
	nonEmpty := func(f string) bson.M { return bson.M{f: bson.M{"$exists": true, "$nin": bson.A{"", nil}}} }
	return bson.M{"$or": bson.A{nonEmpty("video_url"), nonEmpty("embed_url"), nonEmpty("master_playlist_url")}}
}

// Start runs the notifier loop until ctx is cancelled.
func (n *EpisodeNotifier) Start(ctx context.Context) {
	if err := n.baseline(ctx); err != nil {
		log.Printf("[EPISODE-NOTIFY] baseline failed (will retry next start): %v", err)
		return
	}
	ticker := time.NewTicker(episodeNotifyInterval)
	defer ticker.Stop()
	for {
		n.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// baseline stamps every episode that is ALREADY playable when this feature
// first ships, so subscribers aren't flooded with the whole back catalogue.
// Runs once (recorded in the job_state collection).
func (n *EpisodeNotifier) baseline(ctx context.Context) error {
	state := n.db.Collection("job_state")
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if cnt, err := state.CountDocuments(cctx, bson.M{"_id": episodeNotifyBaseline}); err != nil {
		return err
	} else if cnt > 0 {
		return nil
	}
	filter := playableEpisodeFilter()
	filter["subscribers_notified_at"] = bson.M{"$exists": false}
	res, err := n.db.Collection("episodes").UpdateMany(cctx, filter, bson.M{"$set": bson.M{"subscribers_notified_at": time.Now()}})
	if err != nil {
		return err
	}
	if _, err := state.InsertOne(cctx, bson.M{"_id": episodeNotifyBaseline, "created_at": time.Now(), "stamped": res.ModifiedCount}); err != nil {
		return err
	}
	log.Printf("[EPISODE-NOTIFY] baseline stamped %d existing episodes", res.ModifiedCount)
	return nil
}

type pendingEpisode struct {
	ID            primitive.ObjectID `bson:"_id"`
	SeriesID      primitive.ObjectID `bson:"series_id"`
	SeasonID      primitive.ObjectID `bson:"season_id"`
	EpisodeNumber int                `bson:"episode_number"`
	Title         string             `bson:"title"`
}

func (n *EpisodeNotifier) tick(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	filter := playableEpisodeFilter()
	filter["subscribers_notified_at"] = bson.M{"$exists": false}
	filter["updated_at"] = bson.M{"$lte": time.Now().Add(-episodeNotifyQuiet)}

	cursor, err := n.db.Collection("episodes").Find(cctx, filter, options.Find().
		SetProjection(bson.M{"series_id": 1, "season_id": 1, "episode_number": 1, "title": 1}).
		SetLimit(1000))
	if err != nil {
		log.Printf("[EPISODE-NOTIFY] find pending: %v", err)
		return
	}
	var pending []pendingEpisode
	if err := cursor.All(cctx, &pending); err != nil {
		log.Printf("[EPISODE-NOTIFY] decode pending: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	bySeries := map[primitive.ObjectID][]pendingEpisode{}
	for _, ep := range pending {
		bySeries[ep.SeriesID] = append(bySeries[ep.SeriesID], ep)
	}
	for seriesID, eps := range bySeries {
		n.notifySeries(cctx, seriesID, eps)
	}
}

func (n *EpisodeNotifier) notifySeries(ctx context.Context, seriesID primitive.ObjectID, eps []pendingEpisode) {
	var series models.Series
	if err := n.db.Collection("series").FindOne(ctx, bson.M{"_id": seriesID}).Decode(&series); err != nil {
		if err == mongo.ErrNoDocuments {
			n.stamp(ctx, eps) // orphan episodes — nothing to announce
		}
		return
	}
	// Unpublished series: leave the episodes pending so subscribers hear
	// about them once the series goes live.
	if !series.IsPublished && series.ApprovalStatus != "" && series.ApprovalStatus != "approved" {
		return
	}

	// Stamp first (at-most-once): a crash mid-fan-out must not resend.
	n.stamp(ctx, eps)

	subscribers, err := n.library.SubscriberIDs(ctx, seriesID)
	if err != nil || len(subscribers) == 0 {
		return
	}

	// Resolve season numbers to build episode links.
	seasonIDs := map[primitive.ObjectID]bool{}
	for _, ep := range eps {
		seasonIDs[ep.SeasonID] = true
	}
	ids := make([]primitive.ObjectID, 0, len(seasonIDs))
	for id := range seasonIDs {
		ids = append(ids, id)
	}
	seasonNum := map[primitive.ObjectID]int{}
	if cur, err := n.db.Collection("seasons").Find(ctx, bson.M{"_id": bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{"season_number": 1})); err == nil {
		var rows []struct {
			ID           primitive.ObjectID `bson:"_id"`
			SeasonNumber int                `bson:"season_number"`
		}
		if cur.All(ctx, &rows) == nil {
			for _, r := range rows {
				seasonNum[r.ID] = r.SeasonNumber
			}
		}
	}

	// Newest = highest season, then highest episode number.
	sort.Slice(eps, func(i, j int) bool {
		si, sj := seasonNum[eps[i].SeasonID], seasonNum[eps[j].SeasonID]
		if si != sj {
			return si > sj
		}
		return eps[i].EpisodeNumber > eps[j].EpisodeNumber
	})
	latest := eps[0]
	latestSeason := seasonNum[latest.SeasonID]

	title := series.TitleUz
	if title == "" {
		title = series.Title
	}
	actionPath := "/series/" + series.Slug
	if latestSeason > 0 && latest.EpisodeNumber > 0 {
		actionPath = fmt.Sprintf("/series/%s/season/%d/episode/%d", series.Slug, latestSeason, latest.EpisodeNumber)
	}

	var notifTitle, notifMsg, tgText string
	if len(eps) == 1 && latestSeason > 0 {
		notifTitle = "Yangi qism chiqdi"
		notifMsg = fmt.Sprintf("«%s» — %d-fasl %d-qism tomosha qilishga tayyor.", title, latestSeason, latest.EpisodeNumber)
		tgText = fmt.Sprintf("🎬 <b>%s</b>\n%d-fasl %d-qism chiqdi!", html.EscapeString(title), latestSeason, latest.EpisodeNumber)
	} else {
		notifTitle = "Yangi qismlar qo'shildi"
		notifMsg = fmt.Sprintf("«%s» serialiga %d ta yangi qism qo'shildi.", title, len(eps))
		tgText = fmt.Sprintf("🎬 <b>%s</b>\n%d ta yangi qism qo'shildi!", html.EscapeString(title), len(eps))
	}

	sent := 0
	for _, userID := range subscribers {
		if ctx.Err() != nil {
			return
		}
		n.notifications.NotifyUserBoth(ctx, userID, models.NotificationNewEpisode, notifTitle, notifMsg, actionPath,
			map[string]interface{}{
				"series_id":      seriesID.Hex(),
				"series_slug":    series.Slug,
				"episode_count":  len(eps),
				"season_number":  latestSeason,
				"episode_number": latest.EpisodeNumber,
			},
			tgText, "▶️ Tomosha qilish")
		sent++
	}
	log.Printf("[EPISODE-NOTIFY] series %s: %d new episode(s) → %d subscriber(s)", series.Slug, len(eps), sent)
}

func (n *EpisodeNotifier) stamp(ctx context.Context, eps []pendingEpisode) {
	ids := make([]primitive.ObjectID, len(eps))
	for i, ep := range eps {
		ids[i] = ep.ID
	}
	if _, err := n.db.Collection("episodes").UpdateMany(ctx, bson.M{"_id": bson.M{"$in": ids}},
		bson.M{"$set": bson.M{"subscribers_notified_at": time.Now()}}); err != nil {
		log.Printf("[EPISODE-NOTIFY] stamp: %v", err)
	}
}

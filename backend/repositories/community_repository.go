package repositories

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ─── Comment reports ("shikoyat") ────────────────────────────────────────────

// CommentAutoHideReports: at this many distinct reports an approved comment
// is hidden (status → pending) until a moderator reviews it.
const CommentAutoHideReports = 3

var ErrAlreadyReported = errors.New("already reported")

type CommunityRepository struct {
	db       *mongo.Database
	reports  *mongo.Collection
	comments *mongo.Collection
	reviews  *mongo.Collection
}

func NewCommunityRepository(db *mongo.Database) *CommunityRepository {
	return &CommunityRepository{
		db:       db,
		reports:  db.Collection("comment_reports"),
		comments: db.Collection("movie_comments"),
		reviews:  db.Collection("reviews"),
	}
}

func (r *CommunityRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := r.reports.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "comment_id", Value: 1}, {Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)},
	}); err != nil {
		return err
	}
	_, err := r.reviews.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "target_type", Value: 1}, {Key: "target_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "target_type", Value: 1}, {Key: "target_id", Value: 1}, {Key: "helpful_count", Value: -1}}},
	})
	return err
}

// ReportComment records one user's report. Returns the new report count and
// whether the comment was auto-hidden by this report.
func (r *CommunityRepository) ReportComment(ctx context.Context, commentID, userID primitive.ObjectID, reason string) (int, bool, error) {
	_, err := r.reports.InsertOne(ctx, bson.M{
		"comment_id": commentID,
		"user_id":    userID,
		"reason":     reason,
		"created_at": time.Now(),
	})
	if mongo.IsDuplicateKeyError(err) {
		return 0, false, ErrAlreadyReported
	}
	if err != nil {
		return 0, false, err
	}
	var after struct {
		Status       string `bson:"status"`
		ReportsCount int    `bson:"reports_count"`
	}
	err = r.comments.FindOneAndUpdate(ctx, bson.M{"_id": commentID},
		bson.M{"$inc": bson.M{"reports_count": 1}, "$set": bson.M{"reported_at": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&after)
	if err != nil {
		return 0, false, err
	}
	hidden := false
	if after.ReportsCount >= CommentAutoHideReports && (after.Status == "" || after.Status == "approved") {
		if _, err := r.comments.UpdateOne(ctx, bson.M{"_id": commentID},
			bson.M{"$set": bson.M{"status": "pending", "auto_hidden_by_reports": true}}); err == nil {
			hidden = true
		}
	}
	return after.ReportsCount, hidden, nil
}

// ReportedComment is one row of the moderation queue.
type ReportedComment struct {
	ID           string    `json:"id"`
	Content      string    `json:"content"`
	Status       string    `json:"status"`
	AutoHidden   bool      `json:"auto_hidden"`
	ReportsCount int       `json:"reports_count"`
	Reasons      []string  `json:"reasons"`
	AuthorID     string    `json:"author_id"`
	AuthorName   string    `json:"author_name"`
	TargetType   string    `json:"target_type"`
	TargetID     string    `json:"target_id"`
	TargetTitle  string    `json:"target_title"`
	TargetSlug   string    `json:"target_slug"`
	CreatedAt    time.Time `json:"created_at"`
	ReportedAt   time.Time `json:"reported_at"`
}

func (r *CommunityRepository) CountReportedComments(ctx context.Context) int64 {
	n, _ := r.comments.CountDocuments(ctx, bson.M{"reports_count": bson.M{"$gt": 0}})
	return n
}

// ListReportedComments returns comments with open reports, most reported first.
func (r *CommunityRepository) ListReportedComments(ctx context.Context, limit int) ([]ReportedComment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	cur, err := r.comments.Find(ctx, bson.M{"reports_count": bson.M{"$gt": 0}},
		options.Find().SetSort(bson.D{{Key: "reports_count", Value: -1}, {Key: "reported_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]ReportedComment, 0, len(docs))
	for _, d := range docs {
		id, _ := d["_id"].(primitive.ObjectID)
		row := ReportedComment{
			ID:           id.Hex(),
			Content:      docString(d, "content"),
			Status:       docString(d, "status"),
			AutoHidden:   d["auto_hidden_by_reports"] == true,
			ReportsCount: int(docInt(d, "reports_count")),
			TargetType:   docString(d, "target_type"),
		}
		if t, ok := d["created_at"].(primitive.DateTime); ok {
			row.CreatedAt = t.Time()
		}
		if t, ok := d["reported_at"].(primitive.DateTime); ok {
			row.ReportedAt = t.Time()
		}
		// Reasons (distinct, most recent first).
		if rc, err := r.reports.Find(ctx, bson.M{"comment_id": id}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(20)); err == nil {
			var reps []struct {
				Reason string `bson:"reason"`
			}
			if rc.All(ctx, &reps) == nil {
				seen := map[string]bool{}
				for _, rp := range reps {
					if rp.Reason != "" && !seen[rp.Reason] {
						seen[rp.Reason] = true
						row.Reasons = append(row.Reasons, rp.Reason)
					}
				}
			}
		}
		// Author
		if uid, ok := d["user_id"].(primitive.ObjectID); ok {
			row.AuthorID = uid.Hex()
			var u bson.M
			if r.db.Collection("users").FindOne(ctx, bson.M{"_id": uid},
				options.FindOne().SetProjection(bson.M{"first_name": 1, "last_name": 1, "display_name": 1, "telegram_user": 1})).Decode(&u) == nil {
				row.AuthorName = displayNameOf(u)
			}
		}
		// Target (movie or episode → its series)
		targetID, _ := d["target_id"].(primitive.ObjectID)
		if targetID.IsZero() {
			targetID, _ = d["movie_id"].(primitive.ObjectID)
			row.TargetType = "movie"
		}
		row.TargetID = targetID.Hex()
		if row.TargetType == "episode" {
			var ep bson.M
			if r.db.Collection("episodes").FindOne(ctx, bson.M{"_id": targetID}).Decode(&ep) == nil {
				if sid, ok := ep["series_id"].(primitive.ObjectID); ok {
					var s bson.M
					if r.db.Collection("series").FindOne(ctx, bson.M{"_id": sid}).Decode(&s) == nil {
						row.TargetTitle = docString(s, "title")
						row.TargetSlug = docString(s, "slug")
					}
				}
			}
		} else {
			var m bson.M
			if r.db.Collection("movies").FindOne(ctx, bson.M{"_id": targetID},
				options.FindOne().SetProjection(bson.M{"title": 1, "slug": 1})).Decode(&m) == nil {
				row.TargetTitle = docString(m, "title")
				row.TargetSlug = docString(m, "slug")
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// DismissReports clears a comment's reports (moderator decided it's fine)
// and restores it if it had been auto-hidden.
func (r *CommunityRepository) DismissReports(ctx context.Context, commentID primitive.ObjectID) error {
	var d bson.M
	if err := r.comments.FindOne(ctx, bson.M{"_id": commentID}).Decode(&d); err != nil {
		return err
	}
	set := bson.M{"reports_count": 0, "reports_dismissed_at": time.Now(), "auto_hidden_by_reports": false}
	if d["auto_hidden_by_reports"] == true {
		set["status"] = "approved"
	}
	if _, err := r.comments.UpdateOne(ctx, bson.M{"_id": commentID}, bson.M{"$set": set}); err != nil {
		return err
	}
	_, err := r.reports.DeleteMany(ctx, bson.M{"comment_id": commentID})
	return err
}

func displayNameOf(u bson.M) string {
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "." || s == "-" {
			return ""
		}
		return s
	}
	if n := clean(strings.TrimSpace(docString(u, "first_name") + " " + docString(u, "last_name"))); n != "" {
		return n
	}
	if n := clean(docString(u, "display_name")); n != "" {
		return n
	}
	if n := clean(docString(u, "telegram_user")); n != "" {
		return "@" + n
	}
	return "User"
}

// ─── Reviews ("qisqa taqriz") ────────────────────────────────────────────────

const (
	ReviewMinLen = 10
	ReviewMaxLen = 500
)

// Review is a short written review with a star rating.
type Review struct {
	ID           string    `json:"id"`
	TargetType   string    `json:"target_type"`
	TargetID     string    `json:"target_id"`
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name"`
	UserAvatar   string    `json:"user_avatar,omitempty"`
	UserPremium  bool      `json:"user_premium"`
	Rating       int       `json:"rating"`
	Text         string    `json:"text"`
	HelpfulCount int       `json:"helpful_count"`
	HelpfulByMe  bool      `json:"helpful_by_me"`
	Mine         bool      `json:"mine"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (r *CommunityRepository) UpsertReview(ctx context.Context, userID primitive.ObjectID, targetType string, targetID primitive.ObjectID, rating int, text string) error {
	now := time.Now()
	_, err := r.reviews.UpdateOne(ctx,
		bson.M{"user_id": userID, "target_type": targetType, "target_id": targetID},
		bson.M{
			"$set":         bson.M{"rating": rating, "text": text, "updated_at": now},
			"$setOnInsert": bson.M{"created_at": now, "helpful_count": 0, "helpful_by": bson.A{}},
		},
		options.Update().SetUpsert(true))
	return err
}

func (r *CommunityRepository) DeleteReview(ctx context.Context, filter bson.M) (bool, error) {
	res, err := r.reviews.DeleteOne(ctx, filter)
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// ToggleHelpful flips the user's "foydali" mark; returns the new state.
func (r *CommunityRepository) ToggleHelpful(ctx context.Context, reviewID, userID primitive.ObjectID) (bool, int, error) {
	var d bson.M
	if err := r.reviews.FindOne(ctx, bson.M{"_id": reviewID}, options.FindOne().SetProjection(bson.M{"helpful_by": 1, "user_id": 1})).Decode(&d); err != nil {
		return false, 0, err
	}
	if owner, ok := d["user_id"].(primitive.ObjectID); ok && owner == userID {
		return false, 0, errors.New("o'z taqrizingizni belgilab bo'lmaydi")
	}
	marked := false
	if arr, ok := d["helpful_by"].(bson.A); ok {
		for _, v := range arr {
			if id, ok := v.(primitive.ObjectID); ok && id == userID {
				marked = true
				break
			}
		}
	}
	update := bson.M{"$addToSet": bson.M{"helpful_by": userID}, "$inc": bson.M{"helpful_count": 1}}
	if marked {
		update = bson.M{"$pull": bson.M{"helpful_by": userID}, "$inc": bson.M{"helpful_count": -1}}
	}
	var after struct {
		HelpfulCount int `bson:"helpful_count"`
	}
	if err := r.reviews.FindOneAndUpdate(ctx, bson.M{"_id": reviewID}, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&after); err != nil {
		return false, 0, err
	}
	return !marked, after.HelpfulCount, nil
}

// ListReviews returns reviews for a title plus the total count. `viewer`
// may be nil (guest).
func (r *CommunityRepository) ListReviews(ctx context.Context, targetType string, targetID primitive.ObjectID, sortBy string, limit int, viewer *primitive.ObjectID) ([]Review, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	filter := bson.M{"target_type": targetType, "target_id": targetID}
	total, err := r.reviews.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	sort := bson.D{{Key: "helpful_count", Value: -1}, {Key: "created_at", Value: -1}}
	if sortBy == "new" {
		sort = bson.D{{Key: "created_at", Value: -1}}
	}
	cur, err := r.reviews.Find(ctx, filter, options.Find().SetSort(sort).SetLimit(int64(limit)))
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, 0, err
	}

	// The viewer's own review is always included (and first), even if it
	// wouldn't make the page.
	if viewer != nil {
		mineIncluded := false
		for _, d := range docs {
			if uid, _ := d["user_id"].(primitive.ObjectID); uid == *viewer {
				mineIncluded = true
				break
			}
		}
		if !mineIncluded {
			var mine bson.M
			if r.reviews.FindOne(ctx, bson.M{"target_type": targetType, "target_id": targetID, "user_id": *viewer}).Decode(&mine) == nil {
				docs = append([]bson.M{mine}, docs...)
			}
		}
	}

	userIDs := []primitive.ObjectID{}
	for _, d := range docs {
		if uid, ok := d["user_id"].(primitive.ObjectID); ok {
			userIDs = append(userIDs, uid)
		}
	}
	users := map[primitive.ObjectID]bson.M{}
	if len(userIDs) > 0 {
		if uc, err := r.db.Collection("users").Find(ctx, bson.M{"_id": bson.M{"$in": userIDs}},
			options.Find().SetProjection(bson.M{"first_name": 1, "last_name": 1, "display_name": 1, "telegram_user": 1, "profile_image_url": 1, "photo_url": 1, "is_premium": 1, "premium_expires_at": 1})); err == nil {
			var us []bson.M
			if uc.All(ctx, &us) == nil {
				for _, u := range us {
					if id, ok := u["_id"].(primitive.ObjectID); ok {
						users[id] = u
					}
				}
			}
		}
	}

	out := make([]Review, 0, len(docs))
	for _, d := range docs {
		id, _ := d["_id"].(primitive.ObjectID)
		uid, _ := d["user_id"].(primitive.ObjectID)
		u := users[uid]
		rev := Review{
			ID:           id.Hex(),
			TargetType:   targetType,
			TargetID:     targetID.Hex(),
			UserID:       uid.Hex(),
			UserName:     displayNameOf(u),
			Rating:       int(docInt(d, "rating")),
			Text:         docString(d, "text"),
			HelpfulCount: int(docInt(d, "helpful_count")),
		}
		if u != nil {
			rev.UserAvatar = docString(u, "profile_image_url")
			if rev.UserAvatar == "" {
				rev.UserAvatar = docString(u, "photo_url")
			}
			if u["is_premium"] == true {
				if exp, ok := u["premium_expires_at"].(primitive.DateTime); !ok || exp.Time().After(time.Now()) {
					rev.UserPremium = true
				}
			}
		}
		if t, ok := d["created_at"].(primitive.DateTime); ok {
			rev.CreatedAt = t.Time()
		}
		if t, ok := d["updated_at"].(primitive.DateTime); ok {
			rev.UpdatedAt = t.Time()
		}
		if viewer != nil {
			rev.Mine = uid == *viewer
			if arr, ok := d["helpful_by"].(bson.A); ok {
				for _, v := range arr {
					if hid, ok := v.(primitive.ObjectID); ok && hid == *viewer {
						rev.HelpfulByMe = true
						break
					}
				}
			}
		}
		out = append(out, rev)
	}
	return out, total, nil
}

// ReviewOwner returns the author of a review (for staff delete checks).
func (r *CommunityRepository) ReviewByID(ctx context.Context, id primitive.ObjectID) (bson.M, error) {
	var d bson.M
	err := r.reviews.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	return d, err
}

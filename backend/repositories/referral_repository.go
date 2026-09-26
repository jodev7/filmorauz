package repositories

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Referral program ("do'stingni taklif qil").
//
// A referral is created when a brand-new account claims a friend's code, and
// is only REWARDED once that new user actually watches something — so fake
// sign-ups don't earn premium.

const referralAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I

var (
	ErrReferralInvalidCode  = errors.New("taklif kodi topilmadi")
	ErrReferralSelf         = errors.New("o'z kodingizdan foydalanib bo'lmaydi")
	ErrReferralAlreadyTaken = errors.New("hisob allaqachon taklif orqali ro'yxatdan o'tgan")
	ErrReferralTooLate      = errors.New("taklif kodi faqat yangi hisoblar uchun")
)

type ReferralRepository struct {
	users     *mongo.Collection
	referrals *mongo.Collection
	history   *mongo.Collection
}

func NewReferralRepository(db *mongo.Database) *ReferralRepository {
	return &ReferralRepository{
		users:     db.Collection("users"),
		referrals: db.Collection("referrals"),
		history:   db.Collection("watch_history"),
	}
}

func (r *ReferralRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := r.users.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "referral_code", Value: 1}},
		Options: options.Index().SetUnique(true).SetSparse(true),
	}); err != nil {
		return err
	}
	_, err := r.referrals.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "referred_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "referrer_id", Value: 1}, {Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: 1}}},
	})
	return err
}

func randomReferralCode(n int) (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(referralAlphabet)))
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(referralAlphabet[v.Int64()])
	}
	return b.String(), nil
}

// NormalizeReferralCode upper-cases and strips anything outside the alphabet.
func NormalizeReferralCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	var b strings.Builder
	for _, r := range code {
		if strings.ContainsRune(referralAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// GetOrCreateCode returns the user's referral code, creating one on first use.
func (r *ReferralRepository) GetOrCreateCode(ctx context.Context, userID primitive.ObjectID) (string, error) {
	var u struct {
		Code string `bson:"referral_code"`
	}
	if err := r.users.FindOne(ctx, bson.M{"_id": userID}, options.FindOne().SetProjection(bson.M{"referral_code": 1})).Decode(&u); err != nil {
		return "", err
	}
	if u.Code != "" {
		return u.Code, nil
	}
	for attempt := 0; attempt < 6; attempt++ {
		code, err := randomReferralCode(7)
		if err != nil {
			return "", err
		}
		res, err := r.users.UpdateOne(ctx,
			bson.M{"_id": userID, "referral_code": bson.M{"$exists": false}},
			bson.M{"$set": bson.M{"referral_code": code}})
		if mongo.IsDuplicateKeyError(err) {
			continue // collision — try another code
		}
		if err != nil {
			return "", err
		}
		if res.ModifiedCount == 0 {
			// Raced with another request that set it — read it back.
			return r.GetOrCreateCode(ctx, userID)
		}
		return code, nil
	}
	return "", errors.New("could not allocate referral code")
}

// Claim links a new account (referred) to the owner of code.
func (r *ReferralRepository) Claim(ctx context.Context, referredID primitive.ObjectID, code string, maxAccountAge time.Duration) (primitive.ObjectID, error) {
	code = NormalizeReferralCode(code)
	if code == "" {
		return primitive.NilObjectID, ErrReferralInvalidCode
	}
	var referrer struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := r.users.FindOne(ctx, bson.M{"referral_code": code}, options.FindOne().SetProjection(bson.M{"_id": 1})).Decode(&referrer); err != nil {
		return primitive.NilObjectID, ErrReferralInvalidCode
	}
	if referrer.ID == referredID {
		return primitive.NilObjectID, ErrReferralSelf
	}
	var me struct {
		CreatedAt  time.Time          `bson:"created_at"`
		ReferredBy primitive.ObjectID `bson:"referred_by"`
	}
	if err := r.users.FindOne(ctx, bson.M{"_id": referredID}, options.FindOne().SetProjection(bson.M{"created_at": 1, "referred_by": 1})).Decode(&me); err != nil {
		return primitive.NilObjectID, err
	}
	if !me.ReferredBy.IsZero() {
		return primitive.NilObjectID, ErrReferralAlreadyTaken
	}
	if me.CreatedAt.IsZero() || time.Since(me.CreatedAt) > maxAccountAge {
		return primitive.NilObjectID, ErrReferralTooLate
	}
	if _, err := r.referrals.InsertOne(ctx, bson.M{
		"referrer_id": referrer.ID,
		"referred_id": referredID,
		"code":        code,
		"status":      "pending",
		"created_at":  time.Now(),
	}); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return primitive.NilObjectID, ErrReferralAlreadyTaken
		}
		return primitive.NilObjectID, err
	}
	_, _ = r.users.UpdateOne(ctx, bson.M{"_id": referredID}, bson.M{"$set": bson.M{"referred_by": referrer.ID}})
	return referrer.ID, nil
}

// ReferralStats summarises a referrer's program state.
type ReferralStats struct {
	Invited    int64 `json:"invited"`
	Rewarded   int64 `json:"rewarded"`
	RewardDays int64 `json:"reward_days"`
}

func (r *ReferralRepository) Stats(ctx context.Context, referrerID primitive.ObjectID) (ReferralStats, error) {
	var s ReferralStats
	var err error
	if s.Invited, err = r.referrals.CountDocuments(ctx, bson.M{"referrer_id": referrerID}); err != nil {
		return s, err
	}
	if s.Rewarded, err = r.referrals.CountDocuments(ctx, bson.M{"referrer_id": referrerID, "status": "rewarded"}); err != nil {
		return s, err
	}
	cur, err := r.referrals.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"referrer_id": referrerID, "status": "rewarded"}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "days": bson.M{"$sum": "$referrer_reward_days"}}}},
	})
	if err == nil {
		var rows []struct {
			Days int64 `bson:"days"`
		}
		if cur.All(ctx, &rows) == nil && len(rows) > 0 {
			s.RewardDays = rows[0].Days
		}
	}
	return s, nil
}

// PendingReferral is a claimed, not-yet-rewarded referral.
type PendingReferral struct {
	ID         primitive.ObjectID `bson:"_id"`
	ReferrerID primitive.ObjectID `bson:"referrer_id"`
	ReferredID primitive.ObjectID `bson:"referred_id"`
	CreatedAt  time.Time          `bson:"created_at"`
}

// ActivatedPending returns pending referrals whose new user has watched
// something (the activation condition). Referrals older than maxAge expire.
func (r *ReferralRepository) ActivatedPending(ctx context.Context, maxAge time.Duration, limit int) ([]PendingReferral, error) {
	_, _ = r.referrals.UpdateMany(ctx,
		bson.M{"status": "pending", "created_at": bson.M{"$lt": time.Now().Add(-maxAge)}},
		bson.M{"$set": bson.M{"status": "expired"}})

	cur, err := r.referrals.Find(ctx, bson.M{"status": "pending"}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var pending []PendingReferral
	if err := cur.All(ctx, &pending); err != nil {
		return nil, err
	}
	out := pending[:0]
	for _, p := range pending {
		if n, _ := r.history.CountDocuments(ctx, bson.M{"user_id": p.ReferredID}, options.Count().SetLimit(1)); n > 0 {
			out = append(out, p)
		}
	}
	return out, nil
}

// RewardedThisMonth counts a referrer's rewards since the start of the month.
func (r *ReferralRepository) RewardedThisMonth(ctx context.Context, referrerID primitive.ObjectID) int64 {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	n, _ := r.referrals.CountDocuments(ctx, bson.M{"referrer_id": referrerID, "status": "rewarded", "rewarded_at": bson.M{"$gte": start}})
	return n
}

// MarkDone moves a referral out of "pending" exactly once (returns false if
// another worker already did).
func (r *ReferralRepository) MarkDone(ctx context.Context, id primitive.ObjectID, status string, referrerDays, referredDays int) (bool, error) {
	res, err := r.referrals.UpdateOne(ctx, bson.M{"_id": id, "status": "pending"}, bson.M{"$set": bson.M{
		"status":               status,
		"rewarded_at":          time.Now(),
		"referrer_reward_days": referrerDays,
		"referred_reward_days": referredDays,
	}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

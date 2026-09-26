package repositories

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Personal lists ("Oilaviy kechalar", "Eng yaxshi trillerlar"...): a user's
// own named collections of movies/series, shareable by link.

const (
	MaxListsPerUser = 30
	MaxItemsPerList = 200
	maxListTitle    = 60
	maxListDesc     = 300
)

var (
	ErrListNotFound   = errors.New("list not found")
	ErrListLimit      = errors.New("list limit reached")
	ErrListFull       = errors.New("list is full")
	ErrListTitle      = errors.New("title must be 1-60 characters")
	ErrListDesc       = errors.New("description is too long")
	ErrTargetNotFound = errors.New("content not found")
)

type UserList struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID      primitive.ObjectID `bson:"user_id" json:"user_id"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description,omitempty" json:"description,omitempty"`
	IsPublic    bool               `bson:"is_public" json:"is_public"`
	ShareSlug   string             `bson:"share_slug" json:"share_slug"`
	Items       []TargetRef        `bson:"items" json:"-"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

// UserListSummary is a list card: counts and up to 4 cover posters.
type UserListSummary struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	IsPublic    bool      `json:"is_public"`
	ShareSlug   string    `json:"share_slug"`
	Count       int       `json:"count"`
	Covers      []string  `json:"covers"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserListDetail is a list with its items for the list page.
type UserListDetail struct {
	UserListSummary
	OwnerID   string          `json:"owner_id"`
	OwnerName string          `json:"owner_name"`
	IsOwner   bool            `json:"is_owner"`
	Items     []WatchlistItem `json:"items"`
}

type UserListRepository struct {
	col     *mongo.Collection
	users   *mongo.Collection
	library *LibraryRepository
}

func NewUserListRepository(db *mongo.Database, library *LibraryRepository) *UserListRepository {
	return &UserListRepository{col: db.Collection("user_lists"), users: db.Collection("users"), library: library}
}

func (r *UserListRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "updated_at", Value: -1}}},
		{Keys: bson.D{{Key: "share_slug", Value: 1}}, Options: options.Index().SetUnique(true)},
	})
	return err
}

const slugAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func newShareSlug() string {
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(slugAlphabet))))
		b[i] = slugAlphabet[n.Int64()]
	}
	return string(b)
}

// CleanListText validates/normalizes a title and description.
func CleanListText(title, desc string) (string, string, error) {
	title = strings.Join(strings.Fields(title), " ")
	desc = strings.TrimSpace(desc)
	if title == "" || utf8.RuneCountInString(title) > maxListTitle {
		return "", "", ErrListTitle
	}
	if utf8.RuneCountInString(desc) > maxListDesc {
		return "", "", ErrListDesc
	}
	return title, desc, nil
}

func (r *UserListRepository) Create(ctx context.Context, userID primitive.ObjectID, title, desc string, public bool) (*UserList, error) {
	title, desc, err := CleanListText(title, desc)
	if err != nil {
		return nil, err
	}
	n, err := r.col.CountDocuments(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	if n >= MaxListsPerUser {
		return nil, ErrListLimit
	}
	now := time.Now()
	l := &UserList{
		ID: primitive.NewObjectID(), UserID: userID, Title: title, Description: desc,
		IsPublic: public, Items: []TargetRef{}, CreatedAt: now, UpdatedAt: now,
	}
	for attempt := 0; attempt < 5; attempt++ {
		l.ShareSlug = newShareSlug()
		_, err = r.col.InsertOne(ctx, l)
		if !mongo.IsDuplicateKeyError(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Update changes title/description/visibility of the user's own list.
func (r *UserListRepository) Update(ctx context.Context, userID, listID primitive.ObjectID, title, desc *string, public *bool) error {
	set := bson.M{"updated_at": time.Now()}
	if title != nil || desc != nil {
		cur, err := r.get(ctx, userID, listID)
		if err != nil {
			return err
		}
		t, d := cur.Title, cur.Description
		if title != nil {
			t = *title
		}
		if desc != nil {
			d = *desc
		}
		t, d, err = CleanListText(t, d)
		if err != nil {
			return err
		}
		set["title"], set["description"] = t, d
	}
	if public != nil {
		set["is_public"] = *public
	}
	res, err := r.col.UpdateOne(ctx, bson.M{"_id": listID, "user_id": userID}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrListNotFound
	}
	return nil
}

func (r *UserListRepository) Delete(ctx context.Context, userID, listID primitive.ObjectID) error {
	res, err := r.col.DeleteOne(ctx, bson.M{"_id": listID, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrListNotFound
	}
	return nil
}

func (r *UserListRepository) get(ctx context.Context, userID, listID primitive.ObjectID) (*UserList, error) {
	var l UserList
	err := r.col.FindOne(ctx, bson.M{"_id": listID, "user_id": userID}).Decode(&l)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrListNotFound
	}
	return &l, err
}

// AddItem appends a movie/series (no-op if already present; newest first).
func (r *UserListRepository) AddItem(ctx context.Context, userID, listID primitive.ObjectID, targetType string, targetID primitive.ObjectID) error {
	ok, err := r.library.TargetExists(ctx, targetType, targetID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTargetNotFound
	}
	now := time.Now()
	res, err := r.col.UpdateOne(ctx,
		bson.M{
			"_id": listID, "user_id": userID,
			"items":     bson.M{"$not": bson.M{"$elemMatch": bson.M{"target_type": targetType, "target_id": targetID}}},
			"items.199": bson.M{"$exists": false}, // < MaxItemsPerList
		},
		bson.M{
			"$push": bson.M{"items": bson.M{"$each": []TargetRef{{TargetType: targetType, TargetID: targetID, AddedAt: now}}, "$position": 0}},
			"$set":  bson.M{"updated_at": now},
		})
	if err != nil {
		return err
	}
	if res.MatchedCount > 0 {
		return nil
	}
	l, err := r.get(ctx, userID, listID)
	if err != nil {
		return err
	}
	for _, it := range l.Items {
		if it.TargetType == targetType && it.TargetID == targetID {
			return nil // already there
		}
	}
	return ErrListFull
}

func (r *UserListRepository) RemoveItem(ctx context.Context, userID, listID primitive.ObjectID, targetType string, targetID primitive.ObjectID) error {
	res, err := r.col.UpdateOne(ctx, bson.M{"_id": listID, "user_id": userID}, bson.M{
		"$pull": bson.M{"items": bson.M{"target_type": targetType, "target_id": targetID}},
		"$set":  bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrListNotFound
	}
	return nil
}

func (r *UserListRepository) summarize(ctx context.Context, l *UserList) UserListSummary {
	s := UserListSummary{
		ID: l.ID.Hex(), Title: l.Title, Description: l.Description, IsPublic: l.IsPublic,
		ShareSlug: l.ShareSlug, Count: len(l.Items), Covers: []string{}, UpdatedAt: l.UpdatedAt,
	}
	head := l.Items
	if len(head) > 4 {
		head = head[:4]
	}
	if items, err := r.library.HydrateTargets(ctx, head); err == nil {
		for _, it := range items {
			if it.PosterURL != "" {
				s.Covers = append(s.Covers, it.PosterURL)
			}
		}
	}
	return s
}

// ListMine returns the user's lists, most recently updated first.
func (r *UserListRepository) ListMine(ctx context.Context, userID primitive.ObjectID) ([]UserListSummary, error) {
	cur, err := r.col.Find(ctx, bson.M{"user_id": userID}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var lists []UserList
	if err := cur.All(ctx, &lists); err != nil {
		return nil, err
	}
	out := make([]UserListSummary, 0, len(lists))
	for i := range lists {
		out = append(out, r.summarize(ctx, &lists[i]))
	}
	return out, nil
}

// ListsContaining returns ids of the user's lists that hold the target.
func (r *UserListRepository) ListsContaining(ctx context.Context, userID primitive.ObjectID, targetType string, targetID primitive.ObjectID) ([]string, error) {
	cur, err := r.col.Find(ctx,
		bson.M{"user_id": userID, "items": bson.M{"$elemMatch": bson.M{"target_type": targetType, "target_id": targetID}}},
		options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID.Hex())
	}
	return ids, nil
}

// GetBySlug loads a list for its share page. Private lists are only visible
// to their owner (viewer may be NilObjectID for guests).
func (r *UserListRepository) GetBySlug(ctx context.Context, slug string, viewer primitive.ObjectID) (*UserListDetail, error) {
	var l UserList
	err := r.col.FindOne(ctx, bson.M{"share_slug": strings.ToLower(strings.TrimSpace(slug))}).Decode(&l)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrListNotFound
	}
	if err != nil {
		return nil, err
	}
	isOwner := !viewer.IsZero() && viewer == l.UserID
	if !l.IsPublic && !isOwner {
		return nil, ErrListNotFound
	}
	items, err := r.library.HydrateTargets(ctx, l.Items)
	if err != nil {
		return nil, err
	}
	d := &UserListDetail{UserListSummary: r.summarize(ctx, &l), OwnerID: l.UserID.Hex(), IsOwner: isOwner, Items: items}
	var owner struct {
		DisplayName string `bson:"display_name"`
		FirstName   string `bson:"first_name"`
		Username    string `bson:"username"`
	}
	if r.users.FindOne(ctx, bson.M{"_id": l.UserID}, options.FindOne().SetProjection(bson.M{"display_name": 1, "first_name": 1, "username": 1})).Decode(&owner) == nil {
		d.OwnerName = firstValidName(owner.DisplayName, owner.FirstName, owner.Username)
	}
	if d.OwnerName == "" {
		d.OwnerName = "User"
	}
	return d, nil
}

// firstValidName skips the placeholder names Telegram allows ("", ".", "-").
func firstValidName(names ...string) string {
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" && n != "." && n != "-" {
			return n
		}
	}
	return ""
}

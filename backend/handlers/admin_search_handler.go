package handlers

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AdminSearchResult is one hit in the admin command palette.
type AdminSearchResult struct {
	Kind     string `json:"kind"` // "movie" | "series" | "user"
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
}

func bsonString(d bson.M, key string) string {
	s, _ := d[key].(string)
	return s
}

func bsonID(d bson.M) string {
	if id, ok := d["_id"].(primitive.ObjectID); ok {
		return id.Hex()
	}
	return ""
}

func (h *AdminOverviewHandler) findRaw(ctx context.Context, collection string, filter bson.M, projection bson.M, limit int64) []bson.M {
	opts := options.Find().SetProjection(projection).SetLimit(limit).SetSort(bson.D{{Key: "created_at", Value: -1}})
	cursor, err := h.db.Collection(collection).Find(ctx, filter, opts)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)
	var out []bson.M
	if err := cursor.All(ctx, &out); err != nil {
		return nil
	}
	return out
}

// Search GET /api/admin/search?q=...
// Case-insensitive substring search over movies (any approval status),
// series and users, for the admin Ctrl+K palette.
func (h *AdminOverviewHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len([]rune(q)) < 2 {
		c.JSON(http.StatusOK, gin.H{"results": []AdminSearchResult{}})
		return
	}
	if len(q) > 100 {
		q = q[:100]
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rx := primitive.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}
	results := make([]AdminSearchResult, 0, 18)

	movies := h.findRaw(ctx, "movies", bson.M{"$or": bson.A{
		bson.M{"title": rx}, bson.M{"title_uz": rx}, bson.M{"original_title": rx},
		bson.M{"code": rx}, bson.M{"slug": rx},
	}}, bson.M{"title": 1, "year": 1, "code": 1, "approval_status": 1}, 6)
	for _, m := range movies {
		sub := "Kino"
		if code := bsonString(m, "code"); code != "" {
			sub += " · #" + code
		}
		if y, ok := m["year"].(int32); ok && y > 0 {
			sub += " · " + strconv.Itoa(int(y))
		}
		if st := bsonString(m, "approval_status"); st == "pending" || st == "rejected" {
			sub += " · " + st
		}
		results = append(results, AdminSearchResult{Kind: "movie", ID: bsonID(m), Title: bsonString(m, "title"), Subtitle: sub})
	}

	series := h.findRaw(ctx, "series", bson.M{"$or": bson.A{
		bson.M{"title": rx}, bson.M{"title_uz": rx}, bson.M{"slug": rx}, bson.M{"code": rx},
	}}, bson.M{"title": 1, "code": 1}, 6)
	for _, s := range series {
		sub := "Serial"
		if code := bsonString(s, "code"); code != "" {
			sub += " · #" + code
		}
		results = append(results, AdminSearchResult{Kind: "series", ID: bsonID(s), Title: bsonString(s, "title"), Subtitle: sub})
	}

	userOr := bson.A{
		bson.M{"first_name": rx}, bson.M{"last_name": rx},
		bson.M{"display_name": rx}, bson.M{"telegram_user": rx},
	}
	if tgID, err := strconv.ParseInt(strings.TrimPrefix(q, "@"), 10, 64); err == nil {
		userOr = append(userOr, bson.M{"telegram_id": tgID})
	}
	users := h.findRaw(ctx, "users", bson.M{"$or": userOr},
		bson.M{"first_name": 1, "last_name": 1, "display_name": 1, "telegram_user": 1, "telegram_id": 1, "role": 1}, 6)
	for _, u := range users {
		name := strings.TrimSpace(bsonString(u, "first_name") + " " + bsonString(u, "last_name"))
		if name == "" || name == "." || name == "-" {
			name = bsonString(u, "display_name")
		}
		username := bsonString(u, "telegram_user")
		if name == "" || name == "." || name == "-" {
			if username != "" {
				name = "@" + username
			} else {
				name = "User"
			}
		}
		sub := "Foydalanuvchi"
		if username != "" {
			sub += " · @" + username
		}
		if role := bsonString(u, "role"); role != "" && role != "user" {
			sub += " · " + role
		}
		results = append(results, AdminSearchResult{Kind: "user", ID: bsonID(u), Title: name, Subtitle: sub})
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

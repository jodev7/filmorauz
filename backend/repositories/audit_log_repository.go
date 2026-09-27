package repositories

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// auditLogRetention is how long admin audit entries are kept (TTL index).
const auditLogRetention = 180 * 24 * time.Hour

// AuditLog is one mutating request made through the admin API.
type AuditLog struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ActorID    string             `bson:"actor_id" json:"actor_id"`
	ActorRole  string             `bson:"actor_role" json:"actor_role"`
	Method     string             `bson:"method" json:"method"`
	Route      string             `bson:"route" json:"route"` // gin template, e.g. /api/admin/movies/:id
	Path       string             `bson:"path" json:"path"`   // concrete path
	Params     map[string]string  `bson:"params,omitempty" json:"params,omitempty"`
	Body       bson.M             `bson:"body,omitempty" json:"body,omitempty"` // redacted JSON body (small requests only)
	Status     int                `bson:"status" json:"status"`
	IP         string             `bson:"ip,omitempty" json:"ip,omitempty"`
	UserAgent  string             `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	DurationMS int64              `bson:"duration_ms" json:"duration_ms"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}

// AuditLogView is an AuditLog joined with a short actor description.
type AuditLogView struct {
	AuditLog `bson:",inline"`
	Actor    *struct {
		FirstName    string `bson:"first_name" json:"first_name,omitempty"`
		LastName     string `bson:"last_name" json:"last_name,omitempty"`
		DisplayName  string `bson:"display_name" json:"display_name,omitempty"`
		TelegramUser string `bson:"telegram_user" json:"username,omitempty"`
	} `bson:"actor,omitempty" json:"actor,omitempty"`
}

type AuditLogRepository struct {
	col *mongo.Collection
}

func NewAuditLogRepository(db *mongo.Database) *AuditLogRepository {
	return &AuditLogRepository{col: db.Collection("admin_audit_logs")}
}

func (r *AuditLogRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(auditLogRetention.Seconds())),
		},
		{Keys: bson.D{{Key: "actor_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "route", Value: 1}, {Key: "created_at", Value: -1}}},
	})
	return err
}

func (r *AuditLogRepository) Insert(ctx context.Context, entry *AuditLog) error {
	_, err := r.col.InsertOne(ctx, entry)
	return err
}

// AuditLogFilter narrows List results. Zero values mean "any".
type AuditLogFilter struct {
	ActorID string
	Method  string
	Query   string // substring of the concrete path or route
	Failed  bool   // only status >= 400
}

func (r *AuditLogRepository) List(ctx context.Context, f AuditLogFilter, page, limit int) ([]AuditLogView, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	match := bson.M{}
	if f.ActorID != "" {
		match["actor_id"] = f.ActorID
	}
	if f.Method != "" {
		match["method"] = strings.ToUpper(f.Method)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		rx := primitive.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}
		match["$or"] = bson.A{bson.M{"path": rx}, bson.M{"route": rx}}
	}
	if f.Failed {
		match["status"] = bson.M{"$gte": 400}
	}

	total, err := r.col.CountDocuments(ctx, match)
	if err != nil {
		return nil, 0, err
	}

	cursor, err := r.col.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}},
		{{Key: "$skip", Value: int64((page - 1) * limit)}},
		{{Key: "$limit", Value: int64(limit)}},
		{{Key: "$addFields", Value: bson.M{"actor_oid": bson.M{"$convert": bson.M{
			"input": "$actor_id", "to": "objectId", "onError": nil, "onNull": nil,
		}}}}},
		// Classic localField/foreignField $lookup + $arrayElemAt keep this
		// working on MongoDB 4.x (no lookup sub-pipeline / $first needed).
		{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "actor_oid",
			"foreignField": "_id",
			"as":           "actor",
		}}},
		// Keep only a few display fields of the joined user.
		{{Key: "$addFields", Value: bson.M{"actor": bson.M{
			"first_name":    bson.M{"$arrayElemAt": bson.A{"$actor.first_name", 0}},
			"last_name":     bson.M{"$arrayElemAt": bson.A{"$actor.last_name", 0}},
			"display_name":  bson.M{"$arrayElemAt": bson.A{"$actor.display_name", 0}},
			"telegram_user": bson.M{"$arrayElemAt": bson.A{"$actor.telegram_user", 0}},
		}}}},
		{{Key: "$project", Value: bson.M{"actor_oid": 0}}},
	})
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	// Decode entry by entry: one odd document (legacy field types, a
	// non-string name on the joined user...) must not fail the whole page.
	out := []AuditLogView{}
	for cursor.Next(ctx) {
		var v AuditLogView
		if err := cursor.Decode(&v); err == nil {
			out = append(out, v)
			continue
		} else {
			log.Printf("[AUDIT] lenient decode for %v: %v", cursor.Current.Lookup("_id"), err)
		}
		out = append(out, lenientAuditView(cursor.Current))
	}
	if err := cursor.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// rawToString renders any BSON scalar as plain text ("7", "true", ...).
func rawToString(val bson.RawValue) string {
	if s, ok := val.StringValueOK(); ok {
		return s
	}
	var x interface{}
	if val.Unmarshal(&x) != nil || x == nil {
		return ""
	}
	return fmt.Sprint(x)
}

// lenientAuditView builds a view from a raw document field by field,
// skipping anything whose type doesn't fit.
func lenientAuditView(raw bson.Raw) AuditLogView {
	var v AuditLogView
	str := func(key string) string {
		if val, err := raw.LookupErr(key); err == nil {
			return rawToString(val)
		}
		return ""
	}
	num := func(key string) int64 {
		if val, err := raw.LookupErr(key); err == nil {
			if n, ok := val.AsInt64OK(); ok {
				return n
			}
		}
		return 0
	}
	if val, err := raw.LookupErr("_id"); err == nil {
		v.ID, _ = val.ObjectIDOK()
	}
	v.ActorID = str("actor_id")
	v.ActorRole = str("actor_role")
	v.Method = str("method")
	v.Route = str("route")
	v.Path = str("path")
	v.IP = str("ip")
	v.UserAgent = str("user_agent")
	v.Status = int(num("status"))
	v.DurationMS = num("duration_ms")
	if val, err := raw.LookupErr("created_at"); err == nil {
		if t, ok := val.TimeOK(); ok {
			v.CreatedAt = t
		}
	}
	if val, err := raw.LookupErr("params"); err == nil {
		if doc, ok := val.DocumentOK(); ok {
			v.Params = map[string]string{}
			if elems, err := doc.Elements(); err == nil {
				for _, e := range elems {
					v.Params[e.Key()] = rawToString(e.Value())
				}
			}
		}
	}
	if val, err := raw.LookupErr("body"); err == nil {
		if doc, ok := val.DocumentOK(); ok {
			var m bson.M
			if bson.Unmarshal(doc, &m) == nil {
				v.Body = m
			}
		}
	}
	return v
}

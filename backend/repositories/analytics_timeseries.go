package repositories

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Tashkent has no DST, so a fixed +05:00 zone matches Mongo's "+05:00".
var tashkentZone = time.FixedZone("Asia/Tashkent", 5*60*60)

const tashkentMongoTZ = "+05:00"

// DailyPoint is one calendar day (Tashkent time) of dashboard metrics.
type DailyPoint struct {
	Date          string `json:"date"` // YYYY-MM-DD
	NewUsers      int64  `json:"new_users"`
	Views         int64  `json:"views"`
	ActiveViewers int64  `json:"active_viewers"`
	PremiumSales  int64  `json:"premium_sales"`
	StarsRevenue  int64  `json:"stars_revenue"`
}

// PeriodTotals summarises a window of DailyPoints.
type PeriodTotals struct {
	NewUsers         int64   `json:"new_users"`
	Views            int64   `json:"views"`
	AvgActiveViewers float64 `json:"avg_active_viewers"`
	PremiumSales     int64   `json:"premium_sales"`
	StarsRevenue     int64   `json:"stars_revenue"`
}

// DashboardTimeseries holds the last `Days` days plus totals for that window
// and for the equally long window right before it (for "vs previous" deltas).
type DashboardTimeseries struct {
	Days     int          `json:"days"`
	Series   []DailyPoint `json:"series"`
	Current  PeriodTotals `json:"current"`
	Previous PeriodTotals `json:"previous"`
}

type dayAgg struct {
	Day   string `bson:"_id"`
	Count int64  `bson:"count"`
	Sum   int64  `bson:"sum"`
}

func dayKeyExpr(field string) bson.M {
	return bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$" + field, "timezone": tashkentMongoTZ}}
}

// aggregateDaily groups documents matching `match` by calendar day of
// `dateField`, counting them and optionally summing `sumField`.
func aggregateDaily(ctx context.Context, col *mongo.Collection, match bson.M, dateField, sumField string) (map[string]dayAgg, error) {
	group := bson.M{"_id": dayKeyExpr(dateField), "count": bson.M{"$sum": 1}}
	if sumField != "" {
		group["sum"] = bson.M{"$sum": "$" + sumField}
	} else {
		group["sum"] = bson.M{"$sum": 0}
	}
	cursor, err := col.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: group}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rows []dayAgg
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make(map[string]dayAgg, len(rows))
	for _, r := range rows {
		out[r.Day] = r
	}
	return out, nil
}

// DashboardTimeseries builds per-day metrics for the last `days` days
// (inclusive of today, Tashkent time) and the totals of the previous period.
func (r *AnalyticsRepository) DashboardTimeseries(ctx context.Context, days int) (*DashboardTimeseries, error) {
	if days <= 0 {
		days = 30
	}
	if days > 180 {
		days = 180
	}

	now := time.Now().In(tashkentZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tashkentZone)
	currentStart := today.AddDate(0, 0, -(days - 1))
	prevStart := currentStart.AddDate(0, 0, -days)
	since := bson.M{"$gte": prevStart}

	users, err := aggregateDaily(ctx, r.users, bson.M{"created_at": since}, "created_at", "")
	if err != nil {
		return nil, fmt.Errorf("daily users: %w", err)
	}
	views, err := aggregateDaily(ctx, r.contentViewEvents, bson.M{"created_at": since}, "created_at", "")
	if err != nil {
		return nil, fmt.Errorf("daily views: %w", err)
	}
	sales, err := aggregateDaily(ctx, r.premiumPayments, bson.M{"created_at": since, "status": "succeeded"}, "created_at", "stars_amount")
	if err != nil {
		return nil, fmt.Errorf("daily sales: %w", err)
	}

	// Distinct viewers per day: logged-in user id, else IP for anonymous.
	viewerCursor, err := r.contentViewEvents.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"created_at": since}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"day":    dayKeyExpr("created_at"),
				"viewer": bson.M{"$ifNull": bson.A{"$user_id", "$ip"}},
			},
		}}},
		{{Key: "$group", Value: bson.M{"_id": "$_id.day", "count": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, fmt.Errorf("daily viewers: %w", err)
	}
	defer viewerCursor.Close(ctx)
	var viewerRows []dayAgg
	if err := viewerCursor.All(ctx, &viewerRows); err != nil {
		return nil, fmt.Errorf("decode daily viewers: %w", err)
	}
	viewers := make(map[string]int64, len(viewerRows))
	for _, v := range viewerRows {
		viewers[v.Day] = v.Count
	}

	out := &DashboardTimeseries{Days: days, Series: make([]DailyPoint, 0, days)}
	var prevViewerSum, curViewerSum int64
	for d := prevStart; !d.After(today); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		p := DailyPoint{
			Date:          key,
			NewUsers:      users[key].Count,
			Views:         views[key].Count,
			ActiveViewers: viewers[key],
			PremiumSales:  sales[key].Count,
			StarsRevenue:  sales[key].Sum,
		}
		t := &out.Previous
		if !d.Before(currentStart) {
			t = &out.Current
			out.Series = append(out.Series, p)
			curViewerSum += p.ActiveViewers
		} else {
			prevViewerSum += p.ActiveViewers
		}
		t.NewUsers += p.NewUsers
		t.Views += p.Views
		t.PremiumSales += p.PremiumSales
		t.StarsRevenue += p.StarsRevenue
	}
	out.Current.AvgActiveViewers = float64(curViewerSum) / float64(days)
	out.Previous.AvgActiveViewers = float64(prevViewerSum) / float64(days)
	return out, nil
}

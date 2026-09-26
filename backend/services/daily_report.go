package services

import (
	"context"
	"fmt"
	"html"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Daily admin report sent to Telegram every morning (Tashkent time).
//
// Env:
//
//	DAILY_REPORT_ENABLED   "false" turns the scheduled send off (default on)
//	DAILY_REPORT_HOUR      hour of day, Tashkent time, 0-23 (default 9)
//	DAILY_REPORT_CHAT_IDS  comma-separated chat ids; default: every superadmin
//	                       with a Telegram id, plus ADMIN_TELEGRAM_ID

var reportZone = time.FixedZone("Asia/Tashkent", 5*60*60)

var uzMonths = [...]string{"yanvar", "fevral", "mart", "aprel", "may", "iyun", "iyul", "avgust", "sentabr", "oktabr", "noyabr", "dekabr"}
var uzWeekdays = [...]string{"yakshanba", "dushanba", "seshanba", "chorshanba", "payshanba", "juma", "shanba"}

// StarsUSDRate is the approximate USD value of one Telegram Star, used only
// for rough revenue estimates. Override with STARS_USD_RATE.
func StarsUSDRate() float64 {
	if v := strings.TrimSpace(os.Getenv("STARS_USD_RATE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return 0.013
}

// DailyReportTop is one line of the "most watched" list.
type DailyReportTop struct {
	Title string
	Views int64
}

// DailyReportData is everything the report shows; Format turns it into the
// Telegram HTML message.
type DailyReportData struct {
	Day time.Time // the reported calendar day (Tashkent)

	NewUsers      int64
	Views         int64
	ActiveViewers int64
	PremiumSales  int64
	StarsRevenue  int64
	StarsUSDRate  float64

	// Averages of the 7 days before Day, for the ↑/↓ percentages.
	AvgNewUsers      float64
	AvgViews         float64
	AvgActiveViewers float64

	Top []DailyReportTop

	ErrorGroups    int64 // unresolved groups seen on Day
	NewErrorGroups int64 // of those, first seen on Day

	PendingMovies      int64
	PendingSuggestions int64
	ReportedComments   int64
	PendingAppeals     int64
	PlaybackReports    int64
	ScheduledToday     int64
}

func formatCount(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// trend renders the change against the 7-day average ("↑12%", "↓3%", "≈").
func trend(value int64, avg float64) string {
	if avg < 1 {
		return ""
	}
	pct := (float64(value) - avg) / avg * 100
	switch {
	case math.Abs(pct) < 1:
		return "  ≈"
	case pct > 0:
		return fmt.Sprintf("  ↑%.0f%%", pct)
	default:
		return fmt.Sprintf("  ↓%.0f%%", -pct)
	}
}

// Format renders the report as Telegram HTML (all dynamic text escaped).
func (d DailyReportData) Format() string {
	var b strings.Builder
	day := d.Day.In(reportZone)
	fmt.Fprintf(&b, "📊 <b>FilmoraUz — kunlik hisobot</b>\n%d-%s, %s\n\n",
		day.Day(), uzMonths[day.Month()-1], uzWeekdays[day.Weekday()])

	fmt.Fprintf(&b, "👤 Yangi foydalanuvchilar: <b>%s</b>%s\n", formatCount(d.NewUsers), trend(d.NewUsers, d.AvgNewUsers))
	fmt.Fprintf(&b, "▶️ Ko'rishlar: <b>%s</b>%s\n", formatCount(d.Views), trend(d.Views, d.AvgViews))
	fmt.Fprintf(&b, "👥 Faol tomoshabinlar: <b>%s</b>%s\n", formatCount(d.ActiveViewers), trend(d.ActiveViewers, d.AvgActiveViewers))
	sales := fmt.Sprintf("⭐ Premium sotuvlar: <b>%s</b> ta", formatCount(d.PremiumSales))
	if d.StarsRevenue > 0 {
		sales += fmt.Sprintf(" · %s ⭐ (~$%.2f)", formatCount(d.StarsRevenue), float64(d.StarsRevenue)*d.StarsUSDRate)
	}
	b.WriteString(sales + "\n")

	if len(d.Top) > 0 {
		b.WriteString("\n🔥 <b>Eng ko'p ko'rilgan</b>\n")
		for i, t := range d.Top {
			title := strings.TrimSpace(t.Title)
			if title == "" {
				title = "Nomsiz"
			}
			fmt.Fprintf(&b, "%d. %s — %s\n", i+1, html.EscapeString(title), formatCount(t.Views))
		}
	}

	b.WriteString("\n")
	if d.ErrorGroups > 0 {
		line := fmt.Sprintf("🐞 Xatolar: %s ta guruh", formatCount(d.ErrorGroups))
		if d.NewErrorGroups > 0 {
			line += fmt.Sprintf(" (%s ta yangi)", formatCount(d.NewErrorGroups))
		}
		b.WriteString(line + "\n")
	} else {
		b.WriteString("🐞 Xatolar: yo'q ✅\n")
	}

	var queue []string
	add := func(n int64, label string) {
		if n > 0 {
			queue = append(queue, formatCount(n)+" "+label)
		}
	}
	add(d.PendingMovies, "kino tasdiqda")
	add(d.PendingSuggestions, "taklif")
	add(d.ReportedComments, "shikoyatli komment")
	add(d.PendingAppeals, "ban murojaati")
	add(d.PlaybackReports, "pleyer muammosi")
	if len(queue) > 0 {
		b.WriteString("📥 Navbatda: " + strings.Join(queue, " · ") + "\n")
	} else {
		b.WriteString("📥 Navbatda hech narsa yo'q ✅\n")
	}
	if d.ScheduledToday > 0 {
		fmt.Fprintf(&b, "🗓 Bugun e'lon qilinadi: %s ta kino\n", formatCount(d.ScheduledToday))
	}

	b.WriteString("\n<i>Foizlar oldingi 7 kun o'rtachasiga nisbatan.</i>")
	return b.String()
}

// DailyReporter collects the numbers and delivers the report.
type DailyReporter struct {
	DB              *mongo.Database
	Analytics       *repositories.AnalyticsRepository
	Notify          *NotificationService
	AdminTelegramID int64
}

func dailyReportEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DAILY_REPORT_ENABLED")))
	return v != "false" && v != "0" && v != "off"
}

func dailyReportHour() int {
	if h, err := strconv.Atoi(strings.TrimSpace(os.Getenv("DAILY_REPORT_HOUR"))); err == nil && h >= 0 && h <= 23 {
		return h
	}
	return 9
}

func (d *DailyReporter) count(ctx context.Context, col string, filter bson.M) int64 {
	n, err := d.DB.Collection(col).CountDocuments(ctx, filter)
	if err != nil {
		log.Printf("[DAILY REPORT] count %s: %v", col, err)
		return 0
	}
	return n
}

// Collect gathers the report for the calendar day before `now` (Tashkent).
func (d *DailyReporter) Collect(ctx context.Context, now time.Time) (*DailyReportData, error) {
	local := now.In(reportZone)
	todayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, reportZone)
	dayStart := todayStart.AddDate(0, 0, -1)

	// 9 days = the 7 comparison days + the reported day + today.
	ts, err := d.Analytics.DashboardTimeseries(ctx, 9)
	if err != nil {
		return nil, err
	}
	out := &DailyReportData{Day: dayStart, StarsUSDRate: StarsUSDRate()}
	key := dayStart.Format("2006-01-02")
	var n float64
	for _, p := range ts.Series {
		if p.Date == key {
			out.NewUsers, out.Views, out.ActiveViewers = p.NewUsers, p.Views, p.ActiveViewers
			out.PremiumSales, out.StarsRevenue = p.PremiumSales, p.StarsRevenue
			continue
		}
		if p.Date < key {
			out.AvgNewUsers += float64(p.NewUsers)
			out.AvgViews += float64(p.Views)
			out.AvgActiveViewers += float64(p.ActiveViewers)
			n++
		}
	}
	if n > 0 {
		out.AvgNewUsers /= n
		out.AvgViews /= n
		out.AvgActiveViewers /= n
	}

	if top, err := d.Analytics.TopContentBetween(ctx, dayStart, todayStart, 1, 3); err == nil {
		for _, t := range top {
			out.Top = append(out.Top, DailyReportTop{Title: t.Title, Views: t.Views})
		}
	}

	dayRange := bson.M{"$gte": dayStart, "$lt": todayStart}
	out.ErrorGroups = d.count(ctx, "error_groups", bson.M{"resolved": false, "last_seen": dayRange})
	out.NewErrorGroups = d.count(ctx, "error_groups", bson.M{"resolved": false, "first_seen": dayRange})

	out.PendingMovies = d.count(ctx, "movies", bson.M{"approval_status": "pending"})
	out.PendingSuggestions = d.count(ctx, "suggestions", bson.M{"status": models.SuggestionStatusPending})
	out.ReportedComments = d.count(ctx, "movie_comments", bson.M{"reports_count": bson.M{"$gt": 0}})
	out.PendingAppeals = d.count(ctx, "ban_appeals", bson.M{"status": models.BanAppealStatusPending})
	out.PlaybackReports = d.count(ctx, "playback_reports", bson.M{"status": "new"})
	out.ScheduledToday = d.count(ctx, "movies", bson.M{"scheduled_publish_at": bson.M{
		"$gte": todayStart, "$lt": todayStart.AddDate(0, 0, 1),
	}})
	return out, nil
}

// Recipients returns the chat ids the report goes to.
func (d *DailyReporter) Recipients(ctx context.Context) []int64 {
	seen := map[int64]bool{}
	var out []int64
	add := func(id int64) {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("DAILY_REPORT_CHAT_IDS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				add(id)
			}
		}
		return out
	}
	cursor, err := d.DB.Collection("users").Find(ctx,
		bson.M{"role": "superadmin"},
		options.Find().SetProjection(bson.M{"telegram_id": 1, "telegram_chat_id": 1}))
	if err == nil {
		var rows []struct {
			TelegramID     int64 `bson:"telegram_id"`
			TelegramChatID int64 `bson:"telegram_chat_id"`
		}
		if cursor.All(ctx, &rows) == nil {
			for _, r := range rows {
				if r.TelegramChatID != 0 {
					add(r.TelegramChatID)
				} else {
					add(r.TelegramID)
				}
			}
		}
	}
	add(d.AdminTelegramID)
	return out
}

// Send collects and delivers the report now; returns how many chats got it.
func (d *DailyReporter) Send(ctx context.Context, now time.Time) (sent, recipients int, text string, err error) {
	data, err := d.Collect(ctx, now)
	if err != nil {
		return 0, 0, "", err
	}
	text = data.Format()
	chats := d.Recipients(ctx)
	for _, id := range chats {
		if d.Notify != nil && d.Notify.SendTelegramToChat(id, text, "📊 Dashboard", "/admin/dashboard") {
			sent++
		}
	}
	return sent, len(chats), text, nil
}

// claimDay marks `day` as sent; false when another tick/instance already did.
func (d *DailyReporter) claimDay(ctx context.Context, day string) (bool, error) {
	res, err := d.DB.Collection("system_state").UpdateOne(ctx,
		bson.M{"_id": "daily_report", "last_sent_day": bson.M{"$ne": day}},
		bson.M{"$set": bson.M{"last_sent_day": day, "sent_at": time.Now()}},
		options.Update().SetUpsert(true),
	)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil // doc exists and already has today's day
	}
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0 || res.UpsertedCount > 0, nil
}

// Start checks every few minutes and sends once per day after the hour.
func (d *DailyReporter) Start() {
	if d == nil || d.DB == nil || d.Analytics == nil {
		return
	}
	if !dailyReportEnabled() {
		log.Printf("[DAILY REPORT] disabled (DAILY_REPORT_ENABLED=false)")
		return
	}
	hour := dailyReportHour()
	log.Printf("[DAILY REPORT] enabled — daily at %02d:00 Tashkent", hour)
	go func() {
		time.Sleep(time.Minute)
		for {
			d.tick(time.Now(), hour)
			time.Sleep(5 * time.Minute)
		}
	}()
}

func (d *DailyReporter) tick(now time.Time, hour int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DAILY REPORT] panic: %v", r)
		}
	}()
	local := now.In(reportZone)
	if local.Hour() < hour {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ok, err := d.claimDay(ctx, local.Format("2006-01-02"))
	if err != nil {
		log.Printf("[DAILY REPORT] claim: %v", err)
		return
	}
	if !ok {
		return
	}
	sent, total, _, err := d.Send(ctx, now)
	if err != nil {
		log.Printf("[DAILY REPORT] send failed: %v", err)
		return
	}
	log.Printf("[DAILY REPORT] sent to %d/%d chats", sent, total)
}

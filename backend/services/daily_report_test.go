package services

import (
	"strings"
	"testing"
	"time"
)

func TestFormatCount(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1 000", 5430: "5 430", 1234567: "1 234 567", -1200: "-1 200"}
	for in, want := range cases {
		if got := formatCount(in); got != want {
			t.Errorf("formatCount(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestTrend(t *testing.T) {
	if got := trend(120, 100); got != "  ↑20%" {
		t.Errorf("up: %q", got)
	}
	if got := trend(97, 100); got != "  ↓3%" {
		t.Errorf("down: %q", got)
	}
	if got := trend(100, 100.4); got != "  ≈" {
		t.Errorf("flat: %q", got)
	}
	if got := trend(5, 0); got != "" {
		t.Errorf("no baseline: %q", got)
	}
}

func TestDailyReportFormat(t *testing.T) {
	day := time.Date(2026, 9, 24, 0, 0, 0, 0, reportZone) // Thursday
	d := DailyReportData{
		Day: day, NewUsers: 120, Views: 5430, ActiveViewers: 1200,
		PremiumSales: 4, StarsRevenue: 1000, StarsUSDRate: 0.013,
		AvgNewUsers: 100, AvgViews: 5600,
		Top:         []DailyReportTop{{Title: "Tom & Jerry <3>", Views: 340}},
		ErrorGroups: 3, NewErrorGroups: 1,
		PendingMovies: 5, ReportedComments: 1,
		ScheduledToday: 2,
	}
	out := d.Format()
	for _, want := range []string{
		"24-sentabr, payshanba",
		"Yangi foydalanuvchilar: <b>120</b>  ↑20%",
		"Ko'rishlar: <b>5 430</b>  ↓3%",
		"<b>4</b> ta · 1 000 ⭐ (~$13.00)",
		"1. Tom &amp; Jerry &lt;3&gt; — 340",
		"Xatolar: 3 ta guruh (1 ta yangi)",
		"Navbatda: 5 kino tasdiqda · 1 shikoyatli komment",
		"Bugun e'lon qilinadi: 2 ta kino",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "Faol tomoshabinlar: <b>1 200</b>  ") {
		t.Errorf("active viewers should have no trend without a baseline:\n%s", out)
	}

	empty := DailyReportData{Day: day}.Format()
	for _, want := range []string{"Xatolar: yo'q ✅", "Navbatda hech narsa yo'q ✅"} {
		if !strings.Contains(empty, want) {
			t.Errorf("empty report missing %q\n%s", want, empty)
		}
	}
	if strings.Contains(empty, "Eng ko'p") || strings.Contains(empty, "Bugun e'lon") {
		t.Errorf("empty report shows empty sections:\n%s", empty)
	}
}

package services

import (
	"context"
	"fmt"
	"html"
	"log"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
)

// ── Admin list / schedule passthroughs ──

// ListAdminMovies returns one server-side page of the admin movie list.
func (s *MovieService) ListAdminMovies(q repositories.AdminMovieQuery) ([]models.Movie, int64, error) {
	return s.repo.ListAdminQuery(q)
}

// AdminMovieCounts returns the status-tab counters for the admin list.
func (s *MovieService) AdminMovieCounts(q repositories.AdminMovieQuery) (*repositories.AdminMovieStatusCounts, error) {
	return s.repo.AdminStatusCounts(q)
}

// ScheduleMovie schedules an unpublished movie to go live at `at`.
func (s *MovieService) ScheduleMovie(id string, at time.Time, byUserID string) error {
	return s.repo.SetSchedule(id, at, byUserID)
}

// CancelMovieSchedule removes a pending schedule.
func (s *MovieService) CancelMovieSchedule(id string) error {
	return s.repo.ClearSchedule(id)
}

// ── Telegram announcement shared by manual approve and the scheduler ──

// AnnounceApprovedMovie posts a just-approved movie to the Telegram channels.
// The telegram_posted_on_approval flag makes it a one-time post per movie.
func AnnounceApprovedMovie(movies *MovieService, tg *TelegramService, id string) {
	if movies == nil || tg == nil {
		return
	}
	movie, err := movies.GetMovieByID(id)
	if err != nil || movie == nil {
		log.Printf("[TELEGRAM APPROVE] could not fetch movie %s: %v", id, err)
		return
	}
	if movie.TelegramPostedOnApproval {
		log.Printf("[TELEGRAM APPROVE] movie id=%s already posted — skipping duplicate", id)
		return
	}
	poster := movie.PosterURL
	if strings.TrimSpace(poster) == "" {
		poster = movie.BackdropURL
	}
	data := &TelegramMovieData{
		Title:       movie.Title,
		Year:        movie.Year,
		Genres:      movie.Genre,
		GenresUz:    movie.GenresUz,
		Country:     movie.Country,
		CountriesUz: movie.CountriesUz,
		Code:        movie.Code,
		PosterURL:   poster,
		Quality:     movie.Quality,
		Description: movie.Description,
		Slug:        movie.Slug,
		MovieURL:    tg.GetBaseSiteURL() + "/movies/" + movie.Slug,
	}
	posted := tg.PostContentApproval(data, false)
	log.Printf("[TELEGRAM APPROVE] movie id=%s result: posted_to=%v", id, posted)
	if len(posted) == 0 {
		log.Printf("[TELEGRAM APPROVE] movie id=%s no channels received the post — not marking as posted", id)
		return
	}
	if err := movies.MarkTelegramPostedOnApproval(id); err != nil {
		log.Printf("[TELEGRAM APPROVE] failed to mark movie id=%s as posted: %v", id, err)
	}
}

// ── Scheduler ──

// ScheduledPublisher publishes movies whose scheduled time has come: it
// approves them exactly like the admin button would (duplicate check, SEO
// ping, Telegram post) and tells the scheduling admin how it went.
type ScheduledPublisher struct {
	Movies   *MovieService
	Repo     *repositories.MovieRepository
	Telegram *TelegramService     // optional — channel post
	Notify   *NotificationService // optional — DM to the scheduling admin
	Users    *repositories.UserRepository
	Audit    *repositories.AuditLogRepository // optional
}

// Start runs the publisher every minute until the process exits.
func (p *ScheduledPublisher) Start() {
	if p == nil || p.Movies == nil || p.Repo == nil {
		return
	}
	if err := p.Repo.EnsureScheduleIndex(); err != nil {
		log.Printf("[SCHEDULE] index: %v", err)
	}
	go func() {
		time.Sleep(20 * time.Second) // let startup settle
		p.RunOnce(time.Now())
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for now := range t.C {
			p.RunOnce(now)
		}
	}()
}

// RunOnce publishes everything due at `now`. A panic in one publish never
// kills the loop.
func (p *ScheduledPublisher) RunOnce(now time.Time) {
	for i := 0; i < 50; i++ { // bounded per tick
		id, by, err := p.Repo.ClaimDueScheduled(now)
		if err != nil {
			log.Printf("[SCHEDULE] claim failed: %v", err)
			return
		}
		if id == "" {
			return
		}
		p.publish(id, by)
	}
}

func (p *ScheduledPublisher) publish(id, scheduledBy string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[SCHEDULE] publish movie=%s panicked: %v", id, r)
		}
	}()

	prior, _ := p.Movies.GetMovieByID(id)
	err := p.Movies.SetMovieApprovalStatus(id, "approved", "scheduler")
	if err != nil {
		reason := err.Error()
		if IsDuplicateMovieError(err) {
			reason = "Bunday kino allaqachon mavjud (dublikat)"
		}
		log.Printf("[SCHEDULE] publish movie=%s failed: %v", id, err)
		_ = p.Repo.SetScheduleError(id, reason)
		p.tellAdmin(scheduledBy, prior, false, reason)
		p.audit(id, scheduledBy, 500)
		return
	}
	log.Printf("[SCHEDULE] movie=%s published (scheduled by %s)", id, scheduledBy)
	p.audit(id, scheduledBy, 200)
	if prior == nil || !prior.TelegramPostedOnApproval {
		go AnnounceApprovedMovie(p.Movies, p.Telegram, id)
	}
	p.tellAdmin(scheduledBy, prior, true, "")
}

func (p *ScheduledPublisher) audit(movieID, scheduledBy string, status int) {
	if p.Audit == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = p.Audit.Insert(ctx, &repositories.AuditLog{
		ActorID:   scheduledBy,
		ActorRole: "scheduler",
		Method:    "SCHEDULED",
		Route:     "/api/admin/movies/:id/approve",
		Path:      "/api/admin/movies/" + movieID + "/approve",
		Params:    map[string]string{"id": movieID},
		Status:    status,
		CreatedAt: time.Now(),
	})
}

func (p *ScheduledPublisher) tellAdmin(userID string, movie *models.Movie, ok bool, reason string) {
	if p.Notify == nil || p.Users == nil || userID == "" || movie == nil {
		return
	}
	user, err := p.Users.FindByID(userID)
	if err != nil || user == nil {
		return
	}
	title := html.EscapeString(movie.Title)
	if ok {
		p.Notify.SendTelegramToUser(user,
			fmt.Sprintf("✅ Rejalashtirilgan nashr: <b>%s</b> saytda e'lon qilindi.", title),
			"Ko'rish", "/movies/"+movie.Slug)
		return
	}
	p.Notify.SendTelegramToUser(user,
		fmt.Sprintf("⚠️ <b>%s</b> rejalashtirilgan vaqtda e'lon qilinmadi.\nSabab: %s", title, html.EscapeString(reason)),
		"Kinolar", "/admin/movies?status=pending")
}

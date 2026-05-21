package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// SystemHandler powers the admin "VPS status" panel. There are two
// endpoints:
//
//   - GET /api/system/healthz — gated by X-Internal-Token. Returns a
//     snapshot of the host this backend runs on. Designed to be called
//     by other backend instances; here it's also called by ourselves as
//     part of /admin/system/status.
//   - GET /api/admin/system/status — admin-gated. Aggregates the local
//     /healthz with a remote /healthz from the parser host so the
//     dashboard sees both VPS in one request.
type SystemHandler struct {
	cfg        *config.Config
	db         *mongo.Database
	parserURL  string
	httpClient *http.Client
	startTime  time.Time
}

func NewSystemHandler(cfg *config.Config, db *mongo.Database, parserURL string) *SystemHandler {
	return &SystemHandler{
		cfg:        cfg,
		db:         db,
		parserURL:  parserURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		startTime:  time.Now(),
	}
}

// HostStatus is the shape returned by every /healthz across the fleet so
// the frontend can render hosts uniformly.
type HostStatus struct {
	OK            bool               `json:"ok"`
	Host          string             `json:"host"`
	Hostname      string             `json:"hostname"`
	Service       string             `json:"service"`            // "backend" | "parser" — the service that reported this snapshot
	UptimeSeconds uint64             `json:"uptime_seconds"`
	ProcessUptime int64              `json:"process_uptime_seconds"`
	CPUPercent    float64            `json:"cpu_percent"`
	CPUCores      int                `json:"cpu_cores"`
	Memory        memSnapshot        `json:"memory"`
	Disk          diskSnapshot       `json:"disk"`
	LoadAvg       [3]float64         `json:"load_avg"`
	Services      map[string]string  `json:"services"`           // service_name → "up" | "down" | "unknown"
	Checks        map[string]string  `json:"checks,omitempty"`   // ad-hoc info (mongo ping ms, etc.)
	Error         string             `json:"error,omitempty"`
	CheckedAt     string             `json:"checked_at"`
}

type memSnapshot struct {
	TotalMB int64   `json:"total_mb"`
	UsedMB  int64   `json:"used_mb"`
	Percent float64 `json:"percent"`
}

type diskSnapshot struct {
	TotalGB int64   `json:"total_gb"`
	UsedGB  int64   `json:"used_gb"`
	Percent float64 `json:"percent"`
}

// Healthz is the internal-token gated endpoint. It returns metrics for
// the host this backend runs on plus a couple of service-level checks
// (the bot lives on the same VPS so we shell out to systemctl; we also
// ping mongo).
func (h *SystemHandler) Healthz(c *gin.Context) {
	if !h.checkInternalToken(c) {
		return
	}
	c.JSON(http.StatusOK, h.localSnapshot())
}

// AdminSystemStatus is admin-gated. Returns local + remote hosts.
func (h *SystemHandler) AdminSystemStatus(c *gin.Context) {
	local := h.localSnapshot()

	// Remote = parser host. The parser /healthz returns the same shape
	// and reports the worker as a child service.
	remote := h.fetchRemoteHealthz(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"hosts": []HostStatus{local, remote},
	})
}

func (h *SystemHandler) checkInternalToken(c *gin.Context) bool {
	expected := os.Getenv("BOT_INTERNAL_TOKEN")
	if expected == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal token not configured"})
		return false
	}
	provided := c.GetHeader("X-Internal-Token")
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	return true
}

// localSnapshot collects metrics for the backend host. Safe to call from
// admin paths (no auth assumptions inside).
func (h *SystemHandler) localSnapshot() HostStatus {
	now := time.Now().UTC().Format(time.RFC3339)
	hostname, _ := os.Hostname()
	status := HostStatus{
		OK:            true,
		Service:       "backend",
		Host:          publicHostAddr(),
		Hostname:      hostname,
		ProcessUptime: int64(time.Since(h.startTime).Seconds()),
		Services:      map[string]string{},
		Checks:        map[string]string{},
		CheckedAt:     now,
	}

	if info, err := host.Info(); err == nil {
		status.UptimeSeconds = info.Uptime
	}
	// CPU: sample over 200ms; that's enough resolution for a dashboard and
	// short enough not to slow the response down.
	if pcts, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(pcts) > 0 {
		status.CPUPercent = round1(pcts[0])
	}
	if n, err := cpu.Counts(true); err == nil {
		status.CPUCores = n
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		status.Memory = memSnapshot{
			TotalMB: int64(vm.Total / (1024 * 1024)),
			UsedMB:  int64(vm.Used / (1024 * 1024)),
			Percent: round1(vm.UsedPercent),
		}
	}
	if du, err := disk.Usage("/"); err == nil {
		status.Disk = diskSnapshot{
			TotalGB: int64(du.Total / (1024 * 1024 * 1024)),
			UsedGB:  int64(du.Used / (1024 * 1024 * 1024)),
			Percent: round1(du.UsedPercent),
		}
	}
	if la, err := load.Avg(); err == nil {
		status.LoadAvg = [3]float64{round2(la.Load1), round2(la.Load5), round2(la.Load15)}
	}

	// Service checks for this host.
	status.Services["backend"] = "up"
	status.Services["bot"] = systemctlActive("filmorauz-bot.service")

	// Mongo ping with a short timeout — we don't want the dashboard to
	// hang because one server is sluggish.
	if h.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		t0 := time.Now()
		if err := h.db.Client().Ping(ctx, readpref.Primary()); err != nil {
			status.Services["mongodb"] = "down"
			status.Checks["mongodb_error"] = err.Error()
		} else {
			status.Services["mongodb"] = "up"
			status.Checks["mongodb_ping_ms"] = fmt.Sprintf("%d", time.Since(t0).Milliseconds())
		}
	}
	return status
}

// fetchRemoteHealthz calls the parser host's /healthz. Failure is
// returned as a degraded HostStatus so the dashboard renders something
// useful instead of an HTTP error.
func (h *SystemHandler) fetchRemoteHealthz(ctx context.Context) HostStatus {
	url := strings.TrimRight(h.parserURL, "/") + "/healthz"
	remote := HostStatus{
		OK:        false,
		Service:   "parser",
		Host:      url,
		Services:  map[string]string{},
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		remote.Error = err.Error()
		return remote
	}
	if tok := os.Getenv("BOT_INTERNAL_TOKEN"); tok != "" {
		req.Header.Set("X-Internal-Token", tok)
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		remote.Error = "unreachable: " + err.Error()
		return remote
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		remote.Error = fmt.Sprintf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return remote
	}
	var parsed HostStatus
	if err := json.Unmarshal(body, &parsed); err != nil {
		remote.Error = "invalid json: " + err.Error()
		return remote
	}
	parsed.OK = true
	if parsed.Services == nil {
		parsed.Services = map[string]string{}
	}
	return parsed
}

func publicHostAddr() string {
	// Prefer an explicit override, otherwise fall back to the hostname.
	if v := strings.TrimSpace(os.Getenv("PUBLIC_HOST")); v != "" {
		return v
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return ""
}

// systemctlActive returns "up" / "down" / "unknown" based on
// `systemctl is-active <unit>`. Silent on platforms where systemctl
// doesn't exist (dev laptops, mac) — returns "unknown" instead of
// failing the whole response.
func systemctlActive(unit string) string {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "unknown"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "systemctl", "is-active", unit).Output()
	state := strings.TrimSpace(string(out))
	if state == "active" {
		return "up"
	}
	if state == "" {
		return "unknown"
	}
	return "down"
}

func round1(v float64) float64 { return float64(int(v*10)) / 10 }
func round2(v float64) float64 { return float64(int(v*100)) / 100 }

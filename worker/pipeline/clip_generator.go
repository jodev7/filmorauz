package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/filmorauz/worker/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const clipCount = 15
const minClipSeconds = 30
const maxClipSeconds = 90

type ClipInfo struct {
	// Kind discriminates between "movie" and "series" clips. Empty is treated
	// as "movie" for backward compatibility with older flows.
	Kind string `json:"kind,omitempty"`

	// Movie linkage (populated when Kind == "movie")
	MovieID    string `json:"movie_id,omitempty"`
	MovieTitle string `json:"movie_title,omitempty"`
	MovieSlug  string `json:"movie_slug,omitempty"`
	MovieCode  string `json:"movie_code,omitempty"`

	// Series/episode linkage (populated when Kind == "series")
	SeriesID      string `json:"series_id,omitempty"`
	SeriesTitle   string `json:"series_title,omitempty"`
	SeriesSlug    string `json:"series_slug,omitempty"`
	SeasonID      string `json:"season_id,omitempty"`
	SeasonNumber  int    `json:"season_number,omitempty"`
	EpisodeNumber int    `json:"episode_number,omitempty"`
	EpisodeID     string `json:"episode_id,omitempty"`
	Title         string `json:"title,omitempty"`
	ClipIndex     int    `json:"clip_index,omitempty"`
	SourceType    string `json:"source_type,omitempty"`

	// Per-clip output metadata (identical for both kinds)
	Filename    string `json:"filename"`
	Path        string `json:"path"`
	URL         string `json:"url"`
	Duration    int    `json:"duration"`
	Sequence    int    `json:"sequence"`
	StorageType string `json:"storage_type"`

	// AI-generated Instagram/Shorts caption (Gemini path only; empty otherwise).
	Caption string `json:"caption,omitempty"`
	// AI-generated hashtags (Gemini path only; each starts with '#').
	Hashtags []string `json:"hashtags,omitempty"`
}

// clipTarget is a unified descriptor for "what are we clipping, and where do
// the resulting clips go?". Both movie and episode pipelines build one of
// these and hand it to generateClipsForTarget so the heavy-lifting (ffprobe,
// AI analysis, ffmpeg overlay, upload, DB save) lives in one place.
type clipTarget struct {
	Kind string // "movie" | "series"

	// FilenameSlug is used as the first component of each clip's on-disk
	// filename, e.g. "<slug>_<ts>_<idx>.mp4".
	FilenameSlug string

	// FolderSubpath is the subpath (relative to the clips root) under which
	// clip files are stored. For movies this is the canonical per-movie
	// folder; for episodes it is "serials/<slug>/season-N/episode-M".
	FolderSubpath string

	// DisplayLabel is used in human-readable [CLIP] log lines.
	DisplayLabel string

	// Drawtext overlay strings. Callers must already escape ffmpeg-sensitive
	// characters (":", "\", etc.) before setting these.
	TopText    string
	BottomText string

	// Movie fields — only populated when Kind == "movie".
	MovieID    interface{}
	MovieTitle string
	MovieSlug  string
	MovieCode  string

	// Series/episode fields — only populated when Kind == "series".
	SeriesID      primitive.ObjectID
	SeriesTitle   string
	SeriesSlug    string
	SeriesCode    string
	SeasonID      primitive.ObjectID
	SeasonNumber  int
	EpisodeNumber int
	EpisodeID     primitive.ObjectID
}

func sanitizeSlug(slug string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9\-]`)
	sanitized := re.ReplaceAllString(strings.ToLower(slug), "")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}
	return sanitized
}

func sanitizeFilename(name string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9а-яА-ЯёЁүҚқҳЎўНнгМмЛлКкЙйЗзЖжЕеДдГгВвБбАоОъЫыЬьЭэЮюЯяЁё\s\-_]`)
	sanitized := re.ReplaceAllString(name, "")
	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	sanitized = strings.ToLower(sanitized)
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}
	return sanitized
}

// candidateMoment holds a detected timestamp, interest score, and suggested clip duration.
type candidateMoment struct {
	startSec    float64
	durationSec float64 // per-moment clip length (varies by score)
	score       float64
	reason      string
	speechScore float64 // stored during scoring for dynamicDur at selection time

	// Populated only on the Gemini "AI viral" path (see momentsFromGemini):
	fromGemini bool
	geminiSubs []geminiSub // accurate transcript for burnt-in subtitles
	caption    string      // ready-to-post Instagram/Shorts caption
	hashtags   []string    // suggested hashtags (each starts with '#')
}

// geminiSub is one transcript line returned by the parser /clip/gemini-analyze
// endpoint, with an absolute (film-relative) start time in seconds.
type geminiSub struct {
	T    float64 `json:"t"`
	Text string  `json:"text"`
}

type geminiClipSpec struct {
	StartSec  float64     `json:"start_sec"`
	EndSec    float64     `json:"end_sec"`
	Reason    string      `json:"reason"`
	Caption   string      `json:"caption"`
	Hashtags  []string    `json:"hashtags"`
	Subtitles []geminiSub `json:"subtitles"`
}

type geminiUsage struct {
	AudioTokens  int `json:"audio_tokens"`
	TextTokens   int `json:"text_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type geminiAnalyzeResp struct {
	Clips     []geminiClipSpec `json:"clips"`
	Usage     geminiUsage      `json:"usage"`
	CostUSD   float64          `json:"cost_usd"`
	Model     string           `json:"model"`
	ErrorType string           `json:"error_type"`
	Error     string           `json:"error"`
}

// AI analysis types — populated by parser /clip/analyze endpoint.
type speechSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type faceFrame struct {
	Time      float64 `json:"time"`
	FaceCount int     `json:"face_count"`
	MaxSize   float64 `json:"max_size"` // fraction of frame area, 0–1
	CenterX   float64 `json:"cx"`       // 0–1, 0.5 = horizontal center
	CenterY   float64 `json:"cy"`       // 0–1, 0.5 = vertical center
}

type aiVideoData struct {
	SpeechSegments []speechSegment `json:"speech_segments"`
	FaceFrames     []faceFrame     `json:"face_frames"`
}

// windowScore is a package-level type so AI helpers can reference it.
type windowScore struct {
	t   float64
	rms float64
}

// fetchAIAnalysis calls the parser service for Whisper + face analysis.
// Returns nil on any failure — callers treat nil as "AI unavailable".
func (p *Pipeline) fetchAIAnalysis(videoPath string, durationSec float64) *aiVideoData {
	if p.config.ParserURL == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"video_path": videoPath,
		"duration":   durationSec,
	})
	client := &http.Client{Timeout: 20 * time.Minute}
	resp, err := client.Post(p.config.ParserURL+"/clip/analyze", "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[CLIP-AI] analyzer unavailable: %v", err)
		return nil
	}
	defer resp.Body.Close()
	var data aiVideoData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("[CLIP-AI] analyzer response parse error: %v", err)
		return nil
	}
	log.Printf("[CLIP-AI] speech segments: %d", len(data.SpeechSegments))
	log.Printf("[CLIP-AI] faces detected: %d frames", len(data.FaceFrames))
	return &data
}

// aiSpeechScore returns 0–1 speech presence score for clip window [t, t+dur].
func aiSpeechScore(ai *aiVideoData, t, dur float64) float64 {
	if ai == nil || len(ai.SpeechSegments) == 0 {
		return 0
	}
	end := t + dur
	totalSpeech := 0.0
	earlyBonus := 0.0
	for _, seg := range ai.SpeechSegments {
		ol := min64(seg.End, end) - max64(seg.Start, t)
		if ol <= 0 {
			continue
		}
		totalSpeech += ol
		// punchy phrases (2–8 words) get bonus
		if wc := len(strings.Fields(seg.Text)); wc >= 2 && wc <= 8 {
			totalSpeech += ol * 0.3
		}
		if seg.Start < t+3 {
			earlyBonus = 0.2
		}
	}
	return min64(1.0, totalSpeech/dur+earlyBonus)
}

// aiFaceScore returns 0–1 face engagement score for clip window [t, t+dur].
// Close-up faces (large bbox) and center-positioned faces score highest.
func aiFaceScore(ai *aiVideoData, t, dur float64) float64 {
	if ai == nil || len(ai.FaceFrames) == 0 {
		return 0
	}
	end := t + dur
	total, scoreSum := 0.0, 0.0
	for _, ff := range ai.FaceFrames {
		if ff.Time < t || ff.Time > end || ff.FaceCount == 0 {
			continue
		}
		total++
		// Base score by face size: close-up (>8% frame) → 1.0, medium → 0.5, small → 0.2
		var base float64
		switch {
		case ff.MaxSize > 0.08:
			base = 1.0 // close-up
		case ff.MaxSize > 0.03:
			base = 0.5
		default:
			base = 0.2
		}
		// Center bonus: max +0.3 when face is at exact center (cx=0.5, cy=0.5)
		dx := abs64(ff.CenterX - 0.5)
		dy := abs64(ff.CenterY - 0.5)
		centerBonus := max64(0, 0.3*(1.0-(dx+dy)/0.7))
		// Multi-face bonus
		multiFaceBonus := 0.0
		if ff.FaceCount > 1 {
			multiFaceBonus = 0.1
		}
		scoreSum += min64(1.0, base+centerBonus+multiFaceBonus)
	}
	if total == 0 {
		return 0
	}
	return min64(1.0, scoreSum/total)
}

// detectEngagingMoments analyses the video with ffmpeg and returns candidate
// start times ranked by estimated audience interest.
//
// Three passes:
//  1. silencedetect  — marks silent intervals so they can be avoided.
//  2. astats/reset   — measures RMS energy per 4-second window. ffmpeg writes
//     one "Overall:" block per window to stderr; we split on
//     "Overall:" and parse the first "RMS level dB:" in each
//     section (the correct stderr format — NOT lavfi metadata).
//  3. scene detect   — downscaled frame-diff pass finds visual cut timestamps;
//     scene cuts score a bonus as they often mark new action.
//
// / Each selected moment gets a suggested duration based on its score:
// score>10 → 60s, score>6 → 45s, score>2 → 30s, else minClipSeconds (20s).
func (p *Pipeline) detectEngagingMoments(videoPath string, durationSec float64, want int, ai *aiVideoData) []candidateMoment {
	log.Printf("[CLIP-ANALYSIS] Analysing %s (%.0fs) for engaging moments", videoPath, durationSec)

	minSpacing := float64(minClipSeconds) // minimum gap between selected moments

	// Skip the first and last 5 % (intro logos / end credits).
	guardSec := durationSec * 0.05
	if guardSec < 30 {
		guardSec = 30
	}
	usableStart := guardSec
	usableEnd := durationSec - guardSec - minSpacing
	if usableEnd < usableStart {
		usableEnd = usableStart
	}
	log.Printf("[CLIP-ANALYSIS] Usable window: %.1fs – %.1fs", usableStart, usableEnd)

	// ── Pass 1: silence detection ─────────────────────────────────────────────
	silenceArgs := []string{"-i", videoPath, "-af", "silencedetect=n=-35dB:d=1.5", "-f", "null", "-"}
	silenceCmd := exec.Command("ffmpeg", silenceArgs...)
	var silenceOut bytes.Buffer
	silenceCmd.Stderr = &silenceOut
	_ = silenceCmd.Run()
	silenceOutput := silenceOut.String()

	type interval struct{ start, end float64 }
	var silentIntervals []interval
	reSSt := regexp.MustCompile(`silence_start:\s*([\d.]+)`)
	reSEnd := regexp.MustCompile(`silence_end:\s*([\d.]+)`)
	silStarts := reSSt.FindAllStringSubmatch(silenceOutput, -1)
	silEnds := reSEnd.FindAllStringSubmatch(silenceOutput, -1)
	for i, m := range silStarts {
		s := parseFloat(m[1])
		e := durationSec
		if i < len(silEnds) {
			e = parseFloat(silEnds[i][1])
		}
		silentIntervals = append(silentIntervals, interval{s, e})
	}
	log.Printf("[CLIP-ANALYSIS] Silence intervals: %d", len(silentIntervals))

	silenceRatio := func(t, dur float64) float64 {
		total := 0.0
		for _, iv := range silentIntervals {
			ol := min64(t+dur, iv.end) - max64(t, iv.start)
			if ol > 0 {
				total += ol
			}
		}
		return total / dur
	}

	isSilent := func(t, dur float64) bool {
		return silenceRatio(t, dur) > 0.5
	}

	// ── Pass 2: RMS energy per 4-second window via astats ────────────────────
	// Extract audio to a temp mono WAV first so astats runs on a clean stream.
	const windowSec = 4.0
	const sampRate = 16000
	resetN := int(windowSec * sampRate)

	var windows []windowScore

	tmpWav, tmpErr := os.CreateTemp("", "clip-rms-*.wav")
	if tmpErr != nil {
		log.Printf("[CLIP-ANALYSIS] WARN: cannot create temp wav: %v — RMS pass skipped", tmpErr)
	} else {
		tmpWavPath := tmpWav.Name()
		tmpWav.Close()
		defer os.Remove(tmpWavPath)

		// Extract mono 16 kHz WAV — reliable input for astats.
		extractCmd := exec.Command("ffmpeg",
			"-i", videoPath,
			"-vn", "-ac", "1", "-ar", fmt.Sprintf("%d", sampRate),
			"-y", tmpWavPath,
		)
		var extractStderr bytes.Buffer
		extractCmd.Stderr = &extractStderr
		if err := extractCmd.Run(); err != nil {
			log.Printf("[CLIP-ANALYSIS] WARN: audio extract failed: %v", err)
		} else {
			astatCmd := exec.Command("ffmpeg",
				"-i", tmpWavPath,
				"-af", fmt.Sprintf("astats=reset=%d", resetN),
				"-f", "null", "-",
			)
			var astatOut bytes.Buffer
			astatCmd.Stderr = &astatOut
			_ = astatCmd.Run()
			astatOutput := astatOut.String()
			log.Printf("[CLIP-ANALYSIS] astats output: %d chars", len(astatOutput))

			// Split on "Overall" — handles both "Overall:" (older ffmpeg) and "Overall\n" (newer).
			reOverall := regexp.MustCompile(`Overall[:\s]`)
			reRMSLine := regexp.MustCompile(`RMS level dB:\s*([-\d.]+)`)
			sections := reOverall.Split(astatOutput, -1)

			for idx, section := range sections[1:] { // sections[0] is pre-amble
				m := reRMSLine.FindStringSubmatch(section)
				if m == nil {
					continue
				}
				rmsDB := parseFloat(m[1])
				if rmsDB < -91 {
					rmsDB = -91
				}
				windows = append(windows, windowScore{t: float64(idx) * windowSec, rms: rmsDB})
			}
			log.Printf("[CLIP-ANALYSIS] RMS windows parsed: %d", len(windows))
			if len(windows) > 0 {
				log.Printf("[CLIP-ANALYSIS] RMS sample: first=%.1f dB  last=%.1f dB",
					windows[0].rms, windows[len(windows)-1].rms)
			}
		}
	}

	// Fallback: if astats yielded nothing, use volumedetect for a single mean-volume estimate
	// and synthesise uniform windows so spike/energy logic degrades gracefully.
	if len(windows) == 0 {
		log.Printf("[CLIP-ANALYSIS] WARN: 0 RMS windows — attempting volumedetect fallback")
		vdCmd := exec.Command("ffmpeg", "-i", videoPath, "-vn", "-af", "volumedetect", "-f", "null", "-")
		var vdOut bytes.Buffer
		vdCmd.Stderr = &vdOut
		_ = vdCmd.Run()
		reMeanVol := regexp.MustCompile(`mean_volume:\s*([-\d.]+)\s*dB`)
		if m := reMeanVol.FindStringSubmatch(vdOut.String()); m != nil {
			meanDB := parseFloat(m[1])
			log.Printf("[CLIP-ANALYSIS] volumedetect mean_volume: %.1f dB — creating synthetic windows", meanDB)
			nWin := int(durationSec / windowSec)
			if nWin < 1 {
				nWin = 1
			}
			for i := 0; i < nWin; i++ {
				windows = append(windows, windowScore{t: float64(i) * windowSec, rms: meanDB})
			}
		} else {
			log.Printf("[CLIP-ANALYSIS] WARN: volumedetect also failed — audio scoring disabled")
		}
	}

	medianRMS := -40.0
	if len(windows) > 0 {
		sorted := make([]float64, len(windows))
		for i, w := range windows {
			sorted[i] = w.rms
		}
		for i := 1; i < len(sorted); i++ {
			for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			}
		}
		medianRMS = sorted[len(sorted)/2]
	}
	log.Printf("[CLIP-ANALYSIS] Median RMS: %.1f dB  (from %d windows)", medianRMS, len(windows))

	// ── Pass 3: scene-change detection ───────────────────────────────────────
	sceneArgs := []string{
		"-i", videoPath,
		"-vf", "scale=320:-1,select=gt(scene\\,0.35),showinfo",
		"-vsync", "vfr", "-an", "-f", "null", "-",
	}
	sceneCmd := exec.Command("ffmpeg", sceneArgs...)
	var sceneOut bytes.Buffer
	sceneCmd.Stderr = &sceneOut
	_ = sceneCmd.Run()
	sceneOutput := sceneOut.String()

	reSceneTime := regexp.MustCompile(`pts_time:([\d.]+)`)
	sceneMatches := reSceneTime.FindAllStringSubmatch(sceneOutput, -1)
	sceneTimes := make(map[float64]bool)
	for _, m := range sceneMatches {
		sceneTimes[parseFloat(m[1])] = true
	}
	log.Printf("[CLIP-ANALYSIS] Scene cuts detected: %d", len(sceneTimes))

	isNearSceneCut := func(t float64) bool {
		for sc := range sceneTimes {
			if abs64(t-sc) < 2.0 {
				return true
			}
		}
		return false
	}

	// windowRMSAt returns the RMS in the window containing t.
	windowRMSAt := func(t float64) float64 {
		for _, w := range windows {
			if t >= w.t && t < w.t+windowSec {
				return w.rms
			}
		}
		return medianRMS
	}

	// isAudioSpike returns true if t shows a ≥6 dB jump from previous window.
	isAudioSpike := func(idx int) bool {
		return idx > 0 && windows[idx].rms-windows[idx-1].rms >= 6
	}

	// ── AI-based hook + filter helpers ───────────────────────────────────────

	// hookStrength returns 0–1: high = strong opening hook for clip starting at t.
	hookStrength := func(t float64) float64 {
		s := 0.0
		// scene cut in first 5s of clip
		for dt := 0.0; dt < 5.0; dt += 0.5 {
			if isNearSceneCut(t + dt) {
				s += 0.4
				break
			}
		}
		// early speech (first 3s of clip)
		if ai != nil {
			for _, seg := range ai.SpeechSegments {
				if seg.Start >= t && seg.Start < t+3 {
					s += 0.3
					break
				}
			}
		}
		// early face (first 3s of clip)
		if ai != nil {
			for _, ff := range ai.FaceFrames {
				if ff.Time >= t && ff.Time < t+3 && ff.FaceCount > 0 {
					s += 0.2
					break
				}
			}
		}
		// early audio energy
		rms := windowRMSAt(t)
		if rms > medianRMS+3 {
			s += 0.1
		}
		return min64(1.0, s)
	}

	// hookScore2s scores the first 2 seconds of a clip for viral hook quality.
	// Additive: speech start→+1.0, face→+0.8, motion spike→+0.6, scene cut→+0.5.
	// Clips with score < 0.8 are rejected.
	hookScore2s := func(t float64) float64 {
		score := 0.0
		// speech start in first 2s → +1.0
		if ai != nil {
			for _, seg := range ai.SpeechSegments {
				if seg.Start >= t && seg.Start < t+2 {
					score += 1.0
					break
				}
			}
		} else if windowRMSAt(t) > medianRMS-3 {
			score += 0.5 // audio-energy proxy when AI unavailable
		}
		// face in first 1s → +0.8
		if ai != nil {
			for _, ff := range ai.FaceFrames {
				if ff.Time >= t && ff.Time < t+1 && ff.FaceCount > 0 {
					score += 0.8
					break
				}
			}
		}
		// audio motion spike in first 2s → +0.6
		for i, w := range windows {
			if w.t >= t && w.t < t+2 && isAudioSpike(i) {
				score += 0.6
				break
			}
		}
		// scene cut in first 2s → +0.5
		for dt := 0.0; dt < 2.0; dt += 0.5 {
			if isNearSceneCut(t + dt) {
				score += 0.5
				break
			}
		}
		return score
	}

	// midClipBoost returns extra score if a second engagement peak exists inside [t+5, t+dur-5].
	midClipBoost := func(t, dur float64) float64 {
		innerStart := t + 5
		innerEnd := t + dur - 5
		if innerEnd <= innerStart {
			return 0
		}
		for sc := range sceneTimes {
			if sc > innerStart && sc < innerEnd {
				return 0.08
			}
		}
		for i, w := range windows {
			if w.t > innerStart && w.t < innerEnd && isAudioSpike(i) {
				return 0.06
			}
		}
		return 0
	}

	// dynamicDur assigns clip length based on content type.
	// action-heavy → 30–45s, dialogue-heavy → 35–60s, mixed → 40–70s.
	// Hard bounds: min 30s, max 90s.
	dynamicDur := func(t, score, speechSc float64) float64 {
		end := t + 45.0
		sceneCuts := 0
		for sc := range sceneTimes {
			if sc >= t && sc < end {
				sceneCuts++
			}
		}
		norm := min64(1.0, max64(0, score/100.0))
		switch {
		case sceneCuts >= 4 && speechSc < 0.3:
			// action-heavy: rapid cuts, little speech → 30–45s
			return min64(45.0, max64(30.0, 30.0+norm*15.0))
		case speechSc >= 0.5 && sceneCuts < 3:
			// dialogue-heavy: sustained speech, few cuts → 35–60s
			return min64(60.0, max64(35.0, 35.0+norm*25.0))
		default:
			// mixed/engaging → 40–70s
			return min64(70.0, max64(40.0, 40.0+norm*30.0))
		}
	}

	// hasLongSilence checks for any silence gap >3s inside [t, t+dur].
	hasLongSilence := func(t, dur float64) bool {
		end := t + dur
		for _, iv := range silentIntervals {
			olStart := max64(t, iv.start)
			olEnd := min64(end, iv.end)
			if olEnd-olStart > 3.0 {
				return true
			}
		}
		return false
	}

	// isBadClip filters obviously poor clips.
	isBadClip := func(t, dur float64) bool {
		// first 2s silent → reject
		if silenceRatio(t, 2.0) > 0.8 {
			return true
		}
		// silence gap >3s inside clip → reject
		if hasLongSilence(t, dur) {
			return true
		}
		// >40% overall silence
		if silenceRatio(t, dur) > 0.40 {
			return true
		}
		// no speech AND no face AND flat scene
		hasSpeech := ai != nil && aiSpeechScore(ai, t, dur) > 0.05
		hasFace := ai != nil && aiFaceScore(ai, t, dur) > 0.05
		if !hasSpeech && !hasFace {
			hasMotion := false
			for sc := range sceneTimes {
				if sc >= t && sc <= t+dur {
					hasMotion = true
					break
				}
			}
			if !hasMotion {
				return true
			}
		}
		return false
	}

	// ── Build candidates ──────────────────────────────────────────────────────
	var candidates []candidateMoment

	// Pass A: scene-cut candidates — align with nearby speech segment start if AI available.
	for sc := range sceneTimes {
		t := sc
		// Snap to nearby speech segment start (within 3s before cut) for better hooks.
		if ai != nil {
			for _, seg := range ai.SpeechSegments {
				if seg.Start >= t-3 && seg.Start < t && seg.Start >= usableStart {
					t = seg.Start
					break
				}
			}
		}
		if t < usableStart || t > usableEnd {
			continue
		}
		if isSilent(t, minSpacing) {
			continue
		}
		candidates = append(candidates, candidateMoment{startSec: t, score: 5, reason: "scene_cut"})
	}
	log.Printf("[CLIP-ANALYSIS] Scene-cut candidates added: %d", len(candidates))

	// Pass B: audio-window candidates.
	for i, w := range windows {
		t := w.t
		if t < usableStart || t > usableEnd {
			continue
		}
		if isSilent(t, minSpacing) {
			continue
		}
		score := w.rms - medianRMS
		reason := "audio_energy"
		if isAudioSpike(i) {
			score += 8
			reason = "audio_spike"
		}
		if isNearSceneCut(t) {
			score += 5
			if reason == "audio_energy" {
				reason = "scene_cut"
			} else {
				reason = "spike+scene"
			}
		}
		candidates = append(candidates, candidateMoment{startSec: t, score: score, reason: reason})
	}
	log.Printf("[CLIP-BEST] scene+audio candidates: %d", len(candidates))

	// ── Pass C: best-moment peak detection ───────────────────────────────────
	// Scan the usable range at 1-second ticks, compute a combined signal strength,
	// detect local maxima, then place the clip so the peak falls in the first 2–8s.
	const peakTick = 1.0
	const peakWindow = 30.0 // signal window used for peak scoring
	type peakT struct {
		t     float64
		score float64
	}
	var allPeaks []peakT
	for t := usableStart; t <= usableEnd; t += peakTick {
		// Combined signal: speech + face + audio + scene
		sp := aiSpeechScore(ai, t, peakWindow)
		fc := aiFaceScore(ai, t, peakWindow)
		rms := windowRMSAt(t)
		ae := min64(1.0, max64(0, rms-medianRMS)/20.0)
		sc := 0.0
		if isNearSceneCut(t) {
			sc = 1.0
		}
		sig := sp*0.35 + fc*0.25 + ae*0.25 + sc*0.15
		allPeaks = append(allPeaks, peakT{t: t, score: sig})
	}
	// Keep local maxima (score > both neighbours and > 0.15 threshold)
	for i := 1; i < len(allPeaks)-1; i++ {
		p := allPeaks[i]
		if p.score < 0.15 {
			continue
		}
		if p.score <= allPeaks[i-1].score || p.score <= allPeaks[i+1].score {
			continue
		}
		// candidate_start = peak_time - offset so peak lands in first 2–8s of clip
		offset := 3.0 // default: peak at 3s into clip
		if ai != nil && len(ai.FaceFrames) > 0 {
			offset = 2.0 // face-heavy content: lead even earlier
		}
		startT := max64(usableStart, p.t-offset)
		if startT > usableEnd || isSilent(startT, minSpacing) {
			continue
		}
		candidates = append(candidates, candidateMoment{
			startSec: startT,
			score:    p.score * 20, // initial rough score in legacy range
			reason:   "best_moment_peak",
		})
	}
	log.Printf("[CLIP-BEST] candidates after peak pass: %d", len(candidates))

	// ── Determine scoring mode for logging ───────────────────────────────────
	scoringMode := "fallback-scene"
	if ai != nil && len(ai.SpeechSegments) > 0 && len(ai.FaceFrames) > 0 {
		scoringMode = "full-ai"
	} else if ai != nil && (len(ai.SpeechSegments) > 0 || len(ai.FaceFrames) > 0) {
		scoringMode = "partial-ai"
	} else if len(sceneTimes) > 0 && len(windows) > 0 {
		scoringMode = "scene+audio"
	} else if len(sceneTimes) > 0 {
		scoringMode = "fallback-scene"
	}
	log.Printf("[CLIP-BEST] scoring mode: %s", scoringMode)

	// ── Compute best_moment_score for each candidate ──────────────────────────
	// best_moment_score = hook*0.25 + speech*0.20 + face*0.15 + scene*0.15 +
	//                     audio_energy*0.10 + spike*0.05 + mid_boost*0.05 + non_silence*0.05
	for i := range candidates {
		c := &candidates[i]
		clipDur := 45.0 // use 45s scoring window (midpoint of 30–90 range)

		hook := hookStrength(c.startSec)
		sceneAct := 0.0
		if isNearSceneCut(c.startSec) {
			sceneAct = 1.0
		}
		rms := windowRMSAt(c.startSec)
		audioEnergy := min64(1.0, max64(0, rms-medianRMS)/20.0)
		spike := 0.0
		for j, w := range windows {
			if abs64(w.t-c.startSec) < windowSec && isAudioSpike(j) {
				spike = 1.0
				break
			}
		}
		speechSc := aiSpeechScore(ai, c.startSec, clipDur)
		faceSc := aiFaceScore(ai, c.startSec, clipDur)
		c.speechScore = speechSc
		nonSilence := 1.0
		if isSilent(c.startSec, clipDur) {
			nonSilence = 0.0
		}
		midBoost := midClipBoost(c.startSec, clipDur)

		bestMomentScore := hook*0.25 + speechSc*0.20 + faceSc*0.15 + sceneAct*0.15 +
			audioEnergy*0.10 + spike*0.05 + midBoost*0.05 + nonSilence*0.05

		// Degrade gracefully: blend with legacy score when AI unavailable
		if ai == nil {
			bestMomentScore = bestMomentScore*0.3 + (c.score/20.0)*0.7
		}
		c.score = bestMomentScore * 100
	}

	// Sort by viral score descending.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].score > candidates[j-1].score; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	topScore := 0.0
	if len(candidates) > 0 {
		topScore = candidates[0].score
	}
	log.Printf("[CLIP-ANALYSIS] Scored candidates: %d  top_score=%.1f", len(candidates), topScore)

	// ── Select top N with spacing + hook/bad-clip filters ────────────────────
	const regionSec = 300.0 // 5-minute regions for diversity
	const maxPerRegion = 2
	regionCount := make(map[int]int)

	selected := make([]candidateMoment, 0, want)
	for _, c := range candidates {
		if len(selected) >= want {
			break
		}
		// Spacing: enforce minimum 20s gap between clips.
		tooClose := false
		for _, s := range selected {
			if abs64(c.startSec-s.startSec) < minSpacing {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		// Region diversity: max 2 clips per 5-minute region.
		region := int(c.startSec / regionSec)
		if regionCount[region] >= maxPerRegion {
			continue
		}
		// Hook rejection: strict 2s hook score must be ≥ 0.8.
		if hs := hookScore2s(c.startSec); hs < 0.8 {
			log.Printf("[CLIP-ANALYSIS] Rejected weak-hook at %.1fs  hook2s=%.2f", c.startSec, hs)
			continue
		}

		// Dynamic duration based on content type (action/dialogue/mixed).
		c.durationSec = dynamicDur(c.startSec, c.score, c.speechScore)

		// Bad-clip filter (applied with final dur).
		if isBadClip(c.startSec, c.durationSec) {
			log.Printf("[CLIP-ANALYSIS] Rejected bad clip at %.1fs", c.startSec)
			continue
		}

		selected = append(selected, c)
		regionCount[region]++
		log.Printf("[CLIP-ANALYSIS] Selected %.1fs  score=%.1f  dur=%.0fs  reason=%s  region=%d",
			c.startSec, c.score, c.durationSec, c.reason, region)
	}
	log.Printf("[CLIP-AI] selected: %d", len(selected))

	// ── Fallback: pad with evenly-spaced moments if too few smart moments ────
	if len(selected) < want {
		log.Printf("[CLIP-ANALYSIS] Smart moments: %d/%d — padding with evenly-spaced fallbacks", len(selected), want)
		fallbackDurs := []float64{30, 45, 25, 40, 35, 20, 50, 30, 45, 25}
		step := (usableEnd - usableStart) / float64(want)
		fi := 0
		for i := 0; i < want && len(selected) < want; i++ {
			t := usableStart + float64(i)*step
			covered := false
			for _, s := range selected {
				if abs64(t-s.startSec) < minSpacing {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
			dur := fallbackDurs[fi%len(fallbackDurs)]
			fi++
			selected = append(selected, candidateMoment{
				startSec:    t,
				durationSec: dur,
				score:       -999,
				reason:      "fallback_uniform",
			})
			log.Printf("[CLIP-ANALYSIS] Fallback moment %.1fs dur=%.0fs", t, dur)
		}
	}

	// Sort chronologically for natural clip sequencing.
	for i := 1; i < len(selected); i++ {
		for j := i; j > 0 && selected[j].startSec < selected[j-1].startSec; j-- {
			selected[j], selected[j-1] = selected[j-1], selected[j]
		}
	}

	log.Printf("[CLIP-ANALYSIS] Final selection: %d moments", len(selected))
	for i, s := range selected {
		log.Printf("[CLIP-ANALYSIS]   [%d] t=%.1fs dur=%.0fs score=%.1f reason=%s",
			i+1, s.startSec, s.durationSec, s.score, s.reason)
	}
	return selected
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func abs64(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// generateClips is the movie-flow entry point. It builds a "movie" clipTarget
// and delegates the actual clip production to generateClipsForTarget so the
// episode flow (see generateEpisodeClips) can share the same logic.
func (p *Pipeline) generateClips(ctx context.Context, canonicalFolderName string, movieCode string, movieResult *MovieCreationResult, processedMasterPath string, finalUploadsPath string) error {
	if movieResult == nil {
		return fmt.Errorf("movieResult is nil — cannot generate movie clips")
	}
	_ = finalUploadsPath // kept for call-site compatibility; not needed here

	movieSlug := sanitizeSlug(movieResult.Slug)
	if movieSlug == "" {
		movieSlug = sanitizeSlug(movieResult.DisplayTitle)
	}

	// Overlay CTA word depends on content: animated movies say "Multfilm
	// profilda!", everything else "Kino profilda!". Series use "Serial" (see
	// generateEpisodeClips). The on-clip code label stays "Kino kodi".
	bottomText := "Kino profilda\\!"
	if p.movieIsAnimation(ctx, movieResult.MovieID) {
		bottomText = "Multfilm profilda\\!"
	}

	target := clipTarget{
		Kind:          "movie",
		FilenameSlug:  movieSlug,
		FolderSubpath: canonicalFolderName,
		DisplayLabel:  fmt.Sprintf("Movie: %s (code: %s)", movieResult.DisplayTitle, movieCode),
		TopText:       fmt.Sprintf("Kino kodi\\: %s", movieCode),
		BottomText:    bottomText,
		MovieID:       movieResult.MovieID,
		MovieTitle:    movieResult.DisplayTitle,
		MovieSlug:     movieResult.Slug,
		MovieCode:     movieCode,
	}
	return p.generateClipsForTarget(ctx, target, processedMasterPath)
}

// movieIsAnimation reports whether the movie's genres mark it as an animated
// film (multfilm). Best-effort: any lookup failure returns false so clips just
// use the default "Kino" label.
func (p *Pipeline) movieIsAnimation(ctx context.Context, movieID interface{}) bool {
	if p.config.DB == nil || movieID == nil {
		return false
	}
	var oid primitive.ObjectID
	switch v := movieID.(type) {
	case primitive.ObjectID:
		oid = v
	case string:
		parsed, err := primitive.ObjectIDFromHex(v)
		if err != nil {
			return false
		}
		oid = parsed
	default:
		return false
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var doc bson.M
	if err := p.config.DB.Collection("movies").FindOne(lookupCtx, bson.M{"_id": oid}).Decode(&doc); err != nil {
		return false
	}
	animationGenres := []string{"multfilm", "multifilm", "animatsiya", "animation", "cartoon", "мультфильм", "анимация"}
	for _, field := range []string{"genre", "genres_uz"} {
		raw, ok := doc[field]
		if !ok {
			continue
		}
		arr, ok := raw.(bson.A)
		if !ok {
			continue
		}
		for _, g := range arr {
			gs, ok := g.(string)
			if !ok {
				continue
			}
			gl := strings.ToLower(strings.TrimSpace(gs))
			for _, ag := range animationGenres {
				if strings.Contains(gl, ag) {
					return true
				}
			}
		}
	}
	return false
}

// generateEpisodeClips is the serial-episode entry point. It matches the
// movie flow one-for-one: it runs AFTER the episode's HLS + DB metadata have
// been finalised and BEFORE processed_master.mp4 is cleaned up in
// processEpisodeJob. Every episode of every season of every serial goes
// through here. Clip style (vertical 1080×1920, logo, CTA) is identical to
// movie clips; only the top overlay label differs because serials have no
// movie-code equivalent.
func (p *Pipeline) generateEpisodeClips(ctx context.Context, job *models.IngestionJob, processedMasterPath string) error {
	if job == nil {
		return fmt.Errorf("episode clip generation: job is nil")
	}
	if job.SeriesSlug == "" {
		return fmt.Errorf("episode clip generation: series_slug is empty")
	}
	if job.EpisodeID.IsZero() {
		return fmt.Errorf("episode clip generation: episode_id is zero")
	}

	// Folder subpath intentionally mirrors the episode HLS layout so the
	// clip tree stays discoverable:
	//   videos/clips/serials/<slug>/season-N/episode-M/<file>.mp4
	folderSubpath := filepath.Join(
		"serials",
		job.SeriesSlug,
		fmt.Sprintf("season-%d", job.SeasonNumber),
		fmt.Sprintf("episode-%d", job.EpisodeNumber),
	)

	// Pull the series document to show a human-readable title and the shared
	// parent series code on episode clips.
	seriesTitle := job.SeriesSlug
	seriesCode := ""
	if p.config.DB != nil && !job.SeriesID.IsZero() {
		var seriesDoc bson.M
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := p.config.DB.Collection("series").FindOne(lookupCtx, bson.M{"_id": job.SeriesID}).Decode(&seriesDoc)
		cancel()
		if err == nil {
			if t, ok := seriesDoc["title"].(string); ok && strings.TrimSpace(t) != "" {
				seriesTitle = strings.TrimSpace(t)
			}
			if c, ok := seriesDoc["code"].(string); ok && strings.TrimSpace(c) != "" {
				seriesCode = strings.TrimSpace(c)
			}
		} else {
			log.Printf("[CLIP-EPISODE] WARN: could not load series doc %s: %v", job.SeriesID.Hex(), err)
		}
	}

	filenameSlug := sanitizeSlug(fmt.Sprintf("%s-s%d-e%d", job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber))
	if filenameSlug == "" {
		filenameSlug = "episode"
	}
	displayTitle := fmt.Sprintf("%s S%02dE%02d", seriesTitle, job.SeasonNumber, job.EpisodeNumber)

	log.Printf("[CLIPS] episode completed, generating clips series_id=%s season_id=%s episode_id=%s title=%q source=%s",
		job.SeriesID.Hex(), job.SeasonID.Hex(), job.EpisodeID.Hex(), displayTitle, processedMasterPath)

	target := clipTarget{
		Kind:          "series",
		FilenameSlug:  filenameSlug,
		FolderSubpath: folderSubpath,
		DisplayLabel:  fmt.Sprintf("Series: %s S%02dE%02d", seriesTitle, job.SeasonNumber, job.EpisodeNumber),
		TopText:       fmt.Sprintf("Serial kodi\\: %s", firstNonEmptyString(seriesCode, "—")),
		BottomText:    "Serial profilda\\!",
		SeriesID:      job.SeriesID,
		SeriesTitle:   seriesTitle,
		SeriesSlug:    job.SeriesSlug,
		SeriesCode:    seriesCode,
		SeasonID:      job.SeasonID,
		SeasonNumber:  job.SeasonNumber,
		EpisodeNumber: job.EpisodeNumber,
		EpisodeID:     job.EpisodeID,
	}

	err := p.generateClipsForTarget(ctx, target, processedMasterPath)
	if err != nil {
		log.Printf("[CLIP-EPISODE] FAILURE series_slug=%s S%02dE%02d: %v",
			job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber, err)
		return err
	}
	log.Printf("[CLIP-EPISODE] SUCCESS series_slug=%s S%02dE%02d", job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber)
	return nil
}

// ffmpegEscapeDrawtext escapes the bare minimum of characters that break
// ffmpeg's drawtext filter syntax. The movie flow hard-codes its text, but
// episode text comes from user-provided series titles, so dynamic escaping
// is needed.
func ffmpegEscapeDrawtext(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, `:`, `\:`)
	return s
}

// clipSubtitlesEnabled reports whether burnt-in clip subtitles are turned on
// via the CLIP_SUBTITLES env flag. Default OFF — see the call site for why.
func clipSubtitlesEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLIP_SUBTITLES"))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

// clipAIViralEnabled reports whether AI (Gemini) viral clip selection is turned
// on via the CLIP_AI_VIRAL env flag. Default OFF. When on, the worker asks the
// parser to pick viral moments + captions + transcript; on any failure (quota
// exhausted, parser down) it falls back to the heuristic clip generator.
func clipAIViralEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLIP_AI_VIRAL"))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

// fetchGeminiClips calls the parser /clip/gemini-analyze endpoint. It returns
// the parsed response on HTTP 200, or an error (quota/unavailable/other) so the
// caller can fall back to heuristic clip detection.
func (p *Pipeline) fetchGeminiClips(videoPath string, maxClips int) (*geminiAnalyzeResp, error) {
	if p.config.ParserURL == "" {
		return nil, fmt.Errorf("parser URL not configured")
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"video_path": videoPath,
		"max_clips":  maxClips,
	})
	client := &http.Client{Timeout: 40 * time.Minute}
	resp, err := client.Post(p.config.ParserURL+"/clip/gemini-analyze", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data geminiAnalyzeResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini analyze status=%d type=%s: %s", resp.StatusCode, data.ErrorType, data.Error)
	}
	return &data, nil
}

// momentsFromGemini converts Gemini clip specs into candidateMoments carrying
// the per-clip transcript + caption. Durations are clamped to the allowed
// maximum; clips that fall outside the video are skipped.
func momentsFromGemini(clips []geminiClipSpec, durationSec float64) []candidateMoment {
	out := make([]candidateMoment, 0, len(clips))
	for _, c := range clips {
		st := c.StartSec
		dur := c.EndSec - c.StartSec
		if dur <= 0 {
			continue
		}
		if dur > float64(maxClipSeconds) {
			dur = float64(maxClipSeconds)
		}
		if st < 0 {
			st = 0
		}
		if st >= durationSec {
			continue
		}
		if st+dur > durationSec {
			dur = durationSec - st
		}
		if dur < 1 {
			continue
		}
		out = append(out, candidateMoment{
			startSec:    st,
			durationSec: dur,
			reason:      "gemini_viral",
			fromGemini:  true,
			geminiSubs:  c.Subtitles,
			caption:     strings.TrimSpace(c.Caption),
			hashtags:    c.Hashtags,
		})
	}
	return out
}

// buildGeminiASS renders a Gemini transcript (absolute film times) into an ASS
// subtitle document for one clip window, rebased to the clip's local timeline.
// Each line ends when the next begins (or at clip end). Style matches
// buildClipASS via the shared clipSubtitleASSHeader.
func buildGeminiASS(subs []geminiSub, startSec, clipDur float64) string {
	if len(subs) == 0 || clipDur <= 0 {
		return ""
	}
	sorted := append([]geminiSub(nil), subs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].T < sorted[j].T })

	var b strings.Builder
	b.WriteString(clipSubtitleASSHeader)
	count := 0
	for i, s := range sorted {
		text := assEscapeText(s.Text)
		if text == "" {
			continue
		}
		rs := s.T - startSec
		var re float64
		if i+1 < len(sorted) {
			re = sorted[i+1].T - startSec
		} else {
			re = clipDur
		}
		if rs < 0 {
			rs = 0
		}
		if rs >= clipDur {
			continue
		}
		if re > clipDur {
			re = clipDur
		}
		if re-rs < 0.3 {
			re = rs + 0.3
		}
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", assTimestamp(rs), assTimestamp(re), text)
		count++
	}
	if count == 0 {
		return ""
	}
	return b.String()
}

// recordGeminiUsage writes one AI cost row into the clip_ai_usage collection so
// the admin dashboard can show per-content and total spend. Best-effort: a
// failure here never blocks clip generation.
func (p *Pipeline) recordGeminiUsage(ctx context.Context, target clipTarget, resp *geminiAnalyzeResp) {
	if p.config.DB == nil || resp == nil {
		return
	}
	contentID := ""
	title := ""
	switch target.Kind {
	case "movie":
		if oid, ok := target.MovieID.(primitive.ObjectID); ok {
			contentID = oid.Hex()
		} else if s, ok := target.MovieID.(string); ok {
			contentID = s
		}
		title = target.MovieTitle
	case "series":
		contentID = target.EpisodeID.Hex()
		title = fmt.Sprintf("%s S%02dE%02d", target.SeriesTitle, target.SeasonNumber, target.EpisodeNumber)
	}
	doc := bson.M{
		"_id":           primitive.NewObjectID(),
		"content_kind":  target.Kind,
		"content_id":    contentID,
		"title":         title,
		"model":         resp.Model,
		"audio_tokens":  resp.Usage.AudioTokens,
		"text_tokens":   resp.Usage.TextTokens,
		"output_tokens": resp.Usage.OutputTokens,
		"total_tokens":  resp.Usage.TotalTokens,
		"cost_usd":      resp.CostUSD,
		"clip_count":    len(resp.Clips),
		"created_at":    time.Now(),
	}
	if _, err := p.config.DB.Collection("clip_ai_usage").InsertOne(ctx, doc); err != nil {
		log.Printf("[CLIP-GEMINI] WARN: failed to record AI usage: %v", err)
	} else {
		log.Printf("[CLIP-GEMINI] recorded usage: $%.4f (%d tokens) for %s", resp.CostUSD, resp.Usage.TotalTokens, title)
	}
}

// assTimestamp formats seconds as an ASS timecode: H:MM:SS.cc (centiseconds).
func assTimestamp(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	total := int(sec * 100.0) // centiseconds
	cs := total % 100
	total /= 100
	s := total % 60
	total /= 60
	m := total % 60
	h := total / 60
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

// assEscapeText escapes a transcript line for an ASS Dialogue field. Newlines
// become the ASS hard-break "\N"; literal braces would otherwise open an
// override block, so they are neutralised.
func assEscapeText(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", `\N`)
	s = strings.ReplaceAll(s, "{", "(")
	s = strings.ReplaceAll(s, "}", ")")
	return strings.TrimSpace(s)
}

// clipSubtitleASSHeader is the [Script Info] + [V4+ Styles] preamble for the
// burnt-in clip subtitles. PlayResX/Y pin the ASS coordinate space to the
// 1080×1920 Reels canvas so Fontsize and MarginV are true canvas pixels (no
// original_size guesswork). The Default style is bold white with a thick black
// outline, bottom-centred (Alignment=2) with MarginV=700 — placing the text in
// the lower third of the centred movie frame, safely above the CTA at y=1300.
const clipSubtitleASSHeader = `[Script Info]
ScriptType: v4.00+
PlayResX: 1080
PlayResY: 1920
WrapStyle: 0
ScaledBorderAndShadow: yes

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Sans,54,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,1,0,0,0,100,100,0,0,1,3,1,2,60,60,700,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`

// buildClipASS renders the Whisper speech segments that fall inside the clip
// window [startSec, startSec+clipDur] into an ASS subtitle document, with
// timecodes rebased to the clip's own timeline (clip starts at 0). Returns ""
// if there is no usable speech in the window — the caller then skips burning
// subs.
//
// Segments that straddle the window edges are clamped; very short fragments
// (<0.2s of visible time) are dropped so single-word flickers don't appear.
func buildClipASS(ai *aiVideoData, startSec, clipDur float64) string {
	if ai == nil || len(ai.SpeechSegments) == 0 || clipDur <= 0 {
		return ""
	}
	end := startSec + clipDur
	var b strings.Builder
	b.WriteString(clipSubtitleASSHeader)
	count := 0
	for _, seg := range ai.SpeechSegments {
		text := assEscapeText(seg.Text)
		if text == "" {
			continue
		}
		// Overlap of the segment with the clip window.
		s := max64(seg.Start, startSec)
		e := min64(seg.End, end)
		if e-s < 0.2 {
			continue
		}
		// Rebase to the clip-local timeline.
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			assTimestamp(s-startSec), assTimestamp(e-startSec), text)
		count++
	}
	if count == 0 {
		return ""
	}
	return b.String()
}

// ffmpegEscapeSubtitlePath escapes a filesystem path for safe use inside the
// ffmpeg subtitles= filter option. The filtergraph parser treats ':' and '\'
// specially, and ''' terminates the quoted value.
func ffmpegEscapeSubtitlePath(p string) string {
	p = strings.ReplaceAll(p, `\`, `\\`)
	p = strings.ReplaceAll(p, `:`, `\:`)
	p = strings.ReplaceAll(p, `'`, `\'`)
	return p
}

// generateClipsForTarget is the shared clip-production core used by both the
// movie and serial-episode pipelines. It ffprobes the processed master,
// runs AI + heuristic engagement scoring, generates ffmpeg overlay clips,
// uploads them (prod) and persists metadata to MongoDB. Every per-target
// specialisation flows through the clipTarget struct — this function has
// no knowledge of whether it is processing a movie or a series episode.
func (p *Pipeline) generateClipsForTarget(ctx context.Context, target clipTarget, processedMasterPath string) error {
	log.Printf("[CLIP] ===== CLIP GENERATION START =====")
	log.Printf("[CLIP] Target: %s", target.DisplayLabel)
	log.Printf("[CLIP] Kind: %s", target.Kind)
	log.Printf("[CLIP] Folder subpath: %s", target.FolderSubpath)
	log.Printf("[CHECKPOINT] clip_input_path: %s", processedMasterPath)

	// Fail clearly if the processed master is missing or was not passed
	if processedMasterPath == "" {
		return fmt.Errorf("processed master path is empty — cannot generate clips")
	}
	if _, err := os.Stat(processedMasterPath); err != nil {
		return fmt.Errorf("processed master not found at %s: %w", processedMasterPath, err)
	}

	baseVideoPath := processedMasterPath
	log.Printf("[CHECKPOINT] ffprobe_target_path: %s", baseVideoPath)

	baseDir, err := os.Getwd()
	if err != nil {
		log.Printf("[CLIP] ERROR: getwd failed: %v", err)
		return fmt.Errorf("getwd: %w", err)
	}

	if target.FilenameSlug == "" {
		return fmt.Errorf("clipTarget.FilenameSlug is empty")
	}
	if target.FolderSubpath == "" {
		return fmt.Errorf("clipTarget.FolderSubpath is empty")
	}
	clipSlug := target.FilenameSlug

	devMode := p.config.StorageConfig.Mode != "prod"
	var baseURL string
	var storagePath string
	var outDir string

	// Local output layout is identical for movies and series — the only thing
	// that varies is the FolderSubpath under uploads/movies/clips/. Episodes
	// naturally nest under serials/<slug>/season-N/episode-M/.
	outDir = filepath.Join(baseDir, "uploads", "movies", "clips", target.FolderSubpath)
	baseURL = p.config.StorageConfig.BaseURL
	storagePath = fmt.Sprintf("%s/%s", B2VideoRootClips, target.FolderSubpath)
	LogB2Path(ContentTypeClip, storagePath+"/")

	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Printf("[CLIP] ERROR: failed to create output dir: %v", err)
		return fmt.Errorf("failed to create output dir: %w", err)
	}
	log.Printf("[CHECKPOINT] clip_output_directory: %s", outDir)

	durationMs, err := p.getVideoDurationMs(baseVideoPath)
	if err != nil {
		log.Printf("[CLIP] ERROR: ffprobe failed to get duration: %v", err)
		return fmt.Errorf("ffprobe failed to get duration: %w", err)
	}
	durationSec := float64(durationMs) / 1000.0
	log.Printf("[CLIP] Video duration: %.1fs, generating up to %d clips (%d–%ds each)", durationSec, clipCount, minClipSeconds, maxClipSeconds)

	logoPath := filepath.Join(baseDir, "docs", "logo.png")
	logoExists := false
	if _, statErr := os.Stat(logoPath); statErr == nil {
		logoExists = true
		log.Printf("[CLIP] Logo found: %s", logoPath)
	} else {
		log.Printf("[CLIP] WARNING: logo not found at %s, clips will not have logo", logoPath)
	}

	var aiData *aiVideoData
	var moments []candidateMoment
	geminiActive := false

	// AI viral selection (Gemini) — opt-in via CLIP_AI_VIRAL. Gemini "hears" the
	// whole film and returns the most viral moments + caption + an accurate
	// Uzbek transcript per clip. On any failure (quota exhausted, parser down)
	// we fall back to the heuristic detector below, so a run never breaks.
	if clipAIViralEnabled() {
		log.Printf("[CLIP-GEMINI] CLIP_AI_VIRAL on — requesting AI viral clips...")
		gResp, gErr := p.fetchGeminiClips(baseVideoPath, clipCount)
		if gErr != nil {
			log.Printf("[CLIP-GEMINI] unavailable/failed — falling back to heuristic: %v", gErr)
		} else if gResp != nil && len(gResp.Clips) > 0 {
			moments = momentsFromGemini(gResp.Clips, durationSec)
			if len(moments) > 0 {
				geminiActive = true
				p.recordGeminiUsage(ctx, target, gResp)
				log.Printf("[CLIP-GEMINI] using %d AI viral clips (model=%s cost=$%.4f)", len(moments), gResp.Model, gResp.CostUSD)
			}
		} else {
			log.Printf("[CLIP-GEMINI] returned no clips — falling back to heuristic")
		}
	}

	// Heuristic path: Whisper speech + face detection + audio/scene scoring.
	if !geminiActive {
		log.Printf("[CLIP-AI] Starting heuristic AI analysis via parser...")
		aiData = p.fetchAIAnalysis(baseVideoPath, durationSec)
		if aiData == nil {
			log.Printf("[CLIP-AI] AI analysis unavailable — using audio/scene scoring only")
		}
		moments = p.detectEngagingMoments(baseVideoPath, durationSec, clipCount, aiData)
	}
	if len(moments) == 0 {
		return fmt.Errorf("no valid clip moments found in video")
	}
	log.Printf("[CLIP] Starting clip generation for %d selected moments (gemini=%v)...", len(moments), geminiActive)

	topText := target.TopText
	bottomText := target.BottomText

	var generatedClips []ClipInfo
	var failedClips []int

	for i, moment := range moments {
		select {
		case <-ctx.Done():
			log.Printf("[CLIP] Context cancelled, stopping clip generation")
			return ctx.Err()
		default:
		}

		startSec := moment.startSec
		clipDur := moment.durationSec
		// Clamp to valid range.
		if startSec < 0 {
			startSec = 0
		}
		if startSec > durationSec-clipDur {
			startSec = durationSec - clipDur
		}
		if startSec < 0 {
			startSec = 0
		}

		timestamp := time.Now().UnixNano()
		clipFilename := fmt.Sprintf("%s_%d_%d.mp4", clipSlug, timestamp, i+1)
		outPath := filepath.Join(outDir, clipFilename)

		log.Printf("[CLIP] Generating clip %d/%d: start=%.1fs dur=%.1fs reason=%s -> %s",
			i+1, len(moments), startSec, clipDur, moment.reason, outPath)

		// Burn-in subtitles from the Whisper transcript for this clip window.
		// This reuses the speech analysis already produced for moment scoring —
		// no extra cost. subFilter ends with a trailing comma (or is "") so it
		// can be spliced into the filter chain right after scale/pad.
		//
		// Gated behind the CLIP_SUBTITLES env flag (default OFF): local Whisper
		// transcribes Uzbek poorly, so auto-burning subs would stamp mangled
		// text onto every clip. Enable only once transcript quality is
		// acceptable (e.g. a better model or a paid ASR backend).
		// Subtitle source: Gemini-path clips always burn the high-quality Gemini
		// transcript; heuristic-path clips burn the local Whisper transcript only
		// when CLIP_SUBTITLES is enabled (local Uzbek ASR is rough).
		var assDoc string
		if moment.fromGemini {
			assDoc = buildGeminiASS(moment.geminiSubs, startSec, clipDur)
		} else if clipSubtitlesEnabled() {
			assDoc = buildClipASS(aiData, startSec, clipDur)
		}
		subFilter := ""
		var subPath string
		if assDoc != "" {
			if f, ferr := os.CreateTemp("", fmt.Sprintf("clip-sub-%d-*.ass", i+1)); ferr != nil {
				log.Printf("[CLIP] WARN: clip %d subtitle temp create failed: %v", i+1, ferr)
			} else {
				subPath = f.Name()
				if _, werr := f.WriteString(assDoc); werr != nil {
					log.Printf("[CLIP] WARN: clip %d subtitle temp write failed: %v", i+1, werr)
					f.Close()
					os.Remove(subPath)
					subPath = ""
				} else {
					f.Close()
					subFilter = fmt.Sprintf("subtitles=filename='%s',", ffmpegEscapeSubtitlePath(subPath))
					log.Printf("[CLIP] Clip %d: burning subtitles (%d bytes ASS)", i+1, len(assDoc))
				}
			}
		}
		if subPath != "" {
			defer os.Remove(subPath)
		}

		// Build ffmpeg args.
		// Key points:
		//   -ss / -t before -i  → input-level seek + hard duration limit (fast, reliable)
		//   -loop 1 before logo → PNG loops for the full clip so overlay never runs out of frames
		//   :shortest=1 on overlay → stop compositing when the video stream ends
		//   No -progress pipe:1 → stdout not consumed in this path; omit to avoid pipe stalls
		var args []string
		if logoExists {
			// Layout: 1080×1920 canvas with the 16:9 movie centered vertically.
			//
			// scale=1080:-2          → scale movie to 1080px wide, height auto (e.g. 608px for 16:9)
			// pad=1080:1920:0:(oh-ih)/2  → add black bands top+bottom to reach 1920px height;
			//                             movie is centred at y=(1920-608)/2=656
			// drawtext (top)         → "Kino kodi: X" just above movie frame (y=580)
			// drawtext (bottom)      → CTA text below movie frame (y=1300)
			// [1:v]scale=200:-1      → logo 200px wide for better visibility
			// overlay centered       → logo sits below CTA text, centered horizontally
			// Canvas layout (1080×1920 Reels, 16:9 movie centred):
			//   Top band   0–~656px    : movie code text at y=580
			//   Movie frame ~656–1264px
			//   Bottom band ~1264–1920px: CTA text at y=1300 (bottom ~1360),
			//                             logo centred below CTA at y=1390,
			//                             leaving ~170px safe padding from bottom edge.
			log.Printf("[CLIP] Layout: CTA y=1300, logo y=1390, canvas=1080×1920")
			filterComplex := fmt.Sprintf(
				"[0:v]scale=1080:-2,pad=1080:1920:0:(oh-ih)/2:color=black,%s"+
					"drawtext=text='%s':x=(w-text_w)/2:y=580:fontsize=48:fontcolor=white:box=1:boxcolor=black@0.7:boxborderw=8,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=1300:fontsize=44:fontcolor=white:box=1:boxcolor=black@0.7:boxborderw=8[vt];"+
					"[1:v]scale=840:-1[logo];"+
					"[vt][logo]overlay=x=(W-w)/2:y=1390:shortest=1[out]",
				subFilter, topText, bottomText,
			)
			args = []string{
				"-y",
				"-ss", fmt.Sprintf("%.3f", startSec),
				"-t", fmt.Sprintf("%.3f", clipDur),
				"-i", baseVideoPath,
				"-loop", "1", // keep logo PNG looping for full clip duration
				"-i", logoPath,
				"-filter_complex", filterComplex,
				"-map", "[out]",
				"-map", "0:a?",
				"-c:v", "libx264",
				"-preset", "veryfast",
				"-crf", "23",
				"-c:a", "aac",
				"-b:a", "128k",
				"-movflags", "+faststart",
				outPath,
			}
		} else {
			// Same layout without logo: scale+pad to 1080×1920 with centered movie, text in dark bands.
			textFilter := fmt.Sprintf(
				"scale=1080:-2,pad=1080:1920:0:(oh-ih)/2:color=black,%s"+
					"drawtext=text='%s':x=(w-text_w)/2:y=580:fontsize=48:fontcolor=white:box=1:boxcolor=black@0.7:boxborderw=8,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=1300:fontsize=44:fontcolor=white:box=1:boxcolor=black@0.7:boxborderw=8",
				subFilter, topText, bottomText,
			)
			args = []string{
				"-y",
				"-ss", fmt.Sprintf("%.3f", startSec),
				"-t", fmt.Sprintf("%.3f", clipDur),
				"-i", baseVideoPath,
				"-vf", textFilter,
				"-c:v", "libx264",
				"-preset", "veryfast",
				"-crf", "23",
				"-c:a", "aac",
				"-b:a", "128k",
				"-movflags", "+faststart",
				outPath,
			}
		}

		log.Printf("[CLIP] FFmpeg command: ffmpeg %s", strings.Join(args, " "))

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		if err := cmd.Start(); err != nil {
			log.Printf("[CLIP] ERROR: Failed to start ffmpeg for clip %d: %v", i+1, err)
			failedClips = append(failedClips, i+1)
			continue
		}

		if err := cmd.Wait(); err != nil {
			stderr := stderrBuf.String()
			log.Printf("[CLIP] ERROR: Clip %d failed: %v", i+1, err)
			log.Printf("[CLIP] FFmpeg stderr:\n%s", stderr)
			log.Printf("[CLIP] Command was: ffmpeg %s", strings.Join(args, " "))
			failedClips = append(failedClips, i+1)
			continue
		}

		// Validate the generated clip: probe actual duration and resolution.
		// Fail the clip if duration exceeds the allowed maximum.
		if actualMs, probeErr := p.getVideoDurationMs(outPath); probeErr != nil {
			log.Printf("[CLIP] WARNING: Could not probe clip %d duration: %v", i+1, probeErr)
		} else {
			actualSec := float64(actualMs) / 1000.0
			if w, h, _ := p.getInputResolution(outPath); w > 0 {
				log.Printf("[CLIP] Clip %d validated — duration=%.1fs resolution=%dx%d", i+1, actualSec, w, h)
			} else {
				log.Printf("[CLIP] Clip %d validated — duration=%.1fs", i+1, actualSec)
			}
			if actualSec > float64(maxClipSeconds)+1 {
				log.Printf("[CLIP] ERROR: Clip %d duration %.1fs exceeds max %ds — discarding", i+1, actualSec, maxClipSeconds)
				os.Remove(outPath)
				failedClips = append(failedClips, i+1)
				continue
			}
		}

		clipDuration := int(clipDur)
		var clipURL string
		if devMode {
			// Backend serves worker/uploads/movies at /stream.
			// Clips are stored at uploads/movies/clips/{folder}/{file}
			// → URL must be /stream/clips/{folder}/{file}
			clipURL = fmt.Sprintf("%s/stream/clips/%s/%s", baseURL, target.FolderSubpath, clipFilename)
		} else {
			clipURL = fmt.Sprintf("%s/videos/clips/%s/%s", baseURL, target.FolderSubpath, clipFilename)
		}

		clipInfo := ClipInfo{
			Kind:        target.Kind,
			Filename:    clipFilename,
			Path:        filepath.Join(storagePath, clipFilename),
			URL:         clipURL,
			Duration:    clipDuration,
			Sequence:    i + 1,
			ClipIndex:   i + 1,
			StorageType: map[bool]string{true: "local", false: "b2"}[devMode],
			Caption:     moment.caption,
			Hashtags:    moment.hashtags,
		}

		switch target.Kind {
		case "movie":
			var movieIDStr string
			switch v := target.MovieID.(type) {
			case primitive.ObjectID:
				movieIDStr = v.Hex()
			case string:
				movieIDStr = v
			default:
				log.Printf("[CLIP] WARNING: Unexpected MovieID type %T, using empty", target.MovieID)
				movieIDStr = ""
			}
			log.Printf("[CLIP] Using movie_id=%s for clips", movieIDStr)

			clipInfo.MovieID = movieIDStr
			clipInfo.MovieTitle = target.MovieTitle
			clipInfo.MovieSlug = target.MovieSlug
			clipInfo.MovieCode = target.MovieCode
		case "series":
			log.Printf("[CLIP] Using series_id=%s season=%d episode=%d episode_id=%s for clips",
				target.SeriesID.Hex(), target.SeasonNumber, target.EpisodeNumber, target.EpisodeID.Hex())

			clipInfo.SeriesID = target.SeriesID.Hex()
			clipInfo.SeriesTitle = target.SeriesTitle
			clipInfo.SeriesSlug = target.SeriesSlug
			clipInfo.SeasonID = target.SeasonID.Hex()
			clipInfo.SeasonNumber = target.SeasonNumber
			clipInfo.EpisodeNumber = target.EpisodeNumber
			clipInfo.EpisodeID = target.EpisodeID.Hex()
			clipInfo.MovieCode = target.SeriesCode
			clipInfo.Title = fmt.Sprintf("%s S%02dE%02d", target.SeriesTitle, target.SeasonNumber, target.EpisodeNumber)
			clipInfo.SourceType = "series_episode"
		default:
			return fmt.Errorf("unknown clipTarget.Kind %q", target.Kind)
		}

		generatedClips = append(generatedClips, clipInfo)

		log.Printf("[CLIP] Clip %d saved successfully: %s (duration: %ds)", i+1, outPath, clipDuration)
	}

	if len(generatedClips) == 0 {
		log.Printf("[CLIP] ERROR: No clips were generated successfully")
		return fmt.Errorf("no clips generated successfully")
	}

	if len(failedClips) > 0 {
		log.Printf("[CLIP] WARNING: %d clips failed: %v", len(failedClips), failedClips)
	}

	log.Printf("[CLIP] ===== UPLOAD START =====")
	log.Printf("[CLIP] Uploading %d clips to storage...", len(generatedClips))

	if !devMode {
		log.Printf("[CLIP] Production mode: uploading to B2...")
		for i := range generatedClips {
			srcPath := filepath.Join(outDir, generatedClips[i].Filename)
			remotePath := filepath.Join(storagePath, generatedClips[i].Filename)

			log.Printf("[CLIP] Uploading clip %d/%d: %s -> %s", i+1, len(generatedClips), srcPath, remotePath)

			url, err := p.storage.Upload(srcPath, remotePath)
			if err != nil {
				log.Printf("[CLIP] ERROR: Failed to upload clip %d: %v", i+1, err)
				continue
			}
			generatedClips[i].URL = url
			log.Printf("[CLIP] Uploaded: %s", url)
		}
	}
	log.Printf("[CLIP] ===== UPLOAD END =====")

	log.Printf("[CLIP] ===== MONGODB SAVE START =====")
	log.Printf("[CLIP] Saving %d clip records to MongoDB...", len(generatedClips))

	if err := p.saveClipsToMongoDB(ctx, generatedClips); err != nil {
		log.Printf("[CLIP] ERROR: Failed to save clips to MongoDB: %v", err)
		return fmt.Errorf("failed to save clips to MongoDB: %w", err)
	}

	log.Printf("[CLIP] ===== MONGODB SAVE END =====")

	log.Printf("[CLIP] ===== CLEANUP START =====")
	// Note: readyvideo folder cleanup is handled in pipeline.go after clip generation completes
	log.Printf("[CLIP] Clip generation cleanup complete")
	log.Printf("[CLIP] ===== CLEANUP END =====")

	log.Printf("[CLIP] ===== CLIP GENERATION COMPLETE =====")
	log.Printf("[CLIP] Target: %s", target.DisplayLabel)
	if target.Kind == "series" {
		log.Printf("[CLIPS] generated episode clips count=%d series_id=%s season_id=%s episode_id=%s",
			len(generatedClips), target.SeriesID.Hex(), target.SeasonID.Hex(), target.EpisodeID.Hex())
		log.Printf("[CLIP-EPISODE] series_slug=%s season=%d episode=%d clips_generated=%d clips_failed=%d",
			target.SeriesSlug, target.SeasonNumber, target.EpisodeNumber, len(generatedClips), len(failedClips))
	}
	log.Printf("[CLIP] Clips generated: %d/%d", len(generatedClips), clipCount)
	log.Printf("[CLIP] Clips failed: %d", len(failedClips))
	log.Printf("[CLIP] Output location: %s", outDir)
	if len(failedClips) > 0 {
		log.Printf("[CLIP] Failed clip indices: %v", failedClips)
	}

	return nil
}

func (p *Pipeline) saveClipsToMongoDB(ctx context.Context, clips []ClipInfo) error {
	if p.config.DB == nil {
		return fmt.Errorf("database not configured — cannot save clips")
	}

	col := p.config.DB.Collection("clips")
	movieCol := p.config.DB.Collection("movies")

	log.Printf("[CLIP] saveClipsToMongoDB: processing %d clips", len(clips))

	docs := make([]interface{}, 0, len(clips))
	for _, clip := range clips {
		// Default empty kind to "movie" to preserve behaviour for any legacy
		// call paths that never set the field.
		kind := clip.Kind
		if kind == "" {
			kind = "movie"
		}

		now := time.Now()
		switch kind {
		case "movie":
			var movieObjID primitive.ObjectID
			var movieIDValid bool

			if oid, err := primitive.ObjectIDFromHex(clip.MovieID); err == nil {
				movieObjID = oid
				movieIDValid = true
			} else {
				log.Printf("[CLIP] WARNING: could not parse movie_id %q as ObjectID: %v, trying to find by movie_code", clip.MovieID, err)
				if clip.MovieCode != "" {
					var movieDoc bson.M
					if err := movieCol.FindOne(ctx, bson.M{"code": clip.MovieCode}).Decode(&movieDoc); err == nil {
						if oid, ok := movieDoc["_id"].(primitive.ObjectID); ok {
							movieObjID = oid
							movieIDValid = true
							log.Printf("[CLIP] Found movie ObjectID %s for movie_code=%s", movieObjID.Hex(), clip.MovieCode)
						}
					} else {
						log.Printf("[CLIP] ERROR: could not find movie by code=%s: %v", clip.MovieCode, err)
					}
				}
			}

			if !movieIDValid {
				log.Printf("[CLIP] ERROR: skipping clip %s - invalid movie_id %q", clip.Filename, clip.MovieID)
				continue
			}

			docs = append(docs, bson.M{
				"_id":          primitive.NewObjectID(),
				"content_kind": "movie",
				"movie_id":     movieObjID,
				"movie_title":  clip.MovieTitle,
				"movie_slug":   clip.MovieSlug,
				"movie_code":   clip.MovieCode,
				"title":        clip.MovieTitle,
				"filename":     clip.Filename,
				"path":         clip.Path,
				"url":          clip.URL,
				"duration":     clip.Duration,
				"clip_index":   clip.ClipIndex,
				"sequence":     clip.Sequence,
				"storage_type": clip.StorageType,
				"caption":      clip.Caption,
				"hashtags":     clip.Hashtags,
				"created_at":   now,
			})

		case "series":
			seriesObjID, errS := primitive.ObjectIDFromHex(clip.SeriesID)
			if errS != nil || seriesObjID.IsZero() {
				log.Printf("[CLIP] ERROR: skipping series clip %s - invalid series_id %q: %v", clip.Filename, clip.SeriesID, errS)
				continue
			}
			episodeObjID, errE := primitive.ObjectIDFromHex(clip.EpisodeID)
			if errE != nil || episodeObjID.IsZero() {
				log.Printf("[CLIP] ERROR: skipping series clip %s - invalid episode_id %q: %v", clip.Filename, clip.EpisodeID, errE)
				continue
			}
			seasonObjID, errSeason := primitive.ObjectIDFromHex(clip.SeasonID)
			if errSeason != nil || seasonObjID.IsZero() {
				log.Printf("[CLIP] ERROR: skipping series clip %s - invalid season_id %q: %v", clip.Filename, clip.SeasonID, errSeason)
				continue
			}

			docs = append(docs, bson.M{
				"_id":            primitive.NewObjectID(),
				"content_kind":   "series",
				"series_id":      seriesObjID,
				"series_title":   clip.SeriesTitle,
				"series_slug":    clip.SeriesSlug,
				"season_id":      seasonObjID,
				"season_number":  clip.SeasonNumber,
				"episode_number": clip.EpisodeNumber,
				"episode_id":     episodeObjID,
				"movie_code":     clip.MovieCode,
				"title":          clip.Title,
				"filename":       clip.Filename,
				"path":           clip.Path,
				"url":            clip.URL,
				"duration":       clip.Duration,
				"clip_index":     clip.ClipIndex,
				"sequence":       clip.Sequence,
				"source_type":    "series_episode",
				"storage_type":   clip.StorageType,
				"caption":        clip.Caption,
				"hashtags":       clip.Hashtags,
				"created_at":     now,
			})

		default:
			log.Printf("[CLIP] ERROR: skipping clip %s - unknown kind %q", clip.Filename, kind)
			continue
		}
	}

	if len(docs) == 0 {
		return fmt.Errorf("no valid clips to save (all had invalid linkage)")
	}

	result, err := col.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to insert clips into MongoDB: %w", err)
	}

	log.Printf("[CLIP] Saved %d clips to MongoDB", len(result.InsertedIDs))
	if len(clips) > 0 && clips[0].Kind == "series" {
		log.Printf("[CLIPS] saved series episode clips count=%d series_id=%s season_id=%s episode_id=%s",
			len(result.InsertedIDs), clips[0].SeriesID, clips[0].SeasonID, clips[0].EpisodeID)
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

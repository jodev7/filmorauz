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
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const clipCount = 15
const minClipSeconds = 20
const maxClipSeconds = 60

type ClipInfo struct {
	MovieID     string `json:"movie_id"`
	MovieTitle  string `json:"movie_title"`
	MovieSlug   string `json:"movie_slug"`
	MovieCode   string `json:"movie_code"`
	Filename    string `json:"filename"`
	Path        string `json:"path"`
	URL         string `json:"url"`
	Duration    int    `json:"duration"`
	Sequence    int    `json:"sequence"`
	StorageType string `json:"storage_type"`
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
//                      one "Overall:" block per window to stderr; we split on
//                      "Overall:" and parse the first "RMS level dB:" in each
//                      section (the correct stderr format — NOT lavfi metadata).
//  3. scene detect   — downscaled frame-diff pass finds visual cut timestamps;
//                      scene cuts score a bonus as they often mark new action.
//
/// Each selected moment gets a suggested duration based on its score:
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
	const windowSec = 4.0
	const sampRate = 22050
	resetN := int(windowSec * sampRate)

	astatArgs := []string{
		"-i", videoPath,
		"-af", fmt.Sprintf("aresample=%d,astats=reset=%d", sampRate, resetN),
		"-f", "null", "-",
	}
	astatCmd := exec.Command("ffmpeg", astatArgs...)
	var astatOut bytes.Buffer
	astatCmd.Stderr = &astatOut
	_ = astatCmd.Run()
	astatOutput := astatOut.String()

	overallSections := strings.Split(astatOutput, "Overall:")
	reRMSLine := regexp.MustCompile(`RMS level dB:\s*([-\d.]+)`)

	var windows []windowScore
	for idx, section := range overallSections[1:] {
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
	log.Printf("[CLIP-ANALYSIS] Median RMS: %.1f dB", medianRMS)

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

	// isWeakHook rejects clips whose first 3s have no speech, no face, no scene change.
	isWeakHook := func(t float64) bool {
		hasScene := false
		for dt := 0.0; dt < 3.0; dt += 0.5 {
			if isNearSceneCut(t + dt) {
				hasScene = true
				break
			}
		}
		hasSpeech := false
		if ai != nil {
			for _, seg := range ai.SpeechSegments {
				if seg.Start >= t && seg.Start < t+3 {
					hasSpeech = true
					break
				}
			}
		} else {
			// No AI: use audio energy as speech proxy
			hasSpeech = windowRMSAt(t) > medianRMS-5
		}
		hasFace := false
		if ai != nil {
			for _, ff := range ai.FaceFrames {
				if ff.Time >= t && ff.Time < t+3 && ff.FaceCount > 0 {
					hasFace = true
					break
				}
			}
		}
		return !hasScene && !hasSpeech && !hasFace
	}

	// midClipBoost returns extra score if a second engagement peak exists inside [t+5, t+dur-5].
	midClipBoost := func(t, dur float64) float64 {
		innerStart := t + 5
		innerEnd := t + dur - 5
		if innerEnd <= innerStart {
			return 0
		}
		// Check for scene cut or audio spike inside the clip body (not at start)
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

	// isBadClip filters obviously poor clips.
	isBadClip := func(t, dur float64) bool {
		// >40% silence
		if silenceRatio(t, dur) > 0.40 {
			return true
		}
		// no speech AND no face AND flat scene (no change in window)
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
	log.Printf("[CLIP-AI] candidates: %d", len(candidates))

	// ── Compute viral_score for each candidate ────────────────────────────────
	// viral_score = hook*0.25 + scene*0.15 + audio_energy*0.10 + spike*0.10 +
	//               speech*0.15 + face*0.10 + emotion*0.10 + non_silence*0.05
	for i := range candidates {
		c := &candidates[i]
		clipDur := 30.0 // use 30s window for scoring; final dur assigned after selection

		hook := hookStrength(c.startSec)
		sceneAct := 0.0
		if isNearSceneCut(c.startSec) {
			sceneAct = 1.0
		}
		// normalize audio energy: clamp rms-median to [0,20] → 0–1
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
		// emotion proxy: high audio + face present → emotional
		emotionSc := 0.0
		if audioEnergy > 0.5 && faceSc > 0.2 {
			emotionSc += 0.6
		}
		if speechSc > 0.4 && faceSc > 0.15 {
			emotionSc += 0.4
		}
		emotionSc = min64(1.0, emotionSc)
		nonSilence := 1.0
		if isSilent(c.startSec, clipDur) {
			nonSilence = 0.0
		}

		midBoost := midClipBoost(c.startSec, clipDur)
		viralScore := hook*0.25 + sceneAct*0.15 + audioEnergy*0.10 + spike*0.10 +
			speechSc*0.15 + faceSc*0.10 + emotionSc*0.10 + nonSilence*0.05 + midBoost

		// Blend with legacy score for continuity when AI is unavailable.
		if ai == nil {
			viralScore = viralScore*0.3 + (c.score/20.0)*0.7
		}
		c.score = viralScore * 100 // scale to legacy range for duration thresholds
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
		// Hook rejection: skip if first 3s have no speech, face, or scene change.
		if isWeakHook(c.startSec) {
			log.Printf("[CLIP-ANALYSIS] Rejected weak-hook at %.1fs", c.startSec)
			continue
		}

		// Assign per-moment duration based on viral score.
		var dur float64
		switch {
		case c.score > 70:
			dur = 60
		case c.score > 50:
			dur = 45
		case c.score > 30:
			dur = 30
		default:
			dur = float64(minClipSeconds)
		}
		if dur > maxClipSeconds {
			dur = maxClipSeconds
		}
		if dur < minClipSeconds {
			dur = minClipSeconds
		}
		c.durationSec = dur

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

func (p *Pipeline) generateClips(ctx context.Context, canonicalFolderName string, movieCode string, movieResult *MovieCreationResult, processedMasterPath string, finalUploadsPath string) error {
	log.Printf("[CLIP] ===== CLIP GENERATION START =====")
	log.Printf("[CLIP] Movie code: %s", movieCode)
	log.Printf("[CLIP] Movie title: %s", movieResult.DisplayTitle)
	log.Printf("[CLIP] Canonical folder: %s", canonicalFolderName)
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

	movieSlug := sanitizeSlug(movieResult.Slug)
	if movieSlug == "" {
		movieSlug = sanitizeSlug(movieResult.DisplayTitle)
	}

	devMode := p.config.StorageConfig.Mode != "prod"
	var baseURL string
	var storagePath string
	var outDir string

	if devMode {
		outDir = filepath.Join(baseDir, "uploads", "movies", "clips", canonicalFolderName)
		baseURL = p.config.StorageConfig.BaseURL
		storagePath = fmt.Sprintf("videos/clips/%s", canonicalFolderName)
	} else {
		outDir = filepath.Join(baseDir, "uploads", "movies", "clips", canonicalFolderName)
		baseURL = p.config.StorageConfig.BaseURL
		storagePath = fmt.Sprintf("videos/clips/%s", canonicalFolderName)
	}

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

	// AI analysis: Whisper speech + face detection via parser service.
	// Runs concurrently with no blocking if parser is unavailable.
	log.Printf("[CLIP-AI] Starting AI analysis via parser...")
	aiData := p.fetchAIAnalysis(baseVideoPath, durationSec)
	if aiData == nil {
		log.Printf("[CLIP-AI] AI analysis unavailable — using audio/scene scoring only")
	}

	// Smart moment detection with AI-enhanced viral scoring.
	moments := p.detectEngagingMoments(baseVideoPath, durationSec, clipCount, aiData)
	if len(moments) == 0 {
		return fmt.Errorf("no valid clip moments found in video")
	}
	log.Printf("[CLIP] Starting clip generation for %d selected moments...", len(moments))

	topText := fmt.Sprintf("Kino kodi\\: %s", movieCode)
	bottomText := "Kinoni profildagi botdan toping\\!"

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
		clipFilename := fmt.Sprintf("%s_%d_%d.mp4", movieSlug, timestamp, i+1)
		outPath := filepath.Join(outDir, clipFilename)

		log.Printf("[CLIP] Generating clip %d/%d: start=%.1fs dur=%.1fs reason=%s -> %s",
			i+1, len(moments), startSec, clipDur, moment.reason, outPath)

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
				"[0:v]scale=1080:-2,pad=1080:1920:0:(oh-ih)/2:color=black,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=580:fontsize=48:fontcolor=white:box=1:boxcolor=black@0.7:boxborderw=8,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=1300:fontsize=44:fontcolor=white:box=1:boxcolor=black@0.7:boxborderw=8[vt];"+
					"[1:v]scale=840:-1[logo];"+
					"[vt][logo]overlay=x=(W-w)/2:y=1390:shortest=1[out]",
				topText, bottomText,
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
				"scale=1080:-2,pad=1080:1920:0:(oh-ih)/2:color=black,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=580:fontsize=48:fontcolor=white:box=1:boxcolor=black@0.7:boxborderw=8,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=1300:fontsize=44:fontcolor=white:box=1:boxcolor=black@0.7:boxborderw=8",
				topText, bottomText,
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
			clipURL = fmt.Sprintf("%s/stream/clips/%s/%s", baseURL, canonicalFolderName, clipFilename)
		} else {
			clipURL = fmt.Sprintf("%s/videos/clips/%s/%s", baseURL, canonicalFolderName, clipFilename)
		}

		var movieIDStr string
		switch v := movieResult.MovieID.(type) {
		case primitive.ObjectID:
			movieIDStr = v.Hex()
		case string:
			movieIDStr = v
		default:
			log.Printf("[CLIP] WARNING: Unexpected MovieID type %T, using empty", movieResult.MovieID)
			movieIDStr = ""
		}
		log.Printf("[CLIP] Using movie_id=%s for clips", movieIDStr)

		clipInfo := ClipInfo{
			MovieID:     movieIDStr,
			MovieTitle:  movieResult.DisplayTitle,
			MovieSlug:   movieResult.Slug,
			MovieCode:   movieCode,
			Filename:    clipFilename,
			Path:        filepath.Join(storagePath, clipFilename),
			URL:         clipURL,
			Duration:    clipDuration,
			Sequence:    i + 1,
			StorageType: map[bool]string{true: "local", false: "b2"}[devMode],
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
	log.Printf("[CLIP] Movie: %s (code: %s)", movieResult.DisplayTitle, movieCode)
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

		doc := bson.M{
			"_id":          primitive.NewObjectID(),
			"movie_id":     movieObjID,
			"movie_title":  clip.MovieTitle,
			"movie_slug":   clip.MovieSlug,
			"movie_code":   clip.MovieCode,
			"filename":     clip.Filename,
			"path":         clip.Path,
			"url":          clip.URL,
			"duration":     clip.Duration,
			"sequence":     clip.Sequence,
			"storage_type": clip.StorageType,
			"created_at":   time.Now(),
		}
		docs = append(docs, doc)
	}

	if len(docs) == 0 {
		return fmt.Errorf("no valid clips to save (all had invalid movie_id)")
	}

	result, err := col.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to insert clips into MongoDB: %w", err)
	}

	log.Printf("[CLIP] Saved %d clips to MongoDB", len(result.InsertedIDs))
	return nil
}

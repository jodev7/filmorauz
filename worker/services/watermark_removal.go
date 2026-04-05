package services

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
	"strings"
	"time"
)

// WatermarkRemovalConfig holds configuration for watermark removal
type WatermarkRemovalConfig struct {
	// Enabled controls whether watermark removal is performed
	Enabled bool
	// Mode can be "fast" (OpenCV) or "pro" (LaMa)
	Mode string
	// ServiceURL is the URL of the Python watermark removal service
	// If empty, uses direct Python execution
	ServiceURL string
	// TempDir is the temporary directory for processing
	TempDir string
	// SampleCount is the number of frames to sample
	SampleCount int
	// ConfidenceThreshold is the minimum confidence to proceed
	ConfidenceThreshold float64
	// MaxRegions is the maximum watermark regions to detect
	MaxRegions int
	// PythonPath is the path to the Python interpreter
	PythonPath string
	// OCREnabled enables OCR text detection
	OCREnabled bool
	// ProFallbackToFast enables automatic fallback from PRO to FAST
	ProFallbackToFast bool
}

// WatermarkRemovalResult holds the result of watermark removal
type WatermarkRemovalResult struct {
	Success           bool              `json:"success"`
	InputPath         string            `json:"input_path"`
	OutputPath        string            `json:"output_path"`
	WatermarkDetected bool              `json:"watermark_detected"`
	WatermarkRemoved  bool              `json:"watermark_removed"`
	ModeUsed          string            `json:"mode_used"`
	Regions           []WatermarkRegion `json:"regions"`
	FallbackUsed      bool              `json:"fallback_used"`
	Warning           string            `json:"warning"`
	Stages            []string          `json:"stages"`
	TotalTime         float64           `json:"total_time"`
	Error             string            `json:"error"`
}

// WatermarkRegion represents a detected watermark region
type WatermarkRegion struct {
	X               int     `json:"x"`
	Y               int     `json:"y"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	Confidence      float64 `json:"confidence"`
	WatermarkType   string  `json:"watermark_type"`
	DetectionMethod string  `json:"detection_method"`
	Location        string  `json:"location"`
	Text            string  `json:"text"`
	IsStatic        bool    `json:"is_static"`
}

// WatermarkRemovalService handles watermark removal from videos
type WatermarkRemovalService struct {
	config     WatermarkRemovalConfig
	httpClient *http.Client
}

// NewWatermarkRemovalService creates a new watermark removal service
func NewWatermarkRemovalService(config WatermarkRemovalConfig) *WatermarkRemovalService {
	if config.PythonPath == "" {
		config.PythonPath = "python3"
	}
	if config.TempDir == "" {
		config.TempDir = "/tmp/filmora_watermark"
	}
	if config.SampleCount == 0 {
		config.SampleCount = 10
	}
	if config.ConfidenceThreshold == 0 {
		config.ConfidenceThreshold = 0.65
	}
	if config.MaxRegions == 0 {
		config.MaxRegions = 3
	}

	return &WatermarkRemovalService{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

// RemoveWatermark removes watermarks from a video file
// Returns the path to the clean video (or original if no watermark or fallback)
func (s *WatermarkRemovalService) RemoveWatermark(ctx context.Context, inputPath string) (*WatermarkRemovalResult, error) {
	log.Printf("[WATERMARK] Starting watermark removal for: %s", inputPath)

	// Check if enabled
	if !s.config.Enabled {
		log.Printf("[WATERMARK] Watermark removal is disabled")
		return &WatermarkRemovalResult{
			Success:           true,
			InputPath:         inputPath,
			OutputPath:        inputPath,
			WatermarkDetected: false,
			Warning:           "Watermark removal is disabled",
		}, nil
	}

	// Validate input file exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("input file not found: %s", inputPath)
	}

	// Use HTTP service if configured
	if s.config.ServiceURL != "" {
		return s.removeViaHTTP(ctx, inputPath)
	}

	// Use direct Python execution
	return s.removeDirect(ctx, inputPath)
}

// removeViaHTTP calls the Python watermark removal HTTP service
func (s *WatermarkRemovalService) removeViaHTTP(ctx context.Context, inputPath string) (*WatermarkRemovalResult, error) {
	url := fmt.Sprintf("%s/remove", s.config.ServiceURL)

	requestBody := map[string]string{
		"input_path": inputPath,
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[WATERMARK] Calling HTTP service: %s", url)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP service returned status %d", resp.StatusCode)
	}

	var result WatermarkRemovalResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// removeDirect executes the watermark removal Python runner directly.
// Uses worker/watermark_removal/run.py — a fixed-path script that correctly
// sets sys.path before importing the watermark_removal package.
func (s *WatermarkRemovalService) removeDirect(ctx context.Context, inputPath string) (*WatermarkRemovalResult, error) {
	// Generate output path
	outputDir := filepath.Dir(inputPath)
	outputName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(outputDir, outputName+"_clean.mp4")

	// Resolve the worker directory so we can find watermark_removal/run.py.
	// The worker dir contains the watermark_removal/ Python package.
	workerDir := s.resolveWorkerDir()
	scriptPath := filepath.Join(workerDir, "watermark_removal", "run.py")

	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		log.Printf("[WATERMARK] Runner script not found at %s — watermark removal unavailable", scriptPath)
		return &WatermarkRemovalResult{
			Success:      true,
			InputPath:    inputPath,
			OutputPath:   inputPath,
			FallbackUsed: true,
			Warning:      fmt.Sprintf("watermark removal script not found: %s", scriptPath),
		}, nil
	}

	log.Printf("[WATERMARK] Using runner: %s", scriptPath)
	log.Printf("[WATERMARK] Worker dir (PYTHONPATH): %s", workerDir)

	// Build Python command. stdout = JSON result, stderr = logs (captured separately).
	cmd := exec.CommandContext(ctx, s.config.PythonPath, scriptPath, inputPath, outputPath)
	cmd.Env = append(os.Environ(),
		// Make `import watermark_removal` work from run.py
		fmt.Sprintf("PYTHONPATH=%s", workerDir),
		fmt.Sprintf("WATERMARK_ENABLED=%t", s.config.Enabled),
		fmt.Sprintf("WATERMARK_MODE=%s", s.config.Mode),
		fmt.Sprintf("WATERMARK_SAMPLE_COUNT=%d", s.config.SampleCount),
		fmt.Sprintf("WATERMARK_CONFIDENCE_THRESHOLD=%f", s.config.ConfidenceThreshold),
		fmt.Sprintf("WATERMARK_MAX_REGIONS=%d", s.config.MaxRegions),
		fmt.Sprintf("WATERMARK_TEMP_DIR=%s", s.config.TempDir),
		fmt.Sprintf("WATERMARK_OCR_ENABLED=%t", s.config.OCREnabled),
		fmt.Sprintf("WATERMARK_PRO_FALLBACK_TO_FAST=%t", s.config.ProFallbackToFast),
	)

	// Separate stdout (JSON) from stderr (logs) so mixing doesn't break JSON parse.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	log.Printf("[WATERMARK] Starting: %s %s %s %s", s.config.PythonPath, scriptPath, inputPath, outputPath)

	err := cmd.Run()

	// Always surface Python logs regardless of success/failure
	if stderrLog := strings.TrimSpace(stderrBuf.String()); stderrLog != "" {
		log.Printf("[WATERMARK] Python log:\n%s", stderrLog)
	}

	if err != nil {
		log.Printf("[WATERMARK] Python process error: %v", err)
		log.Printf("[WATERMARK] stdout: %s", strings.TrimSpace(stdoutBuf.String()))
		return &WatermarkRemovalResult{
			Success:      true,
			InputPath:    inputPath,
			OutputPath:   inputPath,
			FallbackUsed: true,
			Warning:      fmt.Sprintf("watermark removal process failed: %v", err),
			Error:        stderrBuf.String(),
		}, nil
	}

	jsonOutput := strings.TrimSpace(stdoutBuf.String())
	log.Printf("[WATERMARK] Python result JSON: %s", jsonOutput)

	// Parse JSON result from stdout
	var result WatermarkRemovalResult
	if err := json.Unmarshal([]byte(jsonOutput), &result); err != nil {
		log.Printf("[WATERMARK] Failed to parse JSON result: %v (raw: %q)", err, jsonOutput)
		// If an output file was created anyway, trust it
		if s.fileExists(outputPath) {
			log.Printf("[WATERMARK] Output file exists despite parse error, using it")
			return &WatermarkRemovalResult{
				Success:           true,
				InputPath:         inputPath,
				OutputPath:        outputPath,
				WatermarkDetected: true,
				Warning:           "JSON parse failed but output file exists",
			}, nil
		}
		return &WatermarkRemovalResult{
			Success:      true,
			InputPath:    inputPath,
			OutputPath:   inputPath,
			FallbackUsed: true,
			Warning:      fmt.Sprintf("JSON parse failed: %v", err),
		}, nil
	}

	// No watermark found → use original
	if !result.WatermarkDetected {
		result.OutputPath = inputPath
	}

	// Fallback or missing/invalid output → use original
	if result.FallbackUsed || result.OutputPath == "" || !s.fileExists(result.OutputPath) {
		result.OutputPath = inputPath
		result.FallbackUsed = true
	}

	return &result, nil
}

// resolveWorkerDir returns the directory that contains the watermark_removal Python package.
// It checks several locations in order of reliability.
func (s *WatermarkRemovalService) resolveWorkerDir() string {
	const marker = "watermark_removal/__init__.py"

	// 1. Explicit override via env var
	if dir := os.Getenv("WORKER_DIR"); dir != "" {
		if s.fileExists(filepath.Join(dir, marker)) {
			return dir
		}
	}

	// 2. Directory of the running executable (works for compiled binaries)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if s.fileExists(filepath.Join(dir, marker)) {
			return dir
		}
	}

	// 3. Current working directory (works for `go run` in the worker directory)
	if cwd, err := os.Getwd(); err == nil {
		if s.fileExists(filepath.Join(cwd, marker)) {
			return cwd
		}
	}

	// 4. Fallback: assume cwd is correct
	cwd, _ := os.Getwd()
	return cwd
}

// fileExists checks if a file exists
func (s *WatermarkRemovalService) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DetectWatermarkOnly detects watermarks without removal (for testing/analysis)
func (s *WatermarkRemovalService) DetectWatermarkOnly(ctx context.Context, videoPath string) (*WatermarkRemovalResult, error) {
	if !s.config.Enabled {
		return &WatermarkRemovalResult{
			Success:           true,
			InputPath:         videoPath,
			OutputPath:        videoPath,
			WatermarkDetected: false,
			Warning:           "Watermark detection is disabled",
		}, nil
	}

	// Use HTTP service if configured
	if s.config.ServiceURL != "" {
		url := fmt.Sprintf("%s/detect?path=%s", s.config.ServiceURL, videoPath)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var result WatermarkRemovalResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		return &result, nil
	}

	// Direct detection not implemented in this version
	return &WatermarkRemovalResult{
		Success:           true,
		InputPath:         videoPath,
		OutputPath:        videoPath,
		WatermarkDetected: false,
		Warning:           "Direct detection not implemented, use service mode",
	}, nil
}

// DefaultWatermarkRemovalConfig returns default configuration
func DefaultWatermarkRemovalConfig() WatermarkRemovalConfig {
	enabled := os.Getenv("WATERMARK_ENABLED") == "" || os.Getenv("WATERMARK_ENABLED") == "true"
	mode := os.Getenv("WATERMARK_MODE")
	if mode == "" {
		mode = "fast"
	}
	ocrEnabled := os.Getenv("WATERMARK_OCR_ENABLED") != "false"
	proFallback := os.Getenv("WATERMARK_PRO_FALLBACK_TO_FAST") != "false"

	return WatermarkRemovalConfig{
		Enabled:             enabled,
		Mode:                mode,
		ServiceURL:          os.Getenv("WATERMARK_SERVICE_URL"),
		TempDir:             os.Getenv("WATERMARK_TEMP_DIR"),
		SampleCount:         getEnvInt("WATERMARK_SAMPLE_COUNT", 10),
		ConfidenceThreshold: getEnvFloat("WATERMARK_CONFIDENCE_THRESHOLD", 0.65),
		MaxRegions:          getEnvInt("WATERMARK_MAX_REGIONS", 3),
		PythonPath:          os.Getenv("WATERMARK_PYTHON_PATH"),
		OCREnabled:          ocrEnabled,
		ProFallbackToFast:   proFallback,
	}
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var result int
		if _, err := fmt.Sscanf(val, "%d", &result); err == nil {
			return result
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		var result float64
		if _, err := fmt.Sscanf(val, "%f", &result); err == nil {
			return result
		}
	}
	return defaultVal
}

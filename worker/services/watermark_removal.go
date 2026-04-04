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

// removeDirect executes Python script directly for watermark removal
func (s *WatermarkRemovalService) removeDirect(ctx context.Context, inputPath string) (*WatermarkRemovalResult, error) {
	// Generate output path
	outputDir := filepath.Dir(inputPath)
	outputName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(outputDir, outputName+"_clean.mp4")

	// Ensure temp directory exists
	if err := os.MkdirAll(s.config.TempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Create Python script for watermark removal
	script := s.createPythonScript()

	// Write script to temp file
	scriptPath := filepath.Join(s.config.TempDir, "watermark_remove.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Build Python command
	cmd := exec.CommandContext(ctx, s.config.PythonPath, scriptPath, inputPath, outputPath)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("WATERMARK_ENABLED=%t", s.config.Enabled),
		fmt.Sprintf("WATERMARK_MODE=%s", s.config.Mode),
		fmt.Sprintf("WATERMARK_SAMPLE_COUNT=%d", s.config.SampleCount),
		fmt.Sprintf("WATERMARK_CONFIDENCE_THRESHOLD=%f", s.config.ConfidenceThreshold),
		fmt.Sprintf("WATERMARK_MAX_REGIONS=%d", s.config.MaxRegions),
		fmt.Sprintf("WATERMARK_TEMP_DIR=%s", s.config.TempDir),
		fmt.Sprintf("WATERMARK_OCR_ENABLED=%t", s.config.OCREnabled),
		fmt.Sprintf("WATERMARK_PRO_FALLBACK_TO_FAST=%t", s.config.ProFallbackToFast),
	)

	log.Printf("[WATERMARK] Running Python script...")
	log.Printf("[WATERMARK] Command: %s %s %s %s", s.config.PythonPath, scriptPath, inputPath, outputPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[WATERMARK] Python script error: %v", err)
		log.Printf("[WATERMARK] Output: %s", string(output))
		// Fallback to original video
		return &WatermarkRemovalResult{
			Success:           true,
			InputPath:         inputPath,
			OutputPath:        inputPath,
			WatermarkDetected: true,
			FallbackUsed:      true,
			Warning:           fmt.Sprintf("Watermark removal failed: %v", err),
			Error:             string(output),
		}, nil
	}

	log.Printf("[WATERMARK] Python output: %s", string(output))

	// Parse JSON result
	var result WatermarkRemovalResult
	if err := json.Unmarshal(output, &result); err != nil {
		log.Printf("[WATERMARK] Failed to parse result: %v", err)
		// Check if output video was created
		if _, err := os.Stat(outputPath); err == nil {
			return &WatermarkRemovalResult{
				Success:           true,
				InputPath:         inputPath,
				OutputPath:        outputPath,
				WatermarkDetected: true,
				Warning:           "Could not parse result, but output was created",
			}, nil
		}
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	// If no watermark detected, use original
	if !result.WatermarkDetected {
		result.OutputPath = inputPath
	}

	// If fallback used or no clean output, use original
	if result.FallbackUsed || result.OutputPath == "" || !s.fileExists(result.OutputPath) {
		result.OutputPath = inputPath
		result.FallbackUsed = true
	}

	return &result, nil
}

// createPythonScript creates the Python script for watermark removal
func (s *WatermarkRemovalService) createPythonScript() string {
	return `#!/usr/bin/env python3
"""
Watermark Removal Script - Called by Go worker
"""
import sys
import os
import json
import logging

# Add watermark_removal to path - look for it relative to this script's location
script_dir = os.path.dirname(os.path.abspath(__file__))
# Try different possible locations for watermark_removal package
possible_paths = [
    os.path.join(script_dir, "watermark_removal"),
    os.path.join(script_dir, "..", "watermark_removal"),
    os.path.join(os.path.dirname(script_dir), "watermark_removal"),
]

watermark_path = None
for p in possible_paths:
    if os.path.exists(p) and os.path.isdir(p):
        watermark_path = os.path.dirname(p)
        break

if watermark_path and watermark_path not in sys.path:
    sys.path.insert(0, watermark_path)

try:
    from watermark_removal import WatermarkRemovalPipeline, load_config
except ImportError as e:
    print(json.dumps({"error": f"Import error: {e}"}))
    sys.exit(1)

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(message)s')
logger = logging.getLogger(__name__)


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"error": "Usage: script.py <input_path> <output_path>"}))
        sys.exit(1)
    
    input_path = sys.argv[1]
    output_path = sys.argv[2]
    
    if not os.path.exists(input_path):
        print(json.dumps({"error": f"Input file not found: {input_path}"}))
        sys.exit(1)
    
    try:
        # Load configuration from environment
        config = load_config()
        
        # Create pipeline
        pipeline = WatermarkRemovalPipeline(config=config)
        
        # Process video
        result = pipeline.process(input_path, output_path)
        
        # Output JSON result
        print(json.dumps(result.to_dict()))
        
    except Exception as e:
        print(json.dumps({"error": str(e), "success": False}))
        sys.exit(1)


if __name__ == "__main__":
    main()
`
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

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ProcessRequest represents a request to process a video
type ProcessRequest struct {
	Source       string  `json:"source"`
	SourceID     string  `json:"source_id,omitempty"`
	SourceURL    string  `json:"source_url,omitempty"`
	Title        string  `json:"title,omitempty"`
	InputFile    string  `json:"input_file"`              // Required: path to downloaded file
	OutputFile   string  `json:"output_file,omitempty"`   // Optional: output filename
	CutSeconds   int     `json:"cut_seconds,omitempty"`   // Optional: skip first N seconds
	LogoPath     string  `json:"logo_path,omitempty"`     // Optional: watermark overlay
	LogoPosition string  `json:"logo_position,omitempty"` // Optional: position like "W-w-10:10"
	LogoOpacity  float64 `json:"logo_opacity,omitempty"`  // Optional: 0.0-1.0
}

// ProcessResponse represents the response from processing
type ProcessResponse struct {
	Success       bool   `json:"success"`
	InputFile     string `json:"input_file"`
	OutputFile    string `json:"output_file,omitempty"`
	CutSeconds    int    `json:"cut_seconds"`
	LogoApplied   bool   `json:"logo_applied"`
	FFmpegCommand string `json:"ffmpeg_command,omitempty"`
	Error         string `json:"error,omitempty"`
	Duration      string `json:"duration,omitempty"`
}

// ProcessHandler handles video processing requests
type ProcessHandler struct {
	outputDir string
	tempDir   string
}

// NewProcessHandler creates a new process handler
func NewProcessHandler(outputDir, tempDir string) *ProcessHandler {
	// Ensure directories exist
	os.MkdirAll(outputDir, 0755)
	os.MkdirAll(tempDir, 0755)

	return &ProcessHandler{
		outputDir: outputDir,
		tempDir:   tempDir,
	}
}

// ServeHTTP handles HTTP requests
func (h *ProcessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[WORKER] %s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path != "/process" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	h.handleProcess(w, r)
}

func (h *ProcessHandler) handleProcess(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendError(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse request
	var req ProcessRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendError(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.InputFile == "" {
		h.sendError(w, "input_file is required", http.StatusBadRequest)
		return
	}

	log.Printf("[WORKER] Process request: input=%s, output=%s, cut=%d, logo=%s",
		req.InputFile, req.OutputFile, req.CutSeconds, req.LogoPath)

	// Process the video
	resp := h.processVideo(req)

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (h *ProcessHandler) processVideo(req ProcessRequest) *ProcessResponse {
	resp := &ProcessResponse{
		Success:     false,
		InputFile:   req.InputFile,
		CutSeconds:  req.CutSeconds,
		LogoApplied: req.LogoPath != "" && req.LogoPath != "none",
	}

	// Validate input file exists
	if _, err := os.Stat(req.InputFile); os.IsNotExist(err) {
		resp.Error = fmt.Sprintf("Input file does not exist: %s", req.InputFile)
		return resp
	}

	// Determine output file
	outputFile := req.OutputFile
	if outputFile == "" {
		// Generate output filename from input
		baseName := filepath.Base(req.InputFile)
		ext := filepath.Ext(baseName)
		nameWithoutExt := strings.TrimSuffix(baseName, ext)
		outputFile = filepath.Join(h.outputDir, nameWithoutExt+"_processed.mp4")
	} else if !filepath.IsAbs(outputFile) {
		// Make relative paths absolute
		outputFile = filepath.Join(h.outputDir, outputFile)
	}

	// Ensure output directory exists
	outDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		resp.Error = fmt.Sprintf("Failed to create output directory: %v", err)
		return resp
	}

	// Build FFmpeg command
	cmd := h.buildFFmpegCommand(req, outputFile)
	resp.FFmpegCommand = strings.Join(cmd.Args, " ")

	log.Printf("[WORKER] Running FFmpeg: %s", resp.FFmpegCommand)

	// Execute FFmpeg
	startTime := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(startTime)

	if err != nil {
		resp.Error = fmt.Sprintf("FFmpeg failed: %v\nOutput: %s", err, string(output))
		log.Printf("[WORKER] FFmpeg failed: %v", err)
		return resp
	}

	// Verify output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		resp.Error = "FFmpeg completed but output file not found"
		return resp
	}

	resp.Success = true
	resp.OutputFile = outputFile
	resp.Duration = elapsed.Round(time.Second).String()

	log.Printf("[WORKER] Processing completed: %s -> %s (took %s)", req.InputFile, outputFile, resp.Duration)

	return resp
}

func (h *ProcessHandler) buildFFmpegCommand(req ProcessRequest, outputFile string) *exec.Cmd {
	// Base FFmpeg arguments
	args := []string{
		"-y", // Overwrite output
		"-i", req.InputFile,
		"-c:v", "libx264", // Video codec
		"-preset", "medium",
		"-crf", "23",
		"-c:a", "aac", // Audio codec
		"-b:a", "128k",
		"-movflags", "+faststart", // Enable fast start for web playback
	}

	// Add cut/trim if specified
	if req.CutSeconds > 0 {
		args = append(args, "-ss", fmt.Sprintf("%d", req.CutSeconds))
		log.Printf("[WORKER] Will skip first %d seconds", req.CutSeconds)
	}

	// Add logo overlay if specified
	if req.LogoPath != "" && req.LogoPath != "none" {
		// Check if logo file exists
		if _, err := os.Stat(req.LogoPath); err == nil {
			// Position: default to bottom-right with padding
			position := req.LogoPosition
			if position == "" {
				position = "W-w-10:10" // 10px from bottom-right
			}

			// Opacity: default to 80%
			opacity := req.LogoOpacity
			if opacity <= 0 {
				opacity = 0.8
			}

			// FFmpeg overlay filter for logo
			// Using overlay filter with alpha channel support
			overlayFilter := fmt.Sprintf("'overlay=%s:format=auto'", position)

			args = append(args,
				"-i", req.LogoPath, // Second input: logo
				"-filter_complex", overlayFilter,
			)

			log.Printf("[WORKER] Adding logo overlay: %s at position %s with opacity %.0f%%",
				req.LogoPath, position, opacity*100)
		} else {
			log.Printf("[WORKER] Logo file not found, skipping overlay: %s", req.LogoPath)
		}
	}

	// Output file
	args = append(args, outputFile)

	return exec.Command("ffmpeg", args...)
}

func (h *ProcessHandler) sendError(w http.ResponseWriter, message string, status int) {
	log.Printf("[WORKER] Error: %s", message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

// Getter for LogoPath (field is lowercase in struct)
func (r *ProcessRequest) getLogoPath() string {
	return r.LogoPath
}

// Fix: properly access logo path
func (h *ProcessHandler) buildFFmpegCommandFixed(req ProcessRequest, outputFile string) *exec.Cmd {
	args := []string{
		"-y",
		"-i", req.InputFile,
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
	}

	if req.CutSeconds > 0 {
		args = append(args, "-ss", fmt.Sprintf("%d", req.CutSeconds))
	}

	// Access logo path properly - it's a field in the struct
	logoPath := req.LogoPath
	if logoPath != "" && logoPath != "none" {
		if _, err := os.Stat(logoPath); err == nil {
			position := req.LogoPosition
			if position == "" {
				position = "W-w-10:10"
			}
			overlayFilter := fmt.Sprintf("'overlay=%s:format=auto'", position)
			args = append(args, "-i", logoPath, "-filter_complex", overlayFilter)
			log.Printf("[WORKER] Adding logo: %s at %s", logoPath, position)
		}
	}

	args = append(args, outputFile)
	return exec.Command("ffmpeg", args...)
}

package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// MediaClient talks to the Python parser's universal yt-dlp endpoints
// (/media/probe, /media/download, /media/status, /media/file) used by the
// superadmin "paste any link" download flow.
type MediaClient struct {
	baseURL    string
	http       *http.Client // short-timeout client for probe/status/start
	streamHTTP *http.Client // no-timeout client for large file streaming
}

// NewMediaClient creates a MediaClient pointed at the parser base URL.
func NewMediaClient(baseURL string) *MediaClient {
	return &MediaClient{
		baseURL:    baseURL,
		http:       &http.Client{Timeout: 140 * time.Second},
		streamHTTP: &http.Client{Timeout: 0},
	}
}

// AudioLang is one selectable audio track.
type AudioLang struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ProbeResult describes what qualities/audio tracks a URL offers.
type ProbeResult struct {
	Title      string      `json:"title"`
	Duration   float64     `json:"duration"`
	Extractor  string      `json:"extractor"`
	Qualities  []int       `json:"qualities"`
	AudioLangs []AudioLang `json:"audio_langs"`
}

// MediaStatus is a download progress snapshot.
type MediaStatus struct {
	Status   string  `json:"status"`
	Percent  float64 `json:"percent"`
	FileSize int64   `json:"file_size"`
	Error    string  `json:"error"`
}

// parseEnvelope pulls flat fields out of the parser's {success,error,...} envelope.
func decodeParserResp(body []byte, out interface{}) error {
	// The parser copies flat fields to the root of the envelope, so decoding
	// straight into the target struct works. But first check for an error.
	var env struct {
		Success *bool  `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err == nil {
		if env.Success != nil && !*env.Success {
			if env.Error == "" {
				env.Error = "parser error"
			}
			return fmt.Errorf("%s", env.Error)
		}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// Probe lists available qualities and audio languages for a URL.
func (c *MediaClient) Probe(mediaURL string) (*ProbeResult, error) {
	u := fmt.Sprintf("%s/media/probe?url=%s", c.baseURL, url.QueryEscape(mediaURL))
	resp, err := c.http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var out ProbeResult
	if err := decodeParserResp(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StartDownload kicks off a background download and returns the job ID.
func (c *MediaClient) StartDownload(mediaURL string, height int, audioLang string) (string, error) {
	reqBody := map[string]interface{}{"url": mediaURL}
	if height > 0 {
		reqBody["height"] = height
	}
	if audioLang != "" {
		reqBody["audio_lang"] = audioLang
	}
	payload, _ := json.Marshal(reqBody)

	resp, err := c.http.Post(c.baseURL+"/media/download", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var out struct {
		JobID string `json:"job_id"`
	}
	if err := decodeParserResp(body, &out); err != nil {
		return "", err
	}
	if out.JobID == "" {
		return "", fmt.Errorf("parser did not return a job_id")
	}
	return out.JobID, nil
}

// Status polls a download's progress.
func (c *MediaClient) Status(jobID string) (*MediaStatus, error) {
	u := fmt.Sprintf("%s/media/status?job_id=%s", c.baseURL, url.QueryEscape(jobID))
	resp, err := c.http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var out MediaStatus
	if err := decodeParserResp(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FileStream opens the finished file for streaming. Caller must Close the
// returned reader. contentLength is -1 if unknown.
func (c *MediaClient) FileStream(jobID string) (io.ReadCloser, int64, error) {
	u := fmt.Sprintf("%s/media/file?job_id=%s", c.baseURL, url.QueryEscape(jobID))
	resp, err := c.streamHTTP.Get(u)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, 0, fmt.Errorf("file not available (status %d): %s", resp.StatusCode, string(body))
	}
	return resp.Body, resp.ContentLength, nil
}

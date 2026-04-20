// Command b2-cors inspects, applies, and probes the Backblaze B2 bucket CORS
// rules required for browser-to-B2 direct uploads and for HLS playback of
// .m3u8 / .ts segments served through cdn.filmorauz.net.
//
// Why this exists:
//   B2 bucket CORS is configured on B2's side via the b2_update_bucket API —
//   not in this repo's code. Two symptoms of missing/incorrect rules:
//     - Upload: 401 on preflight OPTIONS, upload never starts.
//     - Playback: "No 'Access-Control-Allow-Origin' header" on .m3u8 / .ts
//       requests, video player fails to start.
//   This tool writes both rules in one call (b2_update_bucket replaces the
//   entire corsRules array, so they must be sent together).
//
// Cloudflare note:
//   cdn.filmorauz.net fronts the bucket, so even a correctly-configured B2
//   rule can end up hidden if Cloudflare cached a non-CORS response (its
//   default cache key does not vary on the Origin request header). If the
//   --probe output below shows B2 returning CORS headers but the CDN URL
//   not, add a Cloudflare Transform Rule on the cdn.filmorauz.net zone:
//     When:  Hostname equals cdn.filmorauz.net
//     Then:  Set static response headers
//            Access-Control-Allow-Origin: https://filmorauz.net
//            Access-Control-Allow-Methods: GET, HEAD, OPTIONS
//            Access-Control-Allow-Headers: Range, Origin, Accept, Content-Type
//            Access-Control-Expose-Headers: Content-Length, Content-Range
//   Then purge the cdn.filmorauz.net cache so the next browser fetch sees
//   the new headers.
//
// Usage:
//   cd backend
//   go run ./cmd/b2-cors                                    # dry-run / inspect
//   go run ./cmd/b2-cors --apply                            # apply defaults
//   go run ./cmd/b2-cors --apply --origin https://a.tld,http://localhost:3000
//   go run ./cmd/b2-cors --probe https://cdn.filmorauz.net/file/filmorauznet/videos/<slug>/master.m3u8
//
// The tool reads B2 credentials and bucket name from backend/.env (same as
// the API server), so there is nothing to configure twice.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
)

type b2Authorize struct {
	AccountID          string `json:"accountId"`
	AuthorizationToken string `json:"authorizationToken"`
	APIURL             string `json:"apiUrl"`
	Allowed            struct {
		BucketID   string `json:"bucketId"`
		BucketName string `json:"bucketName"`
	} `json:"allowed"`
}

type b2CorsRule struct {
	CorsRuleName      string   `json:"corsRuleName"`
	AllowedOrigins    []string `json:"allowedOrigins"`
	AllowedOperations []string `json:"allowedOperations"`
	AllowedHeaders    []string `json:"allowedHeaders"`
	ExposeHeaders     []string `json:"exposeHeaders"`
	MaxAgeSeconds     int      `json:"maxAgeSeconds"`
}

type b2Bucket struct {
	BucketID   string       `json:"bucketId"`
	BucketName string       `json:"bucketName"`
	CorsRules  []b2CorsRule `json:"corsRules"`
}

type b2ListBucketsResponse struct {
	Buckets []b2Bucket `json:"buckets"`
}

func main() {
	apply := flag.Bool("apply", false, "apply CORS rules (otherwise just inspect current rules)")
	originsFlag := flag.String("origin", "", "comma-separated list of allowed origins (default: BASE_SITE_URL + www variant + http://localhost:3000)")
	ruleName := flag.String("name", "filmorauzDirectUpload", "CORS rule name")
	maxAge := flag.Int("max-age", 3600, "preflight cache max-age in seconds")
	probeURL := flag.String("probe", "", "probe CORS response on the given CDN URL (e.g. https://cdn.filmorauz.net/.../master.m3u8); no B2 changes")
	flag.Parse()

	cfg := config.Load()
	origins := buildOrigins(*originsFlag, cfg.BaseSiteURL)

	// Probe mode exits without touching B2 — it just reports what the CDN
	// is currently returning for a CORS preflight and a ranged GET.
	if strings.TrimSpace(*probeURL) != "" {
		probeCORS(*probeURL, firstOrigin(origins))
		return
	}

	if cfg.B2KeyID == "" || cfg.B2AppKey == "" || cfg.B2Bucket == "" {
		log.Fatalf("B2_KEY_ID / B2_APP_KEY / B2_BUCKET must be set in backend/.env")
	}

	log.Printf("allowed origins: %v", origins)

	auth, err := authorizeB2(cfg.B2KeyID, cfg.B2AppKey)
	if err != nil {
		log.Fatalf("authorize failed: %v", err)
	}

	bucket, err := findBucket(auth, cfg.B2Bucket)
	if err != nil {
		log.Fatalf("find bucket %q failed: %v", cfg.B2Bucket, err)
	}
	log.Printf("bucket: name=%s id=%s", bucket.BucketName, bucket.BucketID)
	printCurrentRules(bucket.CorsRules)

	desired := []b2CorsRule{
		{
			CorsRuleName:   *ruleName,
			AllowedOrigins: origins,
			AllowedOperations: []string{
				"b2_upload_file",
				"b2_upload_part",
			},
			AllowedHeaders: []string{
				"authorization",
				"x-bz-file-name",
				"x-bz-content-sha1",
				"content-type",
			},
			ExposeHeaders: []string{},
			MaxAgeSeconds: *maxAge,
		},
		// Playback rule: browser fetches .m3u8 / .ts segments from the bucket
		// (directly or via Cloudflare in front of it). hls.js issues ranged
		// GETs for segments, so Range must be in AllowedHeaders, and
		// Content-Length / Content-Range / Accept-Ranges must be exposed so
		// the player can seek. Header names match the task spec exactly.
		{
			CorsRuleName:   "filmorauzPlayback",
			AllowedOrigins: origins,
			AllowedOperations: []string{
				"b2_download_file_by_name",
				"b2_download_file_by_id",
				"s3_get",
				"s3_head",
			},
			AllowedHeaders: []string{
				"Range",
				"Origin",
				"Accept",
				"Content-Type",
				"Authorization",
			},
			ExposeHeaders: []string{
				"Content-Length",
				"Content-Range",
				"Content-Type",
				"Accept-Ranges",
				"ETag",
			},
			MaxAgeSeconds: *maxAge,
		},
	}

	if !*apply {
		log.Printf("dry-run: would apply %d rule(s). Re-run with --apply to write.", len(desired))
		if out, _ := json.MarshalIndent(desired, "", "  "); len(out) > 0 {
			fmt.Println(string(out))
		}
		return
	}

	updated, err := updateBucketCors(auth, bucket.BucketID, desired)
	if err != nil {
		log.Fatalf("update bucket CORS failed: %v", err)
	}
	log.Printf("applied %d CORS rule(s).", len(updated))
	printCurrentRules(updated)
	log.Printf("done. You may need to wait ~30s for B2 edge to pick this up, then retry the browser upload.")
}

func buildOrigins(flagValue, baseSite string) []string {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		site := strings.TrimSpace(baseSite)
		if site == "" {
			site = "https://filmorauz.net"
		}
		// Also allow the www-variant of the apex — browsers on www.filmorauz.net
		// send Origin: https://www.filmorauz.net, which must match a rule or
		// B2 returns no Access-Control-Allow-Origin.
		origins := []string{site, "http://localhost:3000"}
		if strings.HasPrefix(site, "https://") && !strings.Contains(site, "://www.") {
			origins = append(origins, "https://www."+strings.TrimPrefix(site, "https://"))
		}
		return dedup(origins)
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return dedup(out)
}

func firstOrigin(origins []string) string {
	for _, o := range origins {
		if strings.HasPrefix(o, "http://") || strings.HasPrefix(o, "https://") {
			return o
		}
	}
	return "https://filmorauz.net"
}

// probeCORS fires a real preflight + ranged GET against the given URL and
// prints the CORS-relevant response headers. Use this after --apply to
// confirm the CDN is echoing the rules B2 was told to serve.
func probeCORS(target, origin string) {
	client := &http.Client{Timeout: 20 * time.Second}

	log.Printf("probing target: %s", target)
	log.Printf("probe origin:   %s", origin)

	// --- Preflight (OPTIONS) ---
	fmt.Println()
	fmt.Println("=== OPTIONS preflight ===")
	preReq, err := http.NewRequest("OPTIONS", target, nil)
	if err != nil {
		log.Fatalf("build preflight: %v", err)
	}
	preReq.Header.Set("Origin", origin)
	preReq.Header.Set("Access-Control-Request-Method", "GET")
	preReq.Header.Set("Access-Control-Request-Headers", "Range")
	printCORSResponse(client, preReq)

	// --- Ranged GET (mirrors hls.js segment fetch) ---
	fmt.Println()
	fmt.Println("=== Ranged GET ===")
	getReq, err := http.NewRequest("GET", target, nil)
	if err != nil {
		log.Fatalf("build GET: %v", err)
	}
	getReq.Header.Set("Origin", origin)
	getReq.Header.Set("Range", "bytes=0-0")
	printCORSResponse(client, getReq)

	fmt.Println()
	fmt.Println("Interpretation:")
	fmt.Println("  - OPTIONS missing Access-Control-Allow-Origin => B2 rule missing or not applied")
	fmt.Println("    Fix: run `make b2-cors-apply` (rules above match the task spec).")
	fmt.Println("  - OPTIONS OK but GET missing the header => Cloudflare cached a non-CORS response.")
	fmt.Println("    Fix: add the Cloudflare Transform Rule described at the top of this file,")
	fmt.Println("    then purge the cdn.filmorauz.net cache.")
}

func printCORSResponse(client *http.Client, req *http.Request) {
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  request error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	// Drain a small chunk so the server completes even on GET.
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)
	fmt.Printf("  %s %s -> %d %s\n", req.Method, req.URL.String(), resp.StatusCode, resp.Status)
	corsHeaders := []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Expose-Headers",
		"Access-Control-Max-Age",
		"Vary",
		"Cf-Cache-Status",
		"Server",
	}
	for _, h := range corsHeaders {
		if v := resp.Header.Get(h); v != "" {
			fmt.Printf("  %s: %s\n", h, v)
		} else {
			fmt.Printf("  %s: (missing)\n", h)
		}
	}
}

func dedup(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func authorizeB2(keyID, appKey string) (*b2Authorize, error) {
	req, err := http.NewRequest("GET", "https://api.backblazeb2.com/b2api/v2/b2_authorize_account", nil)
	if err != nil {
		return nil, err
	}
	creds := base64.StdEncoding.EncodeToString([]byte(keyID + ":" + appKey))
	req.Header.Set("Authorization", "Basic "+creds)
	body, _, err := doJSON(req)
	if err != nil {
		return nil, err
	}
	var out b2Authorize
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode authorize: %w (body=%s)", err, string(body))
	}
	return &out, nil
}

func findBucket(auth *b2Authorize, name string) (*b2Bucket, error) {
	payload, _ := json.Marshal(map[string]string{
		"accountId":  auth.AccountID,
		"bucketName": name,
	})
	req, err := http.NewRequest("POST", auth.APIURL+"/b2api/v2/b2_list_buckets", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth.AuthorizationToken)
	req.Header.Set("Content-Type", "application/json")
	body, _, err := doJSON(req)
	if err != nil {
		return nil, err
	}
	var resp b2ListBucketsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode list_buckets: %w (body=%s)", err, string(body))
	}
	for i := range resp.Buckets {
		if resp.Buckets[i].BucketName == name {
			return &resp.Buckets[i], nil
		}
	}
	return nil, fmt.Errorf("bucket %q not found (have %d)", name, len(resp.Buckets))
}

func updateBucketCors(auth *b2Authorize, bucketID string, rules []b2CorsRule) ([]b2CorsRule, error) {
	payload, _ := json.Marshal(map[string]interface{}{
		"accountId": auth.AccountID,
		"bucketId":  bucketID,
		"corsRules": rules,
	})
	req, err := http.NewRequest("POST", auth.APIURL+"/b2api/v2/b2_update_bucket", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth.AuthorizationToken)
	req.Header.Set("Content-Type", "application/json")
	body, status, err := doJSON(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w body=%s status=%d", err, string(body), status)
	}
	var bucket b2Bucket
	if err := json.Unmarshal(body, &bucket); err != nil {
		return nil, fmt.Errorf("decode update_bucket: %w (body=%s)", err, string(body))
	}
	return bucket.CorsRules, nil
}

func doJSON(req *http.Request) ([]byte, int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, resp.StatusCode, fmt.Errorf("B2 API status=%d body=%s", resp.StatusCode, string(body))
	}
	return body, resp.StatusCode, nil
}

func printCurrentRules(rules []b2CorsRule) {
	if len(rules) == 0 {
		log.Printf("current CORS rules: (none)")
		return
	}
	log.Printf("current CORS rules (%d):", len(rules))
	out, _ := json.MarshalIndent(rules, "", "  ")
	fmt.Fprintln(os.Stdout, string(out))
}

// Command b2-cors inspects and (optionally) applies the Backblaze B2 bucket
// CORS rules that are required for browser-to-B2 direct uploads.
//
// Why this exists:
//   B2 bucket CORS is configured on B2's side via the b2_update_bucket API —
//   not in this repo's code. The symptom of missing/incorrect CORS rules is
//   that browsers get a 401 on the preflight OPTIONS request and the upload
//   never starts. This tool fixes that from the developer machine.
//
// Usage:
//   cd backend
//   go run ./cmd/b2-cors --apply                            # apply defaults
//   go run ./cmd/b2-cors                                    # dry-run / inspect
//   go run ./cmd/b2-cors --apply --origin https://a.tld,http://localhost:3000
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
	originsFlag := flag.String("origin", "", "comma-separated list of allowed origins (default: BASE_SITE_URL + http://localhost:3000)")
	ruleName := flag.String("name", "filmorauzDirectUpload", "CORS rule name")
	maxAge := flag.Int("max-age", 3600, "preflight cache max-age in seconds")
	flag.Parse()

	cfg := config.Load()
	if cfg.B2KeyID == "" || cfg.B2AppKey == "" || cfg.B2Bucket == "" {
		log.Fatalf("B2_KEY_ID / B2_APP_KEY / B2_BUCKET must be set in backend/.env")
	}

	origins := buildOrigins(*originsFlag, cfg.BaseSiteURL)
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

	desired := []b2CorsRule{{
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
	}}

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
		return dedup([]string{site, "http://localhost:3000"})
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

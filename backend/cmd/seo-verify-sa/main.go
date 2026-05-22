// seo-verify-sa registers the FilmoraUz service account as a verified owner
// of the site via the Site Verification API, sidestepping the Search Console
// "Add user" dialog (which rejects service-account emails for many properties).
//
// Two phases — run the tool twice:
//
//   ./seo-verify-sa token --creds backend/secrets/google-indexing.json \
//                         --site https://filmorauz.net/
//     → prints a meta tag like <meta name="google-site-verification" content="...">
//       which must be added to the site's <head> and deployed.
//
//   ./seo-verify-sa verify --creds backend/secrets/google-indexing.json \
//                          --site https://filmorauz.net/
//     → tells Google to check the meta tag and, on success, registers the
//       service-account email as a verified owner. Indexing API + Search
//       Console API then accept the SA without any UI change.
//
// Once verify succeeds, the site URL passed in GOOGLE_SEARCH_CONSOLE_SITE_URL
// is already set to https://filmorauz.net/ on prod, so the backend can resume
// sitemap submissions and Indexing API pings immediately.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/filmorauz/backend/services/seo"
)

const (
	scope        = "https://www.googleapis.com/auth/siteverification"
	apiBase      = "https://www.googleapis.com/siteVerification/v1"
	methodMeta   = "META"
	siteTypeSite = "SITE"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: seo-verify-sa <token|verify> --creds=PATH --site=URL")
		os.Exit(2)
	}
	cmd := os.Args[1]

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	creds := fs.String("creds", "", "path to service-account JSON")
	site := fs.String("site", "", "site identifier, e.g. https://filmorauz.net/")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}
	if *creds == "" || *site == "" {
		fmt.Fprintln(os.Stderr, "both --creds and --site are required")
		os.Exit(2)
	}

	ts, err := seo.NewGoogleTokenSourceForTool(*creds, scope)
	if err != nil {
		fail("auth setup: %v", err)
	}
	accessToken, err := ts.Token()
	if err != nil {
		fail("auth token: %v", err)
	}

	switch cmd {
	case "token":
		runToken(accessToken, *site)
	case "verify":
		runVerify(accessToken, *site, ts.Email())
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", cmd)
		os.Exit(2)
	}
}

func runToken(accessToken, site string) {
	payload := map[string]any{
		"site":               map[string]string{"type": siteTypeSite, "identifier": site},
		"verificationMethod": methodMeta,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, apiBase+"/token", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		fail("token request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fail("token endpoint status=%d body=%s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Token  string `json:"token"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		fail("token parse: %v body=%s", err, string(raw))
	}

	// The Site Verification API returns the entire <meta ...> tag in `token`
	// already, but be defensive in case Google ever switches to returning just
	// the content value.
	tag := parsed.Token
	if !strings.Contains(tag, "google-site-verification") {
		tag = fmt.Sprintf(`<meta name="google-site-verification" content="%s">`, parsed.Token)
	}

	fmt.Println("=== ADD THIS META TAG TO frontend/src/app/layout.tsx <head> ===")
	fmt.Println(tag)
	fmt.Println()
	fmt.Println("After deploying, run `seo-verify-sa verify` with the same flags.")
}

func runVerify(accessToken, site, email string) {
	payload := map[string]any{
		"site": map[string]string{"type": siteTypeSite, "identifier": site},
	}
	body, _ := json.Marshal(payload)

	url := apiBase + "/webResource?verificationMethod=" + methodMeta
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		fail("verify request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fail("verify endpoint status=%d body=%s\nHint: confirm the meta tag is live on %s and the deploy reached prod.",
			resp.StatusCode, string(raw), site)
	}
	var parsed struct {
		ID     string   `json:"id"`
		Owners []string `json:"owners"`
		Site   struct {
			Identifier string `json:"identifier"`
		} `json:"site"`
	}
	_ = json.Unmarshal(raw, &parsed)

	fmt.Printf("✓ verified ownership of %s\n", parsed.Site.Identifier)
	fmt.Printf("  service account: %s\n", email)
	if len(parsed.Owners) > 0 {
		fmt.Printf("  registered owners: %s\n", strings.Join(parsed.Owners, ", "))
	}
	fmt.Println("\nIndexing API + Search Console sitemap submission should now succeed.")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

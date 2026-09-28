package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func robotsGroups(body string) map[string][]string {
	groups := map[string][]string{}
	var agents []string
	inRules := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, _ := strings.Cut(line, ":")
		v = strings.TrimSpace(v)
		switch strings.ToLower(k) {
		case "user-agent":
			if inRules {
				agents, inRules = nil, false
			}
			agents = append(agents, v)
		case "allow", "disallow":
			inRules = true
			for _, a := range agents {
				groups[a] = append(groups[a], strings.ToLower(k)+" "+v)
			}
		}
	}
	return groups
}

func TestRobotsAllowsAIBotsAndKeepsPrivatePathsForEveryGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SitemapHandler{baseSiteURL: "https://filmorauz.net"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	h.GetRobotsTxt(c)
	body := w.Body.String()

	groups := robotsGroups(body)
	for _, agent := range []string{"*", "Googlebot", "Yandex", "Bingbot", "GPTBot", "ClaudeBot", "PerplexityBot", "Google-Extended"} {
		rules := strings.Join(groups[agent], "|")
		if !strings.Contains(rules, "allow /") {
			t.Errorf("%s: missing Allow: /", agent)
		}
		// A bot follows only its own group, so each must keep /admin closed.
		if !strings.Contains(rules, "disallow /admin") || !strings.Contains(rules, "disallow /api") {
			t.Errorf("%s: private paths not disallowed: %v", agent, groups[agent])
		}
	}
	if !strings.Contains(body, "Sitemap: https://filmorauz.net/sitemap.xml") || !strings.Contains(body, "https://filmorauz.net/llms.txt") {
		t.Fatalf("sitemap/llms references missing:\n%s", body)
	}
}

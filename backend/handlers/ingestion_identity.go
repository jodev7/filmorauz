package handlers

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type importIdentitySnapshot struct {
	Source    string `json:"source"`
	SourceID  string `json:"source_id"`
	DetailURL string `json:"detail_url"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	Poster    string `json:"poster"`
}

var identityNoiseRe = regexp.MustCompile(`[^a-z0-9\s]+`)

func normalizeIdentityTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "o'zbek tilida", " ")
	s = strings.ReplaceAll(s, "ozbek tilida", " ")
	s = strings.ReplaceAll(s, "uzbekcha", " ")
	s = strings.ReplaceAll(s, "tarjima", " ")
	s = strings.ReplaceAll(s, "premyera", " ")
	s = identityNoiseRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func titleSimilarity(a, b string) float64 {
	a = normalizeIdentityTitle(a)
	b = normalizeIdentityTitle(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	aTokens := strings.Fields(a)
	bTokens := strings.Fields(b)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0
	}
	tokenSet := map[string]struct{}{}
	for _, token := range aTokens {
		tokenSet[token] = struct{}{}
	}
	intersect := 0
	union := len(tokenSet)
	seenB := map[string]struct{}{}
	for _, token := range bTokens {
		if _, ok := seenB[token]; ok {
			continue
		}
		seenB[token] = struct{}{}
		if _, ok := tokenSet[token]; ok {
			intersect++
		} else {
			union++
		}
	}
	jaccard := float64(intersect) / float64(maxInt(union, 1))
	if strings.Contains(a, b) || strings.Contains(b, a) {
		jaccard = maxFloat(jaccard, 0.92)
	}
	return jaccard
}

func normalizeIdentityURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	path := strings.TrimSuffix(parsed.EscapedPath(), "/")
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host + path)
}

func normalizeIdentityType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "series" {
		return "serial"
	}
	return value
}

func identityConfidence(selected, fetched importIdentitySnapshot) float64 {
	type weighted struct {
		score  float64
		weight float64
	}
	parts := []weighted{}

	if strings.TrimSpace(selected.Source) != "" {
		score := 0.0
		if strings.EqualFold(selected.Source, fetched.Source) {
			score = 1.0
		}
		parts = append(parts, weighted{score: score, weight: 0.10})
	}
	if strings.TrimSpace(selected.SourceID) != "" {
		score := 0.0
		if strings.TrimSpace(selected.SourceID) == strings.TrimSpace(fetched.SourceID) {
			score = 1.0
		}
		parts = append(parts, weighted{score: score, weight: 0.25})
	}
	if strings.TrimSpace(selected.DetailURL) != "" {
		score := 0.0
		if normalizeIdentityURL(selected.DetailURL) == normalizeIdentityURL(fetched.DetailURL) {
			score = 1.0
		}
		parts = append(parts, weighted{score: score, weight: 0.20})
	}
	if strings.TrimSpace(selected.Title) != "" {
		parts = append(parts, weighted{score: titleSimilarity(selected.Title, fetched.Title), weight: 0.20})
	}
	if selected.Year > 0 {
		score := 0.0
		if selected.Year == fetched.Year {
			score = 1.0
		}
		parts = append(parts, weighted{score: score, weight: 0.10})
	}
	if normalizeIdentityType(selected.Type) != "" {
		score := 0.0
		if normalizeIdentityType(selected.Type) == normalizeIdentityType(fetched.Type) {
			score = 1.0
		}
		parts = append(parts, weighted{score: score, weight: 0.10})
	}
	if strings.TrimSpace(selected.Poster) != "" {
		score := 0.0
		if normalizeIdentityURL(selected.Poster) == normalizeIdentityURL(fetched.Poster) {
			score = 1.0
		}
		parts = append(parts, weighted{score: score, weight: 0.05})
	}

	totalWeight := 0.0
	totalScore := 0.0
	for _, part := range parts {
		totalWeight += part.weight
		totalScore += part.score * part.weight
	}
	if totalWeight == 0 {
		return 0
	}
	return totalScore / totalWeight
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func identityLogString(snapshot importIdentitySnapshot) string {
	return fmt.Sprintf("source=%s source_id=%s detail_url=%s title=%q year=%d type=%s poster=%s",
		snapshot.Source, snapshot.SourceID, snapshot.DetailURL, snapshot.Title, snapshot.Year, snapshot.Type, snapshot.Poster)
}

package handlers

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// proxyImageAllowedHosts gates which upstream hosts the proxy will fetch from.
// Required because the handler skips TLS verification — without an allowlist
// it would be a public open proxy for any URL the caller supplies.
var proxyImageAllowedHosts = map[string]bool{
	"uzmedia.tv":         true,
	"www.uzmedia.tv":     true,
	"uzmovi.net":         true,
	"uzmovi.com":         true,
	"uzmovi.me":          true,
	"images.uzmovi.net":  true,
	"images.uzmovi.com":  true,
}

var proxyImageClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		// uzmedia.tv ships a self-signed cert (CN=nohttps); skip verification
		// for whitelisted hosts only.
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// GET /api/proxy-image?url=<encoded>
// Streams an image from a whitelisted external host through the backend so
// the browser sees a same-origin HTTPS response, side-stepping mixed-content
// blocks and self-signed-cert handshake failures on the upstream.
func ProxyImage(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("url"))
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing url"})
		return
	}

	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url"})
		return
	}

	host := strings.ToLower(parsed.Host)
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if !proxyImageAllowedHosts[host] {
		c.JSON(http.StatusForbidden, gin.H{"error": "host not allowed"})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", parsed.String(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "build request failed"})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FilmoraUz/1.0)")
	req.Header.Set("Accept", "image/avif,image/webp,image/*,*/*;q=0.8")
	req.Header.Set("Referer", parsed.Scheme+"://"+parsed.Host+"/")

	resp, err := proxyImageClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream fetch failed", "details": err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream status", "status": resp.StatusCode})
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=86400")
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		c.Header("Content-Length", cl)
	}
	c.Status(http.StatusOK)
	io.Copy(c.Writer, resp.Body)
}

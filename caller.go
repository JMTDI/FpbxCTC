package main

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var nonDigitRE = regexp.MustCompile(`[^\d]`)

// sanitizeNumber strips the tel: prefix and removes every non-digit character
// (spaces, hyphens, dots, parentheses, plus signs, etc.) leaving digits only.
func sanitizeNumber(raw string) string {
	s := raw
	if idx := strings.Index(strings.ToLower(s), "tel:"); idx != -1 {
		s = s[idx+4:]
	}
	return nonDigitRE.ReplaceAllString(s, "")
}

// MakeCall builds the click-to-call URL and fires the GET request.
func MakeCall(cfg *Config, rawDest string) error {
	dest := sanitizeNumber(rawDest)
	if dest == "" {
		return fmt.Errorf("destination number is empty after sanitization")
	}

	// Normalise the domain: strip any accidental scheme or trailing slash.
	domain := strings.TrimSpace(cfg.Domain)
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimRight(domain, "/")

	base := fmt.Sprintf("https://%s/ctc.php", domain)

	params := url.Values{}
	params.Set("api", "1")
	params.Set("key", cfg.APIKey)
	params.Set("agent", cfg.AgentNumber)
	params.Set("dest", dest)

	fullURL := base + "?" + params.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fullURL)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return nil
}

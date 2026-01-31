// Package links detects meeting URLs in calendar events.
package links

import (
	"os/exec"
	"regexp"
)

// Meeting link patterns for various services
var patterns = []*regexp.Regexp{
	// Zoom
	regexp.MustCompile(`https?://[\w.-]*zoom\.us/j/[\w?=&-]+`),

	// Microsoft Teams
	regexp.MustCompile(`https?://teams\.microsoft\.com/l/meetup-join/[\w%/-]+`),

	// Google Meet
	regexp.MustCompile(`https?://meet\.google\.com/[\w-]+`),

	// Webex
	regexp.MustCompile(`https?://[\w.-]*\.webex\.com/[\w./-]+`),

	// Generic URL in case nothing else matches (fallback)
	// This is intentionally broad - use last
	regexp.MustCompile(`https?://[^\s<>"]+`),
}

// Detect finds the first meeting link in the given text fields.
// It checks location first, then description, prioritizing known
// meeting services over generic URLs.
func Detect(location, description string) string {
	// Check location first (more likely to have the meeting link)
	if link := detectInText(location); link != "" {
		return link
	}

	// Then check description
	return detectInText(description)
}

// detectInText finds the first meeting link in text.
// It prioritizes known meeting services (Zoom, Teams, etc.) over generic URLs.
func detectInText(text string) string {
	if text == "" {
		return ""
	}

	// Try each pattern in order (specific services first, generic last)
	// We check all specific patterns first before falling back to generic
	for i, pattern := range patterns {
		// Skip the generic URL pattern on first pass
		if i == len(patterns)-1 {
			continue
		}
		if match := pattern.FindString(text); match != "" {
			return match
		}
	}

	// Fall back to generic URL pattern
	if match := patterns[len(patterns)-1].FindString(text); match != "" {
		return match
	}

	return ""
}

// DetectFromEvent is a convenience function that extracts meeting link from event fields.
func DetectFromEvent(location, description, url string) string {
	// Check explicit URL field first
	if url != "" {
		// Verify it looks like a meeting link (not just any URL)
		for i, pattern := range patterns {
			if i == len(patterns)-1 {
				continue // Skip generic pattern
			}
			if pattern.MatchString(url) {
				return url
			}
		}
	}

	// Then try location and description
	return Detect(location, description)
}

// Open opens a URL in the default browser using xdg-open.
func Open(url string) error {
	return exec.Command("xdg-open", url).Start()
}

// Service returns the name of the meeting service for a URL.
func Service(url string) string {
	switch {
	case patterns[0].MatchString(url):
		return "Zoom"
	case patterns[1].MatchString(url):
		return "Teams"
	case patterns[2].MatchString(url):
		return "Meet"
	case patterns[3].MatchString(url):
		return "Webex"
	default:
		return "Meeting"
	}
}

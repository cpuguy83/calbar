package calendar

import (
	"regexp"
	"strings"
)

var (
	ms365TeamsURLRe           = regexp.MustCompile(`(?i)https?://teams\.microsoft\.com/[^\s<>"']+`)
	ms365MeetingIDRe          = regexp.MustCompile(`(?i)\bMeeting ID:\s*([0-9][0-9 ]*[0-9])`)
	ms365PasscodeRe           = regexp.MustCompile(`(?i)\bPasscode:\s*([^\s]+)`)
	ms365DialInRe             = regexp.MustCompile(`(?is)\bDial[- ]in by phone\s*:??\s*(.+?)(?:\s+Find a local number|\s+Phone conference ID:|\s+For organizers:|\s+_{20,}|$)`)
	ms365PhoneConferenceIDRe  = regexp.MustCompile(`(?i)\bPhone conference ID:\s*([0-9][0-9 ]*#?)`)
	ms365SeparatorRe          = regexp.MustCompile(`_{20,}`)
	ms365RepeatedWhitespaceRe = regexp.MustCompile(`[ \t]+`)
	ms365RepeatedBlankLinesRe = regexp.MustCompile(`\n{3,}`)
	ms365TeamsJoinLineURLRe   = regexp.MustCompile(`(?i)\bJoin:\s*(https?://teams\.microsoft\.com/[^\s<>"']+)`)
	ms365TeamsMeetingMarkerRe = regexp.MustCompile(`(?i)microsoft teams(?:\s+meeting|\s+need help\?)?`)
	ms365TeamsMeetingService  = "Microsoft Teams Meeting"
)

func parseMS365MeetingDetails(raw string) (MeetingDetails, string) {
	text := normalizeLineEndings(raw)
	details := extractMS365MeetingDetails(text)

	if start := findMS365TeamsFooterStart(text); start >= 0 {
		text = text[:start]
	}

	return details, normalizeMS365Description(text)
}

func extractMS365MeetingDetails(text string) MeetingDetails {
	var details MeetingDetails
	if text == "" {
		return details
	}

	if ms365TeamsMeetingMarkerRe.MatchString(text) || ms365TeamsURLRe.MatchString(text) {
		details.Service = ms365TeamsMeetingService
	}
	if match := ms365TeamsJoinLineURLRe.FindStringSubmatch(text); len(match) == 2 {
		details.URL = trimDetectedURL(match[1])
	} else if match := ms365TeamsURLRe.FindString(text); match != "" {
		details.URL = trimDetectedURL(match)
	}
	if match := ms365MeetingIDRe.FindStringSubmatch(text); len(match) == 2 {
		details.ID = strings.Join(strings.Fields(match[1]), " ")
	}
	if match := ms365PasscodeRe.FindStringSubmatch(text); len(match) == 2 {
		details.Passcode = strings.TrimSpace(trimDetectedURL(match[1]))
	}
	if match := ms365DialInRe.FindStringSubmatch(text); len(match) == 2 {
		details.DialIn = normalizeMS365InlineText(match[1])
	}
	if match := ms365PhoneConferenceIDRe.FindStringSubmatch(text); len(match) == 2 {
		details.PhoneConferenceID = strings.Join(strings.Fields(match[1]), " ")
	}

	return details
}

func findMS365TeamsFooterStart(text string) int {
	for _, loc := range ms365SeparatorRe.FindAllStringIndex(text, -1) {
		tail := text[loc[0]:min(len(text), loc[0]+1500)]
		if looksLikeMS365TeamsFooter(tail) {
			return loc[0]
		}
	}

	lower := strings.ToLower(text)
	marker := "microsoft teams"
	for offset := 0; ; {
		idx := strings.Index(lower[offset:], marker)
		if idx == -1 {
			return -1
		}
		idx += offset
		tail := text[idx:min(len(text), idx+1500)]
		if looksLikeMS365TeamsFooter(tail) {
			return idx
		}
		offset = idx + len(marker)
	}
}

func looksLikeMS365TeamsFooter(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "microsoft teams") &&
		(strings.Contains(lower, "teams.microsoft.com") ||
			strings.Contains(lower, "join the meeting now") ||
			strings.Contains(lower, "meeting id:") ||
			strings.Contains(lower, "passcode:") ||
			strings.Contains(lower, "dial in by phone") ||
			strings.Contains(lower, "dial-in by phone"))
}

func normalizeMS365Description(text string) string {
	text = normalizeLineEndings(text)
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	lastBlank := false

	for _, line := range lines {
		line = normalizeMS365InlineText(line)
		if line == "" {
			if len(out) > 0 && !lastBlank {
				out = append(out, "")
				lastBlank = true
			}
			continue
		}
		out = append(out, line)
		lastBlank = false
	}

	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}

	return strings.TrimSpace(ms365RepeatedBlankLinesRe.ReplaceAllString(strings.Join(out, "\n"), "\n\n"))
}

func normalizeMS365InlineText(text string) string {
	text = strings.TrimSpace(text)
	text = ms365RepeatedWhitespaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func normalizeLineEndings(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func trimDetectedURL(u string) string {
	return strings.TrimRight(u, `.,;:)]}>"'`)
}

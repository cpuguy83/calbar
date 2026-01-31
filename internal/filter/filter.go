// Package filter provides include/exclude filtering for calendar events.
package filter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/config"
)

// MatchType specifies how a filter rule matches.
type MatchType int

const (
	MatchContains MatchType = iota // Substring match (default)
	MatchExact                     // Exact string match
	MatchPrefix                    // Starts with
	MatchSuffix                    // Ends with
	MatchRegex                     // Regular expression
)

// Filter applies include/exclude rules to events.
type Filter struct {
	mode         string // "or" or "and"
	includeRules []rule // Rules that include events (include)
	excludeRules []rule // Rules that exclude events (exclude)
}

type rule struct {
	field           string
	matchType       MatchType
	pattern         string         // For non-regex matches
	regex           *regexp.Regexp // For regex matches
	caseInsensitive bool
}

// New creates a new filter from configuration.
func New(cfg config.FilterConfig) (*Filter, error) {
	f := &Filter{
		mode: cfg.Mode,
	}

	if f.mode == "" {
		f.mode = "or"
	}

	for i, r := range cfg.Rules {
		compiled, err := compileRule(r)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		if r.Exclude {
			f.excludeRules = append(f.excludeRules, compiled)
		} else {
			f.includeRules = append(f.includeRules, compiled)
		}
	}

	return f, nil
}

// compileRule converts a config FilterRule to an internal rule.
func compileRule(r config.FilterRule) (rule, error) {
	compiled := rule{
		field:           r.Field,
		caseInsensitive: r.CaseInsensitive,
	}

	// Determine match type and pattern from the new typed fields
	switch {
	case r.Regex != "":
		compiled.matchType = MatchRegex
		pattern := r.Regex
		if r.CaseInsensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return compiled, fmt.Errorf("invalid regex %q: %w", r.Regex, err)
		}
		compiled.regex = re

	case r.Exact != "":
		compiled.matchType = MatchExact
		compiled.pattern = r.Exact
		if r.CaseInsensitive {
			compiled.pattern = strings.ToLower(compiled.pattern)
		}

	case r.Prefix != "":
		compiled.matchType = MatchPrefix
		compiled.pattern = r.Prefix
		if r.CaseInsensitive {
			compiled.pattern = strings.ToLower(compiled.pattern)
		}

	case r.Suffix != "":
		compiled.matchType = MatchSuffix
		compiled.pattern = r.Suffix
		if r.CaseInsensitive {
			compiled.pattern = strings.ToLower(compiled.pattern)
		}

	case r.Contains != "":
		compiled.matchType = MatchContains
		compiled.pattern = r.Contains
		if r.CaseInsensitive {
			compiled.pattern = strings.ToLower(compiled.pattern)
		}

	case r.Match != "":
		// Backward compatibility: handle legacy "match" field
		if strings.HasPrefix(r.Match, "regex:") {
			compiled.matchType = MatchRegex
			pattern := strings.TrimPrefix(r.Match, "regex:")
			if r.CaseInsensitive {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return compiled, fmt.Errorf("invalid regex in match %q: %w", r.Match, err)
			}
			compiled.regex = re
		} else {
			compiled.matchType = MatchContains
			compiled.pattern = r.Match
			if r.CaseInsensitive {
				compiled.pattern = strings.ToLower(compiled.pattern)
			}
		}

	default:
		return compiled, fmt.Errorf("no match pattern specified (use contains, exact, prefix, suffix, or regex)")
	}

	return compiled, nil
}

// Apply filters events, returning only those that pass the filter rules.
// - Exclude rules are applied first: any event matching an exclude rule is removed
// - Include rules are applied second: if any include rules exist, only matching events are kept
// - If no rules are defined, all events are returned.
func (f *Filter) Apply(events []calendar.Event) []calendar.Event {
	// No rules = pass everything through
	if len(f.includeRules) == 0 && len(f.excludeRules) == 0 {
		return events
	}

	var filtered []calendar.Event
	for _, event := range events {
		// Check exclude rules first - if any match, skip this event
		if f.matchesExclude(event) {
			continue
		}

		// If no include rules, keep the event (only excludes matter)
		// If include rules exist, event must match them
		if len(f.includeRules) == 0 || f.matchesInclude(event) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// matchesExclude checks if an event matches any exclude rule.
func (f *Filter) matchesExclude(event calendar.Event) bool {
	// Any exclude rule matching means exclude
	for _, r := range f.excludeRules {
		if r.matches(event) {
			return true
		}
	}
	return false
}

// matchesInclude checks if an event matches the include filter rules.
func (f *Filter) matchesInclude(event calendar.Event) bool {
	if f.mode == "and" {
		// All rules must match
		for _, r := range f.includeRules {
			if !r.matches(event) {
				return false
			}
		}
		return true
	}

	// OR mode: any rule must match
	for _, r := range f.includeRules {
		if r.matches(event) {
			return true
		}
	}
	return false
}

// matches checks if an event matches a single rule.
func (r *rule) matches(event calendar.Event) bool {
	value := r.getFieldValue(event)

	// Apply case insensitivity for non-regex matches
	if r.caseInsensitive && r.matchType != MatchRegex {
		value = strings.ToLower(value)
	}

	switch r.matchType {
	case MatchRegex:
		return r.regex.MatchString(value)
	case MatchExact:
		return value == r.pattern
	case MatchPrefix:
		return strings.HasPrefix(value, r.pattern)
	case MatchSuffix:
		return strings.HasSuffix(value, r.pattern)
	case MatchContains:
		fallthrough
	default:
		return strings.Contains(value, r.pattern)
	}
}

// getFieldValue extracts the field value from an event.
func (r *rule) getFieldValue(event calendar.Event) string {
	switch r.field {
	case "title", "summary":
		return event.Summary
	case "organizer":
		return event.Organizer
	case "source", "calendar":
		return event.Source
	case "description":
		return event.Description
	case "location":
		return event.Location
	default:
		return ""
	}
}

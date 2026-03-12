package calendar

import (
	"strings"
	"time"

	ics "github.com/emersion/go-ical"
)

// windowsTZIDToIANA maps common Microsoft timezone IDs used in ICS feeds to IANA
// names that Go can resolve via time.LoadLocation.
var windowsTZIDToIANA = map[string]string{
	"AUS Eastern Standard Time":       "Australia/Sydney",
	"Cen. Australia Standard Time":    "Australia/Adelaide",
	"Central Europe Standard Time":    "Europe/Budapest",
	"Central European Standard Time":  "Europe/Warsaw",
	"Central Standard Time":           "America/Chicago",
	"E. Africa Standard Time":         "Africa/Nairobi",
	"E. Australia Standard Time":      "Australia/Brisbane",
	"E. Europe Standard Time":         "Europe/Chisinau",
	"Eastern Standard Time":           "America/New_York",
	"FLE Standard Time":               "Europe/Kiev",
	"GTB Standard Time":               "Europe/Bucharest",
	"GMT Standard Time":               "Europe/London",
	"Greenwich Standard Time":         "Atlantic/Reykjavik",
	"Mountain Standard Time":          "America/Denver",
	"Pacific Standard Time":           "America/Los_Angeles",
	"Romance Standard Time":           "Europe/Paris",
	"SA Pacific Standard Time":        "America/Bogota",
	"W. Europe Standard Time":         "Europe/Berlin",
	"W. Central Africa Standard Time": "Africa/Lagos",
	"UTC":                             "UTC",
}

// normalizeComponentTimezones converts the ICS component's timezone to IANA
// timezones.
func normalizeComponentTimezones(comp *ics.Component) {
	for name, props := range comp.Props {
		changed := false

		for i := range props {
			tzid := props[i].Params.Get(ics.PropTimezoneID)

			normalized := normalizeTZID(tzid)
			if normalized == "" || normalized == tzid {
				continue
			}

			props[i].Params.Set(ics.PropTimezoneID, normalized)
			changed = true
		}

		if changed {
			comp.Props[name] = props
		}
	}

	for _, child := range comp.Children {
		normalizeComponentTimezones(child)
	}
}

// normalizeTZID converts the given timezone string from Windows timezone ID to
// an IANA timezone.
func normalizeTZID(tzid string) string {
	tzid = strings.Trim(strings.TrimSpace(tzid), "\"")
	if tzid == "" {
		return ""
	}

	if _, err := time.LoadLocation(tzid); err == nil {
		return tzid
	}
	if mapped, ok := windowsTZIDToIANA[tzid]; ok {
		if _, err := time.LoadLocation(mapped); err == nil {
			return mapped
		}
	}

	return tzid
}

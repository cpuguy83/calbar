package calendar

import (
	"strings"
	"time"

	ics "github.com/emersion/go-ical"
	"github.com/thommeo/winianatz"
)


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

	entry, err := winianatz.FromMicrosoftAlias(tzid)
	if err == nil {
		if _, err := time.LoadLocation(entry.IANA); err == nil {
			return entry.IANA
		}
	}

	return tzid
}

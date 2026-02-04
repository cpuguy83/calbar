package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/cpuguy83/calbar/internal/auth"
)

const (
	// MS Graph API endpoint for calendar events
	graphCalendarEndpoint = "https://graph.microsoft.com/v1.0/me/calendarView"

	// Required scope for reading calendars
	calendarReadScope = "Calendars.Read"
)

// tokenProvider can acquire access tokens.
type tokenProvider interface {
	GetToken(ctx context.Context) (*auth.Token, error)
	Close() error
}

// MS365Source fetches events from Microsoft 365 calendar via Graph API.
type MS365Source struct {
	name     string
	auth     tokenProvider
	client   *http.Client
	initOnce sync.Once
	initErr  error
}

// NewMS365Source creates a new MS365 calendar source.
func NewMS365Source(name string) *MS365Source {
	return &MS365Source{
		name: name,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// initAuth initializes the authentication provider.
// Tries broker first, falls back to device code flow.
func (s *MS365Source) initAuth(ctx context.Context) error {
	s.initOnce.Do(func() {
		scopes := []string{calendarReadScope}

		// Try broker first
		broker := auth.NewBroker("", scopes)
		if broker.IsAvailable(ctx) {
			slog.Info("using Microsoft Identity Broker for authentication")
			s.auth = broker
			return
		}

		// Fall back to device code flow
		slog.Info("broker not available, using device code flow")
		deviceCode, err := auth.NewDeviceCodeAuth("", scopes)
		if err != nil {
			s.initErr = fmt.Errorf("initialize device code auth: %w", err)
			return
		}
		s.auth = deviceCode
	})
	return s.initErr
}

// Name returns the display name of this calendar source.
func (s *MS365Source) Name() string {
	return s.name
}

// Fetch retrieves events from Microsoft 365 calendar.
func (s *MS365Source) Fetch(ctx context.Context, end time.Time) ([]Event, error) {
	// Initialize auth on first fetch
	if err := s.initAuth(ctx); err != nil {
		return nil, err
	}

	// Get access token
	token, err := s.auth.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	// Fetch events from Graph API
	// Get events from now until the specified end time
	now := time.Now()

	events, err := s.fetchCalendarView(ctx, token.AccessToken, now, end)
	if err != nil {
		return nil, fmt.Errorf("fetch calendar: %w", err)
	}

	return events, nil
}

// Close cleans up resources.
func (s *MS365Source) Close() error {
	if s.auth != nil {
		return s.auth.Close()
	}
	return nil
}

// graphCalendarResponse is the MS Graph API response for calendar events.
type graphCalendarResponse struct {
	Value    []graphEvent `json:"value"`
	NextLink string       `json:"@odata.nextLink,omitempty"`
}

// graphEvent represents an event from MS Graph API.
type graphEvent struct {
	ID               string               `json:"id"`
	Subject          string               `json:"subject"`
	BodyPreview      string               `json:"bodyPreview"`
	Body             *graphBody           `json:"body,omitempty"`
	Start            graphDateTime        `json:"start"`
	End              graphDateTime        `json:"end"`
	Location         *graphLocation       `json:"location,omitempty"`
	IsAllDay         bool                 `json:"isAllDay"`
	IsCancelled      bool                 `json:"isCancelled"`
	Organizer        *graphOrganizer      `json:"organizer,omitempty"`
	WebLink          string               `json:"webLink"`
	OnlineMeetingURL string               `json:"onlineMeetingUrl,omitempty"`
	OnlineMeeting    *graphOnlineMeeting  `json:"onlineMeeting,omitempty"`
	Recurrence       *graphRecurrence     `json:"recurrence,omitempty"`
	SeriesMasterID   string               `json:"seriesMasterId,omitempty"`
	ShowAs           string               `json:"showAs"`
	ResponseStatus   *graphResponseStatus `json:"responseStatus,omitempty"`
}

type graphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type graphDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type graphLocation struct {
	DisplayName string `json:"displayName"`
}

type graphOrganizer struct {
	EmailAddress graphEmailAddress `json:"emailAddress"`
}

type graphEmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type graphOnlineMeeting struct {
	JoinURL string `json:"joinUrl"`
}

type graphRecurrence struct {
	Pattern graphRecurrencePattern `json:"pattern"`
	Range   graphRecurrenceRange   `json:"range"`
}

type graphRecurrencePattern struct {
	Type           string   `json:"type"`
	Interval       int      `json:"interval"`
	DaysOfWeek     []string `json:"daysOfWeek,omitempty"`
	DayOfMonth     int      `json:"dayOfMonth,omitempty"`
	FirstDayOfWeek string   `json:"firstDayOfWeek,omitempty"`
}

type graphRecurrenceRange struct {
	Type      string `json:"type"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate,omitempty"`
}

type graphResponseStatus struct {
	Response string `json:"response"`
	Time     string `json:"time"`
}

// fetchCalendarView fetches events using the calendarView endpoint (handles recurrence expansion).
func (s *MS365Source) fetchCalendarView(ctx context.Context, accessToken string, start, end time.Time) ([]Event, error) {
	// Build URL with time range
	params := url.Values{}
	params.Set("startDateTime", start.UTC().Format(time.RFC3339))
	params.Set("endDateTime", end.UTC().Format(time.RFC3339))
	params.Set("$orderby", "start/dateTime")
	params.Set("$top", "500") // Fetch up to 500 events
	// Request specific fields to get full details including body
	params.Set("$select", "id,subject,bodyPreview,body,start,end,location,isAllDay,isCancelled,organizer,webLink,onlineMeetingUrl,onlineMeeting,showAs,responseStatus")

	reqURL := graphCalendarEndpoint + "?" + params.Encode()

	var allEvents []Event

	// Handle pagination
	for reqURL != "" {
		events, nextLink, err := s.fetchPage(ctx, accessToken, reqURL)
		if err != nil {
			return nil, err
		}
		allEvents = append(allEvents, events...)
		reqURL = nextLink
	}

	slog.Debug("fetched MS365 events", "count", len(allEvents))
	return allEvents, nil
}

// fetchPage fetches a single page of events.
func (s *MS365Source) fetchPage(ctx context.Context, accessToken, reqURL string) ([]Event, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	// Request times in UTC (always works) and body as plain text
	// We'll convert to local time when parsing
	req.Header.Set("Prefer", `outlook.timezone="UTC", outlook.body-content-type="text"`)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("graph API error: status %d: %s", resp.StatusCode, string(body))
	}

	var graphResp graphCalendarResponse
	if err := json.NewDecoder(resp.Body).Decode(&graphResp); err != nil {
		return nil, "", fmt.Errorf("decode response: %w", err)
	}

	events := make([]Event, 0, len(graphResp.Value))
	for _, ge := range graphResp.Value {
		// Skip cancelled events
		if ge.IsCancelled {
			continue
		}

		event, err := s.convertEvent(ge)
		if err != nil {
			slog.Warn("skip event conversion error", "id", ge.ID, "error", err)
			continue
		}
		events = append(events, event)
	}

	return events, graphResp.NextLink, nil
}

// convertEvent converts a Graph API event to our Event type.
func (s *MS365Source) convertEvent(ge graphEvent) (Event, error) {
	event := Event{
		UID:     ge.ID,
		Summary: ge.Subject,
		Source:  s.name,
		AllDay:  ge.IsAllDay,
		URL:     ge.WebLink,
	}

	// Parse start time
	start, err := parseGraphDateTime(ge.Start)
	if err != nil {
		return event, fmt.Errorf("parse start: %w", err)
	}
	event.Start = start

	// Parse end time
	end, err := parseGraphDateTime(ge.End)
	if err != nil {
		return event, fmt.Errorf("parse end: %w", err)
	}
	event.End = end

	// Location
	if ge.Location != nil && ge.Location.DisplayName != "" {
		event.Location = ge.Location.DisplayName
	}

	// Description - prefer body content over preview
	if ge.Body != nil && ge.Body.Content != "" {
		event.Description = ge.Body.Content
	} else {
		event.Description = ge.BodyPreview
	}

	// Organizer
	if ge.Organizer != nil {
		event.Organizer = ge.Organizer.EmailAddress.Address
	}

	// Online meeting URL (Teams, etc.)
	if ge.OnlineMeeting != nil && ge.OnlineMeeting.JoinURL != "" {
		// Store meeting URL in location if no location set
		if event.Location == "" {
			event.Location = ge.OnlineMeeting.JoinURL
		} else {
			// Append to description so it's detectable
			event.Description = ge.OnlineMeeting.JoinURL + "\n" + event.Description
		}
	} else if ge.OnlineMeetingURL != "" {
		if event.Location == "" {
			event.Location = ge.OnlineMeetingURL
		} else {
			event.Description = ge.OnlineMeetingURL + "\n" + event.Description
		}
	}

	return event, nil
}

// parseGraphDateTime parses a Graph API datetime value.
// Times are stored in UTC; conversion to local happens at display time.
func parseGraphDateTime(gdt graphDateTime) (time.Time, error) {
	// Graph API returns datetime in UTC (as we requested via Prefer header)
	// Format: "2024-01-15T09:00:00.0000000"

	formats := []string{
		"2006-01-02T15:04:05.0000000",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		t, err := time.ParseInLocation(format, gdt.DateTime, time.UTC)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("cannot parse datetime: %s", gdt.DateTime)
}

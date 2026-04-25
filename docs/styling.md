# Styling

CalBar uses built-in GTK/libadwaita styles for the popup by default. You can override them with your own GTK CSS.

## Loading Custom CSS

- Default override path: `~/.config/calbar/style.css`
- Optional config override: `ui.css_file`
- User CSS is loaded after the built-in popup CSS, so you only need to override the selectors you care about

## Example

```css
.popup-container {
    background: rgba(20, 20, 24, 0.72);
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 16px;
}

.status-bar {
    background: rgba(255, 255, 255, 0.03);
}

.sync-button {
    min-width: 30px;
    min-height: 30px;
}

.sync-indicator {
    background: @accent_color;
}

.day-separator,
.all-day-section {
    background: rgba(255, 255, 255, 0.025);
}
```

## Selector Reference

These are the main classes exposed by the popup UI:

- `.popup-container`: outer visible card
- `.popup-header`: top header row
- `.sync-button`, `.sync-indicator`: manual sync button and active-sync dot in the header
- `.event-list`: timed events list container
- `.event-card`: individual timed event row
- `.time-indicator`: left-side time block for an event
- `.event-title`, `.event-meta`, `.event-source`: event text styling
- `.join-btn`: join/open-link button in the list
- `.status-bar`: footer area with sync state and hidden event count
- `.hidden-count`: hidden events indicator in the footer
- `.day-separator`: day labels like "Today" and "Tomorrow"
- `.all-day-section`, `.all-day-row`, `.all-day-title`, `.all-day-meta`: all-day event area
- `.details-header`, `.details-content`, `.details-title`: event details view
- `.details-description`, `.details-section-label`: description block in details view
- `.details-join-btn`: join button in the details view
- `.hide-btn`, `.unhide-btn`, `.unhide-icon-btn`: hide/unhide controls
- `.hidden-events-list`, `.hidden-event-row`, `.hidden-event-title`, `.hidden-event-meta`: hidden events view
- `.empty-state`, `.loading-state`: empty/loading views

## State Classes

State classes are also applied in a few places:

- `.event-card.ongoing`: whole timed row for an event happening now
- `.event-card.imminent`: whole timed row for an event starting within 15 minutes
- `.event-title.ongoing`: active event title
- `.time-indicator.now`: current event time indicator
- `.time-indicator.imminent`: soon-starting event time indicator
- `.stale`: stale sync status styling

## Hyprland Blur

CalBar sets the layer-shell namespace to `calbar-popup`, so Hyprland blur rules should target that namespace:

```ini
layerrule = blur, calbar-popup
layerrule = ignorealpha 0.3, calbar-popup
```

## Built-in Styles

The full built-in selector list lives in `internal/ui/popup.go`.

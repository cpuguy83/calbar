# CalBar

A calendar tray app for Linux desktops, similar to [Dato](https://sindresorhus.com/dato) for macOS.

> **Note:** This is a work in progress, built for my own needs. It may not work for you, and the API/config format may change without notice.

![Status](https://img.shields.io/badge/status-alpha-orange)

## Features

- **Multiple calendar sources**: ICS feeds, CalDAV, Microsoft 365
- **Include/exclude filtering**: Only show events matching specific rules (great for filtering noisy work calendars)
- **System tray integration**: StatusNotifierItem (SNI) for Waybar and other modern tray implementations
- **Meeting link detection**: Automatically detects Zoom, Teams, Meet, and Webex links
- **Desktop notifications**: Configurable reminders before events with "Join" action buttons
- **Standard ICS output**: Synced calendar is a standard ICS file, usable by any calendar app

## Installation

### Build from source

```bash
# Clone the repository
git clone https://github.com/cpuguy83/calbar.git
cd calbar

# Build
make build

# Install to ~/.local/bin (recommended)
make install-user

# Or install system-wide
sudo make install
```

### Dependencies

- Go 1.25+
- D-Bus (for notifications and system tray)
- GTK4 with libadwaita (soon to be optional dmenu support instead of GTK4)

## Configuration

Create your config file at `~/.config/calbar/config.yaml`:

```yaml
# CalBar Configuration

# Sync daemon settings
sync:
  interval: 5m
  output: ~/.local/share/calbar/calendar.ics

# Calendar sources
sources:
  # ICS feed (most common)
  - name: "Personal"
    type: ics
    url: "https://calendar.google.com/calendar/ical/YOUR_CALENDAR_ID/basic.ics"

  # ICS feed with authentication
  - name: "Private Calendar"
    type: ics
    url: "https://calendar.example.com/private/feed.ics"
    username: "myuser"
    password_cmd: "pass show calendar/example"  # Use a password manager

  # CalDAV server
  - name: "CalDAV"
    type: caldav
    url: "https://caldav.example.com/calendars/user/default/"
    username: "myuser"
    password_cmd: "pass show caldav/password"

  # Microsoft 365 via Microsoft Identity Broker (Linux SSO)
  - name: "Work (MS365)"
    type: ms365

# include filters (only matching events are synced)
# Leave empty to sync all events
filters:
  mode: or  # "or" = match any rule, "and" = match all rules
  rules:
    # Only sync events with these titles
    - field: title
      match: "containerd contributors meeting"
      case_insensitive: true

    - field: title
      match: "Team Standup"
      case_insensitive: true

    # Match by organizer
    - field: organizer
      match: "important-person@company.com"

    # Regex matching (prefix with "regex:")
    - field: title
      match: "regex:^\\[Priority\\]"

# Notification settings
notifications:
  enabled: true
  before:
    - 15m
    - 5m

# UI settings
ui:
  time_range: 24h  # Show events for the next 24 hours
  theme: system    # system, light, or dark
```

## Usage

### Running manually

```bash
# Start CalBar
./calbar --config ~/.config/calbar/config.yaml

# With verbose logging
./calbar --config ~/.config/calbar/config.yaml -v
```

### Using systemd (recommended)

```bash
# Enable and start the service
systemctl --user enable --now calbar

# Check status
systemctl --user status calbar

# View logs
journalctl --user -u calbar -f
```

## Calendar Source Setup

### Microsoft 365

For ICS export:
1. Go to Outlook on the web → Calendar → Settings → Shared calendars
2. Publish your calendar and copy the ICS link
3. Add to config as an `ics` type source

For native MS365 integration (uses Linux SSO via Microsoft Identity Broker):
1. Add to config as an `ms365` type source
2. Requires Edge browser signed in to your Microsoft account

### Google Calendar

1. Go to Google Calendar → Settings → Settings for my calendars
2. Select your calendar → Integrate calendar
3. Copy the "Secret address in iCal format"
4. Add to config as an `ics` type source

### CalDAV

1. Get your CalDAV URL from your calendar provider
2. Add to config as a `caldav` type source

## Filtering

CalBar supports include/exclude filtering - you can specify which events to show or hide. This is useful when you have a noisy work calendar but only care about specific meetings.

### Filter fields

- `title` - Event summary/title
- `organizer` - Organizer email address
- `source` - Calendar source name
- `description` - Event description
- `location` - Event location

### Match types

Each rule uses exactly one match type:

| Type | Description | Example |
|------|-------------|---------|
| `contains` | Substring match | `"standup"` matches "Daily Standup" |
| `exact` | Exact string match | `"Team Meeting"` only matches "Team Meeting" |
| `prefix` | Starts with | `"[Priority]"` matches "[Priority] Bug fix" |
| `suffix` | Ends with | `"Review"` matches "Code Review" |
| `regex` | Regular expression | `"stand[- ]?up"` matches "standup", "stand-up" |

### Filter modes

- `or` (default) - Event matches if ANY rule matches
- `and` - Event matches only if ALL rules match

### Examples

```yaml
# Substring match (most common)
filters:
  rules:
    - field: title
      contains: "standup"
      case_insensitive: true

# Exact match
filters:
  rules:
    - field: title
      exact: "Weekly Team Standup"
      case_insensitive: true

# Prefix match (starts with)
filters:
  rules:
    - field: title
      prefix: "[Important]"

# Suffix match (ends with)  
filters:
  rules:
    - field: organizer
      suffix: "@company.com"

# Regex for complex patterns
filters:
  rules:
    - field: title
      regex: "^(Team|Project)\\s+Meeting"
      case_insensitive: true

# Multiple rules with OR logic (match any)
filters:
  mode: or
  rules:
    - field: title
      contains: "standup"
      case_insensitive: true
    - field: title
      contains: "1:1"
      case_insensitive: true
    - field: organizer
      exact: "boss@company.com"
```

## Meeting Link Detection

CalBar automatically detects meeting links in event location and description fields:

| Service | Detected Pattern |
|---------|-----------------|
| Zoom | `zoom.us/j/...` |
| Microsoft Teams | `teams.microsoft.com/l/meetup-join/...` |
| Google Meet | `meet.google.com/...` |
| Webex | `*.webex.com/...` |

When a meeting link is detected:
- The tray popup shows a "Join" button
- Notifications include a "Join Meeting" action
- Clicking opens the link in your default browser

## Troubleshooting

### Tray icon not appearing

Make sure your system tray supports StatusNotifierItem (SNI):
- **Waybar**: Should work out of the box
- **Polybar**: Use `tray-position = right` in your config
- **i3bar**: Doesn't support SNI, consider using Waybar

### Sync not working

```bash
# Run with verbose logging to see what's happening
./calbar --config ~/.config/calbar/config.yaml -v

# Check if the ICS URL is accessible
curl -I "YOUR_ICS_URL"
```

### Notifications not working

Make sure you have a notification daemon running (mako, dunst, etc.):

```bash
# Test notifications
notify-send "Test" "This is a test notification"
```

## License

MIT

## Contributing

Contributions welcome!

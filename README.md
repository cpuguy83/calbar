# CalBar

A Dato-like calendar app for Linux desktops. Syncs calendar feeds from multiple sources, applies include filters, and provides a system tray interface with upcoming events, meeting join buttons, and desktop notifications.

![Status](https://img.shields.io/badge/status-alpha-orange)

## Features

- **Multiple calendar sources**: ICS feeds, CalDAV (Nextcloud, Radicale), iCloud, Microsoft 365
- **include filtering**: Only show events matching specific rules (great for filtering noisy work calendars)
- **System tray integration**: StatusNotifierItem (SNI) for Waybar and other modern tray implementations
- **Meeting link detection**: Automatically detects Zoom, Teams, Meet, and Webex links
- **Desktop notifications**: Configurable reminders before events with "Join" action buttons
- **Standard ICS output**: Synced calendar is a standard ICS file, usable by any calendar app

## Architecture

CalBar consists of two binaries:

| Binary | Purpose |
|--------|---------|
| `calsync` | Daemon that fetches calendars, applies filters, outputs merged ICS file |
| `calbar` | System tray app that reads ICS file, shows popup, sends notifications |

The ICS file (`~/.local/share/calbar/calendar.ics`) acts as the interface between them.

```
External Sources (ICS, CalDAV, iCloud, MS365)
                    │
                    ▼
            ┌──────────────┐
            │   calsync    │  ← Fetch, filter, merge
            └──────┬───────┘
                   │
                   ▼
    ~/.local/share/calbar/calendar.ics
                   │
                   ▼
            ┌──────────────┐
            │   calbar     │  ← Tray, popup, notifications
            └──────────────┘
```

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

- Go 1.21+
- D-Bus (for notifications and system tray)
- GTK4 (optional, for popup window - coming soon)

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
  # Simple ICS feed (most common)
  - name: "Work Calendar"
    type: ics
    url: "https://outlook.office365.com/owa/calendar/YOUR_CALENDAR_ID/calendar.ics"

  # ICS feed with authentication
  - name: "Private Calendar"
    type: ics
    url: "https://calendar.example.com/private/feed.ics"
    username: "myuser"
    password_cmd: "pass show calendar/example"  # Use a password manager

  # CalDAV server (Nextcloud, Radicale, etc.)
  - name: "Nextcloud"
    type: caldav
    url: "https://cloud.example.com/remote.php/dav"
    username: "myuser"
    password_cmd: "pass show nextcloud/password"
    calendars:  # Optional: specific calendars only
      - "Personal"
      - "Work"

  # Apple iCloud Calendar
  - name: "iCloud"
    type: icloud
    username: "your@icloud.com"
    password_cmd: "pass show apple/app-specific-password"
    # Generate app-specific password at https://appleid.apple.com

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

### Quick test

```bash
# Test sync (one-shot mode)
./calsync --once -v

# Check the output
cat ~/.local/share/calbar/calendar.ics
```

### Running manually

```bash
# Start the sync daemon (runs every 5 minutes by default)
./calsync &

# Start the tray app
./calbar
```

### Using systemd (recommended)

```bash
# Enable and start both services
systemctl --user enable --now calsync calbar

# Check status
systemctl --user status calsync calbar

# View logs
journalctl --user -u calsync -f
journalctl --user -u calbar -f
```

## Calendar Source Setup

### Microsoft 365 / Outlook

1. Go to Outlook on the web → Calendar → Settings → Shared calendars
2. Publish your calendar and copy the ICS link
3. Add to config as an `ics` type source

### Google Calendar

1. Go to Google Calendar → Settings → Settings for my calendars
2. Select your calendar → Integrate calendar
3. Copy the "Secret address in iCal format"
4. Add to config as an `ics` type source

### Apple iCloud

1. Go to https://appleid.apple.com → Security → App-Specific Passwords
2. Generate a new app-specific password for CalBar
3. Add to config as an `icloud` type source with your Apple ID email

### Nextcloud / CalDAV

1. Get your CalDAV URL (usually `https://your-server/remote.php/dav`)
2. Add to config as a `caldav` type source

## Filtering

CalBar uses include filtering - only events matching your rules are synced. This is useful when you have a noisy work calendar but only care about specific meetings.

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
# Test with verbose logging
./calsync --once -v

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

Contributions welcome! Please read `AGENTS.md` for the project structure and how different components are organized.

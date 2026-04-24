# CalBar

A calendar tray app for Linux desktops, similar to [Dato](https://sindresorhus.com/dato) for macOS.

> **Note:** This is a work in progress, built for my own needs. It may not work for you, and the API/config format may change without notice.

![Status](https://img.shields.io/badge/status-alpha-orange)

## Features

- **Multiple calendar sources**: ICS feeds, CalDAV, iCloud, Microsoft 365
- **Include/exclude filtering**: Only show events matching specific rules (great for filtering noisy work calendars)
- **Hide events**: Temporarily hide individual events from view (great for dismissed meetings or noise)
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
- GTK4 with libadwaita (optional - use `nogtk` build tag for dmenu-style launcher support)

### NixOS / Home Manager

CalBar provides a Nix flake with multiple packages:

| Package | Description |
|---------|-------------|
| `calbar` | GTK4 UI, wrapped for NixOS (recommended) |
| `calbar-unwrapped` | GTK4 UI, no wrapper (for non-NixOS with GTK in standard paths) |
| `calbar-lite` | No GTK, dmenu-style launcher only |

```nix
# flake.nix
{
  inputs.calbar.url = "github:cpuguy83/calbar";
}
```

**Home Manager module:**

```nix
# In your home.nix
{
  imports = [ inputs.calbar.homeManagerModules.default ];

  services.calbar = {
    enable = true;
    # gtk.disable = true;  # Use dmenu-style launcher instead of GTK
    css = ''
      .popup-container {
        background: rgba(20, 20, 24, 0.72);
        border: 1px solid rgba(255, 255, 255, 0.08);
      }
    '';
    settings = {
      sync.interval = "5m";
      sources = [
        { name = "Work"; type = "ms365"; }
      ];
    };
  };
}
```

## Configuration

Create your config file at `~/.config/calbar/config.yaml`:

```yaml
# CalBar Configuration

# Sync settings
sync:
  interval: 5m         # How often to refresh calendar feeds
  time_range: 14d      # How far ahead to fetch events (supports d/w suffixes)

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
    calendars:                                   # Optional: sync only specific calendars
      - "Personal"
      - "Work"

  # iCloud (CalDAV with iCloud defaults)
  # - name: "iCloud"
  #   type: icloud
  #   username: "apple-id@example.com"
  #   password_cmd: "pass show icloud/app-password"

  # Microsoft 365 via Microsoft Identity Broker (Linux SSO)
  - name: "Work (MS365)"
    type: ms365

  # Source with connection details from an external command.
  # The command must output YAML/JSON with connection fields: type, url, username, password, calendars.
  # Useful when your config is public (e.g. NixOS) and secrets should stay out of it.
  # - name: "Secret Calendar"
  #   config_cmd: "op read op://Vault/Calendar/config"

# Filters (only matching events are shown)
# Leave empty to show all events
filters:
  mode: or  # "or" = match any rule, "and" = match all rules
  rules:
    # Substring match (most common)
    - field: title
      contains: "standup"
      case_insensitive: true

    - field: title
      contains: "Team Meeting"
      case_insensitive: true

    # Match by organizer
    - field: organizer
      exact: "important-person@company.com"

    # Regex matching
    # - field: title
    #   regex: "^\\[Priority\\]"

    # Exclude rules - hide matching events regardless of include rules
    # - field: title
    #   contains: "Canceled:"
    #   exclude: true

# Notification settings
notifications:
  enabled: true
  before:
    - 15m
    - 5m

# UI settings
ui:
  time_range: 24h               # How far ahead to show events in popup (default: 7d)
  theme: system                 # system, light, or dark
  backend: auto                 # auto, gtk, or menu (default: auto)
  max_events: 20                # Max events to show in popup
  event_end_grace: 5m           # Keep events visible after they end
  hover_dismiss_delay: 3s       # Delay before popup auto-dismisses on pointer-leave (0 = never)
  # css_file: ~/.config/calbar/style.css  # Optional GTK CSS override file
  # menu:                       # dmenu-style backend config (when backend is "menu" or GTK unavailable)
  #   program: rofi             # Auto-detected if empty (tries rofi, wofi, fuzzel, bemenu, dmenu)
  #   args: ["-theme", "custom"]
```

## GTK CSS Customization

The GTK popup uses built-in libadwaita-based styles by default, so your system GTK theme already controls most colors and typography. For popup-specific tweaks, calbar will load a user CSS file after its built-in styles.

- Default override path: `~/.config/calbar/style.css`
- Optional config override: `ui.css_file`
- User CSS is loaded after the built-in popup CSS, so you only need to override the selectors you care about

Example:

```css
.popup-container {
    background: rgba(20, 20, 24, 0.72);
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 16px;
}

.status-bar {
    background: rgba(255, 255, 255, 0.03);
}

.day-separator,
.all-day-section {
    background: rgba(255, 255, 255, 0.025);
}
```

### Hyprland Blur

Calbar sets the layer-shell namespace to `calbar-popup`, so Hyprland blur rules should target that namespace:

```ini
layerrule = blur, calbar-popup
layerrule = ignorealpha 0.3, calbar-popup
```

### Useful CSS Classes

These are the main classes exposed by the popup UI:

- `.popup-container`: outer visible card
- `.popup-header`: top header row
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

State classes are also applied in a few places:

- `.ongoing`: active event title
- `.now`: current event time indicator
- `.imminent`: soon-starting event time indicator
- `.stale`: stale sync status styling

The full built-in selector list lives in `internal/ui/popup.go`.

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
3. Optionally specify `calendars` to sync only specific calendars by name

### iCloud

1. Generate an [app-specific password](https://support.apple.com/en-us/102654) for your Apple ID
2. Add to config as an `icloud` type source (uses CalDAV with iCloud defaults)
3. Use your Apple ID as `username` and the app-specific password via `password_cmd`

### Secret Management

Each source field that may contain a secret (`url`, `username`, `password`) has a corresponding `_cmd` variant that runs a shell command to retrieve the value at runtime:

```yaml
- name: "Private"
  type: ics
  url_cmd: "op read op://Vault/Calendar/url"
  username_cmd: "op read op://Vault/Calendar/username"
  password_cmd: "op read op://Vault/Calendar/password"
```

If both a field and its `_cmd` variant are set, the direct value takes precedence.

For full external config (e.g. when your config file is in a public repo), use `config_cmd` to fetch all connection fields from a single command:

```yaml
- name: "Work"
  config_cmd: "pass show calbar/work-config"
  filters:  # Per-source filters can still be set inline
    rules:
      - field: title
        contains: "standup"
```

The command must output YAML or JSON containing `type`, `url`, `username`, `password`, and/or `calendars`.

## Filtering

CalBar supports include/exclude filtering - you can specify which events to show or hide. This is useful when you have a noisy work calendar but only care about specific meetings.

Exclude rules (with `exclude: true`) are applied first to remove unwanted events, then include rules determine which of the remaining events are shown. If no include rules are defined, all non-excluded events are shown.

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

# Exclude rules - hide matching events (applied before include rules)
filters:
  rules:
    - field: title
      contains: "Canceled:"
      exclude: true
    - field: title
      contains: "standup"
      case_insensitive: true
```

### Per-source filters

Filters can also be set on individual sources, applied during sync before global filters:

```yaml
sources:
  - name: "Work"
    type: ms365
    filters:
      rules:
        - field: title
          contains: "standup"
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

## Hiding Events

You can temporarily hide individual events from the calendar view. This is useful for:
- Dismissed or declined meetings that still appear on your calendar
- All-day events you don't need to see
- Recurring events you want to hide just for today

### How it works

**GTK UI:**
- Click an event to open its details
- Click the "Hide" button at the bottom
- Hidden events show a count in the status bar (e.g., "2 hidden")
- Click the hidden count to view and unhide events

**Menu/dmenu UI:**
- Select an event to open its details
- Select "Hide this event"
- A hidden events indicator appears at the bottom of the event list
- Select it to view and unhide events

### Notes

- Hidden events are **ephemeral** - they reset when CalBar restarts
- Hidden events are automatically cleaned up when they end (after the grace period)
- Hiding applies to specific event instances, not recurring series

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

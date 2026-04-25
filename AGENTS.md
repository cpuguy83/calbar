# AGENTS.md

## Project Overview

CalBar is a calendar system tray app for Linux desktops. It fetches events from multiple calendar sources (ICS feeds, CalDAV, iCloud, Microsoft 365), displays them in a GTK4/libadwaita popup or dmenu-style launcher, and provides a StatusNotifierItem tray icon via D-Bus.

Module: `github.com/cpuguy83/calbar`
Go 1.25+, MIT license.

## Build

On Nix and NixOS, enter the dev shell first so GTK and layer-shell libraries are available:

```sh
nix develop
```

```sh
make build    # go build -o calbar ./cmd/calbar
make test     # go test ./...
```

Nix: `nix build` produces three variants — `calbar` (GTK4, NixOS-wrapped), `calbar-unwrapped`, `calbar-lite` (no GTK, `nogtk` tag).

If GTK-linked tests fail with missing `libgtk4-layer-shell.so.0`, run them from `nix develop` or use the `nogtk` tag for non-GTK checks.

## Build Tags

Two mutually exclusive sets control GTK inclusion:

- `//go:build !nogtk && cgo` — GTK-enabled files: `cmd/calbar/run_gtk.go`, `internal/ui/gtk.go`, `internal/ui/popup.go`, `internal/gtk4layershell/layershell.go`
- `//go:build nogtk || !cgo` — Stub/fallback: `cmd/calbar/run_nogui.go`, `internal/ui/gtk_stub.go`

All GTK-enabled files must have the `!nogtk && cgo` build tag.

## Package Layout

| Package | Purpose |
|---------|---------|
| `cmd/calbar` | Entry point, app lifecycle, notification loop, hide/unhide |
| `internal/config` | YAML config loading, defaults, custom duration parsing (supports `d`/`w` suffixes) |
| `internal/calendar` | `Event` struct, `Source` interface, ICS/CalDAV/iCloud/MS365 implementations, ICS merge/serialization, Windows TZID normalization |
| `internal/sync` | Multi-source sync orchestrator with per-source filters |
| `internal/filter` | Include/exclude filter engine (contains/exact/prefix/suffix/regex) |
| `internal/ui` | `UI` interface, GTK backend (`gtk.go`/`popup.go`), stub (`gtk_stub.go`) |
| `internal/ui/menu` | dmenu-style UI backend (rofi, wofi, fuzzel, bemenu, dmenu) |
| `internal/gtk4layershell` | Hand-written purego bindings for `libgtk4-layer-shell.so.0` |
| `internal/tray` | StatusNotifierItem D-Bus tray icon with pixel-rendered icons |
| `internal/notify` | Desktop notifications via D-Bus |
| `internal/links` | Meeting URL detection (Zoom, Teams, Meet, Webex) |
| `internal/auth` | Microsoft Identity Broker D-Bus auth + MSAL device code fallback |

## Critical Pattern: stableCallback

puregotk (pure Go GTK4 bindings via `purego` dlopen) has a **limited number of callback slots**. Every GTK callback must use the `stableCallback[T]` pattern to avoid exhausting them:

```go
type stableCallback[T any] struct {
    once sync.Once
    fn   T
}

func (s *stableCallback[T]) get(init func() T) *T {
    s.once.Do(func() { s.fn = init() })
    return &s.fn
}
```

- Store as a field on the `Popup` struct.
- Access via a `get*Cb()` method that returns the same pointer every time.
- **Never create ad-hoc closures for GTK callbacks** — always use this pattern.
- For per-widget data, use `map[uintptr]*calendar.Event` / `map[uintptr]string` keyed by `widget.GoPointer()` instead of closures.

The same pattern (sync.Once + stored callback) appears in `cmd/calbar/run_gtk.go` for the `updateUI` callback, via a manual `gtkCallbacks` struct rather than the generic `stableCallback[T]` type.

## gtk4-layer-shell Bindings

`internal/gtk4layershell/layershell.go` provides minimal purego bindings. The library is located via `PUREGOTK_LIB_FOLDER` env var (NixOS) or standard dlopen paths. To add a new function:

1. Add the function pointer var (e.g. `xNewFunc func(uintptr, int)`)
2. Add the Go wrapper function
3. Register it in `init()` with `registerFunc(lib, &xNewFunc, "gtk_layer_c_function_name")`

## Popup Architecture (internal/ui/popup.go)

The popup uses a **fullscreen transparent layer-shell overlay** (swaync-style):

- All 4 edges anchored, `exclusive_zone=0` (respects waybar/other panels)
- Content widget aligned top-right with margins inside the fullscreen surface
- `GestureClick` on the window with `graphene.Rect.ContainsPoint` hit-testing detects clicks outside the content and dismisses
- Hover-leave starts a configurable dismiss timer (`hover_dismiss_delay`)
- Window and overlay background are transparent via CSS; `.popup-container` has the visible styling

## Config

YAML at `~/.config/calbar/config.yaml`. See `configs/config.example.yaml`.

Key conventions:
- Most duration fields use plain `time.Duration`. Only `HoverDismissDelay` uses `*time.Duration` (pointer distinguishes "not set" from zero — zero means "never auto-dismiss").
- Custom `UnmarshalYAML` exists on `SyncConfig`, `NotificationConfig`, and `UIConfig`. Each uses a raw intermediary struct to parse duration strings and must preserve all fields when adding new ones.
- `SyncConfig.UnmarshalYAML` uses the custom `parseDuration` function which supports `d` (days) and `w` (weeks) suffixes. `UIConfig.UnmarshalYAML` uses standard `time.ParseDuration` — so `d`/`w` suffixes only work for `sync.interval` and `sync.time_range`, **not** for UI duration fields.
- Source configs support `password_cmd`, `url_cmd`, `username_cmd` fields that execute shell commands to retrieve secrets at runtime. A `config_cmd` field can fetch the entire connection config (type, url, username, password, calendars) from an external command.
- Per-source `filters` allow filtering events at the source level during sync, in addition to global filters.

## Preferences

- Config fields: use pointers only when needed to distinguish "not set" from zero value. Internally keep plain types.
- Don't assume how external tools (swaync, waybar, etc.) work — investigate actual source code when relevant.

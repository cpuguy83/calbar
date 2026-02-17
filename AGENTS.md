# AGENTS.md

## Project Overview

CalBar is a calendar system tray app for Linux desktops. It fetches events from multiple calendar sources (ICS feeds, CalDAV, iCloud, Microsoft 365), displays them in a GTK4/libadwaita popup or dmenu-style launcher, and provides a StatusNotifierItem tray icon via D-Bus.

Module: `github.com/cpuguy83/calbar`
Go 1.25+, MIT license.

## Build

```sh
make build    # go build -o calbar ./cmd/calbar
make test     # go test ./...
```

Nix: `nix build` produces three variants — `calbar` (GTK4, NixOS-wrapped), `calbar-unwrapped`, `calbar-lite` (no GTK, `nogtk` tag).

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
| `internal/calendar` | `Event` struct, `Source` interface, ICS/CalDAV/MS365 implementations |
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

The same pattern appears in `cmd/calbar/run_gtk.go` for the `updateUI` callback.

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
- Duration fields use `*time.Duration` in the YAML-facing struct (pointer = distinguishes "not set" from zero) and plain `time.Duration` internally.
- Custom `UnmarshalYAML` in `UIConfig` parses durations and must preserve all fields when using a raw intermediary struct.
- `password_cmd` fields allow external password managers.

## Preferences

- Config fields: use pointers only when needed to distinguish "not set" from zero value. Internally keep plain types.
- Don't assume how external tools (swaync, waybar, etc.) work — investigate actual source code when relevant.

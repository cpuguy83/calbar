// Package config provides configuration loading for calbar and calsync.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure.
type Config struct {
	Sync          SyncConfig         `yaml:"sync"`
	Sources       []SourceConfig     `yaml:"sources"`
	Filters       FilterConfig       `yaml:"filters"`
	Notifications NotificationConfig `yaml:"notifications"`
	UI            UIConfig           `yaml:"ui"`
}

// SyncConfig configures the sync daemon.
type SyncConfig struct {
	Interval  time.Duration `yaml:"interval"`
	Output    string        `yaml:"output"`
	TimeRange time.Duration `yaml:"time_range"` // How far ahead to fetch events (default: 14 days)
}

// SourceConnectionConfig contains the connection-specific fields for a calendar source.
// These fields describe how to connect to the remote calendar.
// They can be specified inline in the config file, or fetched from a command via config_cmd.
//
// Each sensitive field (url, username, password) has a corresponding _cmd variant
// that executes a shell command to retrieve the value at runtime.
// If both a field and its _cmd variant are set, the direct value takes precedence.
type SourceConnectionConfig struct {
	Type        string   `yaml:"type"` // "ics", "caldav", "icloud", "ms365"
	URL         string   `yaml:"url"`
	URLCmd      string   `yaml:"url_cmd,omitempty"`
	Username    string   `yaml:"username,omitempty"`
	UsernameCmd string   `yaml:"username_cmd,omitempty"`
	Password    string   `yaml:"password,omitempty"`
	PasswordCmd string   `yaml:"password_cmd,omitempty"`
	Calendars   []string `yaml:"calendars,omitempty"` // For CalDAV/iCloud/MS365: which calendars to sync
}

// isEmpty returns true if no connection fields are set.
func (s *SourceConnectionConfig) isEmpty() bool {
	return s.Type == "" &&
		s.URL == "" && s.URLCmd == "" &&
		s.Username == "" && s.UsernameCmd == "" &&
		s.Password == "" && s.PasswordCmd == "" &&
		len(s.Calendars) == 0
}

// SourceConfig configures a calendar source.
// Connection details can be specified inline or fetched from an external command via config_cmd.
// If config_cmd is set, inline connection fields (type, url, username, password, password_cmd, calendars)
// must not be set — the command output provides them.
type SourceConfig struct {
	Name      string       `yaml:"name"`
	ConfigCmd string       `yaml:"config_cmd,omitempty"` // Command that outputs connection config as YAML/JSON
	Filters   FilterConfig `yaml:"filters,omitempty"`    // Per-source filters (include/exclude)

	SourceConnectionConfig `yaml:",inline"` // Inline connection fields (mutually exclusive with config_cmd)
}

// FilterConfig configures event filtering.
type FilterConfig struct {
	Mode  string       `yaml:"mode"` // "or" or "and"
	Rules []FilterRule `yaml:"rules"`
}

// FilterRule defines a single filter rule.
// Use exactly one of: Contains, Exact, Prefix, Suffix, or Regex.
type FilterRule struct {
	Field           string `yaml:"field"`              // "title", "organizer", "source", "description", "location"
	Contains        string `yaml:"contains,omitempty"` // Substring match
	Exact           string `yaml:"exact,omitempty"`    // Exact string match
	Prefix          string `yaml:"prefix,omitempty"`   // Starts with
	Suffix          string `yaml:"suffix,omitempty"`   // Ends with
	Regex           string `yaml:"regex,omitempty"`    // Regular expression
	CaseInsensitive bool   `yaml:"case_insensitive"`
	Exclude         bool   `yaml:"exclude,omitempty"` // If true, exclude matching events instead of including

	// Deprecated: Use Contains, Exact, Prefix, Suffix, or Regex instead.
	// Kept for backward compatibility. If set and no other match type is specified,
	// treated as Contains (or Regex if prefixed with "regex:").
	Match string `yaml:"match,omitempty"`
}

// NotificationConfig configures desktop notifications.
type NotificationConfig struct {
	Enabled bool            `yaml:"enabled"`
	Before  []time.Duration `yaml:"before"`
}

// UIConfig configures the tray app UI.
type UIConfig struct {
	TimeRange         time.Duration  `yaml:"time_range"`          // How far ahead to look (default: 7 days)
	MaxEvents         int            `yaml:"max_events"`          // Max events to show (default: 20)
	Theme             string         `yaml:"theme"`               // "system", "light", "dark"
	Backend           string         `yaml:"backend"`             // "auto", "gtk", "menu" (default: auto)
	Menu              MenuConfig     `yaml:"menu"`                // Menu-specific configuration
	EventEndGrace     time.Duration  `yaml:"event_end_grace"`     // Keep events visible after they end (default: 5m)
	HoverDismissDelay *time.Duration `yaml:"hover_dismiss_delay"` // Delay before dismiss on pointer-leave (default: 5s, 0 = never auto-dismiss)
}

// MenuConfig configures the dmenu-style UI backend.
type MenuConfig struct {
	Program string   `yaml:"program"` // dmenu program to use (auto-detect if empty)
	Args    []string `yaml:"args"`    // extra args to pass to the program
}

// Load reads configuration from the default location (~/.config/calbar/config.yaml).
func Load() (*Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config dir: %w", err)
	}

	path := filepath.Join(configDir, "calbar", "config.yaml")
	return LoadFrom(path)
}

// LoadFrom reads configuration from a specific path.
func LoadFrom(path string) (*Config, error) {
	path = expandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	// Expand paths
	cfg.Sync.Output = expandPath(cfg.Sync.Output)

	return &cfg, nil
}

// applyDefaults sets default values for unspecified config options.
func (c *Config) applyDefaults() {
	if c.Sync.Interval == 0 {
		c.Sync.Interval = 5 * time.Minute
	}
	if c.Sync.TimeRange == 0 {
		c.Sync.TimeRange = 14 * 24 * time.Hour // Default: 14 days
	}
	if c.Sync.Output == "" {
		dataDir, _ := os.UserHomeDir()
		c.Sync.Output = filepath.Join(dataDir, ".local", "share", "calbar", "calendar.ics")
	}
	if c.Filters.Mode == "" {
		c.Filters.Mode = "or"
	}
	if c.UI.TimeRange == 0 {
		c.UI.TimeRange = 7 * 24 * time.Hour // Default: 7 days
	}
	if c.UI.MaxEvents == 0 {
		c.UI.MaxEvents = 20
	}
	if c.UI.Theme == "" {
		c.UI.Theme = "system"
	}
	if c.UI.Backend == "" {
		c.UI.Backend = "auto"
	}
	if c.UI.EventEndGrace == 0 {
		c.UI.EventEndGrace = 5 * time.Minute // Default: 5 minutes
	}
	if c.UI.HoverDismissDelay == nil {
		d := 3 * time.Second
		c.UI.HoverDismissDelay = &d // Default: 3 seconds
	}
	if c.Notifications.Before == nil {
		c.Notifications.Before = []time.Duration{15 * time.Minute, 5 * time.Minute}
	}
}

// runCmd executes a shell command and returns its trimmed stdout.
func runCmd(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GetURL returns the URL for a source, executing url_cmd if needed.
// If both url and url_cmd are set, the direct value takes precedence.
func (s *SourceConnectionConfig) GetURL() (string, error) {
	if s.URL != "" {
		return s.URL, nil
	}
	if s.URLCmd == "" {
		return "", nil
	}
	v, err := runCmd(s.URLCmd)
	if err != nil {
		return "", fmt.Errorf("execute url_cmd: %w", err)
	}
	return v, nil
}

// GetUsername returns the username for a source, executing username_cmd if needed.
// If both username and username_cmd are set, the direct value takes precedence.
func (s *SourceConnectionConfig) GetUsername() (string, error) {
	if s.Username != "" {
		return s.Username, nil
	}
	if s.UsernameCmd == "" {
		return "", nil
	}
	v, err := runCmd(s.UsernameCmd)
	if err != nil {
		return "", fmt.Errorf("execute username_cmd: %w", err)
	}
	return v, nil
}

// GetPassword returns the password for a source, executing password_cmd if needed.
// If both password and password_cmd are set, the direct value takes precedence.
func (s *SourceConnectionConfig) GetPassword() (string, error) {
	if s.Password != "" {
		return s.Password, nil
	}
	if s.PasswordCmd == "" {
		return "", nil
	}
	v, err := runCmd(s.PasswordCmd)
	if err != nil {
		return "", fmt.Errorf("execute password_cmd: %w", err)
	}
	return v, nil
}

// Validate checks that a SourceConfig is well-formed.
// If config_cmd is set, inline connection fields must not be set.
// If config_cmd is not set, type is required.
func (s *SourceConfig) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("source name is required")
	}

	if s.ConfigCmd != "" {
		if !s.SourceConnectionConfig.isEmpty() {
			return fmt.Errorf("source %q: config_cmd and inline connection fields (type, url, url_cmd, username, username_cmd, password, password_cmd, calendars) are mutually exclusive", s.Name)
		}
		return nil
	}

	if s.Type == "" {
		return fmt.Errorf("source %q: type is required when config_cmd is not set", s.Name)
	}

	return nil
}

// ResolvedSource contains the fully resolved configuration for a calendar source,
// with connection details either from inline fields or from config_cmd output.
type ResolvedSource struct {
	Name    string
	Filters FilterConfig
	SourceConnectionConfig
}

// Resolve returns the fully resolved source configuration.
// If config_cmd is set, it executes the command and unmarshals the output as YAML
// to obtain connection details. Otherwise, the inline fields are used directly.
func (s *SourceConfig) Resolve() (*ResolvedSource, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	resolved := &ResolvedSource{
		Name:    s.Name,
		Filters: s.Filters,
	}

	if s.ConfigCmd == "" {
		resolved.SourceConnectionConfig = s.SourceConnectionConfig
		return resolved, nil
	}

	cmd := exec.Command("sh", "-c", s.ConfigCmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("source %q: execute config_cmd: %w", s.Name, err)
	}

	var conn SourceConnectionConfig
	if err := yaml.Unmarshal(out, &conn); err != nil {
		return nil, fmt.Errorf("source %q: parse config_cmd output: %w", s.Name, err)
	}

	if conn.Type == "" {
		return nil, fmt.Errorf("source %q: config_cmd output must include 'type'", s.Name)
	}

	resolved.SourceConnectionConfig = conn
	return resolved, nil
}

// parseDuration parses a duration string with support for days (d) and weeks (w).
// Examples: "14d" (14 days), "2w" (2 weeks), "5m" (5 minutes), "1h" (1 hour).
// Falls back to time.ParseDuration for standard Go duration formats.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// Check for days suffix
	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(numStr, "%d", &days); err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		if days < 0 {
			return 0, fmt.Errorf("invalid duration %q: negative values not allowed", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Check for weeks suffix
	if strings.HasSuffix(s, "w") {
		numStr := strings.TrimSuffix(s, "w")
		var weeks int
		if _, err := fmt.Sscanf(numStr, "%d", &weeks); err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		if weeks < 0 {
			return 0, fmt.Errorf("invalid duration %q: negative values not allowed", s)
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}

	// Fall back to standard Go duration parsing
	return time.ParseDuration(s)
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// UnmarshalYAML implements custom unmarshaling for duration fields.
func (c *SyncConfig) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Interval  string `yaml:"interval"`
		Output    string `yaml:"output"`
		TimeRange string `yaml:"time_range"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}

	if raw.Interval != "" {
		d, err := parseDuration(raw.Interval)
		if err != nil {
			return fmt.Errorf("parse interval: %w", err)
		}
		c.Interval = d
	}
	if raw.TimeRange != "" {
		d, err := parseDuration(raw.TimeRange)
		if err != nil {
			return fmt.Errorf("parse time_range: %w", err)
		}
		c.TimeRange = d
	}
	c.Output = raw.Output
	return nil
}

// UnmarshalYAML implements custom unmarshaling for notification config.
func (c *NotificationConfig) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Enabled bool     `yaml:"enabled"`
		Before  []string `yaml:"before"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}

	c.Enabled = raw.Enabled
	for _, s := range raw.Before {
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("parse notification before duration %q: %w", s, err)
		}
		c.Before = append(c.Before, d)
	}
	return nil
}

// UnmarshalYAML implements custom unmarshaling for UI config.
func (c *UIConfig) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		TimeRange         string     `yaml:"time_range"`
		MaxEvents         int        `yaml:"max_events"`
		Theme             string     `yaml:"theme"`
		Backend           string     `yaml:"backend"`
		Menu              MenuConfig `yaml:"menu"`
		EventEndGrace     string     `yaml:"event_end_grace"`
		HoverDismissDelay *string    `yaml:"hover_dismiss_delay"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}

	if raw.TimeRange != "" {
		d, err := time.ParseDuration(raw.TimeRange)
		if err != nil {
			return fmt.Errorf("parse time_range: %w", err)
		}
		c.TimeRange = d
	}
	if raw.EventEndGrace != "" {
		d, err := time.ParseDuration(raw.EventEndGrace)
		if err != nil {
			return fmt.Errorf("parse event_end_grace: %w", err)
		}
		c.EventEndGrace = d
	}
	if raw.HoverDismissDelay != nil {
		d, err := time.ParseDuration(*raw.HoverDismissDelay)
		if err != nil {
			return fmt.Errorf("parse hover_dismiss_delay: %w", err)
		}
		c.HoverDismissDelay = &d
	}
	c.MaxEvents = raw.MaxEvents
	c.Theme = raw.Theme
	c.Backend = raw.Backend
	c.Menu = raw.Menu
	return nil
}

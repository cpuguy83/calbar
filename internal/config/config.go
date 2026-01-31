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
	Interval time.Duration `yaml:"interval"`
	Output   string        `yaml:"output"`
}

// SourceConfig configures a calendar source.
type SourceConfig struct {
	Name        string       `yaml:"name"`
	Type        string       `yaml:"type"` // "ics", "caldav", "ms365"
	URL         string       `yaml:"url"`
	Username    string       `yaml:"username,omitempty"`
	Password    string       `yaml:"password,omitempty"`
	PasswordCmd string       `yaml:"password_cmd,omitempty"`
	Calendars   []string     `yaml:"calendars,omitempty"` // For CalDAV/MS365: which calendars to sync
	Filters     FilterConfig `yaml:"filters,omitempty"`   // Per-source filters (include)
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
	TimeRange time.Duration `yaml:"time_range"` // How far ahead to look (default: 7 days)
	MaxEvents int           `yaml:"max_events"` // Max events to show (default: 20)
	Theme     string        `yaml:"theme"`      // "system", "light", "dark"
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
	if c.Notifications.Before == nil {
		c.Notifications.Before = []time.Duration{15 * time.Minute, 5 * time.Minute}
	}
}

// GetPassword returns the password for a source, executing password_cmd if needed.
func (s *SourceConfig) GetPassword() (string, error) {
	if s.Password != "" {
		return s.Password, nil
	}
	if s.PasswordCmd == "" {
		return "", nil
	}

	// Execute the password command
	cmd := exec.Command("sh", "-c", s.PasswordCmd)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("execute password_cmd: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
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
		Interval string `yaml:"interval"`
		Output   string `yaml:"output"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}

	if raw.Interval != "" {
		d, err := time.ParseDuration(raw.Interval)
		if err != nil {
			return fmt.Errorf("parse interval: %w", err)
		}
		c.Interval = d
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
		TimeRange string `yaml:"time_range"`
		MaxEvents int    `yaml:"max_events"`
		Theme     string `yaml:"theme"`
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
	c.MaxEvents = raw.MaxEvents
	c.Theme = raw.Theme
	return nil
}

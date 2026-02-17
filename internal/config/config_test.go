package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// Days
		{"1d", 24 * time.Hour, false},
		{"14d", 14 * 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},

		// Weeks
		{"1w", 7 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{"4w", 28 * 24 * time.Hour, false},

		// Standard Go durations
		{"5m", 5 * time.Minute, false},
		{"1h", time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"336h", 14 * 24 * time.Hour, false},
		{"1h30m", time.Hour + 30*time.Minute, false},

		// Edge cases
		{"0d", 0, false},
		{"0w", 0, false},
		{"", 0, false},
		{"  14d  ", 14 * 24 * time.Hour, false},

		// Errors
		{"invalid", 0, true},
		{"d", 0, true},
		{"w", 0, true},
		{"14x", 0, true},
		{"-1d", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSourceConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SourceConfig
		wantErr bool
	}{
		{
			name: "valid inline config",
			cfg: SourceConfig{
				Name: "test",
				SourceConnectionConfig: SourceConnectionConfig{
					Type: "ics",
					URL:  "https://example.com/cal.ics",
				},
			},
		},
		{
			name: "valid config_cmd",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo 'type: ics'",
			},
		},
		{
			name:    "missing name",
			cfg:     SourceConfig{},
			wantErr: true,
		},
		{
			name: "missing type without config_cmd",
			cfg: SourceConfig{
				Name: "test",
				SourceConnectionConfig: SourceConnectionConfig{
					URL: "https://example.com/cal.ics",
				},
			},
			wantErr: true,
		},
		{
			name: "config_cmd with inline type",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo test",
				SourceConnectionConfig: SourceConnectionConfig{
					Type: "ics",
				},
			},
			wantErr: true,
		},
		{
			name: "config_cmd with inline url",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo test",
				SourceConnectionConfig: SourceConnectionConfig{
					URL: "https://example.com",
				},
			},
			wantErr: true,
		},
		{
			name: "config_cmd with inline password",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo test",
				SourceConnectionConfig: SourceConnectionConfig{
					Password: "secret",
				},
			},
			wantErr: true,
		},
		{
			name: "config_cmd with inline username",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo test",
				SourceConnectionConfig: SourceConnectionConfig{
					Username: "user",
				},
			},
			wantErr: true,
		},
		{
			name: "config_cmd with inline password_cmd",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo test",
				SourceConnectionConfig: SourceConnectionConfig{
					PasswordCmd: "pass show foo",
				},
			},
			wantErr: true,
		},
		{
			name: "config_cmd with inline calendars",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo test",
				SourceConnectionConfig: SourceConnectionConfig{
					Calendars: []string{"Personal"},
				},
			},
			wantErr: true,
		},
		{
			name: "config_cmd with inline url_cmd",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo test",
				SourceConnectionConfig: SourceConnectionConfig{
					URLCmd: "echo https://example.com",
				},
			},
			wantErr: true,
		},
		{
			name: "config_cmd with inline username_cmd",
			cfg: SourceConfig{
				Name:      "test",
				ConfigCmd: "echo test",
				SourceConnectionConfig: SourceConnectionConfig{
					UsernameCmd: "echo user",
				},
			},
			wantErr: true,
		},
		{
			name: "valid inline with url_cmd instead of url",
			cfg: SourceConfig{
				Name: "test",
				SourceConnectionConfig: SourceConnectionConfig{
					Type:   "ics",
					URLCmd: "echo https://example.com/cal.ics",
				},
			},
		},
		{
			name: "valid inline with username_cmd instead of username",
			cfg: SourceConfig{
				Name: "test",
				SourceConnectionConfig: SourceConnectionConfig{
					Type:        "caldav",
					URL:         "https://example.com/dav/",
					UsernameCmd: "echo user",
					PasswordCmd: "echo pass",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSourceConfigResolveInline(t *testing.T) {
	cfg := SourceConfig{
		Name: "test",
		SourceConnectionConfig: SourceConnectionConfig{
			Type:      "caldav",
			URL:       "https://example.com/dav/",
			Username:  "user",
			Password:  "pass",
			Calendars: []string{"Work"},
		},
		Filters: FilterConfig{
			Mode: "or",
			Rules: []FilterRule{
				{Field: "title", Contains: "meeting"},
			},
		},
	}

	resolved, err := cfg.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resolved.Name != "test" {
		t.Errorf("Name = %q, want %q", resolved.Name, "test")
	}
	if resolved.Type != "caldav" {
		t.Errorf("Type = %q, want %q", resolved.Type, "caldav")
	}
	if resolved.URL != "https://example.com/dav/" {
		t.Errorf("URL = %q, want %q", resolved.URL, "https://example.com/dav/")
	}
	if resolved.Username != "user" {
		t.Errorf("Username = %q, want %q", resolved.Username, "user")
	}
	if resolved.Password != "pass" {
		t.Errorf("Password = %q, want %q", resolved.Password, "pass")
	}
	if len(resolved.Calendars) != 1 || resolved.Calendars[0] != "Work" {
		t.Errorf("Calendars = %v, want [Work]", resolved.Calendars)
	}
	if len(resolved.Filters.Rules) != 1 {
		t.Errorf("Filters.Rules = %v, want 1 rule", resolved.Filters.Rules)
	}
}

func TestSourceConfigResolveConfigCmd(t *testing.T) {
	cfg := SourceConfig{
		Name:      "test",
		ConfigCmd: `printf 'type: ics\nurl: https://example.com/secret.ics\nusername: u\npassword: p\n'`,
		Filters: FilterConfig{
			Mode: "or",
		},
	}

	resolved, err := cfg.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resolved.Name != "test" {
		t.Errorf("Name = %q, want %q", resolved.Name, "test")
	}
	if resolved.Type != "ics" {
		t.Errorf("Type = %q, want %q", resolved.Type, "ics")
	}
	if resolved.URL != "https://example.com/secret.ics" {
		t.Errorf("URL = %q, want %q", resolved.URL, "https://example.com/secret.ics")
	}
	if resolved.Username != "u" {
		t.Errorf("Username = %q, want %q", resolved.Username, "u")
	}
	if resolved.Password != "p" {
		t.Errorf("Password = %q, want %q", resolved.Password, "p")
	}
	if resolved.Filters.Mode != "or" {
		t.Errorf("Filters.Mode = %q, want %q", resolved.Filters.Mode, "or")
	}
}

func TestSourceConfigResolveConfigCmdJSON(t *testing.T) {
	cfg := SourceConfig{
		Name:      "test",
		ConfigCmd: `printf '{"type":"caldav","url":"https://example.com/dav/","username":"u","password":"p","calendars":["A","B"]}'`,
	}

	resolved, err := cfg.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resolved.Type != "caldav" {
		t.Errorf("Type = %q, want %q", resolved.Type, "caldav")
	}
	if resolved.URL != "https://example.com/dav/" {
		t.Errorf("URL = %q, want %q", resolved.URL, "https://example.com/dav/")
	}
	if len(resolved.Calendars) != 2 || resolved.Calendars[0] != "A" || resolved.Calendars[1] != "B" {
		t.Errorf("Calendars = %v, want [A B]", resolved.Calendars)
	}
}

func TestSourceConfigResolveConfigCmdMissingType(t *testing.T) {
	cfg := SourceConfig{
		Name:      "test",
		ConfigCmd: `printf 'url: https://example.com/cal.ics\n'`,
	}

	_, err := cfg.Resolve()
	if err == nil {
		t.Fatal("Resolve() expected error for missing type in config_cmd output")
	}
}

func TestSourceConfigResolveConfigCmdFails(t *testing.T) {
	cfg := SourceConfig{
		Name:      "test",
		ConfigCmd: "false",
	}

	_, err := cfg.Resolve()
	if err == nil {
		t.Fatal("Resolve() expected error for failed config_cmd")
	}
}

func TestSourceConfigGetPassword(t *testing.T) {
	t.Run("direct password", func(t *testing.T) {
		s := SourceConnectionConfig{Password: "direct"}
		got, err := s.GetPassword()
		if err != nil {
			t.Fatalf("GetPassword() error = %v", err)
		}
		if got != "direct" {
			t.Errorf("GetPassword() = %q, want %q", got, "direct")
		}
	})

	t.Run("password_cmd", func(t *testing.T) {
		s := SourceConnectionConfig{PasswordCmd: "echo secret123"}
		got, err := s.GetPassword()
		if err != nil {
			t.Fatalf("GetPassword() error = %v", err)
		}
		if got != "secret123" {
			t.Errorf("GetPassword() = %q, want %q", got, "secret123")
		}
	})

	t.Run("password takes precedence over password_cmd", func(t *testing.T) {
		s := SourceConnectionConfig{Password: "direct", PasswordCmd: "echo fromcmd"}
		got, err := s.GetPassword()
		if err != nil {
			t.Fatalf("GetPassword() error = %v", err)
		}
		if got != "direct" {
			t.Errorf("GetPassword() = %q, want %q", got, "direct")
		}
	})

	t.Run("no password", func(t *testing.T) {
		s := SourceConnectionConfig{}
		got, err := s.GetPassword()
		if err != nil {
			t.Fatalf("GetPassword() error = %v", err)
		}
		if got != "" {
			t.Errorf("GetPassword() = %q, want empty", got)
		}
	})
}

func TestSourceConfigGetURL(t *testing.T) {
	t.Run("direct url", func(t *testing.T) {
		s := SourceConnectionConfig{URL: "https://example.com"}
		got, err := s.GetURL()
		if err != nil {
			t.Fatalf("GetURL() error = %v", err)
		}
		if got != "https://example.com" {
			t.Errorf("GetURL() = %q, want %q", got, "https://example.com")
		}
	})

	t.Run("url_cmd", func(t *testing.T) {
		s := SourceConnectionConfig{URLCmd: "echo https://example.com/secret.ics"}
		got, err := s.GetURL()
		if err != nil {
			t.Fatalf("GetURL() error = %v", err)
		}
		if got != "https://example.com/secret.ics" {
			t.Errorf("GetURL() = %q, want %q", got, "https://example.com/secret.ics")
		}
	})

	t.Run("url takes precedence over url_cmd", func(t *testing.T) {
		s := SourceConnectionConfig{URL: "https://direct.com", URLCmd: "echo https://fromcmd.com"}
		got, err := s.GetURL()
		if err != nil {
			t.Fatalf("GetURL() error = %v", err)
		}
		if got != "https://direct.com" {
			t.Errorf("GetURL() = %q, want %q", got, "https://direct.com")
		}
	})

	t.Run("no url", func(t *testing.T) {
		s := SourceConnectionConfig{}
		got, err := s.GetURL()
		if err != nil {
			t.Fatalf("GetURL() error = %v", err)
		}
		if got != "" {
			t.Errorf("GetURL() = %q, want empty", got)
		}
	})

	t.Run("url_cmd fails", func(t *testing.T) {
		s := SourceConnectionConfig{URLCmd: "false"}
		_, err := s.GetURL()
		if err == nil {
			t.Fatal("GetURL() expected error for failed url_cmd")
		}
	})
}

func TestSourceConfigGetUsername(t *testing.T) {
	t.Run("direct username", func(t *testing.T) {
		s := SourceConnectionConfig{Username: "user"}
		got, err := s.GetUsername()
		if err != nil {
			t.Fatalf("GetUsername() error = %v", err)
		}
		if got != "user" {
			t.Errorf("GetUsername() = %q, want %q", got, "user")
		}
	})

	t.Run("username_cmd", func(t *testing.T) {
		s := SourceConnectionConfig{UsernameCmd: "echo myuser"}
		got, err := s.GetUsername()
		if err != nil {
			t.Fatalf("GetUsername() error = %v", err)
		}
		if got != "myuser" {
			t.Errorf("GetUsername() = %q, want %q", got, "myuser")
		}
	})

	t.Run("username takes precedence over username_cmd", func(t *testing.T) {
		s := SourceConnectionConfig{Username: "direct", UsernameCmd: "echo fromcmd"}
		got, err := s.GetUsername()
		if err != nil {
			t.Fatalf("GetUsername() error = %v", err)
		}
		if got != "direct" {
			t.Errorf("GetUsername() = %q, want %q", got, "direct")
		}
	})

	t.Run("no username", func(t *testing.T) {
		s := SourceConnectionConfig{}
		got, err := s.GetUsername()
		if err != nil {
			t.Fatalf("GetUsername() error = %v", err)
		}
		if got != "" {
			t.Errorf("GetUsername() = %q, want empty", got)
		}
	})

	t.Run("username_cmd fails", func(t *testing.T) {
		s := SourceConnectionConfig{UsernameCmd: "false"}
		_, err := s.GetUsername()
		if err == nil {
			t.Fatal("GetUsername() expected error for failed username_cmd")
		}
	})
}

func TestSourceConfigYAMLUnmarshal(t *testing.T) {
	t.Run("inline fields", func(t *testing.T) {
		input := `
name: "Personal"
type: ics
url: "https://example.com/cal.ics"
username: "user"
password: "pass"
`
		var cfg SourceConfig
		if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if cfg.Name != "Personal" {
			t.Errorf("Name = %q, want %q", cfg.Name, "Personal")
		}
		if cfg.Type != "ics" {
			t.Errorf("Type = %q, want %q", cfg.Type, "ics")
		}
		if cfg.URL != "https://example.com/cal.ics" {
			t.Errorf("URL = %q, want %q", cfg.URL, "https://example.com/cal.ics")
		}
		if cfg.ConfigCmd != "" {
			t.Errorf("ConfigCmd = %q, want empty", cfg.ConfigCmd)
		}
	})

	t.Run("config_cmd with filters", func(t *testing.T) {
		input := `
name: "Personal"
config_cmd: "op read op://Vault/Cal/config"
filters:
  mode: or
  rules:
    - field: title
      contains: "standup"
`
		var cfg SourceConfig
		if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if cfg.Name != "Personal" {
			t.Errorf("Name = %q, want %q", cfg.Name, "Personal")
		}
		if cfg.ConfigCmd != "op read op://Vault/Cal/config" {
			t.Errorf("ConfigCmd = %q, want %q", cfg.ConfigCmd, "op read op://Vault/Cal/config")
		}
		if cfg.Type != "" {
			t.Errorf("Type = %q, want empty", cfg.Type)
		}
		if len(cfg.Filters.Rules) != 1 {
			t.Errorf("Filters.Rules len = %d, want 1", len(cfg.Filters.Rules))
		}
	})

	t.Run("cmd variant fields", func(t *testing.T) {
		input := `
name: "Secret"
type: ics
url_cmd: "op read op://Vault/Cal/url"
username_cmd: "op read op://Vault/Cal/username"
password_cmd: "op read op://Vault/Cal/password"
`
		var cfg SourceConfig
		if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if cfg.URLCmd != "op read op://Vault/Cal/url" {
			t.Errorf("URLCmd = %q, want %q", cfg.URLCmd, "op read op://Vault/Cal/url")
		}
		if cfg.UsernameCmd != "op read op://Vault/Cal/username" {
			t.Errorf("UsernameCmd = %q, want %q", cfg.UsernameCmd, "op read op://Vault/Cal/username")
		}
		if cfg.PasswordCmd != "op read op://Vault/Cal/password" {
			t.Errorf("PasswordCmd = %q, want %q", cfg.PasswordCmd, "op read op://Vault/Cal/password")
		}
		if cfg.URL != "" {
			t.Errorf("URL = %q, want empty", cfg.URL)
		}
		if cfg.Username != "" {
			t.Errorf("Username = %q, want empty", cfg.Username)
		}
		if cfg.Password != "" {
			t.Errorf("Password = %q, want empty", cfg.Password)
		}
	})
}

// Package menu provides a dmenu-style UI backend for calbar.
package menu

import (
	"errors"
	"os/exec"
)

// Supported dmenu-compatible programs in order of preference.
var supportedPrograms = []string{
	"rofi",
	"wofi",
	"fuzzel",
	"bemenu",
	"dmenu",
}

// Detect finds the first available dmenu-compatible program.
// Returns the program name or an error if none are found.
func Detect() (string, error) {
	for _, prog := range supportedPrograms {
		if path, err := exec.LookPath(prog); err == nil && path != "" {
			return prog, nil
		}
	}
	return "", errors.New("no dmenu-compatible program found (tried: rofi, wofi, fuzzel, bemenu, dmenu)")
}

// Supported returns the list of supported dmenu programs.
func Supported() []string {
	return supportedPrograms
}

// Available returns a list of dmenu programs that are currently installed.
func Available() []string {
	var available []string
	for _, prog := range supportedPrograms {
		if path, err := exec.LookPath(prog); err == nil && path != "" {
			available = append(available, prog)
		}
	}
	return available
}

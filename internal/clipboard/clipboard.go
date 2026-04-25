package clipboard

import (
	"fmt"
	"os/exec"
	"strings"
)

// Copy writes text to the desktop clipboard using common Linux clipboard tools.
func Copy(text string) error {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return fmt.Errorf("clipboard text is empty")
	}

	if err := runTextCommand("wl-copy", nil, clean); err == nil {
		return nil
	}
	if err := runTextCommand("xclip", []string{"-selection", "clipboard"}, clean); err == nil {
		return nil
	}
	if err := runTextCommand("xsel", []string{"--clipboard", "--input"}, clean); err == nil {
		return nil
	}

	return fmt.Errorf("no clipboard tool available (tried wl-copy, xclip, xsel)")
}

func runTextCommand(name string, args []string, text string) error {
	path, err := exec.LookPath(name)
	if err != nil || path == "" {
		return fmt.Errorf("%s not found", name)
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}

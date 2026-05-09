package clipboard

import (
	"fmt"
	"os/exec"
	"strings"
)

func cleanText(text string) (string, error) {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return "", fmt.Errorf("clipboard text is empty")
	}
	return clean, nil
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

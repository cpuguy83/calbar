//go:build linux

package clipboard

import "fmt"

// Copy writes text to the desktop clipboard using common Linux clipboard tools.
func Copy(text string) error {
	clean, err := cleanText(text)
	if err != nil {
		return err
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

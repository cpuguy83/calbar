//go:build !linux && !darwin

package clipboard

import "fmt"

// Copy writes text to the desktop clipboard.
func Copy(text string) error {
	if _, err := cleanText(text); err != nil {
		return err
	}
	return fmt.Errorf("clipboard copy is not implemented on this platform")
}

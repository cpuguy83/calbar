//go:build !linux && !darwin

package links

import "fmt"

// Open opens a URL in the default browser.
func Open(url string) error {
	return fmt.Errorf("opening URLs is not implemented on this platform")
}

// OpenPath opens a local path using the desktop's default handler.
func OpenPath(path string) error {
	return fmt.Errorf("opening paths is not implemented on this platform")
}

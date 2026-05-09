//go:build linux

package links

import "os/exec"

// Open opens a URL in the default browser using xdg-open.
func Open(url string) error {
	return exec.Command("xdg-open", url).Start()
}

// OpenPath opens a local path using the desktop's default handler.
func OpenPath(path string) error {
	return exec.Command("xdg-open", path).Start()
}

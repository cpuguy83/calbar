//go:build darwin

package links

import "os/exec"

// Open opens a URL in the default browser.
func Open(url string) error {
	return exec.Command("open", url).Start()
}

// OpenPath opens a local path using the desktop's default handler.
func OpenPath(path string) error {
	return exec.Command("open", path).Start()
}

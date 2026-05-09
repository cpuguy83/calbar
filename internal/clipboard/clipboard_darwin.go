//go:build darwin

package clipboard

// Copy writes text to the macOS pasteboard using pbcopy.
func Copy(text string) error {
	clean, err := cleanText(text)
	if err != nil {
		return err
	}
	return runTextCommand("pbcopy", nil, clean)
}

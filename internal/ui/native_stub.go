//go:build !darwin

package ui

// NewNative returns nil when the native backend is not available.
func NewNative(cfg Config) UI {
	return nil
}

// NativeAvailable reports whether the native platform backend is available.
func NativeAvailable() bool {
	return false
}

//go:build !nogtk && cgo

// Package gtk4layershell provides puregotk bindings for gtk4-layer-shell.
// This is a minimal hand-written binding for the functions CalBar uses.
package gtk4layershell

import (
	"os"
	"path/filepath"

	"github.com/jwijenbergh/purego"
)

// Edge represents which edge of the screen to anchor to.
type Edge int

const (
	EdgeLeft   Edge = 0
	EdgeRight  Edge = 1
	EdgeTop    Edge = 2
	EdgeBottom Edge = 3
)

// Layer represents the layer to place the window on.
type Layer int

const (
	LayerBackground Layer = 0
	LayerBottom     Layer = 1
	LayerTop        Layer = 2
	LayerOverlay    Layer = 3
)

// KeyboardMode represents how the layer surface handles keyboard focus.
type KeyboardMode int

const (
	KeyboardModeNone      KeyboardMode = 0
	KeyboardModeExclusive KeyboardMode = 1
	KeyboardModeOnDemand  KeyboardMode = 2
)

// Function pointers populated via dlopen
var (
	xIsSupported     func() bool
	xInitForWindow   func(uintptr)
	xSetLayer        func(uintptr, int)
	xSetAnchor       func(uintptr, int, bool)
	xSetMargin       func(uintptr, int, int)
	xSetKeyboardMode func(uintptr, int)
	xSetNamespace    func(uintptr, string)
)

// IsSupported returns true if the compositor supports the layer shell protocol.
func IsSupported() bool {
	return xIsSupported()
}

// InitForWindow sets up a GtkWindow as a layer surface.
// Call this before showing the window.
func InitForWindow(window uintptr) {
	xInitForWindow(window)
}

// SetLayer sets which layer the window should be on.
func SetLayer(window uintptr, layer Layer) {
	xSetLayer(window, int(layer))
}

// SetAnchor anchors the window to an edge of the screen.
func SetAnchor(window uintptr, edge Edge, anchor bool) {
	xSetAnchor(window, int(edge), anchor)
}

// SetMargin sets the margin from an edge when anchored.
func SetMargin(window uintptr, edge Edge, margin int) {
	xSetMargin(window, int(edge), margin)
}

// SetKeyboardMode sets how the window interacts with keyboard focus.
func SetKeyboardMode(window uintptr, mode KeyboardMode) {
	xSetKeyboardMode(window, int(mode))
}

// SetNamespace sets the namespace for the layer surface.
func SetNamespace(window uintptr, namespace string) {
	xSetNamespace(window, namespace)
}

func registerFunc(lib uintptr, fptr any, name string) {
	sym, err := purego.Dlsym(lib, name)
	if err != nil {
		panic(err)
	}
	purego.RegisterFunc(fptr, sym)
}

// findLibrary locates the gtk4-layer-shell library.
// It checks PUREGOTK_LIB_FOLDER first (for NixOS compatibility),
// then falls back to dlopen's default search path.
func findLibrary() string {
	const libName = "libgtk4-layer-shell.so.0"

	// Check PUREGOTK_LIB_FOLDER (used by NixOS wrapper)
	if folder := os.Getenv("PUREGOTK_LIB_FOLDER"); folder != "" {
		path := filepath.Join(folder, libName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Fall back to just the library name; dlopen will search standard paths
	return libName
}

func init() {
	lib, err := purego.Dlopen(findLibrary(), purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}

	// Register C functions
	registerFunc(lib, &xIsSupported, "gtk_layer_is_supported")
	registerFunc(lib, &xInitForWindow, "gtk_layer_init_for_window")
	registerFunc(lib, &xSetLayer, "gtk_layer_set_layer")
	registerFunc(lib, &xSetAnchor, "gtk_layer_set_anchor")
	registerFunc(lib, &xSetMargin, "gtk_layer_set_margin")
	registerFunc(lib, &xSetKeyboardMode, "gtk_layer_set_keyboard_mode")
	registerFunc(lib, &xSetNamespace, "gtk_layer_set_namespace")
}

//go:build !nogtk && cgo

// Package gtk4layershell provides puregotk bindings for gtk4-layer-shell.
// This is a minimal hand-written binding for the functions CalBar uses.
package gtk4layershell

import (
	"github.com/jwijenbergh/purego"
	"github.com/jwijenbergh/puregotk/pkg/core"
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

func init() {
	// Register library metadata
	core.SetPackageName("GTK4LAYERSHELL", "gtk4-layer-shell-0")
	core.SetSharedLibraries("GTK4LAYERSHELL", []string{"libgtk4-layer-shell.so.0"})

	// Load the shared library
	var libs []uintptr
	for _, libPath := range core.GetPaths("GTK4LAYERSHELL") {
		lib, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		libs = append(libs, lib)
	}

	// Register C functions
	core.PuregoSafeRegister(&xIsSupported, libs, "gtk_layer_is_supported")
	core.PuregoSafeRegister(&xInitForWindow, libs, "gtk_layer_init_for_window")
	core.PuregoSafeRegister(&xSetLayer, libs, "gtk_layer_set_layer")
	core.PuregoSafeRegister(&xSetAnchor, libs, "gtk_layer_set_anchor")
	core.PuregoSafeRegister(&xSetMargin, libs, "gtk_layer_set_margin")
	core.PuregoSafeRegister(&xSetKeyboardMode, libs, "gtk_layer_set_keyboard_mode")
	core.PuregoSafeRegister(&xSetNamespace, libs, "gtk_layer_set_namespace")
}

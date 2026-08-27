//go:build !windows

package ui

// screen_other.go — non-Windows fallback for the v1.0.5 screen-fit logic
// (dev/test boxes): no screen metrics available, the caller keeps its
// default window size.

// screenDevicePixels returns the primary screen size in device pixels.
func screenDevicePixels() (w, h float32, ok bool) {
	return 0, 0, false
}

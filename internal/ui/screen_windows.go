//go:build windows

package ui

// screen_windows.go — v1.0.5: primary-screen size in device pixels via the
// Win32 API. Used together with the Fyne canvas scale (which, since the exe
// now ships a PerMonitorV2 DPI-awareness manifest, reflects the REAL display
// scale) to compute how much room the window actually has in logical pixels
// — so the initial window never opens larger than the screen it is on.

import (
	"syscall"
	"unsafe"
)

var (
	user32DLL         = syscall.NewLazyDLL("user32.dll")
	procGetSystemMetr = user32DLL.NewProc("GetSystemMetrics")
)

// screenDevicePixels returns the primary screen size in device pixels.
func screenDevicePixels() (w, h float32, ok bool) {
	if procGetSystemMetr.Find() != nil {
		return 0, 0, false
	}
	cx, _, _ := procGetSystemMetr.Call(0) // SM_CXSCREEN
	cy, _, _ := procGetSystemMetr.Call(1) // SM_CYSCREEN
	if cx == 0 || cy == 0 || cx > 65535 || cy > 65535 {
		return 0, 0, false
	}
	return float32(cx), float32(cy), true
}

// keep unsafe imported for the syscall pattern above ( uintptr conversions ).
var _ = unsafe.Sizeof(0)

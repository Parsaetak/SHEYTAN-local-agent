//go:build headless

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// iconSheet renders every icon in the library on one canvas for inspection.
func iconSheet() fyne.CanvasObject {
	names := []string{
		"chat", "data", "memory", "logs", "sessions", "settings", "send",
		"stop", "new", "search", "browser", "git", "shell", "files",
		"provider", "model", "sandbox", "system", "license", "refresh",
		"export", "folder", "agent", "user", "warn", "info", "tools",
		"context", "spark",
	}
	grid := container.NewGridWithColumns(10)
	for _, n := range names {
		img := canvas.NewImageFromResource(icon(n))
		img.SetMinSize(fyne.NewSize(64, 64))
		grid.Add(container.NewPadded(img))
	}
	logo := canvas.NewImageFromResource(Logo)
	logo.SetMinSize(fyne.NewSize(128, 128))
	return container.NewVBox(
		logo,
		grid,
	)
}

// containerStack wraps obj in a stack (helper for screenshot composition).
func containerStack(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewStack(obj)
}

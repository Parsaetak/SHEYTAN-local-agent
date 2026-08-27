//go:build headless

package ui

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestListRenderRecipe(t *testing.T) {
	a := test.NewApp()
	a.Settings().SetTheme(Theme())
	_ = a

	messages := []string{"alpha message", "beta message", "gamma message"}
	list := widget.NewList(
		func() int { return len(messages) },
		func() fyne.CanvasObject { return widget.NewLabel("template") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(messages[id])
		},
	)
	entry := widget.NewEntry()
	entry.SetPlaceHolder("placeholder here")

	c := software.NewCanvas()
	c.SetContent(list)
	c.Resize(fyne.NewSize(800, 600))

	// Variant A: plain Capture
	a1 := c.Capture()
	f1, _ := os.Create("shots/recipe-a.png")
	_ = png.Encode(f1, a1)
	f1.Close()

	// Variant B: capture after re-resize (forces relayout)
	c.Resize(fyne.NewSize(801, 600))
	a2 := c.Capture()
	f2, _ := os.Create("shots/recipe-b.png")
	_ = png.Encode(f2, a2)
	f2.Close()
}

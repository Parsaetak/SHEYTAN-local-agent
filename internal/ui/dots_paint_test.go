//go:build headless

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestDotsPaint(t *testing.T) {
	_ = newTestApp()
	td := newTypingDots()

	caption := widget.NewLabel("Calling tool: something that is fairly long to fill the strip width")
	caption.Truncation = fyne.TextTruncateClip
	abort := widget.NewButtonWithIcon("Abort", nil, func() {})
	abort.Importance = widget.DangerImportance

	strip := container.NewBorder(nil, nil, container.NewHBox(td.obj), abort, caption)
	section := container.NewVBox(strip)
	section.Hide()

	root := container.NewBorder(nil, section, nil, nil, widget.NewLabel("chat area"))
	c := software.NewCanvas()
	c.SetContent(root)
	c.Resize(fyne.NewSize(700, 300))
	w := test.NewWindow(root)
	w.Resize(fyne.NewSize(700, 300))
	_ = c.Capture()

	section.Show()
	section.Refresh()

	img := c.Capture()
	found := 0
	for y := 0; y < 300; y++ {
		for x := 0; x < 700; x++ {
			r, g, _, _ := img.At(x, y).RGBA()
			if r>>8 > 180 && (g>>8) > 30 && (g>>8) < 140 {
				found++
			}
		}
	}
	t.Logf("ember pixels after show: %d", found)
	t.Logf("dots obj size: %v", td.obj.Size())
	if found < 60 {
		t.Errorf("dots did not paint: only %d ember pixels", found)
	}
}

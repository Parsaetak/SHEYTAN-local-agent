//go:build headless

package ui

import (
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"github.com/sheytan/local-agent/internal/agent"
)

// dumpTree walks a canvas object tree printing widget positions/sizes.
func dumpTree(o fyne.CanvasObject, depth int, out *[]string) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}
	pos := o.Position()
	size := o.Size()
	vis := o.Visible()
	name := fmt.Sprintf("%T", o)
	if txt, ok := o.(interface{ Text() string }); ok {
		name += fmt.Sprintf(" %q", txt.Text())
	}
	*out = append(*out, fmt.Sprintf("%s%s pos=(%.0f,%.0f) size=%.0fx%.0f vis=%v", indent, name, pos.X, pos.Y, size.Width, size.Height, vis))
	if c, ok := o.(*fyne.Container); ok {
		for _, ch := range c.Objects {
			dumpTree(ch, depth+1, out)
		}
	}
}

// TestStripTreeDump renders the running state and dumps the widget tree of
// the activity strip so element positions/visibility can be verified.
func TestStripTreeDump(t *testing.T) {
	d := newScreenshotApp(t)
	content := d.buildRoot()
	d.renderActive()

	// Replicate the screenshot-test flow: switch views away and back, seed
	// activities, then flip to running.
	d.showView("data")
	d.showView("chat")
	d.appendActivity(seedAct("Tool files done (84ms): wrote 482 bytes to sales.csv"))

	c := software.NewCanvas()
	c.SetContent(content)
	c.Resize(fyne.NewSize(1340, 840))
	d.win = test.NewWindow(content)
	d.win.Resize(fyne.NewSize(1340, 840))
	_ = c.Capture()

	d.setRunning(true, "Calling tool: dataAnalysis({\"action\":\"stats\"})")
	_ = c.Capture() // force a paint pass so layouts settle

	var out []string
	dumpTree(d.activitySection, 0, &out)
	for _, l := range out {
		t.Log(l)
	}

	// Assertions: the strip must have real size, and the Abort button must
	// be laid out inside it (this is the regression where Show() without a
	// parent Refresh left the whole section at 0x0 and the strip invisible).
	ss := d.activitySection.Size()
	if ss.Width < 100 || ss.Height < 30 {
		t.Errorf("activity section not laid out: %.0fx%.0f", ss.Width, ss.Height)
	}
	ab := d.abortBtn.Position()
	asz := d.abortBtn.Size()
	if !d.abortBtn.Visible() || asz.Width < 30 || asz.Height < 20 {
		t.Errorf("abort button not visible/laid out: vis=%v size %.0fx%.0f at (%.0f,%.0f)",
			d.abortBtn.Visible(), asz.Width, asz.Height, ab.X, ab.Y)
	}
	// Abort must sit at the right end of the strip, not over the caption.
	// v1.0.0: both live in padded sub-containers, so compare absolute
	// canvas coordinates.
	capAbs := fyne.CurrentApp().Driver().AbsolutePositionForObject(d.activityLbl)
	csz := d.activityLbl.Size()
	abAbs := fyne.CurrentApp().Driver().AbsolutePositionForObject(d.abortBtn)
	if capAbs.X+csz.Width > abAbs.X+4 {
		t.Errorf("caption (right edge %.0f) overlaps abort button (left edge %.0f)", capAbs.X+csz.Width, abAbs.X)
	}
	// The button must be inside the strip's width.
	if ab.X+asz.Width > ss.Width+2 {
		t.Errorf("abort button escapes the strip: right %.0f > strip width %.0f", ab.X+asz.Width, ss.Width)
	}
}

func seedAct(caption string) agent.Activity {
	return agent.Activity{Type: "tool_end", Caption: caption, Timestamp: time.Now()}
}

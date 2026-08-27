//go:build headless

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"

	"github.com/sheytan/local-agent/internal/llm"
)

func TestDebugChatState(t *testing.T) {
	d := newScreenshotApp(t)

	s := d.store.Create()
	s.Title = "Debug session"
	s.Messages = []llm.Message{
		{Role: "user", Content: "Hello agent, please analyze data"},
		{Role: "assistant", Content: "**Done.** Here is the revenue breakdown."},
	}
	_ = d.store.Save(s)

	d.sessions, _ = d.store.List()
	d.active = d.sessions[0]

	t.Logf("active=%q msgs=%d sessions=%d", d.active.Title, len(d.active.Messages), len(d.sessions))

	content := d.buildRoot()
	d.renderActive()

	t.Logf("after renderActive: chatBox children=%d", len(d.chatBox.Objects))

	c := software.NewCanvas()
	c.SetContent(content)
	c.Resize(fyne.NewSize(1340, 840))
	_ = c.Capture()

	t.Logf("after capture: chatBox children=%d", len(d.chatBox.Objects))
}

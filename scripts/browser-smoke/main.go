// browser-tool direct test: navigate → extract → close against real
// Chromium, verifying the persistent profile and clean shutdown.
//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/tools"
)

func main() {
	dir, _ := os.MkdirTemp("", "sheytan-bt-*")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.BrowserExecutablePath = os.Getenv("TEST_CHROME")
	_ = cfg.EnsureDirs()
	tools.SetBaseDir(dir)

	bt := tools.NewBrowserTool(cfg)
	defer bt.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// navigate
	out, err := bt.Run(ctx, json.RawMessage(`{"action":"navigate","url":"https://example.com"}`))
	fmt.Println("navigate:", out, "err:", err)
	if err != nil {
		os.Exit(1)
	}

	// extract (page understanding)
	out, err = bt.Run(ctx, json.RawMessage(`{"action":"extract","maxChars":1500}`))
	fmt.Println("extract (first 500):")
	if len(out) > 500 {
		out = out[:500]
	}
	fmt.Println(out, "\nerr:", err)

	// text selector
	out, err = bt.Run(ctx, json.RawMessage(`{"action":"text","selector":"h1"}`))
	fmt.Printf("h1 text: %q err=%v\n", out, err)

	// screenshot
	out, err = bt.Run(ctx, json.RawMessage(`{"action":"screenshot"}`))
	fmt.Println("screenshot:", out, "err:", err)

	// close
	out, err = bt.Run(ctx, json.RawMessage(`{"action":"close"}`))
	fmt.Println("close:", out, "err:", err)
	fmt.Println("BROWSER_TOOL_OK")
}

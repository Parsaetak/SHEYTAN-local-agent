package cmd

import (
	"fmt"
	"net/http"
	"os"
	"runtime"

	"github.com/sheytan/local-agent/internal/api"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/proc"
)

// Serve starts the HTTP server and (optionally) opens the browser.
func Serve(cfg *config.Config, args []string) int {
	// Parse simple flags
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--port":
			if i+1 < len(args) {
				var p int
				_, _ = fmt.Sscanf(args[i+1], "%d", &p)
				if p > 0 {
					cfg.Port = p
				}
				i++
			}
		case "--host":
			if i+1 < len(args) {
				cfg.Host = args[i+1]
				i++
			}
		case "--no-browser":
			// Skip auto-open
		}
	}

	srv, err := api.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "init server:", err)
		return 1
	}
	if err := srv.EnsureSetup(); err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		return 1
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	url := fmt.Sprintf("http://%s/", addr)
	if cfg.Host == "0.0.0.0" || cfg.Host == "127.0.0.1" || cfg.Host == "localhost" {
		url = fmt.Sprintf("http://localhost:%d/", cfg.Port)
	}

	fmt.Printf("%s v%s\n", config.AppName, config.AppVersion)
	fmt.Printf("  server: %s\n", url)
	fmt.Printf("  models: %s\n", cfg.ModelsDir)
	fmt.Printf("  sessions: %s\n", cfg.SessionsDir)
	fmt.Printf("  data:    %s\n", cfg.DataDir)
	fmt.Println()
	fmt.Println("Open the URL above in your browser. Ctrl-C to stop.")
	fmt.Println()

	go openBrowser(url)

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		return 1
	}
	return 0
}

func openBrowser(url string) {
	// v1.0.4: hidden-console launch via internal/proc.
	switch runtime.GOOS {
	case "darwin":
		_ = proc.Command("open", url).Start()
	case "windows":
		_ = proc.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		_ = proc.Command("xdg-open", url).Start()
	}
}

package desktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/api"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/web"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultWidth     = 1440
	defaultHeight    = 900
	minimumWidth     = 1024
	minimumHeight    = 680
	serverStopPeriod = 10 * time.Second
)

// Run starts the SHEYTAN native desktop application and blocks until the
// desktop application exits.
//
// The React production assets are served from the embedded web.StaticFS by
// Wails. The Go API remains available locally on cfg.Host/cfg.Port, while the
// UI itself does not require a browser tab or a separately running frontend
// server.
func Run(cfg *config.Config) int {
	if cfg == nil {
		fmt.Println("desktop: missing configuration")
		return 1
	}

	srv, err := api.New(cfg)
	if err != nil {
		fmt.Println("desktop: initialize server:", err)
		return 1
	}
	defer srv.Close()

	if err := srv.EnsureSetup(); err != nil {
		fmt.Println("desktop: setup:", err)
		return 1
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("desktop: listen on %s: %v\n", addr, err)
		return 1
	}

	httpServer := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)

	go func() {
		serveErr <- httpServer.Serve(listener)
	}()

	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		fmt.Println("desktop: embedded frontend:", err)
		_ = listener.Close()
		return 1
	}

	app := application.New(application.Options{
		Name:        config.AppName,
		Description: "SHEYTAN Local Agent",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(staticFS),
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main-window",
		Title:            config.AppName,
		Width:            defaultWidth,
		Height:           defaultHeight,
		MinWidth:         minimumWidth,
		MinHeight:        minimumHeight,
		BackgroundColour: application.NewRGB(18, 18, 20),
		URL:              "/",
		Hidden:           false,
		DisableResize:    false,
	})

	window.Center()
	window.Show()

	runErr := app.Run()

	shutdownErr := shutdownHTTPServer(httpServer)
	if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
		fmt.Println("desktop: HTTP shutdown:", shutdownErr)
	}

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Println("desktop: HTTP server:", err)
			return 1
		}
	default:
	}

	if runErr != nil {
		fmt.Println("desktop: application:", runErr)
		return 1
	}

	return 0
}

func shutdownHTTPServer(server *http.Server) error {
	if server == nil {
		return nil
	}

	forceClose := time.AfterFunc(serverStopPeriod, func() {
		_ = server.Close()
	})
	defer forceClose.Stop()

	return server.Shutdown(context.Background())
}

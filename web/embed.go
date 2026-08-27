// Package web holds the embedded static UI assets (HTML/CSS/JS) that ship
// inside the SHEYTAN binary.
package web

import "embed"

// StaticFS is the embedded contents of the /static directory.
//
//go:embed all:static
var StaticFS embed.FS

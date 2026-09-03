// gen-license: regenerates LICENSE from internal/brand so the shipped file
// can never drift from what the app displays.
//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/brand"
)

func main() {
	if err := os.WriteFile("LICENSE", []byte(brand.LicenseText+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("LICENSE regenerated from brand.LicenseText (" + brand.LicenseName + ")")
}

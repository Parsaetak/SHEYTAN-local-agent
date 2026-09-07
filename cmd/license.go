package cmd

import (
	"fmt"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/brand"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
)

// License prints the SHEYTAN™ trademark + full license text.
func License(cfg *config.Config) int {
	fmt.Printf("%s — %s\n", brand.Trademark, brand.FullName)
	fmt.Println(brand.Copyright())
	fmt.Println(brand.TrademarkNotice)
	fmt.Print("\n")
	fmt.Print(brand.LicenseText)
	return 0
}

//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sheytan/local-agent/internal/tools"
)

func main() {
	for _, q := range []string{"Go 1.26 release", "example.com purpose"} {
		out, err := tools.WebSearch{}.Run(context.Background(), json.RawMessage(`{"query":"`+q+`"}`))
		if err != nil {
			fmt.Println(q, "ERR:", err)
			continue
		}
		if len(out) > 300 {
			out = out[:300]
		}
		fmt.Printf("=== %s ===\n%s\n\n", q, out)
	}
}

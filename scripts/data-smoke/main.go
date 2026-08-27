// data-tool smoke test: exercises every dataAnalysis action against a real
// dataset, verifying outputs and the SVG chart renderer end-to-end.
//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/tools"
)

func run(t *tools.DataTool, args string) {
	out, err := t.Run(context.Background(), json.RawMessage(args))
	fmt.Printf("\n=== %s ===\n", args)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	if len(out) > 700 {
		out = out[:700] + "\n…"
	}
	fmt.Println(out)
}

func main() {
	dir, _ := os.MkdirTemp("", "sheytan-data-*")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	_ = cfg.EnsureDirs()
	tools.SetBaseDir(dir)

	csv := `region,product,units,price,revenue,date
Tehran,Laptop,12,900,10800,2026-01-15
Tehran,Phone,30,500,15000,2026-02-03
Mashhad,Laptop,7,890,6230,2026-01-22
Isfahan,Tablet,15,300,4500,2026-03-10
Isfahan,Phone,22,510,11220,2026-02-28
Shiraz,Laptop,4,920,3680,2026-01-09
Shiraz,Tablet,9,290,2610,2026-03-19
Tabriz,Phone,18,495,8910,2026-02-11
Tabriz,Tablet,,310,,2026-03-02
Karaj,Laptop,10,880,8800,2026-01-30
`
	if err := os.WriteFile(filepath.Join(dir, "sales.csv"), []byte(csv), 0o644); err != nil {
		panic(err)
	}

	t := tools.NewDataTool(cfg)
	run(t, `{"action":"profile","path":"sales.csv"}`)
	run(t, `{"action":"stats","path":"sales.csv"}`)
	run(t, `{"action":"correlation","path":"sales.csv"}`)
	run(t, `{"action":"groupby","path":"sales.csv","by":"region","column":"revenue","agg":"sum"}`)
	run(t, `{"action":"filter","path":"sales.csv","column":"revenue","op":">","value":"8000"}`)
	run(t, `{"action":"sort","path":"sales.csv","column":"revenue","desc":true,"limit":5}`)
	run(t, `{"action":"histogram","path":"sales.csv","column":"revenue","bins":5}`)
	run(t, `{"action":"missing","path":"sales.csv"}`)
	run(t, `{"action":"chart","path":"sales.csv","chart":"bar","labelCol":"region","valueCol":"revenue","name":"rev-by-region"}`)
	run(t, `{"action":"chart","path":"sales.csv","chart":"line","labelCol":"date","valueCol":"units","name":"units-over-time"}`)
	run(t, `{"action":"chart","path":"sales.csv","chart":"scatter","column":"price","column2":"revenue","name":"price-vs-revenue"}`)
	run(t, `{"action":"chart","path":"sales.csv","chart":"pie","labelCol":"product","valueCol":"revenue","name":"rev-by-product"}`)
	run(t, `{"action":"convert","path":"sales.csv","format":"json"}`)

	entries, _ := os.ReadDir(cfg.ChartsDir())
	fmt.Printf("\n=== charts in %s ===\n", cfg.ChartsDir())
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(cfg.ChartsDir(), e.Name()))
		s := string(data)
		valid := len(data) > 300 && strings.HasPrefix(s, "<svg") && strings.HasSuffix(strings.TrimSpace(s), "</svg>")
		fmt.Printf("%-28s %6d bytes  valid=%v\n", e.Name(), len(data), valid)
	}

	jdata, _ := os.ReadFile(filepath.Join(dir, "sales.json"))
	var objs []map[string]any
	if err := json.Unmarshal(jdata, &objs); err != nil {
		fmt.Println("JSON CONVERT FAILED:", err)
	} else {
		fmt.Printf("\nconvert → sales.json: %d records OK\n", len(objs))
	}
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/gtkit/go-rag-agent/internal/baselineassets"
)

func main() {
	outDir := flag.String("out", filepath.Join("docs", "baselines", "sdk-positioning"), "baseline output directory")
	flag.Parse()

	if err := baselineassets.GenerateSDKPositioningBaseline(context.Background(), *outDir); err != nil {
		log.Fatalf("generate sdk positioning baseline: %v", err)
	}

	fmt.Println(filepath.Join(*outDir, "eval-report.json"))
	fmt.Println(filepath.Join(*outDir, "trace-summary.json"))
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/gtkit/go-rag-agent/internal/baselineassets"
)

var generateSDKPositioningBaseline = baselineassets.GenerateSDKPositioningBaseline

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatalf("generate sdk positioning baseline: %v", err)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("generate-sdk-positioning-baseline", flag.ContinueOnError)
	flags.SetOutput(stderr)
	outDir := flags.String("out", filepath.Join("docs", "baselines", "sdk-positioning"), "baseline output directory")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	if err := generateSDKPositioningBaseline(ctx, *outDir); err != nil {
		return fmt.Errorf("generate sdk positioning baseline: %w", err)
	}

	if _, err := fmt.Fprintln(stdout, filepath.Join(*outDir, "eval-report.json")); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if _, err := fmt.Fprintln(stdout, filepath.Join(*outDir, "trace-summary.json")); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

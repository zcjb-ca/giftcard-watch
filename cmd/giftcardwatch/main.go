package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/zcjb-ca/giftcard-watch/internal/app"
	"github.com/zcjb-ca/giftcard-watch/internal/config"
	"github.com/zcjb-ca/giftcard-watch/internal/report"
	"github.com/zcjb-ca/giftcard-watch/internal/state"
)

func main() {
	var (
		configPath = flag.String("config", "config/config.example.json", "path to JSON configuration")
		jsonPath   = flag.String("output", "data/result.json", "path for JSON report")
		reportPath = flag.String("report", "data/report.md", "path for Markdown report")
		statePath  = flag.String("state", ".giftcard-watch/state.json", "path for deduplication state")
		rawDir     = flag.String("raw-dir", "", "optional directory for raw API snapshots")
		noOCR      = flag.Bool("no-ocr", false, "disable OCR for a diagnostic structured-data-only run")
		strict     = flag.Bool("strict", false, "exit with status 2 when coverage is incomplete")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	if *noOCR {
		cfg.OCR.Enabled = false
		cfg.OCR.Required = false
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	result := app.Run(ctx, cfg, app.Options{RawDirectory: *rawDir})
	if err := state.Apply(&result, *statePath); err != nil {
		result.Complete = false
		result.Errors = append(result.Errors, err.Error())
	}
	if err := report.Write(result, *jsonPath, *reportPath); err != nil {
		fatal(err)
	}

	fmt.Print(report.Markdown(result))
	if *strict && !result.Complete {
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "giftcard-watch:", err)
	os.Exit(1)
}

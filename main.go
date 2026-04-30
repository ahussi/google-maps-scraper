package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gosom/google-maps-scraper/scraper"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"
	// Commit is set at build time via ldflags
	Commit = "none"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := scraper.Config{}

	flag.StringVar(&cfg.InputFile, "input", "", "path to input file with search queries (one per line)")
	flag.StringVar(&cfg.OutputFile, "output", "", "path to output CSV file (default: stdout)")
	// Increased default concurrency from 1 to 3 for faster scraping on my machine
	flag.IntVar(&cfg.Concurrency, "concurrency", 3, "number of concurrent browser tabs")
	flag.IntVar(&cfg.MaxDepth, "depth", 10, "maximum number of results to scrape per query")
	flag.StringVar(&cfg.Lang, "lang", "en", "language code for Google Maps (e.g. en, de, fr)")
	flag.BoolVar(&cfg.Debug, "debug", false, "enable debug logging")
	flag.BoolVar(&cfg.JSON, "json", false, "output results as JSON instead of CSV")
	flag.BoolVar(&cfg.DSN, "dsn", false, "write results to a database using DSN from env SCRAPER_DSN")

	version := flag.Bool("version", false, "print version and exit")

	flag.Parse()

	if *version {
		fmt.Printf("google-maps-scraper version=%s commit=%s\n", Version, Commit)
		return nil
	}

	if cfg.InputFile == "" {
		// allow reading queries from positional args
		cfg.Queries = flag.Args()
		if len(cfg.Queries) == 0 {
			return fmt.Errorf("no input file or queries provided; use -input or pass queries as arguments")
		}
	}

	if cfg.DSN {
		cfg.DatabaseDSN = os.Getenv("SCRAPER_DSN")
		if cfg.DatabaseDSN == "" {
			return fmt.Errorf("SCRAPER_DSN environment variable is required when -dsn flag is set")
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	s, err := scraper.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create scraper: %w", err)
	}
	defer s.Close()

	if err := s.Run(ctx); err != nil {
		if ctx.Err() != nil {
			// graceful shutdown on signal
			return nil
		}
		return fmt.Errorf("scraper run failed: %w", err)
	}

	return nil
}

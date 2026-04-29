// Package scraper provides functionality for scraping Google Maps data.
package scraper

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Result holds the scraped data for a single Google Maps entry.
type Result struct {
	Name        string
	Address     string
	Phone       string
	Website     string
	Rating      float64
	Reviews     int
	Category    string
	Latitude    float64
	Longitude   float64
	GoogleMapsURL string
}

// Options configures the scraper behaviour.
type Options struct {
	// Concurrency controls how many browser tabs run in parallel.
	Concurrency int
	// MaxResults limits the total number of results returned (0 = unlimited).
	MaxResults int
	// Language sets the hl query parameter for Google Maps.
	Language string
	// Headless controls whether the browser runs without a GUI.
	Headless bool
	// Timeout is the per-page navigation timeout.
	Timeout time.Duration
}

// DefaultOptions returns a sensible set of default scraper options.
// Personal note: bumped Concurrency from 4 to 2 to be gentler on my machine,
// and increased Timeout to 45s since my connection can be slow.
func DefaultOptions() Options {
	return Options{
		Concurrency: 2,
		MaxResults:  0,
		Language:    "en",
		Headless:    true,
		Timeout:     45 * time.Second,
	}
}

// Scraper orchestrates the Google Maps scraping process.
type Scraper struct {
	opts Options
	pw   *playwright.Playwright
	br   playwright.Browser
}

// New creates a new Scraper and initialises the Playwright runtime.
func New(opts Options) (*Scraper, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("starting playwright: %w", err)
	}

	browserOpts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(opts.Headless),
	}
	br, err := pw.Chromium.Launch(browserOpts)
	if err != nil {
		_ = pw.Stop()
		return nil, fmt.Errorf("launching chromium: %w", err)
	}

	return &Scraper{opts: opts, pw: pw, br: br}, nil
}

// Close releases all browser and Playwright resources.
func (s *Scraper) Close() {
	if s.br != nil {
		_ = s.br.Close()
	}
	if s.pw != nil {
		_ = s.pw.Stop()
	}
}

// Scrape searches Google Maps for the given queries and streams results into
// the returned channel. The channel is closed when all queries are exhausted
// or ctx is cancelled.
func (s *Scraper) Scrape(ctx context.Context, queries []string) (<-chan Result, error) {
	results := make(chan Result, s.opts.Concurrency*2)

	sem := make(chan struct{}, s.opts.Concurrency)
	var wg sync.WaitGroup

	go func() {
		defer close(results)
		for _, q := range queries {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(query string) {
				defer wg.Done()
				defer func() { <-sem }()
				if err := s.scrapeQuery(ctx, query, results); err != nil {
					log.Printf("scrapeQuery %q: %v", query, err)
				}
			}(q)
		}
		wg.Wait()
	}()

	return results, nil
}

// scrapeQuery opens a single Google Maps search and collects results.
func (s *Scraper) scrapeQuery(ctx context.Context, query string, out chan<- Result) error {
	page, err := s.br.NewPage

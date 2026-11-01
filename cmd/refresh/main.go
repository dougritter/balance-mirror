package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"balance-mirror/scraper"
	"balance-mirror/server"
)

// refresh scrapes the requested Linktree pages and MERGES the results into
// links.json, preserving pages it did not scrape. It is meant to run from a
// trusted IP (e.g. GitHub Actions with SCRAPERAPI_KEY set), not from the Fly
// server, whose datacenter IP Linktree blocks.
//
// Usage:
//
//	refresh                       # scrape all allowed pages
//	refresh balanceclassturma06   # scrape only this page, keep the rest
//	refresh -o links.json id1 id2 # scrape id1 and id2
type cache struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Pages       map[string][]scraper.Link `json:"pages"`
}

func main() {
	out := flag.String("o", "links.json", "output file path")
	flag.Parse()

	// Positional args are the page IDs to refresh. None == all.
	ids := flag.Args()
	if len(ids) == 0 {
		for id := range server.AllowedIDs {
			ids = append(ids, id)
		}
	}

	// Load existing cache so we only update the requested pages (merge).
	result := loadExisting(*out)
	result.GeneratedAt = time.Now().UTC()

	failed := false
	for _, id := range ids {
		url, ok := server.AllowedIDs[id]
		if !ok {
			log.Printf("ERROR: unknown page id %q", id)
			failed = true
			continue
		}
		links, err := scraper.Scrape(url)
		if err != nil {
			log.Printf("ERROR scraping %s (%s): %v", id, url, err)
			failed = true
			continue
		}
		if len(links) == 0 {
			log.Printf("WARNING: %s returned 0 links (keeping previous value)", id)
			failed = true
			continue
		}
		log.Printf("%s: %d links", id, len(links))
		result.Pages[id] = links
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshal cache: %v", err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatalf("failed to write %s: %v", *out, err)
	}
	log.Printf("wrote %s with %d pages total", *out, len(result.Pages))

	if failed {
		// Non-zero exit so CI surfaces partial failures. The file is still
		// written, preserving previously-good pages.
		os.Exit(1)
	}
}

// loadExisting reads the current links.json so unscraped pages are preserved.
// A missing or unparseable file yields an empty cache.
func loadExisting(path string) cache {
	c := cache{Pages: map[string][]scraper.Link{}}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("no existing %s (%v); starting fresh", path, err)
		return c
	}
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("could not parse existing %s (%v); starting fresh", path, err)
		return cache{Pages: map[string][]scraper.Link{}}
	}
	if c.Pages == nil {
		c.Pages = map[string][]scraper.Link{}
	}
	return c
}

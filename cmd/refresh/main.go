package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"balance-mirror/scraper"
	"balance-mirror/server"
)

// refresh scrapes every allowed Linktree page and writes the results to
// links.json. It is meant to run from a trusted IP (e.g. GitHub Actions with
// SCRAPERAPI_KEY set), not from the Fly server, whose datacenter IP Linktree
// blocks. The server reads the committed links.json at request time.
type cache struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Pages       map[string][]scraper.Link `json:"pages"`
}

func main() {
	out := "links.json"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	result := cache{
		GeneratedAt: time.Now().UTC(),
		Pages:       map[string][]scraper.Link{},
	}

	failed := false
	for id, url := range server.AllowedIDs {
		links, err := scraper.Scrape(url)
		if err != nil {
			log.Printf("ERROR scraping %s (%s): %v", id, url, err)
			failed = true
			continue
		}
		if len(links) == 0 {
			log.Printf("WARNING: %s returned 0 links", id)
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
	if err := os.WriteFile(out, data, 0o644); err != nil {
		log.Fatalf("failed to write %s: %v", out, err)
	}
	log.Printf("wrote %s with %d pages", out, len(result.Pages))

	if failed {
		// Non-zero exit so CI surfaces partial failures, but the file is still
		// written with whatever pages succeeded.
		os.Exit(1)
	}
}

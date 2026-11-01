package main

import (
	"fmt"
	"os"

	"balance-mirror/scraper"
)

// probe reports how many links the scraper can extract from each Linktree
// page when run from the current host (e.g. a GitHub Actions runner).
// Exits non-zero if any page yields zero links, so a failing probe is obvious.
func main() {
	ids := []string{
		"balanceclassturma06",
		"balanceclassturma07",
		"balanceclassturma08",
		"corridaguiada",
	}

	failed := false
	for _, id := range ids {
		links, err := scraper.Scrape("https://linktr.ee/" + id)
		if err != nil {
			fmt.Printf("%-22s ERROR: %v\n", id, err)
			failed = true
			continue
		}
		fmt.Printf("%-22s %d links\n", id, len(links))
		if len(links) == 0 {
			failed = true
		}
	}

	if failed {
		fmt.Println("\nRESULT: BLOCKED or empty — GitHub runner cannot reliably scrape.")
		os.Exit(1)
	}
	fmt.Println("\nRESULT: OK — GitHub runner can scrape Linktree.")
}

package scraper

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type Link struct {
	Text string
	URL  string
}

func isAllowedURL(href string) bool {
	lower := strings.ToLower(href)
	return strings.Contains(lower, "zoom") ||
		strings.Contains(lower, "youtube") ||
		strings.Contains(lower, "youtu.be") ||
		strings.HasSuffix(lower, ".pdf") ||
		strings.Contains(lower, "drive.google.com") ||
		strings.Contains(lower, "bit.ly")
}

// Scrape fetches the given URL and extracts links from it.
// It specifically looks for Linktree-style buttons.
// maxAttempts is how many times fetchDocument tries before giving up.
const maxAttempts = 5

// fetchDocument performs a single browser-like GET and parses the response.
// It returns the status code alongside the document so the caller can retry
// on non-200 responses (Linktree/Fastly intermittently returns 406 to
// datacenter IPs).
func fetchDocument(url string) (*goquery.Document, int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, err
	}
	// Send a full set of browser-like headers. Sending only User-Agent is a
	// common bot signal; from datacenter IPs Linktree/Fastly returns 406 for
	// requests missing the Accept/Accept-Language/Sec-Fetch headers a real
	// browser sends.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, res.StatusCode, nil
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, res.StatusCode, err
	}
	return doc, res.StatusCode, nil
}

func Scrape(url string) ([]Link, error) {
	// Linktree/Fastly intermittently returns 406 to datacenter IPs (Fly,
	// GitHub Actions). Retry with exponential backoff; a real browser never
	// hammers the endpoint, so pacing requests gets us through.
	var doc *goquery.Document
	var lastStatus int
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var err error
		doc, lastStatus, err = fetchDocument(url)
		if err != nil {
			return nil, err
		}
		if lastStatus == 200 {
			break
		}
		log.Printf("attempt %d/%d for %s: status %d, retrying", attempt, maxAttempts, url, lastStatus)
		doc = nil
		if attempt < maxAttempts {
			// Backoff: 1s, 2s, 4s, 8s with jitter.
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			jitter := time.Duration(rand.Int63n(int64(time.Second)))
			time.Sleep(backoff + jitter)
		}
	}

	if doc == nil {
		return nil, fmt.Errorf("failed to fetch %s after %d attempts (last status %d)", url, maxAttempts, lastStatus)
	}

	var links []Link

	// Linktree buttons have a specific data-testid
	doc.Find("a[data-testid='LinkClickTriggerLink']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}

		if !isAllowedURL(href) {
			return
		}

		links = append(links, Link{
			Text: text,
			URL:  href,
		})
	})

	// Also extract links from __NEXT_DATA__ which contains embedded content (like YouTube videos)
	// that might not be rendered as simple <a> tags in the initial HTML.
	nextData := doc.Find("#__NEXT_DATA__").Text()
	if nextData != "" {
		var data interface{}
		if err := json.Unmarshal([]byte(nextData), &data); err == nil {
			extractLinksFromJSON(data, &links)
		}
	}

	return deduplicateLinks(links), nil
}

func extractLinksFromJSON(data interface{}, links *[]Link) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this object looks like a link
		url, hasUrl := v["url"].(string)
		title, hasTitle := v["title"].(string)

		if hasUrl && hasTitle && url != "" && title != "" && isAllowedURL(url) {
			*links = append(*links, Link{
				Text: strings.TrimSpace(title),
				URL:  url,
			})
		}

		// Recurse into values
		for _, val := range v {
			extractLinksFromJSON(val, links)
		}
	case []interface{}:
		// Recurse into elements
		for _, val := range v {
			extractLinksFromJSON(val, links)
		}
	}
}

func deduplicateLinks(links []Link) []Link {
	seen := make(map[string]bool)
	var unique []Link
	for _, link := range links {
		// Create a unique key based on URL (and maybe text?)
		// Linktree matches usually imply same URL = same link
		if !seen[link.URL] {
			seen[link.URL] = true
			unique = append(unique, link)
		}
	}
	return unique
}

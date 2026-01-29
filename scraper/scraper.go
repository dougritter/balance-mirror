package scraper

import (
	"log"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type Link struct {
	Text string
	URL  string
}

// Scrape fetches the given URL and extracts links from it.
// It specifically looks for Linktree-style buttons.
func Scrape(url string) ([]Link, error) {
	// Request the HTML page with a User-Agent to avoid potential blocking
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Printf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
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

		// Filter: Only include links with "zoom" in the URL (case-insensitive)
		if !strings.Contains(strings.ToLower(href), "zoom") {
			return
		}

		links = append(links, Link{
			Text: text,
			URL:  href,
		})
	})

	return links, nil
}

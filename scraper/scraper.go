package scraper

import (
	"encoding/json"
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

		// Filter: Include links with "zoom", "youtube", "youtu.be", ".pdf" or from "drive.google.com"
		lowerHref := strings.ToLower(href)
		isZoom := strings.Contains(lowerHref, "zoom")
		isYouTube := strings.Contains(lowerHref, "youtube") || strings.Contains(lowerHref, "youtu.be")
		isPDF := strings.HasSuffix(lowerHref, ".pdf") || strings.Contains(lowerHref, "drive.google.com")

		if !isZoom && !isYouTube && !isPDF {
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

		if hasUrl && hasTitle && url != "" && title != "" {
			// Apply the same filters
			lowerHref := strings.ToLower(url)
			isZoom := strings.Contains(lowerHref, "zoom")
			isYouTube := strings.Contains(lowerHref, "youtube") || strings.Contains(lowerHref, "youtu.be")
			isPDF := strings.HasSuffix(lowerHref, ".pdf") || strings.Contains(lowerHref, "drive.google.com")

			if isZoom || isYouTube || isPDF {
				*links = append(*links, Link{
					Text: strings.TrimSpace(title),
					URL:  url,
				})
			}
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

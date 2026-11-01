package server

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"balance-mirror/scraper"
)

// AllowedIDs maps the valid IDs to their corresponding Linktree URLs.
var AllowedIDs = map[string]string{
	"balanceclassturma06": "https://linktr.ee/balanceclassturma06",
	"balanceclassturma07": "https://linktr.ee/balanceclassturma07",
	"balanceclassturma08": "https://linktr.ee/balanceclassturma08",
	"corridaguiada":       "https://linktr.ee/corridaguiada",
}

type PageData struct {
	Title string
	Links []scraper.Link
}

type IndexData struct {
	Pages map[string]string
}

// linkCache mirrors the structure written by cmd/refresh into links.json.
type linkCache struct {
	GeneratedAt string                    `json:"generated_at"`
	Pages       map[string][]scraper.Link `json:"pages"`
}

// cachedLinks returns links for the given id from links.json, if present.
// The cache is populated out-of-band (GitHub Actions via a trusted proxy)
// because the Fly server's datacenter IP is blocked by Linktree.
func cachedLinks(id string) ([]scraper.Link, bool) {
	data, err := os.ReadFile("links.json")
	if err != nil {
		return nil, false
	}
	var c linkCache
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("failed to parse links.json: %v", err)
		return nil, false
	}
	links, ok := c.Pages[id]
	if !ok || len(links) == 0 {
		return nil, false
	}
	return links, true
}

func RegisterRoutes() {
	http.HandleFunc("/", handleRoot)
	// We handle all other routes in handleRoot for simplicity in standard lib,
	// or we could use specific paths. Since we have dynamic IDs at root level,
	// we need to parse the path manually if we don't use a router like chi.
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	// Serve static files if path starts with /static/
	if strings.HasPrefix(r.URL.Path, "/static/") {
		http.StripPrefix("/static/", http.FileServer(http.Dir("static"))).ServeHTTP(w, r)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")

	// If root, render index
	if path == "" {
		renderIndex(w)
		return
	}

	// Check if path matches one of our allowed IDs
	if targetURL, ok := AllowedIDs[path]; ok {
		renderLinks(w, path, targetURL)
		return
	}

	http.NotFound(w, r)
}

func renderIndex(w http.ResponseWriter) {
	tmplPath := filepath.Join("templates", "layout.html")
	indexPath := filepath.Join("templates", "index.html")

	tmpl, err := template.ParseFiles(tmplPath, indexPath)
	if err != nil {
		http.Error(w, "Could not load template", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
		return
	}

	data := IndexData{
		Pages: AllowedIDs,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

func renderLinks(w http.ResponseWriter, title, url string) {
	// Prefer the pre-scraped cache (links.json), which is refreshed out-of-band
	// from a trusted IP. Linktree blocks this server's datacenter IP, so a live
	// scrape here will usually fail; it's only a best-effort fallback.
	links, ok := cachedLinks(title)
	if !ok {
		var err error
		links, err = scraper.Scrape(url)
		if err != nil {
			http.Error(w, "Failed to load content", http.StatusInternalServerError)
			log.Printf("No cache and live scrape failed for %s: %v", url, err)
			return
		}
	}
	tmplPath := filepath.Join("templates", "layout.html")
	linksPath := filepath.Join("templates", "links.html")

	tmpl, err := template.ParseFiles(tmplPath, linksPath)
	if err != nil {
		http.Error(w, "Could not load template", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
		return
	}

	data := PageData{
		Title: title,
		Links: links,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

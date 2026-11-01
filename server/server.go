package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// defaultLinksURL points at the committed links.json on GitHub. GitHub's raw
// CDN is a trusted IP (unlike Linktree, which blocks this server), so the
// refresher (GitHub Actions, every 3h) can update links.json and the live site
// picks it up within cacheTTL — no redeploy required.
const defaultLinksURL = "https://raw.githubusercontent.com/dougritter/balance-mirror/main/links.json"

// cacheTTL is how long a fetched links.json is reused before refetching.
const cacheTTL = 5 * time.Minute

var (
	cacheMu      sync.Mutex
	cachedData   *linkCache
	cachedExpiry time.Time
)

func linksURL() string {
	if u := os.Getenv("LINKS_JSON_URL"); u != "" {
		return u
	}
	return defaultLinksURL
}

// loadCache returns the parsed links.json, fetching it from the remote URL and
// memoizing for cacheTTL. On remote failure it falls back to the baked-in
// links.json file, then to any previously cached value.
func loadCache() *linkCache {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	if cachedData != nil && time.Now().Before(cachedExpiry) {
		return cachedData
	}

	if c, err := fetchRemoteCache(linksURL()); err == nil {
		cachedData = c
		cachedExpiry = time.Now().Add(cacheTTL)
		return cachedData
	} else {
		log.Printf("remote links.json fetch failed: %v; trying baked-in file", err)
	}

	if c, err := readLocalCache("links.json"); err == nil {
		cachedData = c
		cachedExpiry = time.Now().Add(cacheTTL)
		return cachedData
	} else {
		log.Printf("local links.json read failed: %v", err)
	}

	// Last resort: stale cached value (may be nil on first boot with no file).
	return cachedData
}

func fetchRemoteCache(url string) (*linkCache, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status %d from %s", res.StatusCode, url)
	}
	var c linkCache
	if err := json.NewDecoder(res.Body).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func readLocalCache(path string) (*linkCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c linkCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// cachedLinks returns links for the given id from the links.json cache.
func cachedLinks(id string) ([]scraper.Link, bool) {
	c := loadCache()
	if c == nil {
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

# How balance-mirror works

balance-mirror is a Go web app that scrapes Linktree pages and re-renders them as a cleaner, minimal list of buttons — filtering down to only the links that matter (Zoom recordings, YouTube videos, Google Drive/PDF files, and bit.ly shortened links).

Live at: https://balance-mirror.fly.dev

---

## Project structure

```
main.go                       Entry point — starts the HTTP server
server/server.go              Routes, handlers, and links.json cache loading
scraper/scraper.go            Fetches and parses a Linktree page (optional ScraperAPI)
cmd/refresh/main.go           CLI to scrape pages and merge into links.json
links.json                    Pre-scraped link cache (served to users)
templates/                    HTML templates (layout, index, links page)
static/                       CSS
docs/                         This documentation
.github/workflows/            CI: refresh-links.yml (cron scrape), fly-deploy.yml
Dockerfile                    Container image for deployment
fly.toml                      fly.io deployment configuration
```

---

## How a request flows

> **Note:** The server no longer scrapes Linktree on every request. Linktree
> blocks the Fly datacenter IP, so links are pre-scraped out-of-band and cached
> in `links.json`. See [`caching-and-refresh.md`](./caching-and-refresh.md) for
> the full architecture, schedules, and runbook.

1. A user visits `/<page-id>` (e.g. `/corridaguiada`).
2. `server.go` checks the ID against the `AllowedIDs` map. Unknown IDs get a 404.
3. For a valid ID, the server reads the cached links for that page from
   `links.json` (fetched from the GitHub raw URL with a 5-minute TTL, with a
   baked-in file as fallback). A live Linktree scrape is only a last-resort
   fallback and generally fails from the server's datacenter IP.
4. The cached `links.json` is produced by the refresh pipeline (`cmd/refresh`
   via GitHub Actions + ScraperAPI), which extracts links from two sources:
   - HTML: `<a data-testid="LinkClickTriggerLink">` elements rendered in the page.
   - JSON: the `__NEXT_DATA__` script block embedded by Next.js, which contains the full link list even when some links aren't rendered as plain `<a>` tags.
5. Both sources apply the same URL filter (see below). Duplicates are removed.
6. The filtered links are passed to `templates/links.html` and rendered as a list of buttons.

---

## URL filter

Only links whose URL matches one of these patterns are included:

| Pattern | Matches |
|---|---|
| `zoom` | Zoom meeting/recording links |
| `youtube` / `youtu.be` | YouTube videos |
| `drive.google.com` | Google Drive files |
| `.pdf` (suffix) | Direct PDF links |
| `bit.ly` | Shortened links (e.g. Zoom recordings shared via bit.ly) |

The filter lives in `scraper.isAllowedURL()` — edit that function to add or remove patterns.

---

## Adding a new page

Open `server/server.go` and add an entry to `AllowedIDs`:

```go
var AllowedIDs = map[string]string{
    "corridaguiada": "https://linktr.ee/corridaguiada",
    "mynewpage":     "https://linktr.ee/mynewpage",   // add here
}
```

The new page will be live at `/<key>` immediately — no other changes needed.

---

## Running locally

```bash
go run main.go
# Server starts on http://localhost:8080
```

---

## Deployment

The app runs on [fly.io](https://fly.io) in the `gru` (São Paulo) region.

```bash
fly deploy        # build and deploy
fly logs          # tail live logs
fly status        # check machine health
```

Configuration is in `fly.toml`. The app auto-stops when idle and auto-starts on incoming requests (min 0 machines running).

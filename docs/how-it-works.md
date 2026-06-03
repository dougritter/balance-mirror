# How balance-mirror works

balance-mirror is a Go web app that scrapes Linktree pages and re-renders them as a cleaner, minimal list of buttons — filtering down to only the links that matter (Zoom recordings, YouTube videos, Google Drive/PDF files, and bit.ly shortened links).

Live at: https://balance-mirror.fly.dev

---

## Project structure

```
main.go               Entry point — starts the HTTP server
server/server.go      Route registration and HTTP handlers
scraper/scraper.go    Fetches and parses a Linktree page
templates/            HTML templates (layout, index, links page)
static/               CSS
docs/                 This documentation
Dockerfile            Container image for deployment
fly.toml              fly.io deployment configuration
```

---

## How a request flows

1. A user visits `/<page-id>` (e.g. `/corridaguiada`).
2. `server.go` checks the ID against the `AllowedIDs` map. Unknown IDs get a 404.
3. For a valid ID, `scraper.Scrape(linktreeURL)` is called.
4. The scraper fetches the Linktree page and extracts links from two sources:
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

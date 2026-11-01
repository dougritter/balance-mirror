# Link caching & refresh architecture

This document explains why balance-mirror no longer scrapes Linktree on every
request, how the pre-scraped cache works, the per-page refresh schedules, and a
runbook for when links stop updating.

> For the general app overview (request flow, URL filter, adding a page), see
> [`how-it-works.md`](./how-it-works.md).

---

## Why this exists — the problem we solved

Originally the server scraped `https://linktr.ee/<id>` live on every request.
This broke: the live site started rendering **empty** link lists.

**Root cause:** Linktree (behind Cloudflare/Fastly) blocks requests coming from
**datacenter IPs**. Our Fly.io server runs in the `gru` datacenter, so Linktree
returned `406 Not Acceptable` (or stripped HTML) to it, while the same requests
from a **residential IP** (a laptop on home internet) worked fine.

Things we tried and what we learned (all verified empirically):

- Adding full browser headers (`Accept`, `Accept-Language`, `Sec-Fetch-*`):
  helped marginally but did **not** reliably defeat the block from datacenter
  IPs.
- Retrying with exponential backoff: made it **worse** — repeated direct
  requests from a datacenter IP increased the block rate, not decreased it.
- GitHub Actions runners (Azure datacenter IPs) were **also** blocked (`406`).

**Conclusion:** You cannot reliably scrape Linktree from any datacenter IP. The
scrape must run through a **trusted exit IP**.

---

## How it works now

Two decoupled halves:

```
┌─ REFRESH (writes the cache) ──────────────────────────────────────────┐
│                                                                        │
│  GitHub Actions (per-page cron)                                        │
│     └─ go run ./cmd/refresh <page-id>                                  │
│           └─ scrape via ScraperAPI  (trusted residential/proxy exit)   │
│                 └─ MERGE result into links.json                        │
│                       └─ commit + push to main                         │
└────────────────────────────────────────────────────────────────────────┘

┌─ SERVE (reads the cache) ─────────────────────────────────────────────┐
│                                                                        │
│  User → /<page-id>                                                     │
│     └─ server reads links.json (NOT a live Linktree scrape)            │
│           ├─ fetch from GitHub raw URL (5-min in-memory TTL)           │
│           ├─ fallback: baked-in links.json in the image                │
│           └─ fallback: last cached value                               │
└────────────────────────────────────────────────────────────────────────┘
```

### Key properties

- **The Fly server never scrapes Linktree.** It only reads `links.json`.
- **No redeploy needed when links change.** The server fetches `links.json`
  from GitHub's raw CDN at runtime (`server.go` → `defaultLinksURL`), so a
  refresh commit is reflected on the live site within the **5-minute TTL**
  (`cacheTTL`).
- **ScraperAPI** (`SCRAPERAPI_KEY`) gives the refresher a trusted exit IP.
  Scraping is only routed through it when the env var is set; locally (from a
  residential IP) it scrapes directly.
- **Merge, not overwrite.** `cmd/refresh` updates only the page(s) it scraped
  and preserves the rest, so per-page schedules don't clobber each other.

---

## The files involved

| File | Role |
|---|---|
| `scraper/scraper.go` | `buildRequestURL()` routes through ScraperAPI when `SCRAPERAPI_KEY` is set; otherwise fetches directly. Light retry (no aggressive backoff). |
| `cmd/refresh/main.go` | CLI that scrapes requested page IDs and **merges** them into `links.json`. No args = all pages. |
| `server/server.go` | `loadCache()` fetches `links.json` from the GitHub raw URL with a TTL; falls back to the baked-in file. `cachedLinks(id)` reads a page. |
| `links.json` | The committed cache: `{ "generated_at": ..., "pages": { "<id>": [ {Text,URL}, ... ] } }`. |
| `.github/workflows/refresh-links.yml` | Per-page cron schedules + manual dispatch. |
| `.github/workflows/fly-deploy.yml` | Deploys on push to `main`, **except** commits that only touch `links.json` (`paths-ignore`). |
| `Dockerfile` | Copies `links.json` into the image as a fallback. |

---

## Refresh schedules

GitHub Actions cron is **UTC only**. Brazil (`America/Sao_Paulo`) is **UTC-3**
year-round (no DST since 2019). Times below are the Brazil-local intent and the
UTC cron actually configured.

| Page | Schedule (BRT) | UTC cron(s) |
|---|---|---|
| `balanceclassturma06` | Tue & Thu, every 30 min, 07:00–08:00 | `0,30 10 * * 2,4` + `0 11 * * 2,4` |
| `balanceclassturma08` | Tue & Thu, every 30 min, 09:00–10:00 | `0,30 12 * * 2,4` + `0 13 * * 2,4` |
| `balanceclassturma07` | Mon & Wed, every 30 min, 08:00–09:00 | `0,30 11 * * 1,3` + `0 12 * * 1,3` |
| `corridaguiada` | Daily, 06:00 & 21:00 | `0 9,0 * * *` |

Notes:

- Each 1-hour "every 30 min" window is **inclusive of the end hour** (e.g.
  07:00, 07:30, **and** 08:00 = 3 runs). That's why there are two cron lines per
  window: `0,30 HH` covers :00 and :30; `0 HH+1` covers the top of the end hour.
- The workflow maps the cron that fired to a page via `github.event.schedule`
  in the "Determine which pages to refresh" step. If none matches, it falls back
  to the `workflow_dispatch` input (empty = all pages).
- Approx. ScraperAPI usage: ~60 requests/month — comfortably inside the free
  tier (~1,000/month).

### Manual refresh

GitHub → **Actions → Refresh Links → Run workflow**:

- Leave **pages** blank → refresh **all** pages.
- Enter one or more space-separated IDs → refresh only those.

Or locally (uses your residential IP, no ScraperAPI needed):

```bash
go run ./cmd/refresh                     # all pages
go run ./cmd/refresh balanceclassturma06 # one page, merged
```

---

## Runbook: links not updating

Work top-down; each step narrows where the break is.

### 1. Is the live site serving the current cache?

```bash
curl -s https://balance-mirror.fly.dev/corridaguiada | grep -c btn-secondary
# >0 = links present. 0 or "Failed to load content" = problem.
```

### 2. Is the committed cache correct?

```bash
curl -s https://raw.githubusercontent.com/dougritter/balance-mirror/main/links.json \
  | python3 -c "import json,sys;d=json.load(sys.stdin);print(d['generated_at']);print({k:len(v) for k,v in d['pages'].items()})"
```

- **Old `generated_at` for the page you expected to update** → the refresh job
  didn't run or didn't change anything. Go to step 3.
- **File correct but live site stale** → the server's 5-min TTL hasn't expired,
  or the runtime fetch is failing. Go to step 4.

### 3. Did the refresh workflow run and succeed?

GitHub → **Actions → Refresh Links**. For the relevant run, check:

- **"Determine which pages to refresh"** logs the expected page (not `ALL`
  unexpectedly). A mismatch means the cron string didn't match a `case` branch
  in `refresh-links.yml`.
- **"Scrape via ScraperAPI"** step: look for `-> N links` per page. If you see
  `406` or `0 links`:
  - `SCRAPERAPI_KEY` secret missing/expired, or
  - ScraperAPI monthly quota exhausted, or
  - Linktree page structure changed (see step 5).
- **"Commit links.json if changed"**: "No changes" is normal if the links are
  genuinely unchanged — not a bug.
- **Schedule didn't fire at all?** GitHub scheduled workflows are often delayed
  5–15+ min and can be **skipped** under high load (especially on free tier).
  Re-run manually to confirm the job itself works.

### 4. Is the server's runtime fetch working?

```bash
fly logs   # look for: "remote links.json fetch failed" or "local links.json read failed"
```

- If remote fetch fails, the server falls back to the **baked-in** `links.json`
  from the last deploy (which can be stale). Fix connectivity to
  `raw.githubusercontent.com`, or redeploy to refresh the baked-in copy.
- The TTL is 5 minutes (`cacheTTL` in `server.go`). Wait it out, or hit the page
  again after 5 min to force a refetch.
- `LINKS_JSON_URL` env var overrides the source URL if you ever point it
  elsewhere (e.g. a fork). Check it's not set to something wrong on Fly.

### 5. Did Linktree change its page structure?

If scrapes return `0 links` even via ScraperAPI, the selectors may be stale.
Verify from a residential IP:

```bash
go run ./cmd/refresh corridaguiada   # run from a laptop on home internet
```

If this also returns 0, update the parsing in `scraper/scraper.go`
(`data-testid="LinkClickTriggerLink"` selector and/or the `__NEXT_DATA__` JSON
walk).

---

## Common gotchas

- **`chore: refresh links.json` commits do NOT deploy.** This is intentional
  (`paths-ignore: links.json` in `fly-deploy.yml`). Real code changes still
  deploy.
- **Timezone.** All crons are UTC. If Brazil ever reintroduces DST, every cron
  in `refresh-links.yml` must shift by one hour.
- **Build artifacts.** `go build ./cmd/...` drops a `refresh` (or `probe`)
  binary in the repo root. These are gitignored — don't commit them.
- **Cron precision.** GitHub cron is best-effort, not exact. A 30-min window may
  occasionally fire fewer times than expected.

---

## Possible future improvements

- **Per-page `generated_at`** in `links.json` so the runbook can tell exactly
  when each page was last refreshed (currently there's a single top-level
  timestamp).
- **Alerting** when a scheduled refresh returns `0 links` (e.g. fail the job
  loudly / notify), so structure changes are caught early.
- **Drop the baked-in fallback** once the runtime fetch is proven reliable, to
  avoid confusion about which copy is being served.
- **Move the cache off git** (Fly volume / object storage / KV) if commit noise
  from refreshes becomes annoying.

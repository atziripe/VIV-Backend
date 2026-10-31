# Load testing

`smoke.js` is a [k6](https://k6.io) script that hits a mix of real,
read-only, authenticated endpoints (`/me`, `/home/today`, `/plans/current`,
`/nutrition/plan`) to answer "how many concurrent users does this
deployment actually hold before latency or errors spike."

**Point this at dev, never at production without the team's explicit
go-ahead** — it generates real traffic and real Neon usage.

## Install k6

```bash
# macOS
brew install k6

# Linux (binary download, no package manager needed)
curl -sSL -o k6.tar.gz https://github.com/grafana/k6/releases/download/v0.54.0/k6-v0.54.0-linux-amd64.tar.gz
tar xzf k6.tar.gz
sudo mv k6-v0.54.0-linux-amd64/k6 /usr/local/bin/
```

## What you need

- A real test account's email/password in the same Firebase project your
  dev backend verifies tokens against (`FIREBASE_PROJECT_ID` in your dev
  env) — create one in Firebase Console → Authentication if you don't
  have one, or use any existing dev account.
- The Firebase **Web API key** for that project — Firebase Console →
  Project Settings → General → "Web API Key". Not secret (the mobile app
  already ships with it), safe to pass on the command line.
- Your dev deployment's public URL.

## Run it

```bash
k6 run \
  -e BASE_URL=https://your-dev-url.up.railway.app \
  -e FIREBASE_WEB_API_KEY=AIza... \
  -e TEST_USER_EMAIL=loadtest@example.com \
  -e TEST_USER_PASSWORD=whatever-it-is \
  loadtest/smoke.js
```

Default run ramps 0 → 50 concurrent virtual users over ~4 minutes (see
the `stages` in `smoke.js`). For a flat run at a fixed concurrency instead:

```bash
k6 run -e VUS=30 -e DURATION=2m -e BASE_URL=... -e FIREBASE_WEB_API_KEY=... \
  -e TEST_USER_EMAIL=... -e TEST_USER_PASSWORD=... loadtest/smoke.js
```

## Reading the result

k6 prints a summary at the end. The numbers that answer the actual
question:

- **`http_req_duration` p(95)`** — if this climbs past ~1-2s as VUs ramp
  up, that's where things start to degrade for real users.
- **`http_req_failed`** — non-zero means requests are erroring outright
  (watch for 500s specifically — Sentry will show the real errors behind
  them, tagged by `endpoint`).
- **`viv_errors`** — this script's own rate of 5xx responses specifically
  (distinct from k6's `http_req_failed`, which also counts 4xx).
- The per-endpoint `viv_*_duration` trends — which of the four tested
  endpoints degrades first, if any.

The VU count where `http_req_duration` p(95) crosses your acceptable
latency (or `viv_errors` starts climbing) is your real answer to "how
many concurrent users does this hold" — not a guess.

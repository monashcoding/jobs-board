# Deploying the scraper on Dokploy

The scraper is a **batch job**: it runs the pipeline, then exits. The container
therefore idles (`tail -f /dev/null`) and a **Dokploy Schedule** execs the
binary on a cron. This gives you a "Run now" button, per-run logs, and schedule
changes without a rebuild.

## 1. Create the application

In Dokploy: **Project → Create Service → Application**.

| Field | Value |
| --- | --- |
| Provider | GitHub → `monashcoding/mploy-golang-core` |
| Branch | `development` |
| Build type | Dockerfile |
| Dockerfile path | `Dockerfile` |

No domain and no port are needed — the scraper exposes no HTTP server, so leave
the Domains tab empty.

## 2. Environment variables

**Environment** tab. All of these are required; the app refuses to start
without them (`internal/config/config.go`).

```
MONGODB_URI=
MONGODB_DATABASE=mploy
MONGODB_COLLECTION=jobs
BRIGHTDATA_CUSTOMER_ID=
BRIGHTDATA_ZONE_NAME=
BRIGHTDATA_PASSWORD=
BRIGHTDATA_API_TOKEN=
OPENAI_API_KEY=
DISCORD_WEBHOOK_URL=
VERSION=1.0.0
```

Optional overrides (leave unset to run everything):

```
BOARD_MAX_LISTINGS=100
BOARD_MAX_PER_EMPLOYER=5
RUN_SCRAPERS=GradConnection,Prosple,LinkedIn,JaneStreet,Atlassian
RUN_PIPELINE_STEPS=WriteJSON,Normalisation,Deduplication,Synchronisation,AIParser,Upload
SCRAPER_JOB_LIMITS=LINKEDIN:10,GRADCONNECTION:5
```

`BOARD_MAX_LISTINGS` caps how many listings are marked board-eligible in one
run; the highest scoring win. It defaults to 100. Set it to 0 to remove the cap.
`BOARD_MAX_PER_EMPLOYER` caps how many of one employer's roles reach the board,
defaulting to 5, so a single large intake cannot take over the forum. It is
applied first, then the overall cap. Set it to 0 to remove it.
Which employers can reach the board at all is a separate, hand-maintained list
in `internal/utils/notable_companies.go`.

Dokploy injects these into the container environment. There is no `.env` file
in the image and none is needed — `godotenv.Load()` failing is now a warning,
not a fatal error.

## 3. Deploy, then schedule

Hit **Deploy**. A healthy deploy shows a running container with no log output;
that is expected, since it is idling until the schedule fires.

Then **Schedules → Create Schedule**:

| Field | Value |
| --- | --- |
| Type | Application / run in container |
| Command | `/app/scraper` |
| Cron | `0 21 * * *` |

Dokploy's cron runs in the **host's** timezone (normally UTC), so pick the
expression to match:

- 8am Sydney = `0 21 * * *` during AEDT (Oct–Apr), `0 22 * * *` during AEST
- The *container's* `TZ` is `Australia/Sydney`, so log timestamps read as local
  time regardless.

If you want both a morning and evening run, create two schedules.

## 4. Verify

Use the Schedule's **Run now**, or from the server:

```bash
docker exec $(docker ps -qf name=mploy) /app/scraper
```

Success looks like `[✓] Environment variables loaded and validated.` followed
by the pipeline logs, and a Discord message from the webhook.

## Notes

- **BrightData whitelisting:** `utils.WhitelistCurrentIP` whitelists the
  machine's public IP on every run. On Dokploy that is your VPS's static IP —
  an improvement over GitHub Actions, whose runner IP changed every run.
- **Debug output** is written to `/app/debug/write-json` inside the container.
  Add a Dokploy volume mounted at `/app/debug` if you want it to survive
  redeploys.
- **GitHub Actions** (`.github/workflows/run-scraper.yml`) is left intact as a
  manual `workflow_dispatch` fallback. Its schedule is still commented out, so
  the two won't double-run.

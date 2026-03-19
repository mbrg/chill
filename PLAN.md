# Chill — Implementation Plan

Reference: [ARCHITECTURE.md](./ARCHITECTURE.md) for decisions and rationale.

## Testability Approach

I (the builder) test everything from the CLI. No phone needed. No waiting for real alerts.

**What I have access to:**
- Oref endpoints (Israeli IP confirmed working: `alerts.json`, `AlertsHistory.json`, `GetDistricts.aspx`, `alertCategories.json`)
- Telegram Bot API via curl (create bot via BotFather HTTP API, send/receive via `api.telegram.org`)
- Go test runner, SQLite CLI, jq, curl

**What I do NOT have:**
- A phone to tap Telegram buttons
- Real alerts on demand

**Therefore:**
- All bot command handlers are pure functions: `(input, store) → (response, sideEffects)`. Tested in Go unit tests.
- Oref client is tested against real recorded fixtures (captured via curl at plan start).
- Full pipeline tested via `--simulate` subcommand that injects fixture data through the entire live stack.
- Telegram delivery verified via Bot API `getUpdates` curl after simulate.
- CLI admin subcommands (`chill user add`, `chill user list`) exist so I can manage test users without Telegram.

---

## Milestone 0: Capture Live Fixtures

**Goal:** Record real Oref responses as test fixtures before writing any code.

### Tasks

- [ ] **0.1 — Record alerts.json (empty / no active alert)**
  - Capture the BOM + CRLF empty response.
  - Verify: `cat testdata/alerts_empty.bin | xxd | head` shows `efbb bf0d 0a`.

- [ ] **0.2 — Record AlertsHistory.json**
  - Capture current history feed. This contains real cat 1, 13, 14 records.
  - Verify: `jq '.[0]' testdata/history.json` shows a valid record with `alertDate`, `title`, `data`, `category`.

- [ ] **0.3 — Record GetDistricts.aspx**
  - Capture full district catalog.
  - Verify: `jq 'length' testdata/districts.json` returns a number > 1000.

- [ ] **0.4 — Record alertCategories.json**
  - Capture full category list.
  - Verify: `jq '.[] | select(.id == 13)' testdata/categories.json` shows `"category": "update"`.

- [ ] **0.5 — Create synthetic alert fixtures**
  - Craft realistic `alerts.json` responses for each lifecycle event based on the real history records:
    - `testdata/alert_rocket.json` — cat 1, areas `["נתיבות", "תקומה"]`
    - `testdata/alert_prealert.json` — cat 14, areas `["אזור תעשייה צפוני אשקלון"]`
    - `testdata/alert_end.json` — cat 13, areas `["נתיבות", "תקומה"]`
    - `testdata/alert_uav.json` — cat 2, areas `["חיפה - כרמל"]`
  - Each must parse as valid JSON matching the real-time `alerts.json` schema: `{id, cat, title, data, desc}`.
  - Verify: `jq '.data' testdata/alert_rocket.json` returns the area list.

**Milestone 0 acceptance:** `ls testdata/*.json` shows at least 8 fixture files. Each is valid JSON (or documented binary for the empty response). `jq . testdata/*.json` succeeds on all JSON files.

---

## Milestone 1: Project Skeleton + Oref Client

**Goal:** A Go binary that can fetch and parse Oref alerts, tested against fixtures and live.

### Tasks

- [ ] **1.1 — Init Go module, project layout**
  - `go mod init github.com/mbrg/chill`
  - Directories: `cmd/chill/`, `internal/oref/`, `internal/store/`, `internal/bot/`, `internal/dispatch/`, `testdata/`
  - Verify: `go build ./cmd/chill && ./chill --help` shows usage and exits 0.

- [ ] **1.2 — Oref HTTP client**
  - Package `internal/oref`. Struct `Client` with `FetchAlerts(ctx) (*AlertResponse, error)`.
  - Sends required headers (`Referer`, `X-Requested-With`, `Accept`).
  - Strips UTF-8 BOM (`ef bb bf`) and trims whitespace before JSON parse.
  - Returns `nil, nil` when response is empty (no active alert) — not an error.
  - Strict JSON unmarshaling with `DisallowUnknownFields` — log warnings on unknown fields.
  - **Unit tests (fixture-based, `httptest.NewServer`):**
    - Serve `testdata/alert_rocket.json` → parse succeeds, `EventType=Alert`, areas `["נתיבות", "תקומה"]`.
    - Serve `testdata/alerts_empty.bin` → returns `nil, nil`.
    - Serve malformed JSON → returns error.
    - Serve JSON with leading BOM → strips and parses correctly.
  - Verify: `go test ./internal/oref/ -v -run TestFetchAlerts` — all pass.

- [ ] **1.3 — Alert parser + lifecycle categorization**
  - Map `cat` → `EventType` enum:
    - `1` → `Alert` (missilealert)
    - `2` → `Alert` (uav)
    - `3` → `Alert` (nonconventional)
    - `4` → `Alert` (warning)
    - `9` → `Alert` (cbrne)
    - `10` → `Alert` (terrorattack)
    - `13` → `EndOfEvent` (update)
    - `14` → `PreAlert` (flash)
    - `15-28` → `Drill` (all drill variants)
    - anything else → `Unknown`
  - Canonical `Event` struct: `{ID, Category, EventType, Title, Areas []string, Description, ReceivedAt}`.
  - **Unit tests (table-driven):** One test case per category ID above + unknown.
  - Verify: `go test ./internal/oref/ -v -run TestParseCategory` — all pass.

- [ ] **1.4 — Districts client**
  - `Client.FetchDistricts(ctx) ([]District, error)` — parse the districts catalog.
  - `District` struct: `{Label, ID, AreaID, AreaName, MigunTime}`.
  - **Unit test:** Serve `testdata/districts.json` via `httptest` → parse succeeds, length > 1000, spot-check known entries.
  - Verify: `go test ./internal/oref/ -v -run TestFetchDistricts` — all pass.

- [ ] **1.5 — Categories client**
  - `Client.FetchCategories(ctx) ([]Category, error)` — parse category metadata.
  - **Unit test:** Serve `testdata/categories.json` → parse succeeds, cat 13 = "update", cat 14 = "flash".
  - Verify: `go test ./internal/oref/ -v -run TestFetchCategories` — all pass.

- [ ] **1.6 — Live smoke test**
  - `go test ./internal/oref/ -v -run TestLive -tags=live` — integration test behind build tag.
  - Calls real Oref endpoints, asserts 200 and parseable response.
  - Verify: `go test ./internal/oref/ -v -run TestLive -tags=live` — passes from this machine.

**Milestone 1 acceptance:** `go test ./internal/oref/ -v` — all fixture-based tests pass. `go test ./internal/oref/ -v -tags=live` — live smoke test passes. `go build ./cmd/chill` succeeds.

---

## Milestone 2: SQLite Store + Dedup

**Goal:** Persist events and users, deduplicate alerts across restarts.

### Tasks

- [ ] **2.1 — SQLite schema + store package**
  - Package `internal/store`. Use `modernc.org/sqlite` (pure Go, zero CGO).
  - Create tables on open: `users`, `events`, `deliveries` (schema per ARCHITECTURE.md).
  - WAL mode enabled.
  - **Unit test:** `store.New(":memory:")` creates tables. Verify via `SELECT name FROM sqlite_master`.
  - Verify: `go test ./internal/store/ -v -run TestNew` — passes.

- [ ] **2.2 — User CRUD**
  - `store.UpsertUser(telegramID int64, area string) error`
  - `store.GetUser(telegramID int64) (*User, error)`
  - `store.DeactivateUser(telegramID int64) error`
  - `store.ActiveUsersByArea(area string) ([]User, error)` — returns users whose area matches.
  - **Unit tests:**
    - Insert user → get user → matches.
    - Upsert same telegram ID with new area → area updated.
    - Deactivate → `ActiveUsersByArea` no longer returns them.
    - Two users in same area, one deactivated → only active one returned.
  - Verify: `go test ./internal/store/ -v -run TestUser` — all pass.

- [ ] **2.3 — Event dedup**
  - `store.InsertEvent(event) (isNew bool, err error)`.
  - Dedup key: `(oref_id, category)`.
  - Same alert ID + same category = `isNew=false`.
  - Same alert ID + different category (alert → end-of-event) = `isNew=true`.
  - **Unit tests:**
    - Insert event → `isNew=true`. Insert same → `isNew=false`.
    - Insert same `oref_id`, different `category` → `isNew=true`.
  - Verify: `go test ./internal/store/ -v -run TestDedup` — all pass.

- [ ] **2.4 — Delivery log**
  - `store.LogDelivery(eventID, userID int64, status string, errMsg string) error`
  - `store.GetDeliveries(eventID int64) ([]Delivery, error)` — for verification.
  - **Unit test:** Log a delivery, retrieve it, fields match.
  - Verify: `go test ./internal/store/ -v -run TestDelivery` — all pass.

- [ ] **2.5 — CLI admin subcommands**
  - `chill user add --telegram-id=123 --area="נתיבות"` — insert user directly.
  - `chill user list` — print all users as a table.
  - `chill user remove --telegram-id=123` — deactivate.
  - `chill events list` — print recent events.
  - `chill deliveries list --event-id=X` — print deliveries for an event.
  - These exist so I can set up test state and verify results without touching Telegram.
  - Verify: `./chill user add --telegram-id=999 --area="נתיבות" && ./chill user list` — shows the user.

**Milestone 2 acceptance:** `go test ./internal/store/ -v` — all pass. `./chill user add --telegram-id=999 --area="נתיבות" && ./chill user list` prints the user. `./chill events list` works (empty is fine). SQLite file exists on disk.

---

## Milestone 3: Poller + Pipeline (no delivery)

**Goal:** A running binary that polls Oref, parses, deduplicates, and logs matched users — but doesn't send anything yet.

### Tasks

- [ ] **3.1 — Poller goroutine**
  - Calls `FetchAlerts()` every 2s. Emits parsed events on a `chan Event`.
  - Exponential backoff on HTTP errors: 2s → 4s → 8s → 16s → 32s → 60s cap. Reset on success.
  - Context-based shutdown (SIGINT/SIGTERM → cancel context → poller exits cleanly).
  - **Unit test:** Feed a mock HTTP server that returns an alert, then empty, then error, then recovery. Assert channel receives exactly the right events and backoff timing is correct.
  - Verify: `go test ./internal/oref/ -v -run TestPoller` — passes.

- [ ] **3.2 — Pipeline: poll → dedup → match → log**
  - Main loop reads from event channel.
  - For each event: insert into store (dedup). If new, query `ActiveUsersByArea` for each area. Log: `"event_type=Alert areas=[נתיבות,תקומה] matched_users=1"`.
  - No Telegram send yet — just structured log output.
  - Verify:
    ```
    ./chill user add --telegram-id=999 --area="נתיבות"
    ./chill run --dry-run
    ```
    Logs show poll results every 2s. If an alert happens to be active, logs show match.

- [ ] **3.3 — `--simulate` subcommand (no delivery)**
  - `chill simulate --fixture=testdata/alert_rocket.json`
  - Loads fixture, pushes through the full pipeline (dedup + match), logs result.
  - Verify:
    ```
    ./chill user add --telegram-id=999 --area="נתיבות"
    ./chill simulate --fixture=testdata/alert_rocket.json
    ```
    Logs: `"event_type=Alert matched_users=1 users=[999] dry_run=true"`
    Run again: `"dedup: skipped oref_id=... category=1"`

**Milestone 3 acceptance:** `./chill simulate --fixture=testdata/alert_rocket.json` logs matched users. Run twice — second time shows dedup skip. `./chill simulate --fixture=testdata/alert_end.json` with same `oref_id` → dedup allows it (different category). `./chill run --dry-run` polls live Oref and logs results. All unit tests pass.

---

## Milestone 4: Telegram Bot + Delivery

**Goal:** Users interact via Telegram. Alerts are delivered via Telegram. Testable end-to-end from CLI.

### Tasks

- [ ] **4.1 — Bot command handlers (pure functions)**
  - Package `internal/bot`. Handlers are methods on a `Bot` struct that take parsed input and return response text + store mutations. They do NOT call Telegram API directly.
  - `HandleStart(telegramID int64, chatID int64) (responseText string, err error)`
  - `HandleSetLocation(telegramID int64, chatID int64, input string) (responseText string, err error)`
  - `HandleStatus(telegramID int64) (responseText string, err error)`
  - `HandleStop(telegramID int64) (responseText string, err error)`
  - **Unit tests (no Telegram, just store):**
    - `HandleStart` → returns welcome text, user created in store.
    - `HandleSetLocation("נתיבות")` → user area updated, response confirms.
    - `HandleSetLocation("תל")` → multiple matches, response lists top 5.
    - `HandleSetLocation("xyzzy")` → no match, response says so.
    - `HandleStatus` → returns current subscription.
    - `HandleStop` → user deactivated, response confirms.
  - Verify: `go test ./internal/bot/ -v` — all pass.

- [ ] **4.2 — Fuzzy location search**
  - `bot.SearchLocation(query string) []District` — search district catalog.
  - Exact match on `label_he` or `areaname` first. Then substring. Then prefix.
  - **Unit tests:** Load districts fixture, search "נתיבות" → exact match. Search "תל" → multiple results include "תל אביב". Search "asdf" → empty.
  - Verify: `go test ./internal/bot/ -v -run TestSearch` — all pass.

- [ ] **4.3 — Message formatter**
  - Package `internal/dispatch`. Pure function: `FormatMessage(event Event) string`.
  - Formats per ARCHITECTURE.md templates (emoji prefix, title, areas, action, timestamp).
  - **Unit tests:** Format each event type (cat 1, 2, 13, 14), assert output matches expected string.
  - Verify: `go test ./internal/dispatch/ -v -run TestFormat` — all pass.

- [ ] **4.4 — Telegram sender**
  - `dispatch.TelegramSender` — sends messages via Bot API HTTP.
  - Implements interface `Sender { Send(chatID int64, text string) error }`.
  - Also: `LogSender` that writes to stdout (for dry-run mode).
  - **Unit test:** `httptest.NewServer` mocks Telegram `sendMessage` endpoint. Send message → mock receives correct chat ID and text.
  - Verify: `go test ./internal/dispatch/ -v -run TestTelegramSender` — passes.

- [ ] **4.5 — Bot update loop (Telegram long-polling)**
  - Goroutine that calls Telegram `getUpdates` with long-polling.
  - Dispatches to handlers from 4.1. Sends response text back via Telegram `sendMessage`.
  - **Unit test:** `httptest.NewServer` mocks both `getUpdates` (returns a `/start` message) and `sendMessage` (records what was sent). Assert handler was called and response sent.
  - Verify: `go test ./internal/bot/ -v -run TestUpdateLoop` — passes.

- [ ] **4.6 — Wire delivery into pipeline**
  - When `--simulate` or live poller emits a matched event, call `Sender.Send(chatID, formattedMessage)` for each matched user.
  - Log delivery to store.
  - Verify:
    ```
    ./chill user add --telegram-id=<MY_CHAT_ID> --area="נתיבות"
    CHILL_TELEGRAM_TOKEN=<token> ./chill simulate --fixture=testdata/alert_rocket.json
    ```
    Then check delivery:
    ```
    ./chill deliveries list --last=1
    ```
    Shows `status=sent`. And verify via Telegram Bot API:
    ```
    curl 'https://api.telegram.org/bot<token>/getUpdates' | jq .
    ```

- [ ] **4.7 — Full pipeline integration test**
  - Test with `httptest` mock for both Oref and Telegram.
  - Inject fixture alert → assert Telegram `sendMessage` was called with correct chat ID and formatted message body.
  - Verify: `go test ./... -v -run TestFullPipeline` — passes.

**Milestone 4 acceptance:**
1. `go test ./... -v` — all unit + integration tests pass.
2. Create a real test bot via BotFather. Get chat ID by messaging the bot and curling `getUpdates`.
3. `./chill user add --telegram-id=<chat_id> --area="נתיבות"`
4. `CHILL_TELEGRAM_TOKEN=<token> ./chill simulate --fixture=testdata/alert_rocket.json`
5. `./chill deliveries list --last=1` → shows `status=sent`.
6. `curl 'https://api.telegram.org/bot<token>/getUpdates'` → confirms message was sent to the chat.
7. Simulate full lifecycle:
   ```
   ./chill simulate --fixture=testdata/alert_prealert.json
   ./chill simulate --fixture=testdata/alert_rocket.json
   ./chill simulate --fixture=testdata/alert_end.json
   ./chill deliveries list --last=5
   ```
   Three deliveries logged, all `status=sent`.

---

## Milestone 5: Operational Hardening

**Goal:** Runs unattended. Fails loud.

### Tasks

- [ ] **5.1 — Structured logging**
  - Use `log/slog` (stdlib). JSON to stdout.
  - Fields: `component` (poller/bot/dispatch), `event_type`, `oref_id`, `areas`, `matched_users`, `error`.
  - Verify: `./chill run --dry-run 2>&1 | head -5 | jq .` — each line is valid JSON.

- [ ] **5.2 — Operator alerting**
  - Config: `CHILL_OPERATOR_CHAT_ID` env var.
  - If poller hits 5 consecutive errors, send Telegram message to operator.
  - Suppress repeat alerts: max 1 per 10 minutes.
  - **Unit test:** Mock sender, simulate 5 failures → sender called once with operator chat ID. Simulate 5 more within 10min → not called again.
  - Verify: `go test ./internal/oref/ -v -run TestOperatorAlert` — passes.

- [ ] **5.3 — Graceful backoff + recovery logging**
  - Log every backoff step: `"backoff escalated to 8s"`, and recovery: `"resumed normal polling after 32s backoff"`.
  - Verify: Run `--dry-run`, temporarily break network → logs show backoff. Restore → logs show recovery.

- [ ] **5.4 — Systemd unit file**
  - `deploy/chill.service` — `Restart=always`, `RestartSec=5`, env file.
  - `deploy/install.sh` — copies binary, installs service, enables.
  - Verify: `cat deploy/chill.service` — valid systemd unit. Manual review only (no systemd on this machine).

- [ ] **5.5 — Graceful shutdown**
  - SIGINT/SIGTERM → cancel context → poller stops → bot stops → pending deliveries flush → exit.
  - Verify: `./chill run --dry-run &; sleep 3; kill $!; wait $!` — exits 0, logs "shutting down".

**Milestone 5 acceptance:** `./chill run --dry-run` produces JSON logs. Kill it — clean shutdown. All unit tests pass. `deploy/` directory has service file and install script.

---

## Milestone 6: CI + Final Verification

**Goal:** Green CI. Documented. Deployable.

### Tasks

- [ ] **6.1 — GitHub Actions CI**
  - `.github/workflows/ci.yml`: `go test ./...` on push + PR.
  - Verify: push commit → CI runs → green check.

- [ ] **6.2 — Full simulate lifecycle test (scripted)**
  - Script `scripts/test-e2e.sh`:
    ```bash
    set -e
    DB=$(mktemp)
    ./chill --db=$DB user add --telegram-id=999 --area="נתיבות"
    ./chill --db=$DB simulate --fixture=testdata/alert_prealert.json --sender=log
    ./chill --db=$DB simulate --fixture=testdata/alert_rocket.json --sender=log
    ./chill --db=$DB simulate --fixture=testdata/alert_end.json --sender=log
    # Second run should dedup all three
    ./chill --db=$DB simulate --fixture=testdata/alert_prealert.json --sender=log 2>&1 | grep "dedup: skipped"
    ./chill --db=$DB simulate --fixture=testdata/alert_rocket.json --sender=log 2>&1 | grep "dedup: skipped"
    ./chill --db=$DB simulate --fixture=testdata/alert_end.json --sender=log 2>&1 | grep "dedup: skipped"
    # Non-matching area user
    ./chill --db=$DB user add --telegram-id=888 --area="חיפה"
    ./chill --db=$DB simulate --fixture=testdata/alert_uav.json --sender=log 2>&1 | grep "matched_users=1"
    echo "ALL PASSED"
    rm -f $DB
    ```
  - Verify: `bash scripts/test-e2e.sh` — prints "ALL PASSED".

- [ ] **6.3 — Contract test: schema drift detection**
  - `go test ./internal/oref/ -v -run TestLive -tags=live` fetches real Oref data and parses it.
  - If Oref changes the JSON schema, this test fails.
  - Verify: Run it. It passes now. If Oref changes schemas in the future, it will catch the break.

**Milestone 6 acceptance:** `go test ./...` — green. `bash scripts/test-e2e.sh` — "ALL PASSED". CI green on GitHub. `--simulate` with real bot token delivers to Telegram.

---

## Out of Scope (Explicitly Deferred)

- WhatsApp / SMS delivery
- Telegram channel ingestion as fallback source
- Daily heartbeat monitoring
- Web UI for configuration
- Multi-language alert translation
- Coordinate/GPS-based location matching
- Historical alert dashboard
- SQLite backup automation

## Order of Work

```
M0 (capture fixtures)
 └─► M1 (oref client + parser)
      └─► M2 (store + dedup + CLI admin)
           └─► M3 (poller + pipeline + simulate)
                └─► M4 (telegram bot + delivery)
                     └─► M5 (ops hardening)
                          └─► M6 (CI + e2e script)
```

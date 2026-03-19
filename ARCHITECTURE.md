# Chill — Oref Alert Notifier Architecture

## What is this

A personal alert notification service that monitors Israel Home Front Command (Oref) real-time alert endpoints and notifies subscribed users via Telegram on every alert lifecycle event (pre-alert, active alert, end-of-event), filtered by their configured location.

Serves up to 100 users. Runs unattended on a small VPS.

## Design Principles

**1. Fewer moving parts beat clever parts.**
One binary, one database file, one external dependency (Telegram Bot API). Every additional component is a thing that breaks at 3am during a real emergency.

**2. The system must be honest about what it is.**
This is a life-safety supplement. It must never silently fail. If it can't do its job, it must say so (daily heartbeat). If Oref changes their API, the parser must fail loud, not send garbled messages.

**3. Optimize for time-to-forget, not time-to-build.**
Every design choice is evaluated by: "will this still work in 18 months without intervention?" Go + SQLite + systemd is a 10/10. Node + Postgres + Docker Compose is a 4/10.

**4. Test without waiting for rockets.**
The system must be fully testable using recorded fixtures. No test should require a live Oref endpoint or a real Telegram bot.

**5. Do the simplest thing that works. Add complexity only when forced by evidence.**
No Telegram ingestion. No multi-source correlation. No WhatsApp. No SMS. These can be added later if needed. They aren't needed now.

## Decisions Log

### D1: Deployment target — IL VPS (~$4/mo)

**Context:** Oref endpoints are geo-restricted to Israeli IPs and require browser-like headers (`Referer`, `X-Requested-With`). A laptop sleeps, reboots, and loses connectivity. 100 users relying on a laptop for emergency alerts is architecturally dishonest.

**Decision:** Deploy on a small VPS with an Israeli IP (e.g., Kamatera, HostGator IL, Oracle Cloud Jerusalem if available). ~$4/mo.

**Alternatives rejected:**
- *Laptop only ($0):* Fails the resilience requirement. Missed alerts during sleep/reboot.
- *Laptop + free-tier cloud fallback:* Two deployment targets to maintain. Free-tier clouds don't have IL regions, so geo-blocking remains unsolved for the cloud leg.

### D2: Alert source — Oref JSON API only

**Context:** The Oref real-time endpoint (`alerts.json`) returns structured JSON with a `cat` (category) field that covers the full alert lifecycle. Confirmed live from Israeli IP on 2026-03-19:

**Category mapping (from `alertCategories.json`, verified live):**
| `cat` | Category name | EventType |
|---|---|---|
| 1 | `missilealert` | Alert |
| 2 | `uav` | Alert |
| 3 | `nonconventional` | Alert |
| 4 | `warning` | Alert |
| 9 | `cbrne` | Alert |
| 10 | `terrorattack` | Alert |
| 13 | `update` | EndOfEvent |
| 14 | `flash` | PreAlert |
| 15-28 | `*drill` | Drill |

**Confirmed in live history feed (`AlertsHistory.json`):**
- `cat=1, title="ירי רקטות וטילים"` — active rocket alert
- `cat=13, title="האירוע הסתיים"` — end-of-event
- `cat=14, title="בדקות הקרובות צפויות להתקבל התרעות באזורך"` — pre-alert

**Empty response quirk:** When no alert is active, `alerts.json` returns a UTF-8 BOM (`ef bb bf`) + CRLF (`0d 0a`) — not valid JSON and not an empty string. Parser must strip BOM and handle this as "no active alert."

**Schema differences between endpoints:**
- Real-time (`alerts.json`): `{id, cat, title, data: [area1, area2, ...], desc}` — `data` is an **array**.
- History (`AlertsHistory.json`): `{alertDate, title, data: "area", category}` — `data` is a **single string**, one record per area.

The official Telegram channel (`@PikudHaOref_all`) also carries lifecycle events, but as unstructured Hebrew text requiring regex parsing.

**Decision:** Use Oref JSON endpoints as the sole data source. Poll `alerts.json` every 2 seconds.

**Alternatives rejected:**
- *Telegram only:* Higher latency (~5-15s relay delay), fragile text parsing, second integration to maintain.
- *Both Oref + Telegram:* Unnecessary complexity. Correlation logic, two failure modes, two parsers. YAGNI — add Telegram fallback only if Oref drops lifecycle categories.

**Endpoints used (all verified live 2026-03-19 from IL IP):**
| Endpoint | Purpose | Poll frequency |
|---|---|---|
| `GET /WarningMessages/alert/alerts.json` | Real-time alerts (primary) | Every 2s |
| `GET /warningMessages/alert/History/AlertsHistory.json` | Recent alert history (timestamps, per-area records) | Every 30s |
| `GET alerts-history.oref.org.il/Shared/Ajax/GetDistricts.aspx?lang=he` | Location catalog (~1700+ areas with shelter times) | Daily refresh |
| `GET /alerts/alertCategories.json` | Category ID → name mapping (28 categories) | On startup + daily |

**Required headers (all requests to oref.org.il):**
```
Referer: https://www.oref.org.il/
X-Requested-With: XMLHttpRequest
Accept: application/json
```

### D3: Language & storage — Go + SQLite

**Context:** "Run and forget" demands minimal runtime dependencies and resistance to software rot. The service manages simple structured data (users, events, delivery log) for at most 100 users.

**Decision:** Single Go binary. SQLite (WAL mode) for all persistent state.

**Rationale:**
- Go compiles to a static binary. No interpreter, no virtualenv, no `node_modules`. `scp` + `systemctl restart` is the entire deploy.
- SQLite WAL mode handles concurrent reads from the bot handler while the poller writes. At 100 users this is not a bottleneck.
- Fewer things to `apt update`. Fewer CVEs to track. Fewer reasons to touch the project.

**Alternatives rejected:**
- *Python:* Dependency rot (`pip`, virtualenv version drift). Fine for prototyping, not for forget.
- *Node/TypeScript:* `node_modules` entropy. Runtime version management.
- *Postgres:* Separate process to run, monitor, backup. Overkill for 100 users and a single-writer workload.

### D4: Delivery channel — Telegram Bot

**Context:** Users need to receive lifecycle notifications on their phone. The Telegram Bot API is already in the stack for user configuration (D5). Adding a second delivery channel (WhatsApp, SMS) means a second integration to build, test, and maintain.

**Decision:** Telegram bot for both configuration and delivery. Single integration point.

**Rationale:**
- Zero cost. Telegram Bot API is free with generous rate limits (30 msg/s to different chats).
- Already required for user config (D5). No new dependency.
- Rich formatting (bold, monospace for timestamps), silent/loud message modes.
- Stable API since 2015. No business verification, no template approval.

**Alternatives rejected:**
- *WhatsApp (Meta Cloud API):* Free tier exists (1000 conversations/mo) but requires Meta business verification, template message approval, and ongoing compliance. Not "forget" grade.
- *WhatsApp (unofficial libs like Baileys):* Violates ToS, breaks on protocol updates. Antithetical to run-and-forget.
- *SMS (Twilio):* ~$0.008/msg. At 100 users × ~5 events/month = $4/mo minimum, scaling with alert volume. Ongoing cost that's unpredictable during escalations.
- *Signal:* No official bot API. Community `signal-cli` breaks on protocol updates. Opposite of run-and-forget.

### D5: User configuration — Telegram bot commands

**Context:** 100 users need to set and change their monitored location. Must be self-service.

**Decision:** Users interact via Telegram bot commands:
- `/start` — Register + prompt for location
- `/setlocation <area>` — Set monitored area (fuzzy match against Oref district catalog)
- `/status` — Show current subscription
- `/stop` — Unsubscribe

Location matching uses the Oref `GetDistricts` catalog. Users type a location name; the bot fuzzy-matches and confirms.

**Alternatives rejected:**
- *Web UI:* Another surface to host, secure, and maintain. TLS cert renewal, auth, static hosting.
- *Config file:* Doesn't scale to 100 users.

### D6: Testing strategy — Fixtures + dry-run mode

**Context:** You cannot wait for a real rocket attack to validate the system. The Oref message format is the most fragile dependency — if it changes silently, the system must fail loud, not send garbage.

**Decision:** Two testing mechanisms:

**Replay fixtures:** Record real `alerts.json` responses for all three lifecycle categories (cat 13, 14, and active threat types) as JSON files in `testdata/`. Contract-test the parser against them. CI runs these on every commit.

**Dry-run mode:** `--dry-run` flag that polls live Oref endpoints but logs parsed events instead of dispatching to Telegram. Validates real connectivity + parsing without spamming users. Used for deployment smoke tests.

**Alternatives considered but deferred:**
- *Daily heartbeat:* Valuable for production monitoring, but it's an operational feature, not a testing mechanism. Tracked in the implementation plan as a future enhancement. Keeping MVP scope tight.

## Data Model

```sql
CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    telegram_id   INTEGER UNIQUE NOT NULL,
    area          TEXT NOT NULL,       -- canonical Oref area name (Hebrew)
    active        INTEGER DEFAULT 1,
    created_at    TEXT DEFAULT (datetime('now')),
    updated_at    TEXT DEFAULT (datetime('now'))
);

CREATE TABLE events (
    id            INTEGER PRIMARY KEY,
    oref_id       TEXT NOT NULL,       -- from alerts.json `id` field
    category      INTEGER NOT NULL,    -- 1, 13, 14, etc.
    title         TEXT NOT NULL,
    areas         TEXT NOT NULL,       -- JSON array of area strings
    description   TEXT,
    received_at   TEXT DEFAULT (datetime('now')),
    UNIQUE(oref_id, category)          -- dedup key
);

CREATE TABLE deliveries (
    id            INTEGER PRIMARY KEY,
    event_id      INTEGER REFERENCES events(id),
    user_id       INTEGER REFERENCES users(id),
    status        TEXT NOT NULL,       -- sent, failed, skipped
    sent_at       TEXT DEFAULT (datetime('now')),
    error         TEXT
);
```

## Component Diagram

```
┌──────────────────────────────────────────────────┐
│                 Single Go Binary                  │
│                                                   │
│  ┌────────────┐         ┌──────────────────────┐ │
│  │  Poller    │         │  Telegram Bot Server │ │
│  │            │         │                      │ │
│  │ alerts.json│         │  /start              │ │
│  │ every 2s   │         │  /setlocation        │ │
│  │            │         │  /status             │ │
│  └─────┬──────┘         │  /stop               │ │
│        │                └──────────┬───────────┘ │
│        ▼                           │             │
│  ┌─────────────┐                   │             │
│  │  Parser +   │                   │             │
│  │  Dedup      │                   ▼             │
│  │             │          ┌────────────────┐     │
│  └─────┬───────┘          │    SQLite      │     │
│        │                  │                │     │
│        ▼                  │  users         │     │
│  ┌─────────────┐          │  events        │     │
│  │  Location   │◄────────▶│  deliveries    │     │
│  │  Matcher    │          │                │     │
│  └─────┬───────┘          └────────────────┘     │
│        │                                         │
│        ▼                                         │
│  ┌─────────────┐                                 │
│  │ Dispatcher  │                                 │
│  │ (Telegram   │                                 │
│  │  sendMsg)   │                                 │
│  └─────────────┘                                 │
│                                                  │
└──────────────────────────────────────────────────┘

External:
  - Oref endpoints (oref.org.il) — polled
  - Telegram Bot API (api.telegram.org) — called
```

## Message Format (Delivered to Users)

```
🔴 ALERT — Rocket/Missile Fire
Areas: תקומה, נתיבות
Action: Enter protected space, stay for 10 minutes
Time: 2026-03-19 20:55 IST
```

```
🟡 PRE-ALERT — Early Warning
Areas: עוטף עזה
Action: Be prepared to enter protected space
Time: 2026-03-19 20:50 IST
```

```
🟢 END — Event Over
Areas: תקומה, נתיבות
Action: You may exit protected space
Time: 2026-03-19 21:07 IST
```

## Failure Modes and Mitigations

| Failure | Detection | Mitigation |
|---|---|---|
| Oref returns 403 (geo/header block) | HTTP status check on every poll | Log error, retry with backoff. If persistent, alert operator via Telegram DM. |
| Oref changes JSON schema | Parser returns error (strict unmarshaling) | Fail loud — log error, notify operator. Do not send garbled messages. |
| Oref endpoint down | HTTP timeout / 5xx | Exponential backoff (2s → 4s → 8s → ... → 60s cap). Resume normal polling on success. |
| Telegram API down | Send failure | Retry with backoff. Log to deliveries table with `status=failed`. |
| Process crash | systemd detects exit | `Restart=always` with `RestartSec=5`. SQLite dedup prevents duplicate notifications on restart. |
| SQLite corruption | Write failure | WAL mode + periodic `.backup` command to a second file. |
| Polling misses a short-lived alert | Alert disappears between polls | 2s poll interval minimizes this. Oref alerts typically persist for their shelter-time duration (30s-90s+). |

# Technical Dossier on Oref Alert Lifecycle Access for Location-Based Personal Notifications

## Executive summary and feasibility

The Oref (Israel Home Front Command / Pikud HaOref) web ecosystem exposes **machine-consumable JSON endpoints** that multiple open-source projects and integrations use to obtain **real-time alerts**, **location catalogs**, and **guidance/translation metadata**. The core real-time endpoint returns a compact JSON object (id, category, title, list of areas, instructions) when an alert is active, and is frequently polled (commonly **every 1–2 seconds**) by clients that emulate the official website behavior. citeturn26view1turn40search12turn38view0turn32view0

However, these endpoints are **not documented as a public developer API**, and in practice appear to be protected by a combination of **geo/IP restrictions** and **header checks** (notably `Referer` and `X-Requested-With`). Attempting to fetch key official JSON metadata (`alertCategories.json`) from this environment returns **HTTP 403 Forbidden**, and the Oref English site root itself also returned **403** here—indicating practical access constraints that you should plan around (e.g., run collectors on an Israeli egress IP). citeturn23view0turn2view0turn26view0turn27view0

For “**alert lifecycle events**” (pre-alert → alert → end-of-event), the most **operationally complete** official signal discoverable from accessible sources is the **official Telegram channel** `@PikudHaOref_all`, which includes:
- “מבזק” (flash / pre-alert) messages warning that alerts are expected soon in specific regions;
- the actual alert messages (e.g., “ירי רקטות וטילים”, “חדירת כלי טיס עוין”) listing regions and localities;
- explicit “עדכון … האירוע הסתיים” (end-of-event) updates. citeturn21view0

Separately, the official iOS app’s version history indicates the Home Front Command introduced distinct message types including **“Update”** and **“End of event”**, and that alerts may be delivered as iOS **Critical Alerts** (audible even in Silent/Focus), and it supports “early warnings.” citeturn36view0

**Feasibility conclusion:** Building a personal SMS/WhatsApp notifier that emits messages on **every lifecycle event** is feasible, but the most robust design is **multi-source**:
1) Prefer the official **app + Telegram** for coverage and explicit end-of-event;
2) Use Oref web JSON endpoints where legally/operationally appropriate (and only from permitted network locations), mainly for low-latency real-time alerts and structured fields. citeturn21view0turn36view0turn40search12

## Primary Oref surfaces and observed technical endpoints

### Web app access constraints observed

Direct access to Oref site content from this environment was restricted (403), and at least one core metadata file is forbidden as well. citeturn2view0turn23view0

Separately, community configurations show that the real-time JSON endpoint can return “Access Denied” unless the request includes browser-like headers such as `Referer: https://www.oref.org.il/` and `X-Requested-With: XMLHttpRequest`. citeturn26view0

In production terms, you should assume:
- **No API keys / OAuth** are used for these endpoints (requests are anonymous), but
- **Access controls** exist (geo/IP + header heuristics), and
- schemas and/or gating can change without notice (multiple projects warn this is “unofficial”). citeturn26view0turn40search12turn27view0

### Endpoints and formats

The following endpoints are referenced by multiple active projects as the functional Oref web “API surface”:

**Real-time current alert**
```text
GET https://www.oref.org.il/warningMessages/alert/Alerts.json
```
Referenced as the primary alert poll endpoint. citeturn12view0turn25view0turn38view0

A widely used alternate casing/path is:
```text
GET https://www.oref.org.il/WarningMessages/alert/alerts.json
```
Used in Home Assistant examples. citeturn26view0turn26view1

**Recent-history alert feed (site history)**
```text
GET https://www.oref.org.il/warningMessages/alert/History/AlertsHistory.json
```
Referenced directly by scripts that also poll the real-time file. citeturn25view0

**City-scoped history (alerts-history portal)**
```text
GET https://alerts-history.oref.org.il/Shared/Ajax/GetAlarmsHistory.aspx?lang=he&mode=1&city_0=<CITY_NAME>
```
Observed in a script pulling city-filtered history (where `city_0` is a city/area name string, example shown in Hebrew). citeturn25view0

**Location catalog**
```text
GET https://alerts-history.oref.org.il/Shared/Ajax/GetDistricts.aspx?lang=<lang>
```
Referenced as the location list endpoint. citeturn12view0turn37view0turn28search0

**Cities/areas catalog (mix)**
```text
GET https://alerts-history.oref.org.il/Shared/Ajax/GetCitiesMix.aspx
```
Added/used by integrations to detect changes in the list of monitored areas. citeturn30view0

**Guidelines / instructions per city**
```text
GET https://alerts-history.oref.org.il/Shared/Ajax/GetAlarmInstructions.aspx?lang=<lang>&from=1&cityid=<CITY_ID>
```
Referenced as a structured instructions endpoint per city. citeturn12view0turn37view0

**Alert category metadata (official, but forbidden here)**
```text
GET https://www.oref.org.il/alerts/alertCategories.json
```
A primary official metadata list, but returned **403 Forbidden** from this environment. citeturn23view0

**Alert translation metadata**
```text
GET https://www.oref.org.il/alerts/alertsTranslation.json
```
Referenced as an alert translation mapping feed. citeturn12view0turn37view0

### Request examples and headers

A commonly documented (working) request pattern for `alerts.json` uses browser-like headers:

```bash
curl 'https://www.oref.org.il/WarningMessages/alert/alerts.json' \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -H 'Referer: https://www.oref.org.il/' \
  -H 'X-Requested-With: XMLHttpRequest'
```

This header requirement is explicitly discussed in community configurations, where missing headers leads to “Access Denied”, and adding `Referer` and `X-Requested-With` is advised. citeturn26view0turn26view1

### Response formats, parsing quirks, and update frequency

The canonical real-time payload example (when alerts exist) is:

```json
{
  "id": "133284777020000000",
  "cat": "1",
  "title": "ירי רקטות וטילים",
  "data": ["תקומה", "נתיבות"],
  "desc": "היכנסו למרחב המוגן ושהו בו 10 דקות"
}
```
citeturn26view1

Notable parsing caveats:
- Some responses may include leading whitespace or non-printable characters; one user notes the “first char … is some whitespace” and that trimming before JSON parse fixes it. citeturn26view1
- Projects explicitly implement defensive parsing and de-dup logic; e.g., removing null characters from JSON and improving event de-duplication is mentioned in release notes/diffs. citeturn40search16turn32view0

Update frequency is not formally documented by Oref in accessible pages, but multiple technical sources indicate near-real-time polling:
- A Python package states it polls “every second like the official website.” citeturn40search12
- A reference implementation loops with `time.sleep(1)` between checks. citeturn38view0
- Another integration’s UI configuration defaults to polling (website + history channels) every **2 seconds**. citeturn32view0

Rate limits are not explicitly published in Oref materials available here. Operationally, third-party dashboards implement fallback mechanisms “when the Oref API cap is reached,” implying throttling/capping exists server-side. citeturn27view0  
For your own service, plan for strict throttling, caching, and exponential backoff (details in later sections).

## Alert lifecycle model and location specification

### Lifecycle event types you can reliably emit

Based on official Telegram messaging patterns and official app release notes, the lifecycle of an incident can be modeled with at least three core event types:

**Pre-alert / early warning**
- Telegram issues “מבזק” (flash) messages stating “in the coming minutes alerts are expected in your area,” with region lists. citeturn21view0
- The official iOS app has “early advance warnings of possible sirens” (user review) and product evolution explicitly supports early warning behavior. citeturn36view0
- Multiple alert pipelines label a pre-alert state and associate it with category **14** in the Oref category system (“הנחיה מקדימה”). citeturn22view0turn40search0

**Alert (take shelter now)**
- The real-time JSON includes a category (`cat`), a title (e.g., “ירי רקטות וטילים”), an area list, and an instruction string (`desc`). citeturn26view1
- Telegram issues dedicated alert posts for specific threats, including “ירי רקטות וטילים” and “חדירת כלי טיס עוין”. citeturn21view0

**End-of-event / all-clear**
- Telegram posts “עדכון … האירוע הסתיים” and tells people they can exit protected spaces. citeturn21view0
- The official iOS app release notes describe a distinct “End of event” message type used for resuming routine at the end of ongoing events. citeturn36view0
- Pipelines associate end updates with category **13**. citeturn22view0turn40search0

Additionally, the official iOS app also distinguishes a general **“Update”** message type (e.g., exercises, software updates), which you may want to include as a fourth event type in your notification taxonomy. citeturn36view0

### Status codes, category IDs, and timestamps

**Oref real-time JSON (`alerts.json`)**
- `id`: string identifier (commonly used for de-dup; treated as unique per alert burst). citeturn26view1turn38view0
- `cat`: category code (string), e.g., `"1"` for “ירי רקטות וטילים” in the example. citeturn26view1turn40search0
- `title`: Hebrew threat title. citeturn26view1turn40search0
- `data`: list of impacted areas/localities (strings, often Hebrew). citeturn26view1turn39view0turn38view0
- `desc`: Hebrew instruction string. citeturn26view1

This payload does **not** include an explicit timestamp field in the example shown; implementations typically timestamp on receipt or use the history feed for normalized `alertDate`-style fields (see below).

**Oref “record”/normalized model (derived by integrations)**
A commonly normalized record includes:
- `alertDate` (Israel timezone),
- `title` (Hebrew),
- `data` (single area name),
- `category` (integer; notes that 14 is pre-alert and 13 is end). citeturn22view0

**Telegram**
Telegram posts embed timestamps in the text (e.g., “(19/3/2026) 20:44” and “עדכון … (19/3/2026) 21:07”), which you can parse into ISO-8601 with timezone `Asia/Jerusalem`. citeturn21view0

### Location specification and how to make it configurable

Across discovered endpoints and observable payloads, Oref location targeting is primarily by **named areas/localities**, not by latitude/longitude:

**In `alerts.json`:** `data` is a list of locality/area names. citeturn26view1turn39view0

**Location catalog (`GetDistricts.aspx?lang=...`):** data is JSON records that include names/labels and regional fields; one project merges multilingual labels across `en/he/ar/ru` and associates a shelter-time field `migun_time`. citeturn37view0turn28search0turn39view0

**Shelter time / zone metadata:** The same loader maps each location’s `migun_time` into a `shelter_time` attribute. citeturn37view0turn39view0

**Guidelines per city (`GetAlarmInstructions.aspx?...cityid=`):** A structured “notes” list is fetched and transformed into guideline objects with codes, modes, and color codes. citeturn37view0turn39view0

**How to make location configurable in your service**
- Offer users a selector based on the **official catalogs** (`GetDistricts`, optionally `GetCitiesMix`) and store their chosen “areas of interest” as a list of canonical names (preferably the Hebrew label keys, since `alerts.json` titles/areas are often Hebrew). citeturn30view0turn37view0turn26view1
- For multilingual UI, build a mapping table from the multilingual location catalog and (if accessible from your collector’s network) `alertsTranslation.json`. The translation feed is modeled as four-language mappings (`heb/eng/rus/arb`) with `catId`, `matrixCatId`, and `updateType`. citeturn37view0turn39view0
- If you need coordinate-based configuration, you must add your own geospatial mapping layer; the official payloads observed here do not provide coordinates in-line. (Several downstream projects maintain their own coordinate/polygon maps, but those are not official Oref outputs in the sources available here.) citeturn22view0turn39view0

## Subscription mechanisms and official channels

### In-browser subscription on the alerts portal

The official alerts-history portal text indicates the site allows users to **enable receiving real-time alerts “on the site”**, and includes an option for **audible alerts** (“I want to also receive sound alerts”). This is a browser/UI subscription, not a developer webhook. citeturn16search0turn24search0

### Official Telegram channels

The official Telegram channel `@PikudHaOref_all` is a high-value source for lifecycle completeness because it explicitly posts:
- hostile aircraft intrusion alerts,
- rocket/missile fire alerts,
- flash pre-alerts (“מבזק”),
- end-of-event updates (“האירוע הסתיים”). citeturn21view0

An additional official channel (`@HanhayotPikudHaOref`) is presented as the official guidance channel and points to official platforms. citeturn21view1

**Practical note:** Telegram provides a clean subscription mechanism for humans; for programmatic access you typically use Telegram’s APIs or a bot approach (not documented by Oref). Treat any programmatic consumption as subject to Telegram’s and channel policies.

### Official mobile app push notifications

The official Home Front Command app (Android listing) states it provides alerts and guidelines in real time “according to your location and areas of interest.” citeturn34view0  
The iOS listing indicates alerts can be delivered as **Critical Alerts** and also references new “Update” and “End of event” message types across versions. citeturn36view0

From what is discoverable here, the app is **not** exposing a public API for third parties; it is the endpoint consumer.

### Alternative (official-leaning) sources

There are public dashboards that explicitly implement an “Israeli IP” proxy to reach Oref endpoints due to geo-blocking. While not official APIs, they demonstrate the operational constraint and typical architecture for external consumers. citeturn27view0

## Practical integration for SMS and WhatsApp delivery

### Recommended integration patterns

A reliable personal notification service should be event-driven internally even if upstream data requires polling:

1) **Collector** (poller + parser)
- Poll `alerts.json` at a conservative frequency (e.g., 1–2 seconds only when risk is high; otherwise 5 seconds), respecting server limitations.
- Poll a history/update feed at a slower cadence (e.g., 10–30 seconds) to capture updates/end messages when available.
- In parallel, ingest Telegram messages (preferred for explicit end-of-event).

2) **Normalizer**
- Convert diverse inputs into your own canonical event schema with `event_type ∈ {pre_alert, alert, end, update}`, normalized `timestamp`, and normalized `areas[]`.

3) **Deduplicator**
- Use `source + upstream_id + event_type + sorted(areas)` as a primary key.
- Apply time-window suppression: e.g., ignore duplicates within 2–5 minutes for the same key, but allow “escalations” (pre_alert → alert) to pass through.

4) **Dispatcher**
- Fan-out messages to SMS and WhatsApp.
- Enforce provider rate limits and message-length constraints.

### Twilio SMS and Twilio WhatsApp

Twilio’s **Messages resource** is the cleanest single API for both SMS and WhatsApp (depending on your sender setup). The official endpoint to create a message is: citeturn42view0

```text
POST https://api.twilio.com/2010-04-01/Accounts/{AccountSid}/Messages.json
Content-Type: application/x-www-form-urlencoded
```

Key fields include:
- `To` (E.164 for SMS, or channel address like `whatsapp:+15552229999`)
- `From` (Twilio number or WhatsApp-enabled channel address)
- `Body` (text)
- Optional `StatusCallback` (delivery state webhooks)
Twilio also documents detailed message status values (queued/sending/sent/delivered/undelivered/failed; and “read” for WhatsApp where supported). citeturn42view0

**Sample Twilio outbound SMS (HTTP form-encoded)**

```bash
curl -X POST "https://api.twilio.com/2010-04-01/Accounts/$TWILIO_ACCOUNT_SID/Messages.json" \
  -u "$TWILIO_ACCOUNT_SID:$TWILIO_AUTH_TOKEN" \
  --data-urlencode "To=+1XXXXXXXXXX" \
  --data-urlencode "From=+1YYYYYYYYYY" \
  --data-urlencode "Body=[Oref] ALERT • Rocket/Missile • Tel Aviv Center • 2026-03-19T20:55+02:00 • Enter protected space now"
```

**Sample Twilio outbound WhatsApp**
```bash
curl -X POST "https://api.twilio.com/2010-04-01/Accounts/$TWILIO_ACCOUNT_SID/Messages.json" \
  -u "$TWILIO_ACCOUNT_SID:$TWILIO_AUTH_TOKEN" \
  --data-urlencode "To=whatsapp:+1XXXXXXXXXX" \
  --data-urlencode "From=whatsapp:+1YYYYYYYYYY" \
  --data-urlencode "Body=[Oref] END • 2026-03-19T21:07+02:00 • You may exit protected space • Areas: Haifa–Carmel, HaMifratz"
```

Twilio warns that messages are queued at prescribed rate limits and can be delayed if you exceed sending capacity. citeturn42view0

### WhatsApp Business Platform Cloud API

Meta provides an official “WhatsApp Business Platform and Cloud API Examples” repository, which is a practical starting point to implement Cloud API send/receive patterns. citeturn42view2

Because this report could not further retrieve the specific code files in that repository (tooling limit), the **exact REST path and JSON payload structure for Cloud API message send** should be treated as **unspecified in this dossier**; use Meta’s official examples repo as the primary source of truth for sender endpoints and payload fields. citeturn42view2turn42view1

That said, a commonly used Cloud API send pattern resembles:

```json
{
  "messaging_product": "whatsapp",
  "to": "<recipient_phone_e164_without_plus_or_with_plus_per_meta_spec>",
  "type": "text",
  "text": { "body": "..." }
}
```

Treat the above as a conceptual template and verify against Meta’s official documentation/examples before implementation. citeturn42view2turn42view1

### Message templates and operational behavior

For high-urgency alerts, keep content short and consistent, and include:
- Event type (PRE-ALERT / ALERT / END)
- Threat type (rocket/missile, UAV, etc.)
- Timestamp (local)
- Relevant areas (truncate after N items)
- Instruction line (e.g., “Enter protected space now.” / “Event ended—may exit.”)

Because Oref titles and instructions frequently arrive in Hebrew (`title`, `desc`), either:
- send Hebrew as-is, or
- translate using official translation metadata feeds (`alertsTranslation.json`) when accessible to your collector network. citeturn26view1turn37view0turn39view0

### Data-flow diagram

```mermaid
flowchart TD
  A[Oref Web Endpoints\nAlerts.json / AlertsHistory.json\n(geo/header restricted)] -->|poll| N[Normalizer\ncanonical event schema]
  T[Official Telegram\n@PikudHaOref_all\n(pre-alert/alert/end)] -->|parse messages| N
  N --> D[Dedup + Correlator\nid + type + areas\nTTL windows]
  D --> Q[Queue/Dispatcher\nrate-limit + retry]
  Q --> S[SMS Provider\nTwilio Messages API]
  Q --> W[WhatsApp Provider\nTwilio WhatsApp or Meta Cloud API]
  Q --> L[Event Store\nSQLite/Postgres\nfor auditing & replays]
```

## Legal, privacy, and operational considerations

### Unofficial consumption risk and fragility

Several developer-facing artifacts explicitly treat these endpoints as **unofficial** and warn that upstream JSON schemas may change without notice. citeturn40search12  
Additionally, key metadata (`alertCategories.json`) returned **403 Forbidden** here, strongly suggesting Oref is enforcing access controls that you must not bypass unlawfully. citeturn23view0turn26view0

### Safety and reliance

The official app listings and official Telegram channel are designed for life-safety communication. Your personal service should be framed as a **supplement**, not a replacement for official channels (sirens, app alerts, official guidance). The official app emphasizes real-time alerts and guidelines and provides official contact routes (e.g., 104 call center referenced in app “What’s New”). citeturn34view0turn36view0

### Phone number handling, opt-in, and WhatsApp policy constraints

Phone numbers are personal data. If you expand beyond “personal use” (e.g., notify multiple people), implement:
- explicit opt-in,
- easy opt-out,
- minimal retention (store only what you must: delivery logs + dedup keys),
- secure secrets management.

For WhatsApp specifically, Meta’s ecosystem policies can change and are actively enforced; use the official Meta examples and platform policy as your guardrails (the examples repo indicates it is “Meta Platform Policy licensed”). citeturn42view2

## Recommended implementation plan and effort

### Minimal viable personal service

Goal: One user, one or a few locations, lifecycle notifications via SMS.

Milestones and rough effort:
- Collector MVP (poll alerts.json + parse + dedup on `id`): 4–6 hours. citeturn26view1turn38view0
- Add header controls and Israeli egress deployment (small VM/container in IL): 2–4 hours (environment + monitoring), noting the practical geo/header constraints. citeturn26view0turn27view0turn2view0
- Twilio SMS integration using Messages API + basic retry: 2–3 hours. citeturn42view0
- Add Telegram ingestion for end-of-event + pre-alert completeness: 3–5 hours. citeturn21view0

**Total MVP estimate:** ~11–18 hours.

### Robust production-grade service

Goal: Multi-source ingestion, WhatsApp delivery, auditing, and resilience.

Milestones and rough effort:
- Multi-source ingestion (Oref endpoints + Telegram), normalized schema, durable event store (Postgres): 12–18 hours. citeturn21view0turn25view0turn38view0
- Location catalog + multilingual mapping (GetDistricts + GetCitiesMix) with periodic refresh: 6–10 hours. citeturn30view0turn37view0
- Translation layer using official translation feed when accessible: 4–8 hours. citeturn37view0turn39view0
- Delivery subsystem with per-channel throttles, dead-letter queue, and templating: 8–14 hours. citeturn42view0
- Observability (metrics, alerting, dashboards), plus chaos drills (simulate high-volume alerts): 6–10 hours.
- Security hardening (secrets manager, least privilege, audit logs, data retention): 6–12 hours.

**Total robust estimate:** ~42–72 hours.

## Data source comparison table

| source | URL | format | auth | update frequency | reliability |
|---|---|---|---|---|---|
| Oref real-time alert feed | `https://www.oref.org.il/warningMessages/alert/Alerts.json` | JSON | none, but appears geo/header restricted | often polled 1s–2s by clients citeturn38view0turn40search12turn32view0 | high timeliness when reachable; access constraints significant citeturn26view0turn2view0 |
| Oref real-time alert feed (alt path) | `https://www.oref.org.il/WarningMessages/alert/alerts.json` | JSON | none, but header checks observed citeturn26view0 | user-configured (examples show 2–5s) citeturn26view0turn32view0 | similar to above |
| Oref history feed | `https://www.oref.org.il/warningMessages/alert/History/AlertsHistory.json` | JSON | none (constraints likely similar) | not specified; typically slower poll | useful for timestamps/history correlation citeturn25view0 |
| Alerts-history city history | `https://alerts-history.oref.org.il/Shared/Ajax/GetAlarmsHistory.aspx?...` | JSON | none observed | on-demand / periodic | good for per-city history; schema may change citeturn25view0 |
| Locations catalog | `https://alerts-history.oref.org.il/Shared/Ajax/GetDistricts.aspx?lang=` | JSON | none observed | refresh daily/weekly | essential for area lists + shelter times citeturn37view0turn28search0 |
| Cities/areas catalog (mix) | `https://alerts-history.oref.org.il/Shared/Ajax/GetCitiesMix.aspx` | JSON | none observed | refresh ~daily (integration checks 12h) citeturn30view0 | essential to keep area list current |
| Category metadata | `https://www.oref.org.il/alerts/alertCategories.json` | JSON | forbidden here (403) citeturn23view0 | unknown | likely authoritative, but access-limited |
| Official Telegram alerts | `https://t.me/s/PikudHaOref_all` | Telegram message stream (HTML page / Telegram platform) | subscribe via Telegram | near-real-time | very strong lifecycle coverage (pre-alert + end-of-event) citeturn21view0 |
| Official mobile app | App Store / Google Play listings | push notifications | user install | real-time | highest end-user reliability, but no public API citeturn34view0turn36view0 |

**Unspecified items:** Public rate limits for Oref endpoints, official developer ToS for programmatic access, and any official email/SMS subscription features were not discoverable from accessible English/Hebrew pages in this environment; where relevant pages were gated (403), they are explicitly noted. citeturn2view0turn23view0
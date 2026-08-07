# Tag simulation (per-tag + legacy global)

Synthetic tag samples for PLC-off / lab UI without pretending the whole plant is live.

## Source of truth

| Control | Default | Auto on disconnect? | Effect |
|---------|---------|---------------------|--------|
| **Per-tag `simulate`** | **false** | **Never** | Mock samples for that tag only; **real OPC continues** for other tags |
| Legacy `tag_simulation` / `LEVEL2_TAG_SIMULATION` | **off** | **Never** | All enabled tags mocked; **OPC collect paused** (master) |
| `LEVEL2_SIM_BROWSER` | lab `.env.example` may set `1` | **Never** (env only) | Full in-memory browse + all samples |

**Prefer per-tag `simulate`.** The global master remains for “simulate everything” lab setups; it is opt-in and never turns on when OPC drops.

Disconnect honesty (no global/sim-browser): Overview TAGS must not show green “all good” from stale Live Good on disconnected devices — except tags with `simulate=true`, which keep Good mock samples.

## Per-tag simulate

YAML / project.xlsx column `simulate` (same pattern as `writable`):

```yaml
tags:
  - id: demo_sp
    node_id: ns=4;i=1
    datatype: float64
    enabled: true
    simulate: true   # default false when omitted
```

### Runtime (PLC connected vs not)

| Tag | PLC up | PLC down |
|-----|--------|----------|
| `simulate: false` | Live OPC values | Stale / bad (honest) |
| `simulate: true` | **Mock overrides** that tag (OPC skipped for it) | Mock continues |

Collector always runs a mock loop for tags with `simulate=true`. OPC subscribe lists exclude those tags so writers do not fight.

### API

```http
PATCH /api/v1/devices/{device_id}/tags/{tag_id}
{"simulate": true}

POST /api/v1/devices/{device_id}/tags/simulate
{"simulate": true, "tag_ids": ["a","b"]}
# or all tags on device:
{"simulate": true, "all": true}

POST /api/v1/tags/simulate
{"simulate": false, "all": true}
{"simulate": true, "tag_ids": ["a"], "device_id": "plc"}
```

Applies immediately (config gen reload) — **no collector recreate** for per-tag flags.

`GET /api/v1/status/summary` includes `tags_simulated` (count of tags with `simulate=true`, or all enabled under global/sim browser).

## Legacy global master

```yaml
tag_simulation: false
```

```bash
LEVEL2_TAG_SIMULATION=true
```

```http
GET  /api/v1/tag-simulation
PUT  /api/v1/tag-simulation
{"enabled": true}
```

After enabling global master: recreate collector so the process starts with OPC paused and mock for all enabled tags.

Admin Overview keeps a demoted “legacy: simulate all tags” control; primary UX is per-tag on **DB write list**.

## Writes / safety

- Global master or `LEVEL2_SIM_BROWSER` active → all Write API calls **409**.
- Per-tag: writes to a tag with `simulate=true` → **409**; other writable tags still go to the PLC when write gate is on.

## Status / UI

- `tags_simulated` — pill on Overview TAGS card and DB write list header
- Filter: “Simulated only” on DB write list
- Bulk: enable/disable simulation for all or selected tags

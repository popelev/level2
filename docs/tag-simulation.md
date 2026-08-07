# Tag simulation (opt-in)

Synthetic tag samples for PLC-off / lab UI without pretending the plant is live.

## Defaults

| Control | Default | Auto on disconnect? |
|---------|---------|---------------------|
| `tag_simulation` / `LEVEL2_TAG_SIMULATION` | **off** | **Never** |
| `LEVEL2_SIM_BROWSER` | lab `.env.example` may set `1` | **Never** (env only) |

Disconnect honesty (sim **off**): Overview COLLECTOR / TAGS must not show green “all good” from stale Live Good samples. Status counts disconnected devices’ Live Good as bad; OPC poll failure emits Bad samples into FanIn.

## Enable tag simulation

Primary controls (same pattern as `opc_write_enabled`):

```yaml
# config.yaml — default false
tag_simulation: false
```

```bash
# env overrides YAML
LEVEL2_TAG_SIMULATION=true
```

API (persists YAML; **recreate collector** to apply sample path):

```http
GET  /api/v1/tag-simulation
PUT  /api/v1/tag-simulation
{"enabled": true}
```

Admin UI Overview has a checkbox that calls the same PUT.

After enabling: `docker compose -f deploy/platform/docker-compose.yml up -d --force-recreate` (or equivalent) so the process starts `mock.NewDemo`.

## What it does

When **active** (`tag_simulation` or `LEVEL2_SIM_BROWSER` at process start):

1. Starts `internal/driver/mock.NewDemo` → synthetic Good samples for configured enabled tags.
2. Skips real OPC collect loops (no dual writers).
3. `/readyz` and `collector_ready` are true with `ready_detail`: `tag simulation` or `sim browser`.
4. Status JSON: `tag_simulation: true` (and `sim_browser: true` when browse sim is on).
5. UI labels **sim** / **simulation · not live PLC** (not green “live healthy”).

`LEVEL2_SIM_BROWSER=1` additionally replaces OPC browse/expand with `simbrowser` (full PLC-off). Tag simulation alone keeps the real device hub (Servers may show down) while feeding synthetic tag values.

## Writes / safety

While tag simulation or sim browser is **active**:

- `PUT /api/v1/tags/{id}/value` and batch write return **409** — **no writes to the real PLC**.
- Mock driver has no OPC Writer; a sim-only write store is not in MVP (TODO if needed for CI write tests).

Turn simulation **off** and recreate collector before enabling `opc_write_enabled` against a real PLC.

## Status fields

`GET /api/v1/status/summary` includes:

- `tag_simulation` — process feeding synthetic samples
- `sim_browser` — full browse sim
- `ready_detail` — `ready` | `not connected` | `tag simulation` | `sim browser`

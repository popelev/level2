# Level2 platform collector (M1–M4)

Go service: OPC UA leaf tags → TimescaleDB (`collector.samples`) + REST/WS API + Admin UI.

Capacity / free disk: collector mounts the smoke Timescale volume (`smoke_timeseries`) read-only at `/var/lib/level2/dbdisk` and uses **one Statfs** on that path for both `disk_total_bytes` and `disk_avail_bytes`. Soft limit: `database.capacity_percent` (byte limit = disk_total × percent / 100). `free_bytes` is room under that limit (capped by volume free). Full-disk policy: `database.full_policy` (`stop` | `drop_oldest` | `rotate` | `expand_limit`). Env overrides: `LEVEL2_DB_CAPACITY_PERCENT`, `LEVEL2_DB_FULL_POLICY`, absolute `LEVEL2_DB_CAPACITY_BYTES`.

With **`drop_oldest`**, the historian drops multiple oldest Timescale chunks toward the limit (proactive trim near 90%). While still over the hard limit but trim can progress, flush returns busy and the collector **spools** batches (no halt / no drop of in-flight samples); halt only when nothing left to drop. Details: [docs/db-capacity-policy.md](../../docs/db-capacity-policy.md).

## PLC off

```bash
cd ~/level2
docker run --rm -e GOTOOLCHAIN=auto -v "$PWD":/src -w /src golang:1.24 go test ./...

cd deploy/platform
cp -n config.example.yaml config.yaml
cp -n .env.example .env
# LEVEL2_SIM_BROWSER=1 enables Browse/Expand + synthetic samples without PLC
# Opt-in tag samples only (default off): prefer per-tag `simulate` — docs/tag-simulation.md
# Legacy global: LEVEL2_TAG_SIMULATION / tag_simulation — NEVER auto on disconnect
# Optional write: LEVEL2_OPC_WRITE_ENABLED=true (+ per-tag writable); LEVEL2_API_TOKEN=…
docker compose build
docker compose up -d
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/api/v1/tags
curl -s 'http://127.0.0.1:8080/api/v1/browse?node_id=ns%3D4%3Bi%3D4207'
# UI: http://<vm-ip>:8080/
# Swagger: http://<vm-ip>:8080/docs
# ready should be OK in demo mode; values appear within ~1s
```

## API

Complete route list as registered by `internal/api.Server.Mount` (+ collector `wireHTTP` for health/metrics).

| Method | Path | Notes |
|--------|------|-------|
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Readiness (OPC connected when not sim) |
| GET | `/metrics` | Prometheus |
| GET | `/api/v1/openapi.yaml` | Embedded OpenAPI 3 YAML |
| GET | `/docs` | Swagger UI (loads `/api/v1/openapi.yaml`) |
| GET | `/api/v1/status/summary` | Overview pills / counters |
| GET | `/api/v1/tags` | Configured tags + live sample (`?device_id=`) |
| GET | `/api/v1/tags/{id}/value` | Last live sample |
| GET | `/api/v1/tags/{id}/history?from=&to=&limit=` | Timescale history |
| PUT | `/api/v1/tags/{id}/value` | OPC Write; optional verify; gate + `writable` (+ token) — [opc-write-mode.md](../../docs/opc-write-mode.md) |
| POST | `/api/v1/tags/values` | Batch OPC Write (partial success, max 100) |
| GET | `/api/v1/ws/stream` | Live WebSocket (`?tag_id=` / `?tag_ids=`; `?token=` if auth) |
| GET | `/api/v1/devices` | Device list |
| POST | `/api/v1/devices` | Create device |
| PUT | `/api/v1/devices/{id}` | Update device |
| DELETE | `/api/v1/devices/{id}` | Delete device |
| GET | `/api/v1/browse?node_id=` | OPC browse (or sim) |
| POST | `/api/v1/expand` | `{"node_id","parent_tag_id","max_depth"}` |
| POST | `/api/v1/devices/{id}/tags` | Upsert one tag |
| PUT | `/api/v1/devices/{id}/tags/{tagId}` | Upsert tag by id |
| PATCH | `/api/v1/devices/{id}/tags/{tagId}` | Partial flags (`simulate`, `writable`, `enabled`) |
| DELETE | `/api/v1/devices/{id}/tags/{tagId}` | Delete one tag |
| DELETE | `/api/v1/devices/{id}/tags` | Remove all tags on device |
| POST | `/api/v1/devices/{id}/tags/bulk` | Bulk upsert tags |
| POST | `/api/v1/devices/{id}/tags/sync` | Overwrite datatypes from OPC — [opc-datatype-sync.md](../../docs/opc-datatype-sync.md) |
| POST | `/api/v1/devices/{id}/tags/import` | Multipart `file=.xlsx` (`?replace=1`) |
| GET | `/api/v1/devices/{id}/tags.xlsx` | Export plant-format tag list |
| POST | `/api/v1/devices/{id}/tags/simulate` | Bulk per-tag simulate on device |
| POST | `/api/v1/devices/{id}/tags/writable` | Bulk per-tag writable on device |
| POST | `/api/v1/tags/simulate` | Bulk simulate across devices |
| GET/PUT | `/api/v1/tag-simulation` | Legacy global sim preference — [tag-simulation.md](../../docs/tag-simulation.md) |
| GET | `/api/v1/project.xlsx` | Export project workbook |
| POST | `/api/v1/project/preview` | Preview import |
| POST | `/api/v1/project/import` | Import project |
| POST | `/api/v1/project/validate` | Validate against Address Space |
| POST | `/api/v1/project/compare` | Compare JSON |
| POST | `/api/v1/project/compare.xlsx` | Compare → xlsx |
| GET | `/api/v1/diagnostics/logs?category=&errors_only=&limit=` | Ring log (`opc_read` / `opc_write` / `db_write`) |
| DELETE | `/api/v1/diagnostics/logs` | Clear ring log |
| POST | `/api/v1/diagnostics/reset` | Clear ring log + Overview drop counters |
| GET | `/api/v1/diagnostics/capacity` | DB size, ETA, capacity fields |
| GET | `/api/v1/database/status` | DB connection / size summary |
| GET | `/api/v1/database/capacity-policy` | Capacity percent + full-disk policy |
| PUT | `/api/v1/database/capacity-policy` | Persist policy to YAML |
| POST | `/api/v1/database/wipe-samples?confirm=wipe` | Wipe historian samples; optional `{"clear_tags":true}` |

### OpenAPI / Swagger

- **`/docs` + [`api/openapi.yaml`](../../api/openapi.yaml) v1.2** — full collector HTTP surface (integration + admin), embedded in the binary.
- Narrative clients guide: [docs/external-client-api.md](../../docs/external-client-api.md). Models: [docs/l2-model-integration.md](../../docs/l2-model-integration.md). Capacity: [docs/db-capacity-policy.md](../../docs/db-capacity-policy.md).

## PLC on

Set `LEVEL2_SIM_BROWSER=0`, copy OPC credentials from smoke (or fill `.env`):

```bash
python3 deploy/platform/sync_opc_from_smoke.py   # VM lab helper
docker compose up -d --force-recreate
curl -s http://127.0.0.1:8080/readyz
curl -s 'http://127.0.0.1:8080/api/v1/tags?device_id=s7_1500' | head
```

Stop Telegraf smoke input to avoid duplicate writes: `docker stop level2-telegraf`.

Confirm leaf `ns=4;i=4208` writes to `collector.samples`.

### Acceptance scripts

```bash
cd ~/level2/deploy/platform
chmod +x verify_offline.sh verify_plc_on.sh verify_all.sh

# PLC off only (sim browser + demo samples)
./verify_offline.sh

# PLC on only (live S7-1500)
./verify_plc_on.sh

# Both modes in sequence (toggles LEVEL2_SIM_BROWSER in .env)
./verify_all.sh both
```

Run unit tests in Docker (no local Go required):

```bash
cd ~/level2
docker run --rm -e GOTOOLCHAIN=auto -v "$PWD":/src -w /src golang:1.24 go test ./...
```

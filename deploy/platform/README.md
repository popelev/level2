# Level2 platform collector (M1–M4)

Go service: OPC UA leaf tags → TimescaleDB (`collector.samples`) + REST/WS API + Admin UI.

Capacity / free disk: collector mounts the smoke Timescale volume (`smoke_timeseries`) read-only at `/var/lib/level2/dbdisk` and uses **one Statfs** on that path for both `disk_total_bytes` and `disk_avail_bytes`. Soft limit: `database.capacity_percent` (byte limit = disk_total × percent / 100). `free_bytes` is room under that limit (capped by volume free). Full-disk policy: `database.full_policy` (`stop` | `drop_oldest` | `rotate` | `expand_limit`). Env overrides: `LEVEL2_DB_CAPACITY_PERCENT`, `LEVEL2_DB_FULL_POLICY`, absolute `LEVEL2_DB_CAPACITY_BYTES`. Details: [docs/db-capacity-policy.md](../../docs/db-capacity-policy.md).

## PLC off

```bash
cd ~/level2
docker run --rm -e GOTOOLCHAIN=auto -v "$PWD":/src -w /src golang:1.24 go test ./...

cd deploy/platform
cp -n config.example.yaml config.yaml
cp -n .env.example .env
# LEVEL2_SIM_BROWSER=1 enables Browse/Expand + synthetic samples without PLC
docker compose build
docker compose up -d
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/api/v1/tags
curl -s 'http://127.0.0.1:8080/api/v1/browse?node_id=ns%3D4%3Bi%3D4207'
# UI: http://<vm-ip>:8080/
# ready should be OK in demo mode; values appear within ~1s
```

## API

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/v1/tags` | configured tags + live sample |
| GET | `/api/v1/tags/{id}/value` | last sample |
| GET | `/api/v1/tags/{id}/history?from=&to=&limit=` | Timescale |
| GET | `/api/v1/devices` | device list |
| GET | `/api/v1/browse?node_id=` | OPC browse (or sim) |
| POST | `/api/v1/expand` | `{"node_id","parent_tag_id","max_depth"}` |
| GET | `/api/v1/devices/{id}/tags.xlsx` | export plant-format tag list |
| POST | `/api/v1/devices/{id}/tags/import` | multipart `file=.xlsx` (`?replace=1` to replace tags) |
| POST | `/api/v1/devices/{id}/tags` | upsert one tag |
| POST | `/api/v1/devices/{id}/tags/sync` | refresh datatypes from OPC by NodeId (`{"tag_ids":[]}` optional = all) |
| DELETE | `/api/v1/devices/{id}/tags` | remove all tags on device |
| PUT | `/api/v1/tags/{id}/value` | 501 until write phase — [docs/opc-write-mode.md](../../docs/opc-write-mode.md) |
| GET | `/api/v1/ws/stream` | live samples WebSocket |
| GET | `/api/v1/diagnostics/logs?category=&errors_only=&limit=` | OPC/DB ring log |
| DELETE | `/api/v1/diagnostics/logs` | clear ring log |
| GET | `/api/v1/diagnostics/capacity` | DB size, ETA, capacity policy fields |
| GET | `/api/v1/database/capacity-policy` | capacity percent + full-disk policy |
| PUT | `/api/v1/database/capacity-policy` | persist policy to YAML |
| GET | `/metrics` | Prometheus |

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


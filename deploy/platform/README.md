# Level2 platform collector (M1–M4)

Go service: OPC UA leaf tags → TimescaleDB (`collector.samples`) + REST/WS API + Admin UI.

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
| PUT | `/api/v1/tags/{id}/value` | 501 until write phase |
| GET | `/api/v1/ws/stream` | live samples WebSocket |
| GET | `/metrics` | Prometheus |

## PLC on

Set `LEVEL2_SIM_BROWSER=0`, fill OPC credentials like smoke, confirm leaf `ns=4;i=4208` writes to `collector.samples`.

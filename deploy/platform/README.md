# Level2 platform collector (M1)

Go service: OPC UA leaf tags → TimescaleDB (`collector.samples`).

## PLC off (this milestone)

Build and unit-test without a real PLC:

```bash
cd ~/level2
docker run --rm -e GOTOOLCHAIN=auto -v "$PWD":/src -w /src golang:1.24 go test ./...
```

## Run next to smoke stack

Smoke Timescale must already be up (`deploy/smoke`).

```bash
cd ~/level2/deploy/platform
cp .env.example .env
# fill OPC_UA_USERNAME / OPC_UA_PASSWORD like smoke
cp config.example.yaml config.yaml
# discover smoke network name if needed:
# docker network ls | grep smoke
sudo docker compose build
sudo docker compose up -d
sudo docker compose logs -f collector
curl -s http://127.0.0.1:8080/healthz
```

Grafana SQL (after PLC on and data flows):

```sql
SELECT time, tag_id, value_num, quality
FROM collector.samples
ORDER BY time DESC
LIMIT 50;
```

## Config notes

- Only **leaf** scalars: float64, int64, bool, string
- Custom UDT parent nodes (e.g. `ns=4;i=4207`) are rejected with a clear error — expand in M2

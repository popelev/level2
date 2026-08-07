# Smoke: OPC UA → Telegraf → TimescaleDB → Grafana

**Legacy connectivity stack** to verify the plant path to **S7-1500 OPC UA**.  
The primary collector is the Go service in [`deploy/platform/`](../platform/README.md) (same Timescale volume/network). Keep Timescale (+ Grafana) from this compose; stop Telegraf when the platform collector is writing (`docker stop level2-telegraf`) to avoid duplicate samples.

## Services

| Container    | Port  | Purpose                             |
|--------------|-------|-------------------------------------|
| timescaledb  | 5432  | History (PostgreSQL + Timescale)    |
| telegraf     | —     | OPC UA read → Timescale write       |
| grafana      | 3000  | Charts                              |

## Before start

1. In TIA: enable OPC UA Server on S7-1500 with 1–2 test tags.
2. On Windows: **UaExpert** → `opc.tcp://<PLC-IP>:4840` with the same **username/password** and Security as on the PLC → copy **NodeId**.
3. From the VM: `ping <PLC-IP>` must work (Bridged NIC for PLC access).

## Commands on the Ubuntu VM

```bash
cd ~/level2/deploy/smoke
cp .env.example .env
nano .env
```

Set in `.env`:

- `PLC_OPC_ENDPOINT=opc.tcp://10.14.10.16:4840` — your PLC IP
- `OPC_UA_USERNAME` / `OPC_UA_PASSWORD` — **same** as in UaExpert
- DB and Grafana passwords (lab defaults are fine)

If UaExpert uses **Basic256Sha256** + **Sign & Encrypt**, `telegraf.conf` is already set that way.
On first start Telegraf creates a client certificate; you must **trust it on the S7-1500** (see Certificates below).

Then set NodeIds in Telegraf config:

```bash
nano telegraf/telegraf.conf
```

Find `[[inputs.opcua]]` / `nodes` and replace placeholders with NodeIds from UaExpert  
(example: `{name="temp", id="ns=3;s=\"MyDB\".\"Temp\"}"`).

Start:

```bash
docker compose pull
docker compose up -d
docker compose ps
docker compose logs -f telegraf
```

Leave logs with `Ctrl+C`.

Grafana in the browser (Windows or VM):

```text
http://<VM-IP>:3000
```

Login: `admin` / password from `.env` (`GF_SECURITY_ADMIN_PASSWORD`).

Datasource **TimescaleDB** is provisioned.  
Explore → TimescaleDB → table like `opcua` (Telegraf measurement name).

Provisioned dashboards (folder **Level2**):

- **Level2 Trends** — smoke / legacy example series
- **Plant Overview** — multi-select structures → repeating rows with Value / Unit / Min / Max gauges — see [grafana/README-opc-structure.md](grafana/README-opc-structure.md)
- **OPC Structure Measure** — template: pick structure root once → gauge / unit / min / max / trend for `rValueOut`/`realValue` + scale

```text
http://<VM-IP>:3000/d/level2-plant-overview/plant-overview
http://<VM-IP>:3000/d/level2-opc-structure/opc-structure-measure
```

## Update from git

```bash
cd ~/level2
git pull
cd deploy/smoke
docker compose up -d
```

## Stop

```bash
cd ~/level2/deploy/smoke
docker compose down
```

DB data stays in Docker volume `smoke_timeseries` (until `down -v`).

## Certificates (Sign & Encrypt)

If the PLC uses **Basic256Sha256** + **Sign & Encrypt**, Telegraf needs a client certificate.

**Once** on the VM (from `deploy/smoke`):

```bash
sudo apt install -y openssl
mkdir -p telegraf/opcua
openssl req -x509 -newkey rsa:2048 \
  -keyout telegraf/opcua/key.pem \
  -out telegraf/opcua/cert.pem \
  -days 825 -nodes \
  -subj "/CN=level2-telegraf-opcua"
chmod 644 telegraf/opcua/cert.pem
chmod 600 telegraf/opcua/key.pem
```

Then:

```bash
sudo docker compose up -d --force-recreate telegraf
sudo docker compose logs -f telegraf
```

Copy the cert to Windows and **trust it on the S7-1500** (TIA Online → OPC UA Certificates → Trusted):

```bash
cp telegraf/opcua/cert.pem ~/telegraf-opcua-cert.pem
```

Without trust on the PLC: `BadSecurityChecksFailed` / certificate untrusted.

## If Telegraf cannot connect

| Symptom | Check |
|---------|--------|
| timeout / dial | PLC IP, Bridged, `ping`, port 4840 |
| BadIdentityTokenInvalid | Username/password in `.env` match UaExpert |
| BadSecurityChecksFailed / Untrusted | Trust Telegraf cert on S7-1500 (see above) |
| Bad NodeId | Re-copy NodeId from UaExpert |
| UaExpert OK, Telegraf not | Same endpoint, Security, user; logs: `docker compose logs telegraf` |

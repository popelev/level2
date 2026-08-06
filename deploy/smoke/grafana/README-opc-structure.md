# OPC Structure Measure — Grafana template

RU / EN guide for the provisioned dashboard **OPC Structure Measure**.

## Files / Файлы

| Path | Role |
|------|------|
| `deploy/smoke/grafana/provisioning/dashboards/json/level2-opc-structure.json` | Template dashboard |
| `deploy/smoke/grafana/provisioning/dashboards/dashboards.yaml` | Provider (folder **Level2**) |
| `deploy/smoke/grafana/provisioning/datasources/timescaledb.yaml` | TimescaleDB datasource |

Grafana image: `grafana/grafana:11.3.1` (`deploy/smoke/docker-compose.yml`).

**Library panels:** Grafana file provisioning does **not** support library panels. This dashboard is a reusable **template**: duplicate it (Save as) and set the `structure` variable. Optionally, after Save as, create a Library panel from the gauge in the UI.

## URL (lab VM)

```text
http://<VM-IP>:3000/d/level2-opc-structure/opc-structure-measure
```

Folder: **Level2**. Login: `admin` / `GF_SECURITY_ADMIN_PASSWORD` from `deploy/smoke/.env`.

## How to use / Как пользоваться

### RU

1. Откройте дашборд **OPC Structure Measure**.
2. Вверху выберите переменную **Structure** — корень OPC-структуры (один раз).
3. Панели сами подтянут дочерние теги значения, шкалы и единицы.
4. Для отдельного измерения: **Share → Save as** (или Duplicate), задайте другое значение **Structure**.

Можно вставить sanitized prefix вручную (**allow custom value**), если тега ещё нет в выборке за последний час.

### EN

1. Open **OPC Structure Measure**.
2. Set dashboard variable **Structure** once (structure root prefix).
3. Panels auto-resolve child series for value / min / max / unit.
4. Clone via **Save as**, change **Structure** for the next instrument.

## Tag naming (`sanitizeTagID`)

Collector IDs are lowercased; spaces / `.` / `-` / `/` → `_` (`internal/driver/opcua/browse.go`).

| Browse name (OPC) | Sanitized suffix | Notes |
|-------------------|------------------|--------|
| `rValueOut` | `_rvalueout` | Siemens / plant (lab) |
| `realValue` | `_realvalue` | Generic English |
| `strScale.min` | `_strscale_min` | Siemens nested scale |
| `strScale.max` | `_strscale_max` | |
| `min Scale` | `_min_scale` | Generic (space → `_`) |
| `max Scale` | `_max_scale` | |
| `sUnit` | `_sunit` | Siemens |
| `unit` | `_unit` | Generic |

Example (lab): structure

`objects_serverinterfaces_tankhouse_data_2_e3_machines_metso_machines_apm_oil_temp`

→ tags `…_rvalueout`, `…_strscale_min`, `…_strscale_max`, `…_sunit`.

SQL matches **both** naming schemes via `IN (...)`.

## Panels

- **Gauge** — last `value`; min/max from scale tags (`configFromData`).
- **Unit / Min / Max** — last sample stats (`value_text` for unit).
- **Resolved tags** — which child `tag_id`s were found (last hour).
- **Trend** — value + min/max scale over time.

## Apply after git pull (level2-vm)

```bash
cd ~/level2
git pull
cd deploy/smoke
docker compose up -d --force-recreate grafana
# or: docker restart level2-grafana  (volume mount usually reloads within updateIntervalSeconds=30)
```

Provisioning polls JSON every 30s; recreate/restart if the new file does not appear.

# OPC Structure Measure — Grafana template

RU / EN guide for the provisioned dashboard **OPC Structure Measure**.

## Files / Файлы

| Path | Role |
|------|------|
| `deploy/smoke/grafana/provisioning/dashboards/json/level2-opc-structure.json` | Single-structure template |
| `deploy/smoke/grafana/provisioning/dashboards/json/level2-plant-overview.json` | Multi-structure plant overview |
| `deploy/smoke/grafana/provisioning/dashboards/dashboards.yaml` | Provider (folder **Level2**) |
| `deploy/smoke/grafana/provisioning/datasources/timescaledb.yaml` | TimescaleDB datasource |

Grafana image: `grafana/grafana:11.3.1` (`deploy/smoke/docker-compose.yml`).

**Library panels:** Grafana file provisioning does **not** support library panels. The single-structure dashboard is a reusable **template**: duplicate it (Save as) and set the `structure` variable. Optionally, after Save as, create a Library panel from the gauge in the UI.

## URL (lab VM)

```text
http://<VM-IP>:3000/d/level2-plant-overview/plant-overview
http://<VM-IP>:3000/d/level2-opc-structure/opc-structure-measure
```

Folder: **Level2**. Login: `admin` / `GF_SECURITY_ADMIN_PASSWORD` from `deploy/smoke/.env`.

## Plant Overview / Обзор установок

Dashboard **Plant Overview** lists several OPC structure prefixes at once. Each selected structure gets a repeating row: gauge (value + scale) + unit + min/max, with a data link to **OPC Structure Measure**.

**Row JSON quirk (Grafana):** nested panels under a row are loaded only when `collapsed: true`. For an expanded repeating row (`collapsed: false`), Value/Unit/Min/Max must be **siblings after the row** in the top-level `panels` array (row itself keeps `"panels": []`). Nested panels + `collapsed: false` → UI shows `(0 panels)`.

### How to add structures / Как добавить структуры

### RU

1. Откройте **Plant Overview**.
2. Вверху переменная **Structures** (multi-select) — выберите нужные префиксы или **All**.
3. Под каждым заголовком структуры сразу видны Value / Unit / Min / Max.
4. Чтобы **добавить новый** префикс в список:
   - **Dashboard settings → Variables → Structures** → **Custom options** в формате `shortlabel : full_prefix` (через запятую) → **Update** → **Save dashboard**.
   - Либо правьте `query` / `options` в `level2-plant-overview.json` в git (provisioning перезапишет UI-правки при recreate).
5. Клик по Value / Unit → drill-down на `level2-opc-structure` с `var-structure=…`.

Префиксы — sanitized `tag_id` без суффикса `_rvalueout` / `_realvalue` (см. таблицу ниже). Примеры из lab уже засеяны (oil temp/level, air vessel, cathode mass/current).

### EN

1. Open **Plant Overview**.
2. Use multi-select **Structures** (or **All**).
3. Gauges for each structure appear under the repeating row titles.
4. To **add** a prefix: **Dashboard settings → Variables → Structures → Custom options** (`shortlabel : full_prefix`, comma-separated), save; or edit the provisioned JSON in git so recreate keeps it.
5. Panel links open **OPC Structure Measure** with that structure.

## How to use — OPC Structure Measure / Шаблон одной структуры

### RU

1. Откройте дашборд **OPC Structure Measure**.
2. Вверху выберите переменную **Structure** — корень OPC-структуры (один раз).
3. Панели сами подтянут дочерние теги значения, шкалы и единицы.
4. Для отдельного измерения: **Share → Save as** (или Duplicate), задайте другое значение **Structure**.

Можно вставить sanitized prefix вручную (**allow custom value**), если тега ещё нет в выборке за последний час. Несколько установок сразу — см. **Plant Overview** выше.

### EN

1. Open **OPC Structure Measure**.
2. Set dashboard variable **Structure** once (structure root prefix).
3. Panels auto-resolve child series for value / min / max / unit.
4. Clone via **Save as**, change **Structure** for the next instrument. For many at once, use **Plant Overview**.

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
- **Unit / Min / Max** — last sample stats (`value_text` for unit; `value_num` for scales).
- **Trend** — value over time (min/max scale stay on Stat panels / gauge only).

### Why Unit can show "No data"

Historian already stores Siemens `sUnit` in `collector.samples.value_text` (e.g. `barg`). Collector string write path is fine.

Grafana **Stat** with empty `reduceOptions.fields` reduces **numeric fields only**, so a string `unit` column becomes "No data". Fix: `fields: "/.*/"` + `textMode: "value"`.

### Short labels / Короткие подписи

Dropdown and row titles hide the common `objects_serverinterfaces_` prefix (`${structure:text}`). SQL still uses the **full** sanitized prefix as the variable value.

### Apply after git pull (level2-vm)

```bash
cd ~/level2
git pull
cd deploy/smoke
docker compose up -d --force-recreate grafana
# or: docker restart level2-grafana  (volume mount usually reloads within updateIntervalSeconds=30)
```

Provisioning polls JSON every 30s; recreate/restart if the new file does not appear.

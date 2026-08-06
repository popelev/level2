# Database capacity policy

When TimescaleDB growth approaches the configured disk fraction, the collector applies a **full-disk policy** before each `WriteBatch`.

## Settings

| Key | Where | Values | Default |
|-----|--------|--------|---------|
| `capacity_percent` / `db_capacity_percent` | YAML `database.capacity_percent` or env `LEVEL2_DB_CAPACITY_PERCENT` | 1–100 | `90` |
| `full_policy` / `db_full_policy` | YAML `database.full_policy` or env `LEVEL2_DB_FULL_POLICY` | see below | `stop` |

Optional absolute override (unchanged): `LEVEL2_DB_CAPACITY_BYTES` — when set, it is the byte ceiling (percent still stored for UI).

### Byte limit

```
limit_bytes = disk_total × capacity_percent / 100
```

`disk_total` and `disk_avail` come from the same Statfs on the mounted DB volume (`LEVEL2_DB_DATA_PATH`, default `/var/lib/level2/dbdisk` — compose binds `smoke_timeseries`, i.e. Timescale `/var/lib/postgresql/data`).

`free_bytes` = room under `limit_bytes` (also capped by `disk_avail`). Source: `under_limit` | `disk_avail` | `env_limit`. Postgres `database_size_bytes` is logical size on that volume.

### Policies

| Policy | Behavior (Phase 1) |
|--------|--------------------|
| `stop` | Skip `WriteBatch`, log diagnostics, increment `level2_historian_capacity_halts_total`. **Do not spool.** |
| `drop_oldest` | Prefer Timescale `drop_chunks` on `collector.samples`; fallback `DELETE … WHERE time < …`. Then write. |
| `rotate` | **Phase 2 stub** — same as `stop`, UI explains rotation is not implemented. |
| `expand_limit` | User raises the capacity slider; until then same as `stop`. |

## UI

**Capacity** page: slider 1–100%, radio for policy, computed byte limit, Save → `PUT /api/v1/database/capacity-policy` (persists YAML via config store).

**Database** page: read-only summary of the current policy.

## API

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/v1/diagnostics/capacity` | sizes, ETA, `capacity_percent`, `full_policy`, `limit_bytes`, `used_over_limit` |
| GET | `/api/v1/database/capacity-policy` | current policy + limit |
| PUT | `/api/v1/database/capacity-policy` | `{"capacity_percent":90,"full_policy":"stop"}` |

## Example YAML

```yaml
database:
  url: "postgres://…"
  capacity_percent: 90
  full_policy: drop_oldest
```

See also [deploy/platform/README.md](../deploy/platform/README.md).

## Lab wipe samples

`POST /api/v1/database/wipe-samples?confirm=wipe` truncates `collector.samples`.

Because Phase 1 FanIn suppresses historian writes when value/quality match the Live store, a bare truncate would leave Timescale empty until some tag actually changes. After a successful wipe the collector therefore:

1. Snapshots all Live samples.
2. Clears the Live store (so the next poll is a “first sample”).
3. Writes the snapshot back with `WriteBatch` (`reseeded`, `live_cleared` in the JSON response; `reseed_error` if the batch fails — Live stays cleared so the next poll still refills).

Optional body `{"clear_tags":true}` also clears monitored tags in config (same as Projects → Clear tags). Details on on-change suppress: [opc-subscription-mode.md](opc-subscription-mode.md) Phase 1.

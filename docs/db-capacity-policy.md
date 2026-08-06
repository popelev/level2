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

`disk_total` comes from Statfs on the mounted DB volume (`LEVEL2_DB_DATA_PATH`, default `/var/lib/level2/dbdisk`).

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

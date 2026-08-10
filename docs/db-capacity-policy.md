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

| Policy | Behavior |
|--------|----------|
| `stop` | Skip `WriteBatch`, log diagnostics, increment `level2_historian_capacity_halts_total`. **Do not spool.** |
| `drop_oldest` | See algorithm below. |
| `rotate` | **Phase 2 stub** — same as `stop`, UI explains rotation is not implemented. |
| `expand_limit` | User raises the capacity slider; until then same as `stop`. |

### `drop_oldest` algorithm

1. **Proactive trim** when `used ≥ 90%` of `limit_bytes` (not only when over the hard limit).
2. Free toward a **target** of `85%` of limit: estimate oldest chunk sizes via `timescaledb_information.chunks` + `pg_total_relation_size`, then one `drop_chunks(older_than ⇒ …)` covering **N** oldest chunks (proportional to overrun, cap 64/pass). Fallback: single-chunk / `DELETE` slice.
3. If after trim `used < limit` → allow `WriteBatch`.
4. If still over limit but trim made (or can make) progress → return **`ErrCapacityBusy`**: flush path **spools** the batch; replay waits until under limit. No halt, no data loss.
5. **Halt** (`ErrCapacityHalt`, no spool) only when nothing left to drop and size is still over the hard limit (or size re-check fails with zero progress).

`pg_database_size` may lag after drops (bloat); estimated freed bytes count as progress so the collector spools instead of falsely halting.

## UI

**Capacity** page: slider 1–100%, radio for policy, computed byte limit, Save → `PUT /api/v1/database/capacity-policy` (persists YAML via config store).

**Database** page: read-only summary of the current policy.

## API

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/v1/diagnostics/capacity` | sizes, ETA, `capacity_percent`, `full_policy`, `limit_bytes`, `used_over_limit` |
| GET | `/api/v1/database/capacity-policy` | current policy + limit |
| PUT | `/api/v1/database/capacity-policy` | `{"capacity_percent":90,"full_policy":"stop"}` |

OpenAPI documents the same routes (v1.2.1+); see `/docs`.

## Example YAML

```yaml
database:
  url: "postgres://…"
  capacity_percent: 90
  full_policy: drop_oldest
```

See also [deploy/platform/README.md](../deploy/platform/README.md).

## Lab runbook (capacity hygiene)

Lab VM (`level2-vm`, `~/level2/deploy/platform`) often runs a large tag set. A tiny `capacity_percent` (e.g. **2%**) makes `used_over_limit=true`, flush returns `ErrCapacityBusy`, and **spool grows** while `samples_written_total` stalls — even with `drop_oldest` trimming.

### Healthy lab defaults

| Setting | Lab recommendation | Why |
|---------|-------------------|-----|
| `capacity_percent` | **40–90** (not 1–5) | Soft ceiling with headroom for on-change + reseed; 2% of a ~40 GiB volume ≈ 800 MiB and fills quickly |
| `full_policy` | **`drop_oldest`** | Trim + spool while busy; prefer over `stop` so iterations are not blocked |
| Wipe | occasional reset | Clears historian backlog; **not** the only long-term control |

Check pressure:

```bash
curl -s http://127.0.0.1:8080/api/v1/diagnostics/capacity
# watch: used_over_limit, free_bytes, spool_depth, samples_written_total, capacity_halts_total
curl -s http://127.0.0.1:8080/api/v1/status/summary   # spool_depth, samples_per_sec
```

### When lab is blocked (`used_over_limit` + growing spool)

1. **Raise percent** (preferred first step — policy should keep up without perpetual wipe):

   ```bash
   curl -sS -X PUT http://127.0.0.1:8080/api/v1/database/capacity-policy \
     -H 'Content-Type: application/json' \
     -d '{"capacity_percent":40,"full_policy":"drop_oldest"}'
   ```

   Persists into `config.yaml` via the config store.

2. **Wipe + reseed** if Timescale is already far over a sensible limit or you need a clean historian for the next iteration (see below).

3. **Clear spool** only after an intentional wipe when you do **not** need replay of deferred pre-wipe batches (otherwise replay re-inflates the DB for hours):

   ```bash
   cd ~/level2/deploy/platform
   docker compose stop collector
   docker volume rm "$(docker volume ls -q | grep -i collector_spool | head -1)"
   docker compose up -d collector
   ```

4. **Deploy fresh main** if the collector image predates multi-chunk `drop_oldest` / spool-on-busy:

   ```bash
   cd ~/level2 && git pull --ff-only
   cd deploy/platform && docker compose build collector && docker compose up -d collector
   ```

5. Optional host hygiene (not Timescale policy): prune unused Docker images after large CI/lab rebuilds — `docker image prune -f` (do not remove the running Timescale volume).

### Success signals

- `used_over_limit=false`, `free_bytes > 0`
- `samples_written_total` / `samples_per_sec` increasing
- `spool_depth` stable or falling; `capacity_halts_total` not climbing under `drop_oldest`

## Lab wipe samples

`POST /api/v1/database/wipe-samples?confirm=wipe` truncates `collector.samples`.

Because Phase 1 FanIn suppresses historian writes when value/quality match the Live store, a bare truncate would leave Timescale empty until some tag actually changes. After a successful wipe the collector therefore:

1. Snapshots all Live samples.
2. Clears the Live store (so the next poll is a “first sample”).
3. Writes the snapshot back with `WriteBatch` (`reseeded`, `live_cleared` in the JSON response; `reseed_error` if the batch fails — Live stays cleared so the next poll still refills).

Optional body `{"clear_tags":true}` also clears monitored tags in config (same as Projects → Clear tags). Details on on-change suppress: [opc-subscription-mode.md](opc-subscription-mode.md) Phase 1.

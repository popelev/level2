# OPC datatype resolution and Sync

How Level2 assigns `tag.datatype` (`bool` | `int64` | `uint` | `float64` | `string` | `datetime`) when **adding** tags from Address Space and when running **Sync from OPC**.

**Status:** implemented (batched DataType Attribute Read + Siemens name refine).

Related: on-change historian — [opc-subscription-mode.md](opc-subscription-mode.md); PLC write — [opc-write-mode.md](opc-write-mode.md).

---

## 1. Goals

1. Prefer the OPC UA **DataType** attribute (batched Read, ≤100 nodes) over name guessing.
2. **Sync always overwrites** configured `datatype` for the selected tags (or all tags on the device).
3. Show expand/add **progress** (`Browsing…`, `Reading datatypes N/M…`, `Writing tags…`) — no silent Guess-only path for bulk add.
4. Refine known Siemens mismatches where Attribute type ≠ runtime Value encoding.

---

## 2. Pipelines

### 2.1. Address Space → DB write list (add / expand)

1. Folder checkbox stays instant (selection only).
2. Prefetch / **Write selected to DB** expands the subtree, then **`fillExpandedDataTypes`**: batched `AttributeIDDataType` Read.
3. Each leaf: `resolveMappedDataType(opcMapped, browseHint)`.
4. Bulk upsert stores the resolved `datatype`.

Guess is used only when the Attribute Read fails or the type NodeId is unmapped (e.g. vendor `ns≠0`).

### 2.2. Sync from OPC (DB write list)

API: `POST /api/v1/devices/{id}/tags/sync`  
Body: `{"tag_ids":[]}` = all device tags; non-empty = only those ids.

UI: **Sync selected** / **Sync bad** / **Sync all**.

Implementation (`handleSyncTags` → `ApplyDataTypesFromOPC`):

1. Load tags from config.
2. Batched DataType Read for their NodeIds.
3. **Always set** `tag.DataType = resolveMappedDataType(...)` (does **not** keep an existing `float64` “because it looks valid”).
4. Persist via `SetDeviceTags` and restart poll for the device.

`updated` in the JSON response counts tags whose datatype **string changed**.

---

## 3. Mapping rules

| OPC DataType (ns=0) | Level2 |
|---------------------|--------|
| Boolean | `bool` |
| Float, Double | `float64` |
| signed integers | `int64` |
| unsigned integers / Byte | `uint` |
| String, LocalizedText, ByteString, XMLElement | `string` (ByteString may be refined → `datetime`) |
| DateTime, UtcTime | `datetime` |
| vendor `ns≠0` / read error | empty → **GuessDataType(name)** |

### Siemens name refine (`resolveMappedDataType`)

Plant servers often advertise the wrong Attribute type while the Value is still a Siemens encoding:

| Guess from name | OPC mapped | Result |
|-----------------|------------|--------|
| `datetime` | `string` or `float64` | **`datetime`** |
| `string` (e.g. `sUnit`) | `float64` | **`string`** |
| other | mapped | mapped as-is |

Name heuristics (`GuessDataType`) — fallback / refine only:

- **string:** `sUnit`, `unit`, `*_sunit`, `sName`, `sText`, …
- **datetime:** `dateandtime`, `datetime`, leaf `Time` / `*_time` (not `timeout` / `runtime` / `lifetime`)
- **bool:** `b*`, `*bool*`, `enable`, mode/maintenance flags, `Dcswitch`-style switches (see code)
- default: `float64`

---

## 4. Why Sync “didn’t fix” older tags

1. Sync overwrites config, but the **UI must refresh** (or re-open DB write list).
2. If Attribute Read returns a **plausible but wrong** type and the name does **not** match refine rules, Level2 keeps the OPC Attribute (by design for true analogs).
3. After datatype fix, quality may stay `bad` for one poll cycle until the next Read with the new decoder.

**User action after deploy:** **Sync bad** or **Sync all**, then hard-refresh the UI.

---

## 5. Tests

See `internal/driver/opcua/datatype_test.go`:

- `ApplyDataTypesFromOPC_OverwritesWrongFloat64` / `_SUnit`
- `resolveMappedDataType` ByteString / Float + DT / sUnit refine
- `GuessDataType` bare `Time`, `LastCycleDateAndTime`, `sUnit`

---

## 6. Operator checklist

| Step | Where |
|------|--------|
| Add large folder | Address Space — wait for datatype progress, then Write to DB |
| Fix wrong types | DB write list → Sync selected / Sync bad / Sync all |
| Wipe history (lab) | Config → Database → type `WIPE` |
| Jenkins CI | System → **Jenkins** (same host `:8081`) |

#!/bin/bash
# Acceptance checks for PLC-on (real OPC UA). Run on VM from deploy/platform.
set -euo pipefail
BASE="${BASE_URL:-http://127.0.0.1:8080}"
DEV="${DEVICE_ID:-s7_1500}"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "OK: $*"; }

echo "=== ensure leaf tag 4208 in DB list ==="
if ! curl -sf "$BASE/api/v1/tags?device_id=$DEV" | grep -q 'ns=4;i=4208'; then
  curl -sf -X POST "$BASE/api/v1/devices/$DEV/tags" \
    -H 'Content-Type: application/json' \
    -d '{"id":"opc_measure_rvalue","node_id":"ns=4;i=4208","datatype":"float64","enabled":true,"interval_ms":1000}' >/dev/null
  sleep 3
fi

echo "=== P6 Telegraf must be stopped (no duplicate writes) ==="
if docker ps --format '{{.Names}}' 2>/dev/null | grep -qx level2-telegraf; then
  fail "level2-telegraf is running — run: docker stop level2-telegraf"
fi
ok "Telegraf not running"

echo "=== readyz / devices (P1) ==="
code=$(curl -s -o /tmp/readyz.txt -w '%{http_code}' "$BASE/readyz")
[[ "$code" == "200" ]] || fail "readyz HTTP $code"
curl -sf "$BASE/api/v1/devices" | grep -q "\"id\":\"$DEV\"" || fail "device $DEV missing"
curl -sf "$BASE/api/v1/devices" | grep -q '"connected":true' || fail "device not connected"
ok "readyz 200, connected"

echo "=== live float 4208 (P1) ==="
TAG4208=$(curl -sf "$BASE/api/v1/tags?device_id=$DEV" | python3 -c "
import json,sys
for row in json.load(sys.stdin):
    t=row.get('tag',{})
    if t.get('node_id')=='ns=4;i=4208':
        print(t.get('id',''))
        break
")
[[ -n "$TAG4208" ]] || fail "no monitored tag for ns=4;i=4208 — add opc_measure_rvalue in Monitor"
q4208=$(curl -sf "$BASE/api/v1/tags?device_id=$DEV" | python3 -c "
import json,sys
tid='$TAG4208'
for row in json.load(sys.stdin):
    if row.get('tag',{}).get('id')==tid:
        print(row.get('sample',{}).get('quality',-1))
        break
")
[[ "$q4208" == "0" ]] || fail "$TAG4208 quality=$q4208 (want good)"
val=$(curl -sf "$BASE/api/v1/tags/${TAG4208}/value" | grep -o '"value_num":[0-9.eE+-]*' | head -1)
[[ -n "$val" ]] || fail "no value_num for $TAG4208"
ok "4208 live ($TAG4208) $val"

echo "=== structure 4207 read error (P2) ==="
curl -sf -X POST "$BASE/api/v1/devices/$DEV/tags" \
  -H 'Content-Type: application/json' \
  -d '{"id":"_verify_struct","node_id":"ns=4;i=4207","datatype":"float64","enabled":true,"interval_ms":1000}' >/dev/null
sleep 3
q=$(curl -sf "$BASE/api/v1/tags?device_id=$DEV" | python3 -c "
import json,sys
for row in json.load(sys.stdin):
    if row.get('tag',{}).get('id')=='_verify_struct':
        print(row.get('sample',{}).get('quality',-1))
        break
")
curl -sf -X DELETE "$BASE/api/v1/devices/$DEV/tags/_verify_struct" >/dev/null || true
[[ "$q" == "1" ]] || fail "structure node should poll as bad quality, got quality=$q"
ok "4207 structure → bad quality"

echo "=== string leaf 4209 if present (P3) ==="
if curl -sf "$BASE/api/v1/tags?device_id=$DEV" | grep -q 'ns=4;i=4209'; then
  curl -sf "$BASE/api/v1/tags?device_id=$DEV" | grep 'ns=4;i=4209' | grep -q 'value_text\|value_num' && ok "string tag sampled" || ok "string tag configured (value optional)"
else
  echo "SKIP P3: add ns=4;i=4209 to config for full string check"
fi

echo "=== expand UDT 4207 (P4) ==="
exp=$(curl -sf -X POST "$BASE/api/v1/expand" -H 'Content-Type: application/json' \
  -d "{\"device_id\":\"$DEV\",\"node_id\":\"ns=4;i=4207\",\"parent_tag_id\":\"verify\",\"max_depth\":4}")
echo "$exp" | grep -q '4208' || fail "expand missing leaf 4208"
ok "expand returns leaves"

echo "=== browse address space (P8) ==="
root=$(curl -sf "$BASE/api/v1/browse?device_id=$DEV&node_id=ns%3D0%3Bi%3D84")
echo "$root" | grep -q 'ns=0;i=85' || fail "root children need canonical ns=0 node ids"
objs=$(curl -sf "$BASE/api/v1/browse?device_id=$DEV&node_id=ns%3D0%3Bi%3D85")
echo "$objs" | grep -q 'Server\|DeviceSet\|PLC' || fail "Objects folder empty or wrong"
ok "browse Root → Objects"

echo "=== API history + metrics (P7) ==="
curl -sf "$BASE/api/v1/tags/${TAG4208}/history?limit=5" | grep -q 'time' || fail "history empty"
curl -sf -o /dev/null -w '%{http_code}' "$BASE/metrics" | grep -q 200 || fail "metrics"
curl -sf -o /dev/null -w '%{http_code}' "$BASE/" | grep -q 200 || fail "ui"
ok "history, metrics, ui"

echo "=== samples in Timescale (P1) ==="
docker exec level2-timescaledb psql -U level2 -d level2 -t -c \
  "SELECT count(*) FROM collector.samples WHERE tag_id='${TAG4208}' AND time > now() - interval '5 minutes';" \
  | tr -d ' ' | grep -qv '^0$' || fail "no recent rows in collector.samples for $TAG4208"
ok "DB rows for $TAG4208"

echo "ALL PLC-ON CHECKS PASSED"

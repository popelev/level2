#!/bin/bash
set -euo pipefail
cd ~/level2/deploy/platform
sleep 4
echo "=== healthz ==="
curl -sf http://127.0.0.1:8080/healthz; echo
echo "=== devices ==="
curl -sf http://127.0.0.1:8080/api/v1/devices; echo
echo "=== browse root ==="
curl -sf 'http://127.0.0.1:8080/api/v1/browse?node_id=ns%3D0%3Bi%3D85'; echo
echo "=== browse structure ==="
curl -sf 'http://127.0.0.1:8080/api/v1/browse?node_id=ns%3D4%3Bi%3D4207'; echo
echo "=== expand ==="
printf '%s' '{"node_id":"ns=4;i=4207","parent_tag_id":"tank"}' > /tmp/expand.json
curl -sf -X POST http://127.0.0.1:8080/api/v1/expand -H 'Content-Type: application/json' -d @/tmp/expand.json; echo
echo "=== ui ==="
curl -sf -o /dev/null -w 'ui:%{http_code}\n' http://127.0.0.1:8080/
echo "=== metrics ==="
curl -sf -o /dev/null -w 'metrics:%{http_code}\n' http://127.0.0.1:8080/metrics
echo OK

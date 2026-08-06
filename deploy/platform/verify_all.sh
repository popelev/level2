#!/bin/bash
# Run offline (sim) and live (PLC) acceptance when possible.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

run_off() {
  echo "========== PLC OFF (sim) =========="
  export LEVEL2_SIM_BROWSER=1
  grep -q '^LEVEL2_SIM_BROWSER=' .env 2>/dev/null && sed -i 's/^LEVEL2_SIM_BROWSER=.*/LEVEL2_SIM_BROWSER=1/' .env || echo 'LEVEL2_SIM_BROWSER=1' >> .env
  docker compose up -d --force-recreate collector
  sleep 8
  bash ./verify_offline.sh
}

run_on() {
  echo "========== PLC ON (live) =========="
  if [[ -x ./sync_opc_from_smoke.py ]] || [[ -f ./sync_opc_from_smoke.py ]]; then
    python3 ./sync_opc_from_smoke.py 2>/dev/null || true
  fi
  export LEVEL2_SIM_BROWSER=0
  grep -q '^LEVEL2_SIM_BROWSER=' .env && sed -i 's/^LEVEL2_SIM_BROWSER=.*/LEVEL2_SIM_BROWSER=0/' .env || echo 'LEVEL2_SIM_BROWSER=0' >> .env
  docker stop level2-telegraf 2>/dev/null || true
  docker compose up -d --force-recreate collector
  sleep 15
  bash ./verify_plc_on.sh
}

case "${1:-both}" in
  off) run_off ;;
  on) run_on ;;
  both) run_off; run_on ;;
  *) echo "usage: $0 [off|on|both]"; exit 1 ;;
esac

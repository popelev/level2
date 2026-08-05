# Level 2 — industrial data collection platform

# Now: OPC UA smoke (S7-1500) → Telegraf → TimescaleDB → Grafana
# Next: custom Go collector + Web Admin UI (replaces Telegraf)

# Repository: https://github.com/popelev/level2

## Quick start (Ubuntu VM)

1. Install Docker (see plan / already done on the lab VM).
2. Clone the repository:

```bash
git clone https://github.com/popelev/level2.git
cd level2
```

3. Follow [deploy/smoke/README.md](deploy/smoke/README.md).

## Layout

```
deploy/smoke/       # Telegraf smoke (working)
deploy/platform/    # Go collector + Admin UI
```

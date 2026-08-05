# Level 2 — промышленная платформа сбора данных
#
# Сейчас: smoke-тест OPC UA (S7-1500) → Telegraf → TimescaleDB → Grafana
# Дальше: свой Go-коллектор + Web Admin UI (замена Telegraf)
#
# Репозиторий: https://github.com/popelev/level2

## Быстрый старт (Ubuntu VM)

1. Установите Docker (см. план / уже сделано на стенде).
2. Клонируйте репозиторий:

```bash
git clone https://github.com/popelev/level2.git
cd level2/deploy/smoke
```

3. Следуйте инструкциям в [deploy/smoke/README.md](deploy/smoke/README.md).

## Структура

```
deploy/smoke/       # Telegraf smoke (работает)
deploy/platform/    # Go collector M1 (PLC-off scaffold)
cmd/collector/
internal/
```

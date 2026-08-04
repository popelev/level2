# Smoke: OPC UA → Telegraf → TimescaleDB → Grafana

Временный стенд для проверки связи с **S7-1500 OPC UA**.  
Потом Telegraf заменим своим Go-коллектором; Timescale и Grafana останутся.

## Что поднимается

| Контейнер    | Порт  | Назначение                          |
|--------------|-------|-------------------------------------|
| timescaledb  | 5432  | История (PostgreSQL + Timescale)    |
| telegraf     | —     | Чтение OPC UA, запись в Timescale   |
| grafana      | 3000  | Графики                             |

## Перед запуском

1. В TIA: OPC UA Server на S7-1500, security **None**, 1–2 тестовых тега.
2. На Windows: **UaExpert** → `opc.tcp://<IP-PLC>:4840` → скопировать **NodeId**.
3. С VM: `ping <IP-PLC>` должен проходить (для PLC сеть VM = Bridged).

## Команды на Ubuntu VM

```bash
cd ~/level2/deploy/smoke
cp .env.example .env
nano .env
```

В `.env` укажите:

- `PLC_OPC_ENDPOINT=opc.tcp://192.168.x.x:4840` — IP вашего PLC
- пароли БД и Grafana (можно оставить примеры для лабы)

Затем NodeId в конфиге Telegraf:

```bash
nano telegraf/telegraf.conf
```

Найдите секцию `[[inputs.opcua]]` / `nodes` и замените плейсхолдеры на NodeId из UaExpert  
(пример: `{name="temp", id="ns=3;s=\"MyDB\".\"Temp\"}"`).

Запуск:

```bash
docker compose pull
docker compose up -d
docker compose ps
docker compose logs -f telegraf
```

Выход из логов: `Ctrl+C`.

Grafana в браузере (Windows или VM):

```text
http://<IP-VM>:3000
```

Логин: `admin` / пароль из `.env` (`GF_SECURITY_ADMIN_PASSWORD`).

Datasource **TimescaleDB** уже прописан (provisioning).  
Explore → выбрать TimescaleDB → таблица вроде `opcua` (имя measurement из telegraf).

## Обновление из git

```bash
cd ~/level2
git pull
cd deploy/smoke
docker compose up -d
```

## Остановка

```bash
cd ~/level2/deploy/smoke
docker compose down
```

Данные БД сохраняются в Docker volume `smoke_timeseries` (пока не сделаете `down -v`).

## Если Telegraf не коннектится

| Симптом | Что проверить |
|---------|----------------|
| timeout / dial | IP PLC, Bridged, `ping`, порт 4840 |
| Bad security | В TIA и telegraf: None / Anonymous |
| Bad NodeId | Скопировать NodeId ещё раз из UaExpert |
| UaExpert ок, Telegraf нет | Тот же endpoint строкой, логи: `docker compose logs telegraf` |

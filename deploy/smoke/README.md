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

1. В TIA: OPC UA Server на S7-1500, 1–2 тестовых тега.
2. На Windows: **UaExpert** → `opc.tcp://<IP-PLC>:4840` с тем же **логином/паролем** и Security, что разрешены на PLC → скопировать **NodeId**.
3. С VM: `ping <IP-PLC>` должен проходить (для PLC сеть VM = Bridged).

## Команды на Ubuntu VM

```bash
cd ~/level2/deploy/smoke
cp .env.example .env
nano .env
```

В `.env` укажите:

- `PLC_OPC_ENDPOINT=opc.tcp://10.14.10.16:4840` — IP вашего PLC
- `OPC_UA_USERNAME` / `OPC_UA_PASSWORD` — **те же**, что в UaExpert
- пароли БД и Grafana (можно оставить примеры для лабы)

Если в UaExpert Security **Basic256Sha256** + **Sign & Encrypt** — в `telegraf.conf` уже так настроено.
Telegraf при первом старте создаст клиентский сертификат; его нужно **доверять на S7-1500** (см. ниже «Сертификаты»).

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

## Сертификаты (Sign & Encrypt)

UaExpert и Telegraf при **Sign & Encrypt** используют клиентский сертификат. PLC должен ему **доверять**.

1. Запустите Telegraf один раз (он создаст cert):
   ```bash
   docker compose up -d telegraf
   docker compose logs telegraf
   ```
2. Скопируйте сертификат на хост VM:
   ```bash
   docker cp level2-telegraf:/etc/telegraf/opcua/cert.pem ~/telegraf-opcua-cert.pem
   ```
3. На Windows (TIA / Online): OPC UA → Certificates / Trusted clients — **импортировать** `telegraf-opcua-cert.pem` и поместить в **Trusted**.
   (Либо Online: отклоненные сертификаты → Move to trusted.)
4. Перезапуск Telegraf:
   ```bash
   docker compose restart telegraf
   docker compose logs -f telegraf
   ```

Без шага «доверия» на PLC типичны ошибки вроде `BadSecurityChecksFailed` / `BadCertificateUntrusted`.

## Если Telegraf не коннектится

| Симптом | Что проверить |
|---------|----------------|
| timeout / dial | IP PLC, Bridged, `ping`, порт 4840 |
| BadIdentityTokenInvalid | Логин/пароль в `.env` как в UaExpert |
| BadSecurityChecksFailed / Untrusted | Доверие к сертификату Telegraf на S7-1500 (см. выше) |
| Bad NodeId | Скопировать NodeId ещё раз из UaExpert |
| UaExpert ок, Telegraf нет | Тот же endpoint, Security, user; логи: `docker compose logs telegraf` |

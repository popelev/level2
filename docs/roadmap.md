# Level2 — roadmap развития

Снимок: **август 2026**. Источник приоритетов — согласованные планы итераций A/B и бэклог Jira ([SCRUM](https://popelevfedor.atlassian.net/browse/SCRUM)).

Эпик платформы: **[SCRUM-5](https://popelevfedor.atlassian.net/browse/SCRUM-5)**.  
Эпик рефакторинга: **[SCRUM-18](https://popelevfedor.atlassian.net/browse/SCRUM-18)**.  
Эпик матмодели (внешний репо): **[SCRUM-17](https://popelevfedor.atlassian.net/browse/SCRUM-17)**.

---

## Цель платформы

Level2 — **стабильный plant gateway**: OPC UA collector → Live / TimescaleDB → REST/WS API + Admin UI.

| Слой | Ответственность |
|------|-----------------|
| Level2 | OPC, poll/reconnect, coerce, Live, historian, write gate, capacity, diag, Admin UI |
| Math / control model | Отдельный GitHub-репо и контейнер; только HTTP/WS к `LEVEL2_API_URL` |
| PLC | Авторитетные значения и AccessLevel |

Контракт API: [`api/openapi.yaml`](../api/openapi.yaml) (сейчас **v1.2.1**), `GET /api/v1/openapi.yaml`, UI `/docs`. Подробнее: [l2-model-integration.md](l2-model-integration.md), [external-client-api.md](external-client-api.md).

---

## Принцип «math-model снаружи»

- Код матмодели **не** живёт в репозитории Level2 и **не** вшивается в образ collector.
- В Level2 — только **контракт** (OpenAPI), чеклист интеграции и стабильный API.
- Реализация модели и Cursor-агент — в **отдельном** репо ([SCRUM-17](https://popelevfedor.atlassian.net/browse/SCRUM-17)).
- Bind по стабильному `tag_id`; write только при `enable_write` + `writable` (+ токены ролей — [SCRUM-15](https://popelevfedor.atlassian.net/browse/SCRUM-15)).

---

## Текущий статус (Done)

| Область | Что готово | Jira / docs |
|---------|------------|-------------|
| OPC Write MVP | `PUT` value + `enable_write` gate | [SCRUM-6](https://popelevfedor.atlassian.net/browse/SCRUM-6), [opc-write-mode.md](opc-write-mode.md) |
| OpenAPI / Swagger | Spec + `/docs`; расширение до full surface v1.2.1 | [SCRUM-7](https://popelevfedor.atlassian.net/browse/SCRUM-7) |
| Write Phase 2–3 | Batch write, `LEVEL2_API_TOKEN`, `writable`, WS filter | [SCRUM-8](https://popelevfedor.atlassian.net/browse/SCRUM-8) |
| Simulation | Per-tag sim + UI | [SCRUM-9](https://popelevfedor.atlassian.net/browse/SCRUM-9), [tag-simulation.md](tag-simulation.md) |
| Writable UI | Колонка + bulk | [SCRUM-10](https://popelevfedor.atlassian.net/browse/SCRUM-10) |
| Verify | Write-then-verify readback | [SCRUM-11](https://popelevfedor.atlassian.net/browse/SCRUM-11) |
| Status honesty | Stale Live / Quality при PLC off | [SCRUM-12](https://popelevfedor.atlassian.net/browse/SCRUM-12) |
| CI | Jenkins Test Result + Coverage | [SCRUM-13](https://popelevfedor.atlassian.net/browse/SCRUM-13) |
| Tests | Coverage ~90% + усиление assert’ов | [SCRUM-14](https://popelevfedor.atlassian.net/browse/SCRUM-14) |
| Historian on-change | Phase 1 suppress в FanIn | [opc-subscription-mode.md](opc-subscription-mode.md) |
| Capacity | `stop` / `drop_oldest` / `expand_limit`; spool при trim; wipe+reseed; lab runbook | [db-capacity-policy.md](db-capacity-policy.md), [SCRUM-19](https://popelevfedor.atlassian.net/browse/SCRUM-19) |
| Refactor waves 1–4 | api/cmd/core splits; Jenkins green | [SCRUM-18](https://popelevfedor.atlassian.net/browse/SCRUM-18) |

### Явно вне roadmap

| Тема | Почему не в плане |
|------|-------------------|
| Harness / testharness | Локальные эксперименты, не продукт |
| Coverage race / `main()` % | Не цель платформы |
| Код матмодели в Level2 | Нарушает принцип «снаружи» |

---

## Волна A — ближайшая (P0)

| # | Тема | Pri | Jira | Done-критерии |
|---|------|-----|------|---------------|
| A1 | **Lab capacity hygiene** | P0 | [SCRUM-19](https://popelevfedor.atlassian.net/browse/SCRUM-19) | ✅ Lab: `drop_oldest` + разумный `capacity_percent` (не 2%), wipe/reseed при backlog, spool clear после wipe; runbook в [db-capacity-policy.md](db-capacity-policy.md) |
| A2 | **TypeMismatch retry** на OPC write | P0 | [SCRUM-16](https://popelevfedor.atlassian.net/browse/SCRUM-16) | Один авто-retry с alternate width (float64→float32 и аналоги) при `BadTypeMismatch`; лог в diag; тесты + запись в [opc-write-mode.md](opc-write-mode.md) |
| A3 | **Split tokens/roles** (минимум write vs admin) | P0 | [SCRUM-15](https://popelevfedor.atlassian.net/browse/SCRUM-15) | ✅ `LEVEL2_API_TOKEN_WRITE` / `_ADMIN` (+ legacy shared); write≠admin; docs + OpenAPI v1.3.0; lab open при пустых токенах |

**Выход волны A:** безопасный write для внешней модели + контролируемый lab disk + минимальное разделение прав.

---

## Волна B — следующий горизонт

| # | Тема | Pri | Jira | Done-критерии |
|---|------|-----|------|---------------|
| B1 | **Contract pack / OpenAPI hygiene** для внешнего math-model | P1 | [SCRUM-17](https://popelevfedor.atlassian.net/browse/SCRUM-17) (+ Level2-чеклист [SCRUM-20](https://popelevfedor.atlassian.net/browse/SCRUM-20)) | В Level2: актуальный OpenAPI, чеклист интеграции, без кода модели. Реализация модели — только в отдельном репо |
| B2 | **Poll health metrics** + лёгкий Hub/WS coalesce | P1 | [SCRUM-21](https://popelevfedor.atlassian.net/browse/SCRUM-21) | Метрики poll latency / batch / errors видны в `/metrics` или status; WS не раздувает клиентов при статичном процессе (лёгкий coalesce, без ломки контракта) |
| B3 | **Capacity `rotate`**: реализовать **или** убрать stub | P3 | [SCRUM-22](https://popelevfedor.atlassian.net/browse/SCRUM-22) | Либо рабочий rotate + UI/docs, либо удаление из API/UI/`full_policy` + docs; хвост эпика [SCRUM-18](https://popelevfedor.atlassian.net/browse/SCRUM-18) |

**Выход волны B:** внешняя модель может опираться на чистый контракт; ops видит здоровье poll; capacity без ложных обещаний `rotate`.

---

## Волна C — позже (P2+)

| # | Тема | Pri | Jira | Done-критерии |
|---|------|-----|------|---------------|
| C1 | **OPC Subscription Phase 2** | P2 | [SCRUM-23](https://popelevfedor.atlassian.net/browse/SCRUM-23) | Реальный Subscription/MonitoredItems (или явный отказ с docs); fallback на poll; Siemens limits учтены — [opc-subscription-mode.md](opc-subscription-mode.md) |
| C2 | **Ops dashboard signals** | P2 | [SCRUM-24](https://popelevfedor.atlassian.net/browse/SCRUM-24) | UI/API сигналы: spool depth/ETA, verify fails, capacity pressure — без отдельного «мониторинг-продукта» |
| C3 | **UX write safety** | P2 | [SCRUM-26](https://popelevfedor.atlassian.net/browse/SCRUM-26) | UI подтверждения/предупреждения для опасных write; согласовано с gate + roles |
| C4 | **Hub/FanIn out of `api`** (точечно) | P3 | [SCRUM-25](https://popelevfedor.atlassian.net/browse/SCRUM-25) ⊂ [SCRUM-18](https://popelevfedor.atlassian.net/browse/SCRUM-18) | Перенос пакетов без смены семантики; не big-bang; Jenkins после волны |

---

## Сводка приоритетов

| Pri | Волны | Фокус |
|-----|-------|--------|
| **P0** | A | Lab disk, TypeMismatch, write vs admin tokens |
| **P1** | B | Контракт для модели, poll metrics / WS coalesce |
| **P2** | C | Subscription, ops signals, write UX |
| **P3** | B3, C4 | `rotate` decision, точечный refactor Hub/FanIn |

---

## Ссылки на docs

| Doc | Тема |
|-----|------|
| [l2-model-integration.md](l2-model-integration.md) | Math-model снаружи, closed loop |
| [external-client-api.md](external-client-api.md) | Внешний клиент API |
| [opc-write-mode.md](opc-write-mode.md) | Write gate, verify, TypeMismatch |
| [opc-subscription-mode.md](opc-subscription-mode.md) | Phase 1 suppress / Phase 2 subscribe |
| [db-capacity-policy.md](db-capacity-policy.md) | Capacity policies, wipe |
| [opc-datatype-sync.md](opc-datatype-sync.md) | DataType sync |
| [tag-simulation.md](tag-simulation.md) | Tag simulation |
| [../deploy/platform/README.md](../deploy/platform/README.md) | Lab deploy / API table |
| [../api/openapi.yaml](../api/openapi.yaml) | Canonical OpenAPI |

---

## Как обновлять этот файл

1. После закрытия карточки волны — строка Done + статус Jira.
2. Новые темы — сначала Jira под SCRUM-5 / SCRUM-17 / SCRUM-18, затем строка в таблице волны.
3. Не смешивать с Harness / coverage-гонка / код модели в этом репо.

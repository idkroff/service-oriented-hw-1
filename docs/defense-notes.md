# Defense Notes — HW7

Шпаргалка к защите. Все темы сгруппированы вокруг технологий, реально использованных в репозитории.

## 1. Observability: RED / USE / Golden Signals

- **RED (Rate / Errors / Duration)** — то, что я собираю с HTTP-сервисов: `http_requests_total`, `http_request_errors_total`, `http_request_duration_seconds`. Применяется к "request-driven" сервисам.
- **USE (Utilization / Saturation / Errors)** — для ресурсов (CPU, диск, БД). На уровне Postgres это видно в дашборде: connections (saturation), cache hit (utilization), deadlocks/temp files (errors).
- **Golden Signals (Google SRE)** — latency, traffic, errors, saturation. По сути RED + saturation.

## 2. Prometheus: histogram vs summary

- **Histogram** хранит counter для каждого bucket (`_bucket{le="0.1"}`, `_bucket{le="0.5"}`, `_count`, `_sum`). Перцентили считаются на сервере через `histogram_quantile(...)`, агрегируются по `sum by (le)`. Можно агрегировать кросс-инстансы.
- **Summary** считает перцентили на клиенте, агрегировать кросс-инстансы нельзя (нельзя суммировать перцентили).
- В проекте — histogram (`prometheus.DefBuckets`), потому что мы хотим p95 по всем инстансам сервиса.

Почему `histogram_quantile` принимает агрегированный `_bucket`, а не `_count`/`_sum`: ему нужна полная функция распределения по бакетам.

## 3. SLO / SLA / SLI / Error Budget

- **SLI** (Indicator) — измеряемая величина (например, доля 2xx).
- **SLO** (Objective) — целевое значение SLI (например, "99.5% за 30 дней").
- **SLA** (Agreement) — контрактное обязательство перед клиентом, обычно слабее SLO; нарушение влечёт компенсацию.
- **Error Budget** — `1 - SLO`. При 99.5% бюджет = 0.5% = ~22 минуты простоя в месяц. Когда бюджет выгорает — фриз релизов / фокус на надёжность.

В нашем CI пороги отказа (`< 95%`, `> 1000ms`) намеренно мягче SLO — это "система сломана", а не "SLO нарушен". CI должен падать только на жёстких отказах, не на каждом скачке.

## 4. k6 thresholds → exit code

- В `options.thresholds` задаются условия на метрики k6: `http_req_duration: ['p(95)<500']` — p95 < 500ms.
- Если хотя бы один threshold не выполнен, k6 завершается с **exit code 99**. CI воспринимает это как fail.
- `--summary-export` пишет финальный JSON, который сохраняется как артефакт.
- `checks` — это assert'ы внутри сценария (`check(res, {"200": r => r.status === 200})`); их success rate тоже можно превратить в threshold.

## 5. Alertmanager vs Grafana Alerts

Это **два разных движка**:

- **Prometheus rules + Alertmanager** — правила в `alerts.yml`, evaluation на Prometheus, маршрутизация в Alertmanager. Декларативные, in-repo, типовая прод-схема.
- **Grafana Unified Alerting** — отдельная подсистема Grafana. Может ссылаться на тот же Prometheus, но хранит правила в Grafana DB. Удобно для ad-hoc UI.

Я выбрал Prometheus rules, потому что: (а) "alerts-as-code" в git; (б) проще для CI; (в) Alertmanager — стандарт.

## 6. Грязные углы реализации

### chi RoutePattern в metrics middleware

`chi.RouteContext(r.Context()).RoutePattern()` возвращает шаблон маршрута (`/products/{id}`), но **только после** того, как chi сматчил route. Если читать его до `next.ServeHTTP`, получим пустую строку → бесконечное число серий с label `endpoint=""`.

Решение: читать `RoutePattern()` **после** `next.ServeHTTP`. См. `services/marketplace-api/internal/middleware/metrics.go`.

### /metrics и OpenAPI validator

`oapi-codegen` middleware валидирует каждый запрос по спецификации. Если зарегистрировать `/metrics` внутри группы с этим middleware, validator вернёт 4xx, потому что `/metrics` нет в OpenAPI. Решение: `r.Handle("/metrics", ...)` на корневом роутере **до** `r.Group(...)` с validator.

### Multi-DB Postgres вместо двух контейнеров

Init-script `infra/postgres/init/01-create-dbs.sql` создаёт обе БД при старте контейнера. Это позволяет:
- держать инфраструктуру простой (один контейнер);
- запускать миграции goose в каждой БД независимо (независимая `goose_db_version`);
- не возиться с advisory locks и конфликтами enum-типов между сервисами.

### k6 vs ORDER_RATE_LIMIT_MINUTES / HasActiveOrder

В prod-конфиге marketplace-api ограничивает: один заказ в минуту на пользователя + не более одного активного заказа. В нагрузочном тесте 10 VU × 60 секунд это убивает 99% запросов на POST /orders.

Решение в `docker-compose.test.yml`: `ORDER_RATE_LIMIT_MINUTES=0`. В k6 сценарии: на каждый VU свой user (создаётся в setup), после POST /orders сразу cancel — освобождает HasActiveOrder.

### Extract auth: почему именно так

Задание требует "взаимодействия между сервисами в integration-тесте". Простой способ — переместить аутентификацию в user-service:
- user-service выдаёт JWT (register/login/refresh) и валидирует через `POST /auth/validate`.
- marketplace-api на каждый authorized endpoint делает синхронный HTTP-call.
- В integration-тесте `POST /products` → marketplace-api → `POST /auth/validate` → user-service → OK.

Минусы: добавлен сетевой round-trip к p95 (5–10 мс), нет circuit breaker'а — для прода нужен.

## 7. Альтернативы и trade-off'ы

- **JWT валидация локально vs cross-service.** Локально дешевле (no RTT), но затрудняет ротацию ключей и расходится в правах. Cross-service дороже, но даёт центральный источник истины.
- **PostgreSQL exporter vs custom queries.** Дефолтный exporter покрывает `pg_stat_database`, `pg_stat_activity` — достаточно для USE-метрики. Для глубокой телеметрии (топ-N slow queries) нужен `pg_stat_statements` + custom_queries.yaml.
- **goose vs Atlas/Liquibase/Flyway.** goose минимален и встраивается в Go-бинарник через subprocess (`./goose ... up`).
- **Testcontainers vs docker-compose в integration.** Testcontainers — гибче (контейнеры из Go-кода), но добавляет зависимости и сложности с CI. compose-up в TestMain — проще, что я и выбрал.

## 8. Что часто спрашивают

**Q: Почему histogram, а не summary?**
A: Можно агрегировать перцентили по нескольким инстансам через `sum by (le)`. Summary даёт перцентили только per-instance.

**Q: Что делать при выгорании error budget?**
A: Заморозить feature-релизы, фокус на reliability work; постмортемы на каждый инцидент, пожирающий бюджет.

**Q: Почему `for: 5m` важно?**
A: Чтобы алерт не срабатывал на короткий всплеск ошибок (deploy, GC pause). `for` фильтрует флапающие правила.

**Q: Чем отличается scrape_interval от evaluation_interval?**
A: scrape — частота сбора метрик; evaluation — частота прогона recording/alerting rules. Должны быть согласованы: evaluation >= scrape.

**Q: Почему `or vector(0)` в PromQL?**
A: Когда трафика нет, `sum(rate(http_requests_total[5m])) == 0`, и деление даёт NaN. `or vector(0)` подставляет нулевое значение и панель не "ломается".

**Q: Где cleanup в integration-тестах?**
A: `t.Cleanup(func() { TRUNCATE ... })`. Делается через прямой pgx-pool к Postgres (и `marketplace`, и `user_service` DB).

# Маркетплейс — архитектура (HW7: CI/CD, Testing & Observability)

Микросервисный учебный маркетплейс с полным CI/CD-пайплайном, тестами на трёх уровнях, метриками Prometheus, дашбордами Grafana, нагрузочными тестами k6, алертами Alertmanager и SLI/SLO.

## Запуск стека

```bash
# Полный стек (с метриками/Grafana/Alertmanager) — для разработки и защиты:
docker compose up -d --wait

# Только сервисы (postgres + user-service + marketplace-api), без observability:
docker compose up -d --wait postgres user-service marketplace-api

# Тестовый профиль (rate-limit выключен, доступен k6):
docker compose -f docker-compose.yml -f docker-compose.test.yml up -d --wait
```

После старта доступны:

| Компонент          | URL                              | Назначение                                   |
| ------------------ | -------------------------------- | -------------------------------------------- |
| marketplace-api    | <http://localhost:8080>          | REST API товаров/заказов/промокодов          |
| marketplace /metrics | <http://localhost:8080/metrics> | Prometheus-метрики marketplace-api           |
| user-service       | <http://localhost:8000>          | Регистрация/логин/JWT; `POST /auth/validate` для marketplace-api |
| user /metrics      | <http://localhost:8000/metrics>  | Prometheus-метрики user-service              |
| Postgres           | localhost:5432                   | БД `marketplace` (товары/заказы) и `user_service` (пользователи) |
| postgres-exporter  | <http://localhost:9187/metrics>  | Метрики PostgreSQL для Prometheus            |
| Prometheus         | <http://localhost:9090>          | Скрейп + UI                                  |
| Alertmanager       | <http://localhost:9093>          | Активные алерты                              |
| Grafana            | <http://localhost:3000>          | Дашборды (anonymous Viewer включён)          |

Smoke-проверки:

```bash
curl http://localhost:8000/health   # {"service":"user-service","status":"ok"}
curl http://localhost:8080/health   # {"service":"marketplace-api","status":"ok"}
curl http://localhost:9090/-/ready  # Prometheus is Ready.
```

## Архитектура

```text
client ─▶ marketplace-api ─POST /auth/validate─▶ user-service
            │                                       │
            └──── marketplace DB ──── postgres ──── user_service DB
                            ▲
                            └── postgres-exporter ─▶ Prometheus ─▶ Grafana
                                                            │
                                                            └─▶ Alertmanager
```

Доменная модель:

| Сервис            | Стек | БД              | Зачем                              |
| ----------------- | ---- | --------------- | ---------------------------------- |
| marketplace-api   | Go (chi + oapi-codegen + pgx) | `marketplace`   | Каталог, заказы, промокоды |
| user-service      | Go (chi)                       | `user_service`  | Аутентификация, JWT, /auth/validate |
| Postgres 16       | —    | две БД, мульти-init | Хранение                          |
| postgres-exporter | —    | —               | Метрики Postgres для Prometheus    |
| Prometheus        | —    | —               | Сбор метрик + alert evaluation     |
| Alertmanager      | —    | —               | Маршрутизация и хранение алертов   |
| Grafana 11        | —    | —               | Дашборды (provisioning из репо)    |

C4-диаграмма (исторический документ): [docs/c4-container.puml](docs/c4-container.puml).

## CI Pipeline

Конфиг: [`.github/workflows/ci.yml`](.github/workflows/ci.yml). Jobs (последовательно/параллельно):

1. **lint-build** (matrix по сервисам) — `go vet`, `go build`, `golangci-lint`.
2. **unit-tests** (matrix) — `go test ./...` с coverage.
3. **integration-tests** — `docker compose up -d --wait` → `go test -tags=integration ./test/integration/...`.
4. **e2e-tests** — `docker compose up --wait` → `go test -tags=e2e ./test/e2e/...`.
5. **load-tests** — `docker compose --profile loadtest run k6 ...`, артефакт `summary.json`.
6. **metrics-check** — комплексный сценарий: нагрузка → запрос Prometheus → проверка SLI (`scripts/check-sli.sh`).

Любой failing job красит весь пайплайн. Логи `docker compose logs` сохраняются как артефакты в `if: always()` шагах.

## Тесты

| Слой         | Где                              | Запуск                                                   |
| ------------ | -------------------------------- | -------------------------------------------------------- |
| Unit         | `services/*/internal/**/*_test.go` | `make test-unit` (вне docker)                          |
| Integration  | `services/marketplace-api/test/integration` | `make up-test && make test-integration`         |
| E2E          | `services/marketplace-api/test/e2e`         | `make up-test && make test-e2e`                 |
| Load (k6)    | `loadtests/scenarios.js`         | `make up-test && make test-load`                          |

Интеграционные тесты доказывают межсервисное взаимодействие: запрос `POST /products` в marketplace-api приводит к синхронному HTTP-вызову `POST /auth/validate` в user-service (см. `services/marketplace-api/internal/clients/userclient.go`). Без живого user-service marketplace-api отвечает 503 `AUTH_UNAVAILABLE`.

## Метрики

Оба Go-сервиса экспортируют идентичный набор RED-метрик через middleware на chi:

| Метрика                            | Тип       | Labels                            | Описание                          |
| ---------------------------------- | --------- | --------------------------------- | --------------------------------- |
| `http_requests_total`              | Counter   | method, endpoint, status          | Все обработанные запросы          |
| `http_request_errors_total`        | Counter   | method, endpoint, error_type      | error_type: client_error/server_error |
| `http_request_duration_seconds`    | Histogram | method, endpoint                  | Длительность запроса              |
| `auth_validate_calls_total` (только marketplace-api) | Counter | outcome | success/client_error/server_error/timeout/unreachable |

`endpoint` — это chi RoutePattern (не raw URL), поэтому без кардинального взрыва.

## Дашборды Grafana

Provisioning: [`infra/grafana/provisioning`](infra/grafana/provisioning) + [`infra/grafana/dashboards`](infra/grafana/dashboards). Загружаются автоматически при `docker compose up`.

- `marketplace-api` — p50/p95/p99 latency по endpoint, error rate, availability, RPS, распределение статусов, success/error для auth-validate.
- `user-service` — p50/p95/p99 latency, error rate, RPS, распределение статусов.
- `postgres` — connections by DB, cache hit ratio, transactions/sec, deadlocks/temp files.

Все дашборды auto-refresh 5–10s.

## Нагрузочные тесты

k6-сценарий [`loadtests/scenarios.js`](loadtests/scenarios.js):

- 10 VUs × 60 секунд (соответствует требованию ≥10 VU, ≥30 сек).
- `setup()` создаёт по SELLER+USER+product на каждый VU, чтобы обойти `HasActiveOrder` constraint.
- 70% GET /products, 20% GET /products/{id}, 10% POST /orders + сразу cancel (освобождает HasActiveOrder).
- В тестовом override отключён rate-limit (`ORDER_RATE_LIMIT_MINUTES=0`).
- Thresholds:
  - `http_req_duration: p(95)<500` (ms)
  - `http_req_failed: rate<0.01`
  - `checks: rate>0.99`

При превышении порога k6 возвращает exit code != 0, CI падает.

## Алерты

Файл [`infra/prometheus/alerts.yml`](infra/prometheus/alerts.yml):

| Alert         | Expr                                                                                          | for | Severity |
| ------------- | --------------------------------------------------------------------------------------------- | --- | -------- |
| HighErrorRate | `sum(rate(http_request_errors_total[5m])) / sum(rate(http_requests_total[5m])) > 0.05`        | 5m  | warning  |
| HighLatency   | `histogram_quantile(0.95, sum by (le, job) (rate(http_request_duration_seconds_bucket[5m]))) > 1` | 5m | warning |
| ServiceDown   | `up{job=~"marketplace-api\|user-service"} == 0`                                              | 1m  | critical |

Демонстрация (для защиты):

```bash
make demo-alert
# stop user-service → ждёт 150s → проверяет Alertmanager API
# при успехе печатает firing alert
# в конце автоматически restart user-service
```

Открыть UI: <http://localhost:9093/#/alerts>.

## SLI / SLO

| SLI                            | PromQL                                                                                                                          | SLO     | Порог отказа | Обоснование                                                                                                  |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------- | ------- | ------------ | ------------------------------------------------------------------------------------------------------------ |
| **API Availability** (marketplace-api) | `1 - sum(rate(http_request_errors_total{job="marketplace-api"}[5m])) / sum(rate(http_requests_total{job="marketplace-api"}[5m])) or vector(1)` | > 99.5% | < 95%        | 99.5% ≈ 22 мин простоя в месяц — норма для CRUD API над одной Postgres. Ниже 95% = ~36 ч простоя/мес — авария. |
| **API Latency p95** (marketplace-api) | `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{job="marketplace-api"}[5m])))`                  | < 500ms | > 1000ms     | Цепочка client → api → user-svc (+5–10 ms) → pg (5–20 ms). 500ms = 10× буфер; 1s = деградация для пользователя. |
| **Auth Cross-Service Success** | `sum(rate(auth_validate_calls_total{outcome="success"}[5m])) / sum(rate(auth_validate_calls_total[5m]))`                        | > 99%   | < 95%        | Доступность user-service для marketplace-api. <95% = частые таймауты/network errors → каскадная деградация. |

SLI используются:
- **в алертах** ([`infra/prometheus/alerts.yml`](infra/prometheus/alerts.yml)): HighErrorRate ≈ Availability SLO, HighLatency ≈ Latency SLO, ServiceDown ≈ availability с другой стороны.
- **в CI** ([`scripts/check-sli.sh`](scripts/check-sli.sh)): после k6 запрашиваются те же PromQL, при превышении порогов CI красный.
- **в дашбордах** Grafana (`marketplace-api`): availability и error rate видны как stat-панели со светофором.

Запустить локально:

```bash
make up-test
make check-sli
cat scripts/sli-output.json
```

## Защита (план)

1. `docker compose up -d --wait` — показать логи запуска (postgres healthy → user-service healthy → marketplace-api healthy → prom/grafana).
2. `curl /health` обоих сервисов + Grafana UI.
3. Зелёный CI на GitHub Actions (любой push).
4. `make test-unit && make test-integration && make test-e2e` — все тесты проходят.
5. Сломать что-нибудь (например, `git stash`-нуть проверку статуса) — показать красный пайплайн.
6. Grafana → marketplace-api dashboard — наблюдать live-метрики.
7. `make demo-alert` — показать firing ServiceDown в Alertmanager UI.
8. Объяснить SLI/SLO: PromQL, обоснование порогов, как используются в CI.

Теоретические темы — см. [docs/defense-notes.md](docs/defense-notes.md).

---

## Историческая часть: домены

| Домен            | Что делает                                                          |
| ---------------- | ------------------------------------------------------------------- |
| **Identity**     | Регистрация, аутентификация, JWT, профили (user-service)            |
| **Catalog**      | Управление товарами, категориями, ценами (marketplace-api)          |
| **Orders**       | Оформление заказов, жизненный цикл статусов (marketplace-api)       |
| **Promo**        | Промокоды и скидки (marketplace-api)                                |
| **Frontend** (запланирован) | SPA для покупателей и продавцов                          |
| **Notifications** (запланирован) | Email/SMS/Push                                        |

Полная карта в [docs/c4-container.puml](docs/c4-container.puml).

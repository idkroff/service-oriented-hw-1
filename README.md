# Маркетплейс — архитектура

## Запуск

```bash
docker-compose up --build
curl http://localhost:8000/health
# {"status":"ok","service":"user-service"}
```

C4-диаграмма: [docs/c4-container.puml](docs/c4-container.puml) — открыть на [plantuml.com](https://www.plantuml.com/plantuml) или через плагин PlantUML в IDE.

---

## Домены

| Домен | Что делает |
|---|---|
| **Frontend** | SPA для покупателей и продавцов |
| **Identity** | Регистрация, аутентификация, JWT, профили |
| **Catalog** | Управление товарами, категориями, ценами |
| **Feed** | Персонализированная лента: фильтрация по истории покупок, кэш рекомендаций в Redis |
| **Orders** | Оформление заказов, жизненный цикл статусов |
| **Payments** | Платежи через внешний шлюз, учёт транзакций |
| **Notifications** | Уведомления о статусах заказов (Email / SMS / Push) |

---

## Сервисы и данные

Каждый сервис владеет своей БД

| Сервис | Технология | БД |
|---|---|---|
| Web Application | React SPA / Nginx | — |
| API Gateway | Nginx | — |
| User Service | Go | PostgreSQL |
| Catalog Service | Go | PostgreSQL |
| Feed Service | Go | Redis |
| Order Service | Go | PostgreSQL |
| Payment Service | Go | PostgreSQL |
| Notification Service | Go | — (stateless) |

**Синхронные вызовы (REST):**
- Feed → Catalog: запрос товаров для ленты
- Order → Catalog: проверка наличия при создании заказа

**Асинхронные события (Kafka):**

| Событие | Кто публикует | Кто слушает |
|---|---|---|
| `order.created` | Order | Payment, Feed, Notification |
| `order.status_changed` | Order | Notification |
| `payment.processed` | Payment | Order, Notification |
| `payment.failed` | Payment | Order, Notification |
| `catalog.product_updated` | Catalog | Feed (инвалидация кэша) |

---

## Варианты архитектуры

### A. Модульный монолит
Все домены в одном процессе, одна БД.

**Плюсы:** простой деплой, транзакции без распределённых протоколов, легко дебажить
**Минусы:** только вертикальное масштабирование, единый стек, риск размывания границ модулей

### B. Микросервисы
Каждый домен — отдельный сервис со своей БД, REST + Kafka.

**Плюсы:** независимое масштабирование, изоляция отказов, независимые релизы
**Минусы:** распределённые транзакции, операционный оверхед, сложность отладки
---

## Выбираем: микросервисы

Каждый домен из задания имеет разные нагрузочные характеристики: лента читается в разы чаще заказов, каталог обновляется продавцами пачками, платежи требуют отдельного аудит-лога. Монолит не даёт их масштабировать независимо. 

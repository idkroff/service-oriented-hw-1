.PHONY: help up down logs ps build-svc \
		test-unit test-integration test-e2e test-load \
		check-sli demo-alert \
		dashboards-validate compose-config

# DOCKER_COMPOSE детектится автоматически:
#   1) `docker compose` (v2 plugin, GitHub Actions / Docker Desktop)
#   2) `docker-compose` (legacy binary, Homebrew install)
DOCKER_COMPOSE := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")
COMPOSE := $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.test.yml

# Авто-детект нативной архитектуры хоста. Без этого Docker Desktop на Apple Silicon
# может по дефолту запускать всё под linux/amd64 (Rosetta), что (а) убивает Go-ассемблер
# в билдере, (б) приводит к platform-mismatch при запуске уже собранных образов.
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_M),arm64)
	export DOCKER_DEFAULT_PLATFORM := linux/arm64
endif
ifeq ($(UNAME_M),aarch64)
	export DOCKER_DEFAULT_PLATFORM := linux/arm64
endif

# Порт, на котором публикуется postgres-контейнер. Можно переопределить, если 5432
# на хосте уже занят (например, у вас локальный postgres или ssh-туннель). Влияет
# и на DSN, который видят integration/e2e тесты.
POSTGRES_HOST_PORT ?= 5432
export POSTGRES_HOST_PORT
TEST_MARKETPLACE_DSN := postgres://postgres:postgres@localhost:$(POSTGRES_HOST_PORT)/marketplace?sslmode=disable
TEST_USER_DSN := postgres://postgres:postgres@localhost:$(POSTGRES_HOST_PORT)/user_service?sslmode=disable

help: ## Список целей
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "} {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

up: ## Поднять стек (prod compose)
	$(DOCKER_COMPOSE) up -d --wait

up-test: ## Поднять стек с тестовым override
	$(COMPOSE) up -d --wait

down: ## Остановить стек и удалить volumes
	$(COMPOSE) down -v || $(DOCKER_COMPOSE) down -v

logs: ## docker compose logs -f
	$(COMPOSE) logs -f

ps:
	$(COMPOSE) ps

compose-config: ## Проверить корректность docker-compose
	$(COMPOSE) config --quiet

test-unit: ## Запустить unit-тесты обоих сервисов
	cd services/marketplace-api && go test ./... -count=1
	cd services/user-service   && go test ./... -count=1

test-integration: ## Запустить интеграционные тесты (требуют up-test)
	cd services/marketplace-api && \
		INTEGRATION_MARKETPLACE_DSN="$(TEST_MARKETPLACE_DSN)" \
		INTEGRATION_USER_DSN="$(TEST_USER_DSN)" \
		go test -tags=integration -count=1 -v ./test/integration/...

test-e2e: ## Запустить E2E (требует up-test)
	cd services/marketplace-api && \
		E2E_MARKETPLACE_DSN="$(TEST_MARKETPLACE_DSN)" \
		go test -tags=e2e -count=1 -v ./test/e2e/...

test-load: ## Запустить k6 против УЖЕ запущенного стека (нужно сделать up-test первым)
	$(COMPOSE) --profile loadtest run --rm --no-deps k6 run /scripts/scenarios.js \
		--summary-export=/scripts/summary.json

check-sli: ## CI-сценарий блока 8: нагрузка + проверка порогов
	bash scripts/check-sli.sh

demo-alert: ## Демонстрация срабатывания ServiceDown
	bash scripts/demo-alert.sh

generate-openapi: ## Перегенерировать api.gen.go
	cd services/marketplace-api && make generate

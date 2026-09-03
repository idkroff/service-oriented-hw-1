#!/usr/bin/env bash
# demo-alert.sh — демонстрация срабатывания alert ServiceDown
# (для защиты ДЗ, блок 9 задания).
#
# Алгоритм:
#   1. docker compose stop user-service
#   2. Ждём 2 минуты (1m for: + 30s evaluation_interval + scrape_interval).
#   3. Проверяем Alertmanager API на наличие ServiceDown в state=active.
#   4. Восстанавливаем user-service.

set -euo pipefail

AM_URL="${AM_URL:-http://localhost:9093}"
COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.test.yml)
SERVICE="${SERVICE:-user-service}"
ALERTNAME="${ALERTNAME:-ServiceDown}"
WAIT_SECS="${WAIT_SECS:-150}"

# Apple Silicon: явно указываем нативную платформу.
if [[ "$(uname -m)" == "arm64" || "$(uname -m)" == "aarch64" ]]; then
    export DOCKER_DEFAULT_PLATFORM="${DOCKER_DEFAULT_PLATFORM:-linux/arm64}"
fi

if docker compose version >/dev/null 2>&1; then
    DC=(docker compose)
else
    DC=(docker-compose)
fi

cleanup() {
    echo ">>> Restarting ${SERVICE}..."
    "${DC[@]}" "${COMPOSE_FILES[@]}" start "${SERVICE}" || true
}
trap cleanup EXIT

echo ">>> Stopping ${SERVICE} to trigger ${ALERTNAME}..."
"${DC[@]}" "${COMPOSE_FILES[@]}" stop "${SERVICE}"

echo ">>> Waiting ${WAIT_SECS}s for alert to fire..."
for i in $(seq 1 "${WAIT_SECS}"); do
    sleep 1
    response=$(curl -fsS "${AM_URL}/api/v2/alerts" || echo '[]')
    firing=$(echo "${response}" \
        | jq -r --arg name "${ALERTNAME}" \
            '[.[] | select(.labels.alertname == $name and .status.state == "active")] | length')
    if [[ "${firing}" -gt 0 ]]; then
        echo ">>> ${ALERTNAME} is FIRING (after ${i}s)"
        echo "${response}" \
            | jq --arg name "${ALERTNAME}" \
                '.[] | select(.labels.alertname == $name)'
        exit 0
    fi
done

echo ">>> ${ALERTNAME} did NOT fire within ${WAIT_SECS}s"
echo ">>> Текущие алерты:"
curl -fsS "${AM_URL}/api/v2/alerts" | jq -r '.[] | "\(.labels.alertname) state=\(.status.state)"' || true
exit 1

#!/usr/bin/env bash
# check-sli.sh — комплексная проверка для CI job metrics-check (блок 8 задания).
#
# Что делает:
#   1. Запускает k6 (~60s) против поднятого docker compose.
#   2. Ждёт +20s, чтобы Prometheus успел сделать 2-4 scrape после окончания нагрузки.
#   3. Запрашивает 3 SLI у Prometheus API:
#        - API Availability (1 - error rate)
#        - API Latency p95 (секунды)
#        - Auth Cross-Service Success rate
#   4. Сравнивает с порогами отказа (см. README, раздел SLI/SLO).
#   5. Выходит 0/1, пишет результат в scripts/sli-output.json.
#
# Запуск:
#   bash scripts/check-sli.sh
#
# Перед запуском нужен docker compose -f docker-compose.yml -f docker-compose.test.yml up -d --wait.

set -euo pipefail

PROM_URL="${PROM_URL:-http://localhost:9090}"
COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.test.yml)
OUTPUT_DIR="${OUTPUT_DIR:-$(dirname "$0")}"
OUTPUT_FILE="${OUTPUT_DIR}/sli-output.json"

# Apple Silicon: явно указываем нативную платформу, иначе Docker Desktop
# может уйти в Rosetta-эмуляцию linux/amd64 (segfault в Go-сборке + platform mismatch).
if [[ "$(uname -m)" == "arm64" || "$(uname -m)" == "aarch64" ]]; then
    export DOCKER_DEFAULT_PLATFORM="${DOCKER_DEFAULT_PLATFORM:-linux/arm64}"
fi

# detect docker compose (plugin v2) vs docker-compose (legacy)
if docker compose version >/dev/null 2>&1; then
    DC=(docker compose)
else
    DC=(docker-compose)
fi

ERR_RATE_FAIL_THRESHOLD="0.01"     # > 1% — провал
P95_FAIL_THRESHOLD="0.5"           # > 500ms (секунды) — провал
AUTH_SUCCESS_FAIL_THRESHOLD="0.95" # < 95% — провал

run_k6() {
    echo ">>> Running k6 (60s, 10 VU)..."
    # --no-deps: не трогать уже запущенные postgres/marketplace/user — иначе compose
    # попытается пересоздать их и упрётся в port mapping.
    "${DC[@]}" "${COMPOSE_FILES[@]}" --profile loadtest run --rm --no-deps \
        k6 run /scripts/scenarios.js \
        --quiet \
        --summary-export=/scripts/summary.json \
        || { echo "k6 thresholds failed"; return 1; }
}

# query <PromQL> -> печатает значение или "empty"
query() {
    local q="$1"
    curl -sG --data-urlencode "query=${q}" "${PROM_URL}/api/v1/query" \
        | jq -r '.data.result[0].value[1] // "empty"'
}

compare_lt() {
    # compare_lt <value> <threshold>: возвращает 0 если value < threshold
    awk -v v="$1" -v t="$2" 'BEGIN { exit !(v < t) }'
}

compare_gt() {
    # compare_gt <value> <threshold>: возвращает 0 если value > threshold
    awk -v v="$1" -v t="$2" 'BEGIN { exit !(v > t) }'
}

main() {
    run_k6 || true   # не падаем сразу — может не сработать threshold k6, но мы валидируем SLI отдельно.

    echo ">>> Waiting 20s for Prometheus to scrape post-load metrics..."
    sleep 20

    local err_rate p95 auth_success traffic_rps
    # `or vector(0)` — если http_request_errors_total ещё не появлялся (0 ошибок за окно),
    # серии в Prometheus нет, и сырой rate(...) вернёт empty. Подменяем на 0.
    err_rate=$(query '(sum(rate(http_request_errors_total{job="marketplace-api"}[2m])) or vector(0)) / sum(rate(http_requests_total{job="marketplace-api"}[2m]))')
    p95=$(query 'histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{job="marketplace-api"}[2m])))')
    auth_success=$(query '(sum(rate(auth_validate_calls_total{outcome="success"}[2m])) or vector(0)) / sum(rate(auth_validate_calls_total[2m]))')
    # Защита от "нет трафика": проверяем, что hist-серии для marketplace-api вообще существуют.
    traffic_rps=$(query 'sum(rate(http_requests_total{job="marketplace-api"}[2m]))')

    echo "error_rate=${err_rate}"
    echo "p95=${p95}"
    echo "auth_success_rate=${auth_success}"
    echo "traffic_rps=${traffic_rps}"

    local failed=0

    if [[ "${traffic_rps}" == "empty" || "${traffic_rps}" == "NaN" ]] || compare_lt "${traffic_rps}" "0.1"; then
        echo "FAIL: нет трафика к marketplace-api (rps=${traffic_rps})"
        failed=1
    elif [[ "${err_rate}" == "empty" || "${err_rate}" == "NaN" ]]; then
        # Этого не должно случаться благодаря `or vector(0)`, но на всякий случай.
        echo "FAIL: error_rate is empty/NaN"
        failed=1
    elif compare_gt "${err_rate}" "${ERR_RATE_FAIL_THRESHOLD}"; then
        echo "FAIL: error_rate ${err_rate} > ${ERR_RATE_FAIL_THRESHOLD}"
        failed=1
    else
        echo "PASS: error_rate ${err_rate} <= ${ERR_RATE_FAIL_THRESHOLD}"
    fi

    if [[ "${p95}" == "empty" || "${p95}" == "NaN" ]]; then
        echo "FAIL: p95 is empty/NaN"
        failed=1
    elif compare_gt "${p95}" "${P95_FAIL_THRESHOLD}"; then
        echo "FAIL: p95 ${p95}s > ${P95_FAIL_THRESHOLD}s"
        failed=1
    else
        echo "PASS: p95 ${p95}s <= ${P95_FAIL_THRESHOLD}s"
    fi

    if [[ "${auth_success}" == "empty" || "${auth_success}" == "NaN" ]]; then
        echo "WARN: auth_success_rate empty (нет авторизованного трафика?)"
        # Не падаем — auth не используется в большинстве endpoints в нагрузочном профиле.
    elif compare_lt "${auth_success}" "${AUTH_SUCCESS_FAIL_THRESHOLD}"; then
        echo "FAIL: auth_success_rate ${auth_success} < ${AUTH_SUCCESS_FAIL_THRESHOLD}"
        failed=1
    else
        echo "PASS: auth_success_rate ${auth_success} >= ${AUTH_SUCCESS_FAIL_THRESHOLD}"
    fi

    mkdir -p "${OUTPUT_DIR}"
    cat > "${OUTPUT_FILE}" <<EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "metrics": {
    "error_rate": "${err_rate}",
    "p95_seconds": "${p95}",
    "auth_success_rate": "${auth_success}"
  },
  "thresholds": {
    "error_rate_max": ${ERR_RATE_FAIL_THRESHOLD},
    "p95_seconds_max": ${P95_FAIL_THRESHOLD},
    "auth_success_rate_min": ${AUTH_SUCCESS_FAIL_THRESHOLD}
  },
  "verdict": "$([[ ${failed} -eq 0 ]] && echo PASS || echo FAIL)"
}
EOF

    echo ">>> Wrote ${OUTPUT_FILE}"

    exit "${failed}"
}

main "$@"

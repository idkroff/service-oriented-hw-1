# BUILDPLATFORM = архитектура хоста (arm64 на Mac M-series, amd64 в GHA Linux).
# Без этого Docker может выбрать кросс-архитектурный образ под Rosetta-эмуляцией
# и Go-ассемблер падает с segfault. См. docs/defense-notes.md.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Генерируем OpenAPI-кодовую базу + сам бинарь сервиса.
RUN mkdir -p internal/generated && \
    go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
    --config=api/oapi-codegen.yaml api/openapi.yaml && \
    go build -o /out/server ./cmd/

# Goose ставится в отдельной module-aware install-команде, чтобы не загрязнять
# go.mod сервиса транзитивными драйверами (mysql/sqlite/clickhouse и т.п.).
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

FROM --platform=$BUILDPLATFORM alpine:3.21
RUN apk add --no-cache wget
WORKDIR /app
COPY --from=builder /out/server .
COPY --from=builder /go/bin/goose .
COPY --from=builder /app/migrations ./migrations
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["./entrypoint.sh"]

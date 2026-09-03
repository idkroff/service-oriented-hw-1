# BUILDPLATFORM = архитектура хоста (arm64 на Mac M-series, amd64 в GHA Linux).
# Без этого Docker может выбрать кросс-архитектурный образ под эмуляцией → segfault.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/server ./cmd/

# Goose ставится через go install с пинной версией, чтобы не тянуть его драйверы в go.mod.
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

FROM --platform=$BUILDPLATFORM alpine:3.21
RUN apk add --no-cache wget
WORKDIR /app
COPY --from=builder /out/server .
COPY --from=builder /go/bin/goose .
COPY --from=builder /app/migrations ./migrations
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh
EXPOSE 8000
ENTRYPOINT ["./entrypoint.sh"]

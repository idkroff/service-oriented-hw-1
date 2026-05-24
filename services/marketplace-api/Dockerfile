FROM --platform=linux/arm64 golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p internal/generated && \
    go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
    --config=api/oapi-codegen.yaml api/openapi.yaml && \
    go build -o server ./cmd/ && \
    go build -o goose github.com/pressly/goose/v3/cmd/goose

FROM --platform=linux/arm64 alpine:3.21
RUN apk add --no-cache wget
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /app/goose .
COPY --from=builder /app/migrations ./migrations
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["./entrypoint.sh"]

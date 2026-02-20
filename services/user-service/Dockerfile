FROM --platform=linux/arm64 golang:1.23-alpine AS builder
WORKDIR /app
COPY main.go .
RUN go mod init user-service && go build -o user-service .

FROM --platform=linux/arm64 alpine:3.21
RUN apk add --no-cache wget
WORKDIR /app
COPY --from=builder /app/user-service .
EXPOSE 8000
CMD ["./user-service"]

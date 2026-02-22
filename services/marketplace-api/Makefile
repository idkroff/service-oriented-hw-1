.PHONY: generate build tidy

generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
		--config=api/oapi-codegen.yaml api/openapi.yaml

build: generate
	go build -o bin/server ./cmd/

tidy:
	go mod tidy

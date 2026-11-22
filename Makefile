SHELL := /bin/sh

APP        := gonews
PKG        := github.com/gonews/gonews
CMD        := ./cmd/server
BIN_DIR    := bin
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: help run build build-windows test vet fmt fmt-check cover lint tidy clean \
	release-snapshot docker docker-run compose-up compose-down

help: ## Показать список целей
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

run: ## Запустить сервер локально
	go run $(CMD)

build: ## Собрать бинарник для текущей платформы
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP) $(CMD)

build-windows: ## Кросс-сборка gonews.exe
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP).exe $(CMD)

test: ## Запустить все тесты
	go test -count=1 ./...

vet: ## go vet
	go vet ./...

fmt: ## Форматирование
	gofmt -w .

fmt-check: ## Проверка форматирования (для CI)
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

cover: ## Покрытие тестами
	go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

tidy: ## go mod tidy
	go mod tidy

clean: ## Удалить артефакты
	rm -rf $(BIN_DIR) dist coverage.out coverage.html

release-snapshot: ## Собрать все платформы через GoReleaser (без публикации)
	goreleaser release --snapshot --clean

docker: ## Собрать Docker-образ
	docker build -t $(APP):$(VERSION) .

docker-run: ## Запустить контейнер
	docker run --rm -p 8080:8080 -v gonews-data:/data $(APP):$(VERSION)

compose-up: ## docker compose up
	docker compose up -d --build

compose-down: ## docker compose down
	docker compose down

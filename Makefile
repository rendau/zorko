.DEFAULT_GOAL := build

BINARY_NAME = svc
BUILD_PATH = cmd/build
# сведения о сборке (internal/constant): версия, полный коммит, время сборки
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
BUILT_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -X github.com/rendau/zorko/internal/constant.Version=$(VERSION) \
	-X github.com/rendau/zorko/internal/constant.Commit=$(COMMIT) \
	-X github.com/rendau/zorko/internal/constant.BuiltAt=$(BUILT_AT)
SERVICE_NAME = zorko_v1

# откуда refresh-vendor-proto берёт общие .proto: репозиторий protos организации из module_owner,
# если при генерации к нему был доступ; пусто — только те, что пришли с шаблоном
PROTOS_REPO =
# vendor repo sparse dirs
protos-dirs = google protoc-gen-openapiv2 common
# пакет common.proto — пакет сервиса (go_package в файле нет или он чужой): для всех плагинов
COMMON_PKG = Mcommon/common.proto=$(shell go list -m)/pkg/proto/common
vendor-proto-dirs = vp-common

# Тулчейн генерации proto — локально в проекте (bin/, в .gitignore), версии закреплены:
# у всех разработчиков генерация даёт одинаковый вывод. protoc-gen-go и grpc-gateway берутся
# версий рантайм-библиотек из go.mod — сгенерированный код совпадает с тем, с чем собирается сервис.
LOCAL_BIN := $(CURDIR)/bin
PROTOC_VERSION := 35.1
PROTOC_GEN_GO_GRPC_VERSION := v1.6.2
PROTOC_GEN_GO_VERSION = $(shell go list -m -f '{{.Version}}' google.golang.org/protobuf)
GRPC_GATEWAY_VERSION = $(shell go list -m -f '{{.Version}}' github.com/grpc-ecosystem/grpc-gateway/v2)

PROTOC_OS := $(shell uname -s | sed -e 's/Darwin/osx/' -e 's/Linux/linux/')
PROTOC_ARCH := $(shell uname -m | sed -e 's/arm64/aarch_64/' -e 's/aarch64/aarch_64/')
PROTOC_URL = https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-$(PROTOC_OS)-$(PROTOC_ARCH).zip
PROTOC := $(LOCAL_BIN)/protoc

# плагины указываются явно: отсутствующий локальный плагин — ошибка, а не молчаливый глобальный из PATH
PROTOC_PLUGINS = \
	--plugin=protoc-gen-go=$(LOCAL_BIN)/protoc-gen-go \
	--plugin=protoc-gen-go-grpc=$(LOCAL_BIN)/protoc-gen-go-grpc \
	--plugin=protoc-gen-grpc-gateway=$(LOCAL_BIN)/protoc-gen-grpc-gateway \
	--plugin=protoc-gen-openapiv2=$(LOCAL_BIN)/protoc-gen-openapiv2

.SILENT:

build:
	mkdir -p $(BUILD_PATH)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_PATH)/$(BINARY_NAME) cmd/main.go

clean:
	rm -rf $(BUILD_PATH) admin-ui admin/.output

# интерфейс (ADR-007): один бандл + предсжатые .br/.gz → admin-ui (ADMIN_DIR сервера по умолчанию);
# Dockerfile копирует admin-ui
build-admin:
	cd admin && pnpm install --frozen-lockfile && pnpm generate
	rm -rf admin-ui && cp -R admin/.output/public admin-ui

lint:
	golangci-lint run

# Установка тулчейна генерации proto в bin/ — в итоге там только бинарники: protoc и плагины.
# protoc: из релиза GitHub берётся один bin/protoc (архив — во временном каталоге, удаляется);
# скачивается, только если в bin/ нет protoc нужной версии. include/ релиза не нужен —
# стандартные google/protobuf/*.proto лежат в vendor-proto.
# Плагины: go install с закреплёнными версиями (из кэша модулей — без сети).
proto-tools:
	mkdir -p $(LOCAL_BIN)
	if [ "$$($(PROTOC) --version 2>/dev/null)" != "libprotoc $(PROTOC_VERSION)" ]; then \
		tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
		curl -fsSL -o "$$tmp/protoc.zip" $(PROTOC_URL) && \
		unzip -qjo "$$tmp/protoc.zip" bin/protoc -d $(LOCAL_BIN); \
	fi
	GOBIN=$(LOCAL_BIN) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(LOCAL_BIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	GOBIN=$(LOCAL_BIN) go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@$(GRPC_GATEWAY_VERSION)
	GOBIN=$(LOCAL_BIN) go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@$(GRPC_GATEWAY_VERSION)

proto-tools-version: proto-tools
	$(PROTOC) --version
	for p in protoc-gen-go protoc-gen-go-grpc protoc-gen-grpc-gateway protoc-gen-openapiv2; do \
		printf '%-24s %s\n' $$p "$$(go version -m $(LOCAL_BIN)/$$p | awk '$$1=="mod"{print $$3}')"; \
	done

# Устанавливаем vendor-proto файлы
refresh-vendor-proto:
	if [ -z "$(PROTOS_REPO)" ]; then \
		echo "PROTOS_REPO не задан: vendor-proto/ пришёл с шаблоном и обновляется вместе с ним."; \
		echo "Свой репозиторий protos: make refresh-vendor-proto PROTOS_REPO=<git-url>"; \
		exit 1; \
	fi
	rm -rf vendor-proto && \
		git clone --depth=1 --filter=blob:none --sparse $(PROTOS_REPO) vendor-proto
	cd vendor-proto && git sparse-checkout set $(protos-dirs)
	cd vendor-proto && rm -rf .git .idea && find . -type f ! -name '*.proto' -delete

vp-%:
	mkdir -p pkg/proto
	$(PROTOC) $(PROTOC_PLUGINS) -I vendor-proto \
	--go_out pkg/proto --go_opt paths=source_relative \
	--go_opt=$(COMMON_PKG) \
	--go-grpc_out pkg/proto --go-grpc_opt paths=source_relative --go-grpc_opt=$(COMMON_PKG) \
	--grpc-gateway_out pkg/proto --grpc-gateway_opt paths=source_relative --grpc-gateway_opt=$(COMMON_PKG) \
	vendor-proto/$(subst vp-,,$@)/*.proto

generate-vendor-proto-dirs: $(vendor-proto-dirs)

generate-proto-$(SERVICE_NAME):
	mkdir -p pkg/proto
	$(PROTOC) $(PROTOC_PLUGINS) -I vendor-proto -I api/proto \
	--go_out pkg/proto --go_opt paths=source_relative \
	--go_opt=$(COMMON_PKG) \
	--go-grpc_out pkg/proto --go-grpc_opt paths=source_relative --go-grpc_opt=$(COMMON_PKG) \
	--grpc-gateway_out pkg/proto --grpc-gateway_opt paths=source_relative --grpc-gateway_opt=$(COMMON_PKG) \
	--openapiv2_out=json_names_for_fields=false,allow_merge=true,merge_file_name=api:docs --openapiv2_opt=$(COMMON_PKG) \
	api/proto/$(SERVICE_NAME)/*.proto

generate-proto: proto-tools generate-vendor-proto-dirs generate-proto-$(SERVICE_NAME)

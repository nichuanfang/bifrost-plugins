.PHONY: all build dev clean install help test deps build-all

# Colors
COLOR_RESET   = \033[0m
COLOR_INFO    = \033[36m
COLOR_SUCCESS = \033[32m
COLOR_WARNING = \033[33m
COLOR_ERROR   = \033[31m
COLOR_BOLD    = \033[1m

# 默认插件（可通过命令行覆盖）
PLUGIN_NAME ?= pii-masking
PLUGINS_DIR = plugins

# 插件路径
PLUGIN_DIR = $(PLUGINS_DIR)/$(PLUGIN_NAME)
MAIN_FILE  = $(PLUGIN_DIR)/main.go
OUTPUT_DIR = ./build

# Platform detection
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	PLUGIN_EXT = .so
	PLATFORM = linux
endif
ifeq ($(UNAME_S),Darwin)
	PLUGIN_EXT = .so
	PLATFORM = darwin
endif

UNAME_M := $(shell uname -m)
ifeq ($(UNAME_M),x86_64)
	ARCH = amd64
endif
ifeq ($(UNAME_M),arm64)
	ARCH = arm64
endif
ifeq ($(UNAME_M),aarch64)
	ARCH = arm64
endif

# Output file
OUTPUT = $(OUTPUT_DIR)/$(PLUGIN_NAME)$(PLUGIN_EXT)

help: ## 显示帮助信息
	@echo '$(COLOR_BOLD)Bifrost 多插件构建系统$(COLOR_RESET)'
	@echo ''
	@echo '$(COLOR_BOLD)Usage:$(COLOR_RESET) make [target] PLUGIN_NAME=插件名'
	@echo ''
	@echo '$(COLOR_BOLD)可用目标:$(COLOR_RESET)'
	@echo '  make dev                  # 开发模式构建当前插件 (默认 pii-masking)'
	@echo '  make build                # 生产模式构建当前插件'
	@echo '  make build PLUGIN_NAME=xxx # 指定插件构建'
	@echo '  make build-all            # 构建所有插件'
	@echo '  make clean                # 清理构建产物'
	@echo '  make install              # 安装到 ~/.bifrost/plugins'
	@echo ''
	@echo '$(COLOR_BOLD)示例:$(COLOR_RESET)'
	@echo '  make dev PLUGIN_NAME=pii-masking'
	@echo '  make build PLUGIN_NAME=pii-masking'
	@echo '  make build-all'
	@echo ''

_clean_build_dir:
	@echo "$(COLOR_INFO)Cleaning build directory for $(PLUGIN_NAME)...$(COLOR_RESET)"
	@rm -rf $(OUTPUT_DIR)
	@echo "$(COLOR_SUCCESS)✓ Build directory cleaned$(COLOR_RESET)"

build: _clean_build_dir ## 生产构建（推荐用于部署）
	@mkdir -p $(OUTPUT_DIR)
	@echo "$(COLOR_INFO)Building $(PLUGIN_NAME) for current platform (production)...$(COLOR_RESET)"
	@cd $(PLUGIN_DIR) && CGO_ENABLED=1 go build -buildmode=plugin -ldflags="-w -s" -trimpath \
		-o ../../$(OUTPUT) main.go
	@echo "$(COLOR_SUCCESS)✓ Plugin built successfully: $(OUTPUT)$(COLOR_RESET)"

dev: _clean_build_dir ## 开发构建（速度快，无优化）
	@mkdir -p $(OUTPUT_DIR)
	@echo "$(COLOR_INFO)Building $(PLUGIN_NAME) for development (no optimizations)...$(COLOR_RESET)"
	@cd $(PLUGIN_DIR) && CGO_ENABLED=1 go build -buildmode=plugin \
		-o ../../$(OUTPUT) main.go
	@echo "$(COLOR_SUCCESS)✓ Plugin built successfully: $(OUTPUT)$(COLOR_RESET)"

build-all: ## 构建 plugins/ 目录下所有插件
	@echo "$(COLOR_INFO)Building all plugins...$(COLOR_RESET)"
	@for dir in $(PLUGINS_DIR)/*/; do \
		if [ -f "$$dir/main.go" ]; then \
			PLUGIN=$$(basename $$dir); \
			echo "$(COLOR_INFO)→ Building $$PLUGIN...$(COLOR_RESET)"; \
			$(MAKE) build PLUGIN_NAME=$$PLUGIN --no-print-directory; \
		fi \
	done
	@echo "$(COLOR_SUCCESS)✓ All plugins built successfully$(COLOR_RESET)"

install: build ## 构建并安装到 Bifrost 默认插件目录
	@echo "$(COLOR_INFO)Installing $(PLUGIN_NAME) to ~/.bifrost/plugins...$(COLOR_RESET)"
	@mkdir -p ~/.bifrost/plugins
	@cp $(OUTPUT) ~/.bifrost/plugins/
	@echo "$(COLOR_SUCCESS)✓ Installed: ~/.bifrost/plugins/$(PLUGIN_NAME)$(PLUGIN_EXT)$(COLOR_RESET)"

clean: ## 清理所有构建产物
	@echo "$(COLOR_INFO)Cleaning all build artifacts...$(COLOR_RESET)"
	@rm -rf $(PLUGINS_DIR)/*/build
	@echo "$(COLOR_SUCCESS)✓ All clean$(COLOR_RESET)"

test: ## 运行测试
	@echo "$(COLOR_INFO)Running tests for $(PLUGIN_NAME)...$(COLOR_RESET)"
	@cd $(PLUGIN_DIR) && go test -v ./...

deps: ## 更新依赖
	@echo "$(COLOR_INFO)Updating dependencies for $(PLUGIN_NAME)...$(COLOR_RESET)"
	@cd $(PLUGIN_DIR) && go mod tidy
	@echo "$(COLOR_SUCCESS)✓ Dependencies updated$(COLOR_RESET)"

.DEFAULT_GOAL := help
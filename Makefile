.PHONY: dev build frontend frontend-dev frontend-build clean deps package lint lint-frontend lint-go

APP_NAME  := goink
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DIR := build
LDFLAGS   := -X internal/version.Version=$(VERSION)

# 启动 Wails 开发模式（Go 后端 + Vite HMR 前端）
dev:
	wails dev -tags webkit2_41

# 下载运行时依赖（Git + ONNX Runtime），已有则跳过
deps:
	@if [ ! -f "$(BUILD_DIR)/runtime/git/git" ] && [ ! -d "$(BUILD_DIR)/runtime/git/mingw64" ]; then \
		bash scripts/download-git.sh; \
	else \
		echo "Git runtime 已存在，跳过下载"; \
	fi
	@if [ ! -f "$(BUILD_DIR)/runtime/libonnxruntime.so" ] && [ ! -f "$(BUILD_DIR)/runtime/libonnxruntime.dylib" ] && [ ! -f "$(BUILD_DIR)/runtime/onnxruntime.dll" ]; then \
		bash scripts/download-onnx.sh; \
	else \
		echo "ONNX Runtime 已存在，跳过下载"; \
	fi

# 构建前端
frontend:
	cd frontend && npm ci && npm run build

# 生产构建（需先 deps）
build: deps frontend
	wails build -tags webkit2_41 -o $(APP_NAME) -ldflags "$(LDFLAGS)"

# 纯前端开发（浏览器模式，后端不可用）
frontend-dev:
	cd frontend && npm run dev

# 纯前端构建
frontend-build:
	cd frontend && npm run build

# 前端 ESLint 检查（阻断 error，允许现有 warn）
lint-frontend:
	cd frontend && npx eslint .

# Go 后端 golangci-lint 检查（配置见 .golangci.yml）
lint-go:
	CGO_ENABLED=1 golangci-lint run --timeout=10m ./...

# 汇总：前端 + 后端 lint
lint: lint-frontend lint-go

# 打包（按当前平台）
package:
	@case "$$(uname -s)" in \
		MINGW*|MSYS*|CYGWIN*) $(MAKE) package-windows ;; \
		Linux)                $(MAKE) package-linux ;; \
		Darwin)               $(MAKE) package-macos ;; \
		*) echo "请使用 package-windows / package-linux / package-macos"; exit 1 ;; \
	esac

# Windows Inno Setup 安装包
package-windows: build
	export VERSION=$(VERSION) && iscc $(BUILD_DIR)/package/windows/setup.iss

# Linux AppImage
package-linux: build
	bash $(BUILD_DIR)/package/linux/build-appimage.sh $(VERSION)

# macOS DMG
package-macos: build
	bash $(BUILD_DIR)/package/macos/build-dmg.sh $(VERSION)

# 清理构建产物
clean:
ifeq ($(OS),Windows_NT)
	powershell -Command "Remove-Item -Recurse -Force frontend/dist, frontend/node_modules, $(BUILD_DIR)/runtime, $(BUILD_DIR)/dist, $(BUILD_DIR)/bin -ErrorAction SilentlyContinue"
	powershell -Command "Remove-Item -Force goink.exe -ErrorAction SilentlyContinue"
else
	rm -rf frontend/dist frontend/node_modules $(BUILD_DIR)/runtime $(BUILD_DIR)/dist $(BUILD_DIR)/bin $(APP_NAME)
endif

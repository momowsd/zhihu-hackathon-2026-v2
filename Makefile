# 在项目根目录执行，例如: make backend
# 需已安装: go, node, npm

ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

.PHONY: help backend frontend install-frontend test-backend test-frontend test build-frontend

help:
	@echo "用法（在项目根目录）:"
	@echo "  make backend          # 启动 Go 后端 (默认 :8080)"
	@echo "  make frontend         # 启动 Vite 前端（默认 http://localhost:5180）"
	@echo "  make install-frontend # npm install"
	@echo "  make test-backend     # go test ./..."
	@echo "  make test-frontend    # npm run build（类型检查+构建）"
	@echo "  make test             # 前后端校验都跑一遍"

backend:
	cd "$(ROOT)backend" && go run ./cmd/server

frontend:
	cd "$(ROOT)frontend" && npm run dev

install-frontend:
	cd "$(ROOT)frontend" && npm install

test-backend:
	cd "$(ROOT)backend" && go test ./...

build-frontend:
	cd "$(ROOT)frontend" && npm run build

test-frontend: build-frontend

test: test-backend test-frontend

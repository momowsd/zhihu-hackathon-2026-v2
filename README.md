# zhihu-hackathon-2026-v2

知乎黑客松 2026 第二期：大模型效果 1v1 盲评与打榜站点。

## 功能

- Vue3 前端：Dashboard、盲评、Ranking、Endpoint Arena、Admin。
- Go Gin 后端：JWT 登录、SQLite/GORM、盲评 session、投票、Elo 统计。
- 玩法 1：从后台题库随机抽题，匿名比较两个模型回答。
- 玩法 2：提交 OpenAI Chat Completions 兼容 endpoint，与基线模型回答盲评。
- 本地密钥策略：用户 endpoint 的 Bearer/API Key 只在单次请求内使用，不持久化。

## 本地开发

```bash
cd backend
go mod tidy
go run ./cmd/server
```

```bash
cd frontend
npm install
npm run dev
```

本地前端开发地址：`http://localhost:5180`（避免与默认 Vite 端口 5173 冲突）。

默认账号：

- 管理员：`admin` / `admin123`
- 普通用户：`demo` / `user123`

## Docker Compose

```bash
docker compose up --build
```

Compose 启动后前端映射：`http://localhost:5183`。

# zhihu-hackathon-2026-v2

知乎黑客松 2026 第二期：大模型效果 1v1 盲评与打榜站点。

## 功能

- Vue3 前端：Dashboard、盲评、Ranking、Endpoint Arena、Admin。
- Go Gin 后端：JWT 登录、SQLite/GORM、盲评 session、投票、Elo 统计。
- 登录方式：账号密码登录，以及可选的知乎官方 OAuth 授权码登录。
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

### 知乎 OAuth 登录（可选）

后端通过环境变量启用知乎官方 OAuth 登录，密钥只在服务端使用，不要提交到仓库：

```bash
export ZHIHU_APP_ID="你的 app_id"
export ZHIHU_APP_KEY="你的 app_key"
export ZHIHU_REDIRECT_URI="http://localhost:8080/api/v1/auth/zhihu/callback"
export FRONTEND_ORIGIN="http://localhost:5180"
export ZHIHU_OPENAPI_HOST="openapi.zhihu.com"
make backend
```

`ZHIHU_REDIRECT_URI` 必须与知乎侧应用登记的回调地址完全一致。黑客松 RFC 中提供了两个测试应用：`187`（不展示手机号/邮箱授权）与 `188`（展示手机号/邮箱授权），对应 app key 请从内部 RFC 获取后注入本地环境变量。

## Docker Compose

```bash
docker compose up --build
```

Compose 将前端映射到主机 **80** 端口，浏览器访问 **`http://localhost`**（或 **`http://<公网IP>`**）即可，无需写端口；云上请在安全组放行 **TCP 80**。

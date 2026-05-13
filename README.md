# 看山模型竞技场 · LLM Blind Evaluation Arena

> 有问题，就会有答案；有模型，就会有江湖。

知乎黑客松 2026 第二期作品：一个面向中文社区的**大模型 1v1 盲评与打榜站点**。
在不知道模型身份的情况下，让用户对中文场景下的两段回答做四档评分，
系统用 Elo 与模型互评矩阵聚合出榜单，让「哪个模型更会回答」变成**可观察、可讨论、可迭代的数据**。

## 关于本项目

### 立意：知乎在大模型时代的生态位

国外有 LMSYS Chatbot Arena 这样的盲评榜单，但**中文语境、文化梗、社区表达习惯**是缺的；
而要让一个评测榜单可信，**评审者本身的质量**比模型本身还关键——它需要既懂内容、又愿意认真看回答的人。

知乎刚好满足这一前提：**高质量提问者 + 大量大 V 与从业者**，长期沉淀的「什么是好的回答」的判断力，是其它平台很难复制的。所以这个项目的产品立意是：

> 知乎不必去和大厂卷训练，但完全可以做**「中文世界里最有公信力的大模型评估场」**。
> 这既是对当下用户的内容服务，也是知乎在 AI 时代一个清晰、可长期经营的生态位。

### 核心玩法

- **玩法 1 · 主题化 1 vs 1 盲评**：选一个主题——**弱智吧Case评估 / 小说创作评估 / 短剧剧本生成 / 高情商回复**——从题库随机抽题，左右两侧匿名展示两个模型的回答，在 **A 更好 / B 更好 / 都好 / 都不好** 四档中选一提交。流程里由知乎 IP **「刘看山」** 充当出题主持人。
- **玩法 2 · 自带 Endpoint 打榜**：提交 OpenAI Chat Completions 兼容 endpoint，把自己的模型 / 微调版本送进擂台。**Bearer / API Key 只在本次请求内使用，不落库**。
- **Ranking · 可解释、可迭代的榜单**：站内 Elo 与国际 Arena 的综合 / 创作榜对照展示，配 **价格–能力散点图** 与 **模型互评热力图（Peer Matrix）**。
- **Admin / Dashboard**：题库后台 + 用户数 / 投票数 / 题目数 / 模型数的近 14 天趋势。

### 数据基础

仓库内的 `eval-workspace/` 是这套榜单的内容护城河，每个领域都包含原始 query、system prompt / user prompt、模型调用脚本与各模型的 `responses/*.jsonl`，**所有题库与模型回答都可复现、可审计、可继续扩充**。

`eval-workspace/model-peer-evals/` 用**模型互评做冷启动**：每个模型同时充当评委对其他模型打分，生成的 `peer_votes.jsonl` 在后端启动时幂等导入，让用户**第一天打开 Ranking 就能看到一份合理的榜单**，而不是空表。

### 设计上的几个关键决策

1. **盲评是默认形态**——比打分表门槛更低、抗背书污染、收敛更快。
2. **四档评分（A / B / 都好 / 都不好）**——保留「都菜 / 都行」的真实信号，让 Elo 不被强制扭曲。
3. **Elo 之外保留 Rank / 胜率 / 票数 / 互评矩阵**——单一指标会被刷，多视角才稳。
4. **Endpoint 自带打榜 + 不存密钥**——把"评估"开放给任何模型作者，但用产品级承诺保护信任。
5. **知乎 IP 嵌入流程**——刘看山做"出题主持人"不是装饰，是把社区氛围搬进 AI 产品里。

> 项目内置 `/about` 页（登录后顶栏头像下拉菜单中的「关于本项目」；未登录可从首页「关于本项目」进入）展示同一份介绍。

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

- 管理员：`admin` / `********`
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

### HTTPS（可选）

将证书放到仓库根目录 `deploy/ssl/`，文件名固定为 **`fullchain.pem`** 与 **`privkey.pem`**（可用软链接指向阿里云或 Let’s Encrypt 导出的文件）。与覆盖文件一起启动：

```bash
docker compose -f docker-compose.yml -f docker-compose.https.yml up -d --build
```

安全组需额外放行 **TCP 443**。容器内 Nginx 在检测到上述两个文件后会自动启用 443，并把 **HTTP 重定向到 HTTPS**。请同步把环境变量里的站点地址改为 `https://...`（例如 `FRONTEND_ORIGIN`、`ZHIHU_REDIRECT_URI`）。

# 理财收益追踪（money-tacker）

自托管 Web 应用：用户录入持仓流水，系统结合[中国银行代销理财净值页](https://www.bankofchina.com/sourcedb/srfd6_2024/)采集的公开数据，按持仓账户计算市值与收益。

公开净值与个人仓位分离。界面上的收益数字必须标明对应的净值日，避免把旧净值当成「今天」。

**收益为估算，以发行机构 / 代销机构结算为准。** 本项目不连接银行账户，也不代销下单。

## 能做什么

- 用户名 + 密码登录；数据按账号隔离
- 用产品代码建仓，同一产品可追加买入、部分或全部赎回
- 总览：总市值、累计收益、各账户「当日收益」（各产品展示日可能不同）
- 持仓详情：流水、按日收益、公共净值序列
- 进程内定时全量采集中行代销目录，从采集成功日起积累净值快照（不能回填更早历史）
- 产品不在架时仍可赎回与作废流水，不可追加买入

一期不做：银行授权、全市场历史净值回填、申赎费 / 税费精确核算、FIFO 批次赎回、投顾建议。

## 技术栈

| 层 | 选择 |
| --- | --- |
| 前端 | Vue 3 + TypeScript + Vite + Pinia + vue-router（包管理仅 pnpm） |
| 后端 | Go（`net/http` + chi）、goose 迁移、bcrypt Session |
| 数据 | SQLite（WAL），默认路径 `data/app.db` |
| 采集 | goquery 解析静态 HTML；cron 默认上海时区 `10:00 / 16:00 / 21:00`，启动时再跑一次 |

开发时 Vite 与 Go 两个进程；生产可用「一个 Go 进程 + `web/dist`」。

## 目录

```text
cmd/server/          入口
internal/            HTTP、采集、收益计算、存储
migrations/          SQLite 迁移（embed）
web/                 Vue SPA
docs/                PRD、技术设计、ADR
Dockerfile           多阶段生产镜像
docker-compose.yml   本机构建 money-tacker:local 并启动
CONTEXT.md           领域用语
```

## 环境要求

- Go（模块声明见 `go.mod`）
- Node.js + [pnpm](https://pnpm.io/)（前端构建）
- 能访问中行代销净值页（采集任务）

## 本地开发

终端 1，仓库根目录：

```bash
make run
```

等价于 `go run ./cmd/server`，默认监听 `:8080`。首次启动会创建 `data/app.db` 并尝试全量采集，可能耗时较长。

终端 2：

```bash
cd web
pnpm install
pnpm dev
```

Vite 默认 `http://127.0.0.1:5173`，并将 `/api` 代理到 `http://127.0.0.1:8080`。请在 **5173** 打开页面，以便 Cookie 与 API 同源代理。

库中尚无用户时，可在「注册」页创建第一个账号。之后默认关闭注册；若要开放，设置 `REGISTER_OPEN=true` 后重启服务。

### 测试

```bash
make test
```

## 生产运行（静态资源由 Go 托管）

```bash
cd web
pnpm install
pnpm build
cd ..
go run ./cmd/server
```

当仓库根目录存在 `web/dist` 时，服务端会托管 SPA，可直接访问 `APP_ADDR`（默认 `http://127.0.0.1:8080`）。

## Docker 部署

镜像在本机多阶段构建：前端 `pnpm` 走 `https://registry.npmmirror.com`，Go module 走 `https://goproxy.cn`；运行时是 `scratch`（仅二进制、CA 证书、上海时区、`web/dist`）。`docker-compose.yml` 将产物打成 `money-tacker:local`，并设置 `pull_policy: never`，**不会从仓库拉取同名镜像**，只启动这次（或此前）本机构建的标签。

首次或代码变更后：

```bash
docker compose up -d --build
```

之后若镜像已存在、无需重建：

```bash
docker compose up -d
```

浏览器打开 `http://127.0.0.1:8080`。SQLite 落在仓库旁的 `data/`（已挂载到容器 `/app/data`）。首次启动会全量采集中行代销目录，日志里可能较安静地跑一段时间。

常用命令：

```bash
docker compose logs -f app
docker compose down
```

重建并换用新镜像（不删宿主机 `data/`）：

```bash
docker compose up -d --build --force-recreate
```

Linux 上若容器写不了 `data/`，把目录属主改成镜像内用户：

```bash
mkdir -p data
sudo chown -R 65532:65532 data
```

需要开放注册或管理令牌时，在 `docker-compose.yml` 的 `environment` 里增加对应变量后重新 `up`。构建参数也可覆盖镜像源，例如：

```bash
docker compose build --build-arg GOPROXY=https://goproxy.cn,direct --build-arg NPM_REGISTRY=https://registry.npmmirror.com
```

单独构建（不经 Compose）时，同样打本地标签再跑：

```bash
docker build -t money-tacker:local .
docker run --rm -p 8080:8080 -v "${PWD}/data:/app/data" money-tacker:local
```

## 配置

通过环境变量覆盖，未设置时使用括号内默认值。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `APP_ADDR` | `:8080` | 监听地址 |
| `APP_ENV` | `dev` | 环境名 |
| `APP_TZ` | `Asia/Shanghai` | 自然日、cron、展示日所用时区 |
| `SQLITE_PATH` | `data/app.db` | SQLite 文件 |
| `SESSION_HOURS` | `168` | Session 有效小时数 |
| `CRAWL_CRON` | `0 10,16,21 * * *` | 采集 cron（按时区） |
| `CRAWL_DELAY_MS` | `500` | 分页请求间隔（毫秒） |
| `CRAWL_BASE_URL` | 中行代销净值页 | 采集入口 |
| `DISPLAY_LAG_DAYS` | `1` | 展示日相对自然日的延迟天数 |
| `REGISTER_OPEN` | `false` | 已有用户后是否仍允许注册 |
| `BCRYPT_COST` | `12` | 密码哈希 cost |
| `APP_ADMIN_TOKEN` | 空 | 管理接口令牌；为空时仅本机 Host 可调管理接口 |

手动触发采集（本机或带令牌）：

```bash
curl -X POST http://127.0.0.1:8080/api/admin/crawl
# 若设置了 APP_ADMIN_TOKEN：
curl -X POST -H "X-Admin-Token: <token>" http://127.0.0.1:8080/api/admin/crawl
```

查看采集任务：`GET /api/admin/crawl-runs`（同样需要管理权限）。

## 几个容易混淆的概念

更完整的用语表见 [CONTEXT.md](./CONTEXT.md)。

- **净值日**：该条净值的截止日期，不是打开应用的日历日。
- **展示日**：该产品「当日收益」对应的结算日：不晚于（自然日 − `DISPLAY_LAG_DAYS`）的、库中已有的最大净值日。
- **挂零**：某一自然日没有净值日等于这一天的记录。详情按日收益在挂零日填 `0.00`；市值始终按最新已公布净值。
- **当日收益**（总览）与 **按日收益**（详情日历）口径不同，两处都应写明日期。
- **累计收益** = 未实现收益 + 已实现收益（平均成本法）。

## 文档

| 文档 | 内容 |
| --- | --- |
| [docs/PRD-理财收益追踪应用.md](./docs/PRD-理财收益追踪应用.md) | 产品范围与验收 |
| [docs/技术设计.md](./docs/技术设计.md) | 实现合同 |
| [docs/中国银行理财产品净值数据源说明.md](./docs/中国银行理财产品净值数据源说明.md) | 公开页能提供什么、不能提供什么 |
| [docs/adr/](./docs/adr/) | 架构决策 |
| [CONTEXT.md](./CONTEXT.md) | 领域语言 |

## 许可

仓库内未附 SPDX 许可证文件；使用前请与仓库所有者确认。

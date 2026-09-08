# 壁纸星球 · WallPlanet

Go + PostgreSQL 服务端、React Web 与管理后台。浅色界面以提供的四张 Web 原型为布局基准，含首页瀑布流、详情多图预览、排行榜、个人中心及响应式布局。

## 本机入口

- 正式站点：<http://localhost:8080>；后台：<http://localhost:8080/admin>
- 独立演示：<http://localhost:8081>；后台：<http://localhost:8081/admin>
- API 文档：`server/openapi.json`，运行时为 `/api/v1/openapi.json`。

本次初始化已在提供的 PG 实例创建 `wallplanet` 与隔离演示库 `wallplanet_demo`。正式库不包含演示作品。管理员账号和随机生成的密码记录在本机 `.env` 的 `ADMIN_USERNAME` / `ADMIN_PASSWORD`，该文件已被 Git 和 Docker 构建排除。

未配置 Apify Token 时不会产生真实采集调用。演示模式始终强制禁止保存真实 Token 和启动采集。当前本机正式环境的配置和实网验证结果见 [采集验证记录](docs/APIFY-LIVE.md)。

如本机无法直连 X 图片 CDN，可在服务端 `.env` 设置 `MEDIA_PROXY_URL=http://127.0.0.1:7897`（端口按实际代理填写）。仅支持管理员控制的可信 HTTP(S) 代理；未配置时继续使用带公网 IP 校验的直连下载。代理由其自身解析图片域名，目标及重定向仍限制为 `https://pbs.twimg.com`。Docker 默认不继承本机回环地址，可用 `DOCKER_MEDIA_PROXY_URL=http://host.docker.internal:7897` 单独配置，并确保代理允许容器访问。

## 启动与开发

需要 Go 1.26、Node.js 24、pnpm 9，以及可连接的 PostgreSQL。

```powershell
# 工程根目录：安装依赖、构建前后端
./scripts/build.ps1

# 正式模式（前台运行，Ctrl+C 停止）
./scripts/start.ps1

# 另开终端启动独立演示模式
./scripts/start.ps1 -Demo
```

全新环境先复制 `.env.example` 为 `.env`，填写 PG 连接并生成 32 字节 Base64 `MASTER_KEY`。`scripts/init.ps1` 在没有 `.env` 时会生成主密钥与配置文件，然后提示填写连接。随后执行：

```powershell
./scripts/init.ps1 -CreateAdmin
./scripts/init.ps1 -Demo -CreateAdmin
```

脚本在需要创建管理员时交互读取密码，不提供固定默认密码。已存在的账号不会被覆盖。服务端也支持 `init-db`、`migrate`、`create-admin` 和 `seed-demo` 子命令。`init-db` 只允许创建上述两个库；迁移仅修改所连接的业务库，不触碰其他库。

开发时，在 `server` 目录运行 `go run .`，另一个终端在 `web` 运行 `pnpm dev`，访问 <http://localhost:5173>。Vite 将 `/api`、`/media` 转发到 8080。

## 功能与运营

1. 使用管理员账号登录后台，在「Apify 配置」保存 Token。Token 使用 AES-256-GCM 加密，接口仅返回配置状态与掩码；务必备份并保留原 `MASTER_KEY`。
2. 在「作者与主题」维护主题、作者和推荐信息。在「采集来源」添加 X 作者组（2–100 个用户名）或关键词，选择主题映射、附加标签与尺寸限制。
3. 新来源默认暂停，管理界面预填每 7 天、最多 100 条。手动触发前会显示费用确认，开启定时采集时会显示月度估算。当前 Actor 为 `xquik/x-tweet-scraper`，结果费 $0.15 / 1000 条，另计平台用量；无单独启动费或查询费。作者组每 20 位组成一条 OR 查询，所有查询一次提交、共享整批条数上限，详见 [Xquik 批量采集](docs/XQUIK.md)。每个来源只保留一个活动任务，全局串行运行，两次启动至少间隔 5 分钟。
4. 图片成功落盘且符合分辨率要求后自动发布。后台支持编辑标题、描述、标签、主题、推荐标记及上下架。「首页配置」管理最多 6 张横幅及站内链接。
5. 查看「任务记录」了解状态、运行 ID、已处理与导入数量。部分图片失败时保留成功图片，可以重试一个失败条目或整个任务的失败条目。

账号抓取使用 `from:账号 filter:images since:日期`，关键词抓取使用对应搜索表达式。两种来源都通过 Actor 的 `searchTerms` 输入实现日期水位；不混传 `startUrls` 或 `twitterHandles`。首次回溯 7 天，之后从上次成功水位前一天开始，依靠帖子和媒体标识去重。每次结果受 `maxItems` 限制，不承诺高频账号的完整历史归档。

启动超时会进入「等待核对」，服务端按时间和输入比对最近的 Actor 运行；无法唯一匹配时保留任务，管理员可在后台关联已核对的 run ID，或确认未产生运行。不会自动重复提交状态不明的付费任务。

原图支持静态 JPEG、PNG、WebP，默认短边至少 720、长边至少 1280；最大 25 MB、8000 万像素。PNG/WebP 动画和视频不会导入。图片仅从 `pbs.twimg.com` 的 HTTPS 地址下载，重定向和 DNS 地址经过检查。文件按 SHA-256 复用，并生成宽度至多 720 的 JPEG 缩略图。

## 数据与统计约定

- 一条帖子是一组作品，详情缩略图切换其中各张图片；下载当前图片的原图。
- 游客可浏览、搜索、查看详情和排行；登录后可点赞、收藏、下载、订阅。浏览记录保留最近 50 张。
- 排行统计近 7 天、30 天、全部时间，热度为 `点赞 + 收藏×4 + 下载×2`；同一用户、同一作品在同一 UTC 日内重复下载仅计分一次。每小时刷新快照，作者和渠道汇总关联作品的站内热度；零互动按发布时间稳定排序。
- 下载次数记录已响应的下载请求，不表示浏览器已保存完成。X 原帖互动量保留为独立元数据，不加入站内统计。
- 站内只有 X 渠道。作者是采集来源的创作者，主题由平台维护；用户可订阅三种对象，订阅流取其关联作品的并集。
- 会员、支付、评论、站内通知、用户上传与原生 App 尚未开放，相关预留入口会明确提示。
- 默认使用 Cookie 会话和 CSRF Token，禁止跨来源写操作。公开静态文件不包含密钥，正式与演示使用不同 Cookie 名称与独立数据库。

## 测试

```powershell
# 单元测试 + 类型检查与生产构建
./scripts/test.ps1

# 集成测试使用唯一 test_<随机值> schema，结束后只清理该 schema
./scripts/test.ps1 -Integration

# 先启动 8081 演示实例并准备演示管理员，再运行浏览器测试
cd web
pnpm exec playwright install chromium
cd ..
./scripts/test.ps1 -Integration -Browser
```

浏览器测试覆盖 1440、1024、390 宽度，截图输出在 `artifacts/`；用户流程包括注册、搜索、收藏、点赞、订阅、下载、全屏预览、资料编辑及后台权限隔离。浏览器测试只针对独立演示环境，会产生测试账号和暂停的测试来源，不调用 Apify。

服务端测试覆盖密码与密钥加密、来源 URL、图片格式及去重、Actor 输入与字段解析、错误响应、部分导入重试、Dataset 分页恢复、迁移、任务锁、游标分页及排行榜计算。样本和模拟 HTTP 服务用于自动测试，真实 Actor 运行结果单独记录在 [采集验证记录](docs/APIFY-LIVE.md)。

## Docker 部署

```powershell
docker compose build
docker compose run --rm app init-db
docker compose run --rm app migrate
docker compose run --rm app create-admin
docker compose up -d app

# 可选的独立演示服务
docker compose --profile demo run --rm demo init-db
docker compose --profile demo run --rm demo seed-demo
docker compose --profile demo run --rm demo create-admin
docker compose --profile demo up -d demo
```

Compose 连接外部 PG，不再启动一个新 PG 服务；媒体保存在命名卷中。创建管理员命令读取 `.env` 中的管理员配置，或者通过 `docker compose run -e ADMIN_USERNAME -e ADMIN_PASSWORD ...` 传入环境变量。

上线时设置 `DEPLOY_ORIGIN=https://你的域名`、`COOKIE_SECURE=true`，反向代理 `/` 到应用即可。备份 PostgreSQL、媒体卷和 `MASTER_KEY`；三者需要一起恢复。健康检查位于 `/api/v1/health`，任务状态在后台可查看。

## 工程结构

- `server/`：Go HTTP API、认证、采集调度、Apify 适配、图片存储、嵌入式 SQL 迁移与测试。
- `web/`：React 用户端和管理员界面、响应式样式、固定本地演示素材与 Playwright 测试。
- `scripts/`：Windows 初始化、构建、启动和验证脚本；OpenAPI 文档生成脚本。

视觉素材来源见 `web/public/assets/SOURCES.md`。原型中的独立素材未提供，因此使用同题材图片，未把整张原型当成网页背景。

## NAS 产物部署

通过 `scripts/release.ps1` 在开发机打包 Linux 可执行文件和前端静态文件，NAS 仅装配运行时镜像。默认端口 18082，详细步骤见 [部署说明](docs/DEPLOYMENT.md)。

# NAS 产物部署

面向 Linux amd64 NAS，默认端口 18082，部署目录 `/vol1/1000/Server/wallplanet`。构建在开发机完成，NAS 无需 Go、Node.js 或源码。

## 构建与传输

在 Windows 项目目录运行：

```powershell
./scripts/release.ps1 -Tag 20260908-1
scp releases/wallplanet-20260908-1-linux-amd64.tar.gz nas:/vol1/1000/Server/wallplanet/releases/
scp deploy/compose.yaml nas:/vol1/1000/Server/wallplanet/compose.yaml
```

首次使用时先通过 SSH 创建目标目录、`releases/20260908-1` 和 `data`。压缩包只包含 Linux 可执行文件、前端生产文件和运行时 Dockerfile，不包含 `.env`、源文件、依赖目录或采集图片。

在 NAS 单独创建权限为 600 的 `.env`：

```dotenv
DATABASE_URL=postgres://USER:PASSWORD@DB_HOST:5432/wallplanet?sslmode=disable
MASTER_KEY=REPLACE_WITH_EXISTING_KEY_OR_NEW_32_BYTE_BASE64_KEY
DEPLOY_ORIGIN=http://NAS_IP:18082
DEPLOY_PORT=18082
RELEASE_TAG=20260908-1
MEDIA_PROXY_URL=
COOKIE_SECURE=false
TZ=Asia/Hong_Kong
```

迁移已有数据库必须沿用原 MASTER_KEY，否则无法解密已保存 Token。不要把配置提交到 GitHub。首次新装按照 README 初始化数据库和管理员；已有安装沿用账号、会话和数据。图片单独迁移到 `data/`，停用旧正式实例以免多个本地存储实例产生不一致。

## 启动

```sh
cd /vol1/1000/Server/wallplanet
chmod 600 .env
tar -xzf releases/wallplanet-20260908-1-linux-amd64.tar.gz -C releases/20260908-1
docker build -t wallplanet:20260908-1 releases/20260908-1
docker compose up -d
docker compose ps
curl -f http://127.0.0.1:18082/api/v1/health
```

镜像只装配产物，不在 NAS 编译。以 UID 1000 / GID 1001 运行，`data` 必须允许此用户读写；宿主机用户不同需同步调整运行时 Dockerfile。NAS 配置使用 host 网络，应用直接监听 `DEPLOY_PORT`；同机 PostgreSQL 映射端口和代理请使用 `127.0.0.1` 连接，避免 NAS 回连自身局域网地址失败。容器重启策略为 `unless-stopped`，数据目录持久化，沿用外部 PostgreSQL。

## 更新、回退与备份

每次使用新标签构建，把新包解压到对应版本目录并创建同名镜像，再修改 `.env` 中 `RELEASE_TAG` 后执行 `docker compose up -d`。保留上一版目录和镜像。回退需确认数据库迁移兼容旧版本，然后恢复旧标签并重建容器。

更新前备份外部 PostgreSQL 的 `wallplanet` 库以及 NAS 的 `.env`、`data/`。不要将数据库备份或密钥放进公开仓库。`docker compose logs --tail=100` 查看服务日志，`docker compose stop` 暂停服务。修改访问地址时同步更新 `DEPLOY_ORIGIN`，否则登录写操作的来源检查会拒绝请求。

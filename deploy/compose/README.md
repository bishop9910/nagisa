# 容器化部署（Docker Compose）

一套 `docker compose up -d` 就能跑起来的完整网盘：**nagisa**（REST + gRPC + 前端 + `/docs/`）加 **SeaweedFS**（S3 对象存储）。镜像里已经包含构建好的前端，所以打开 `http://127.0.0.1:8000` 就是网盘界面，不再是占位页。

> 只想了解镜像本身（分层、为什么用 alpine、如何单跑一个容器）：看 [`docs/deployment.md`](../../docs/deployment.md#7-容器化) 第 7 节。本文只讲这套 Compose 怎么用。

---

## 1. 快速开始

```bash
cd deploy/compose
cp .env.example .env
```

编辑 `.env`，**至少改这两项**：

| 变量 | 怎么填 |
| --- | --- |
| `JWT_SECRET` | `openssl rand -base64 48` 的输出。它同时签发访问令牌与所有签名下载/分享地址；留空则每次重启随机生成，重启后所有令牌和下载链接立即失效 |
| `ADMIN_PASSWORD` | 内置管理员 `admin` 的初始口令。留空则首启随机生成并只在日志里打印一次（见第 4 节） |

然后：

```bash
docker compose up -d --build      # 首次构建约几分钟（Go 模块 + 前端依赖）
docker compose ps                 # 两个服务都应是 healthy
curl -s http://127.0.0.1:8000/v1/system/health
```

期望最后一条返回：

```json
{"status":"ok", "checks":{"database":"ok", "object_storage":"ok"}, ...}
```

浏览器打开 `http://127.0.0.1:8000` 即可。API 文档在 `http://127.0.0.1:8000/docs/`。

> 端口冲突时改 `.env` 里的 `NAGISA_HTTP_PORT`（默认 8000）。容器内部的 8000/8333/9000 不动。

---

## 2. 目录里有什么

```
deploy/compose/
  docker-compose.yml        两个服务、两个命名卷、健康检查与日志轮转
  .env.example              全部可调项 + 说明，复制成 .env 后生效
  seaweedfs/s3.json         SeaweedFS 的 S3 凭据，必须与 .env 里的 S3_ACCESS_KEY/S3_SECRET_KEY 一致
  data/.keep                无用，只用来在镜像里占住 /app/data 并带上 uid 65532
```

镜像构建上下文是**仓库根目录**（`context: ../..`），因为一个镜像里同时编译 Go 服务与前端。

> ⚠️ **`docker.io` 连不上的网络（中国大陆很常见）先看一眼 `.env` 里的 `REGISTRY_MIRROR`。** 报错长这样：
>
> ```
> Error response from daemon: failed to resolve reference "docker.io/chrislusf/seaweedfs:4.05":
>   failed to do request: Head "https://registry-1.docker.io/v2/...": dial tcp ...: i/o timeout
> ```
>
> 三个基础镜像（`node:24-alpine`、`golang:1.25`、`alpine:3.21`）与对象存储镜像都在 Docker Hub 上，所以都需要镜像站。`.env.example` 里默认填了一个实测可用的（`docker.1panel.live/`），换掉即可。**注意运行段是 alpine 而不是 distroless**：distroless 在 `gcr.io`，Docker Hub 的镜像站不代理它，连不上 `gcr.io` 的网络根本构建不出来——原因见 [`docs/deployment.md`](../../docs/deployment.md#71-镜像)。

---

## 3. 上传模式：proxy 还是 presigned

这是唯一一个开箱即用会踩坑的选择，所以默认选了最不容易出问题的那条。

| | `proxy`（默认） | `presigned` |
| --- | --- | --- |
| 字节路径 | 浏览器 → nagisa → SeaweedFS | 浏览器 → SeaweedFS |
| 需要浏览器能访问 S3 端口 | 否 | **是**（要设 `S3_PUBLIC_ENDPOINT`） |
| 需要对象存储给浏览器回 CORS 预检 | 否 | **是**（SeaweedFS 侧配置） |
| 服务端开销 | 每个分片都过一遍 Go | 只签发地址 |
| 适用 | 任何浏览器、任何地址，包括手机连局域网 IP | 大文件多、想让存储直接吃带宽 |

切换方式：把 `.env` 里的 `UPLOAD_MODE` 改成 `presigned`，同时填上 `S3_PUBLIC_ENDPOINT`（例如 `192.168.1.23:8333`，必须是**浏览器**能连到的地址，不能写 `seaweedfs:8333`），然后 `docker compose up -d`。

改成 `presigned` 后如果浏览器报 CORS 或连接失败，两条路：把 SeaweedFS 的 CORS 配好（见 [SeaweedFS S3 CORS](https://github.com/seaweedfs/seaweedfs/wiki/S3-CORS)），或直接退回 `UPLOAD_MODE=proxy`——功能上没有任何缺失，只是字节多走一跳。

> `S3_PUBLIC_ENDPOINT` 留空时，`GetDownloadUrl` 不会下发对象存储直链，而是回落到服务端流式端点 `/v1/files/{id}/content`（支持 `Range`）。所以**留空也能正常下载**，这是刻意设计的：签名地址里绑定的主机名对另一台设备往往是错的。

---

## 4. 首次启动要做的事

1. **确认健康**：`docker compose ps` 两个服务都 `healthy`；`curl .../v1/system/health` 里 `object_storage` 为 `ok`。
2. **拿到管理员口令**：如果 `.env` 里设了 `ADMIN_PASSWORD` 就用它；留空的话：
   ```bash
   docker compose logs nagisa | grep -i "generated password"
   ```
   那条 WARN 的 `password` 字段就是初始口令，只出现一次。登录后立刻改掉。
3. **别把 8333 暴露到公网**（`presigned` 模式下它需要被浏览器访问，但那应当只在可信网络里）。

---

## 5. 数据、备份与升级

两个命名卷：

| 卷 | 内容 | 丢了会怎样 |
| --- | --- | --- |
| `nagisa-data` → `/app/data` | SQLite 数据库 + RSA 口令私钥 | 元数据全丢；私钥丢失会让客户端已缓存的公钥失效 |
| `seaweedfs-data` → `/data` | 文件内容 | 内容全丢 |

**备份必须三件套一起**（数据库快照 + 对象存储 + 私钥），只有数据库没法恢复文件。在线一致快照：

```bash
# 数据库（.backup 是 SQLite 的在线一致快照，不要直接 cp 运行中的文件）
docker compose exec nagisa /app/nagisa -health http://127.0.0.1:8000/v1/system/health >/dev/null
docker run --rm -v nagisa-data:/data -v "$PWD:/backup" alpine \
  sh -c 'apk add -q sqlite && sqlite3 /data/nagisa.db ".backup /backup/nagisa-$(date +%F-%H%M).db"'

# 对象存储
docker run --rm -v seaweedfs-data:/data -v "$PWD:/backup" alpine \
  tar czf /backup/seaweedfs-$(date +%F).tar.gz -C /data .
```

恢复时把卷重新挂上去即可；数据库与对象存储的快照不必是同一时刻，但要成对保留。

升级：`git pull && docker compose up -d --build`。卷不动，数据保留。`NAGISA_VERSION` 只影响镜像 tag 与 `/v1/system/info` 显示的版本号。

> ⚠️ **不要起两个 nagisa 副本指向同一个 SQLite 文件。** 写事务靠**进程内**互斥锁串行化（`internal/data/data.go` 的 `writeMu`），跨进程无效；需要横向扩展就按 [`docs/deployment.md`](../../docs/deployment.md#24-auto_migrate-与切换到-mysql) 换成 MySQL。所以本 compose 里 nagisa 是单实例，没有 `deploy.replicas`。

---

## 6. 常见改动

### 想在宿主机上直接看到数据文件（bind mount）

命名卷的文件在 `/var/lib/docker/volumes/` 下，不直观。改成 bind mount：

```yaml
    volumes:
      - ../data/nagisa:/app/data
```

但容器以 uid 65532 运行，宿主机目录必须让这个 uid 能写：

```bash
mkdir -p data/nagisa
sudo chown -R 65532:65532 data/nagisa      # 或把 NAGISA_UID/NAGISA_GID 设成 id -u / id -g
```

忘了这一步的典型现象是启动即失败，日志里是 `data: open database: unable to open database file`。

### 前面挂一层 HTTPS 反代

局域网里用 `http://<IP>` 打开时浏览器不提供 `crypto.subtle`，登录会报「无法加密口令」（原因见 [`docs/deployment.md`](../../docs/deployment.md#34-局域网按-ip-访问安全上下文与证书)）。加一层 Caddy/nginx 即可，注意：

- 反代要转发 `Host`、`X-Forwarded-For`、`X-Forwarded-Proto`；上传下载要关掉缓冲（nginx：`proxy_buffering off; proxy_request_buffering off;`），超时对齐 `600s`。
- `PUBLIC_BASE_URL` **保持留空**，让服务端下发相对地址——这样局域网 IP、localhost、反代域名三种入口都正确。
- 反代后面 `UPLOAD_MODE=proxy` 依然是最省事的选择。

`deploy/Caddyfile` 是给「后端跑在宿主机」写的（`reverse_proxy 127.0.0.1:8000`），容器部署时改成 `reverse_proxy nagisa:8000` 或宿主机端口即可。

### 用外部/已有 S3 存储

删掉 `seaweedfs` 服务与 `seaweedfs-data` 卷，改 `.env` 里的对象存储变量，并把 `docker-compose.yml` 里 nagisa 的 `S3_ENDPOINT` 从 `seaweedfs:8333` 换成实际地址（`KRATOS_` 与 `S3_*` 这类环境变量本来就是为覆盖默认值准备的，不改代码）。能力要求清单见 [`docs/deployment.md`](../../docs/deployment.md#17-换后端时的兼容性核对清单)。

### 只跑后端、不要对象存储

`.env` 里把 `S3_ENDPOINT` 留空并删掉 `seaweedfs` 服务，服务能起来，但上传/下载会返回 `NETDISK_UNAVAILABLE`——适合只想看接口与文档的场景。

---

## 7. 排障

| 现象 | 原因 / 处理 |
| --- | --- |
| 启动即 `S3 bucket probe failed` / 容器反复重启 | 对象存储没就绪或凭据不匹配。`docker compose logs seaweedfs`；`seaweedfs/s3.json` 与 `.env` 的 key 必须一致 |
| `unable to open database file` | bind mount 目录属主不对，见第 6 节；或 `NAGISA_UID/NAGISA_GID` 与卷属主不一致 |
| 每次重启都要求重新登录、下载链接失效 | `JWT_SECRET` 没设或为空 |
| 页面能开、上传 404/失败 | 看 `docker compose logs nagisa`；`presigned` 模式下多半是 `S3_PUBLIC_ENDPOINT` 或 CORS |
| 想重置全部数据 | `docker compose down -v`（**会删掉两个卷**） |
| 想看服务端日志 | `docker compose logs -f nagisa`（日志按 json-file 轮转，单文件 10 MB、保留 3 个） |
| gRPC 客户端连不上 | 默认只绑 `127.0.0.1:9000`；要对外就把 `docker-compose.yml` 里那条 port 映射前的 `127.0.0.1:` 去掉 |
| 运行镜像是 alpine，可以 `docker compose exec nagisa sh` 进容器 | 查健康状态用 `docker inspect --format '{{.State.Health.Status}}' $(docker compose ps -q nagisa)`；`/app/nagisa -health http://127.0.0.1:8000/v1/system/health` 也能直接当探针跑 |
| 需要翻文件/看数据库 | `docker compose exec nagisa sh`，或借一次性容器挂同一个卷：`docker run --rm -it --volumes-from $(docker compose ps -q nagisa) alpine sh`（卷里就是 `/app/data`） |
| **拉镜像超时**：`failed to resolve reference "docker.io/...": i/o timeout` | 这台机器连不上 Docker Hub。在 `.env` 里设 `REGISTRY_MIRROR=<镜像站>/`（模板默认给了一个实测可用的），然后重新 `docker compose up -d --build`。四个镜像（node/golang/alpine/seaweedfs）都走这个前缀 |
| 构建时 Go 模块下载失败 | 构建容器需要能访问 Go 模块代理与 npm registry；给 Docker 守护进程配好代理，或把 `GOPROXY` 通过 Dockerfile 的构建参数传入 |

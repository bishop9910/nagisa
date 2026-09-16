# 部署与运维

本文覆盖 Nagisa 网盘后端的部署落地：对象存储（默认 SeaweedFS）与 SQLite 的准备、反向代理与 TLS、首次启动检查清单、运维任务、前端发布与容器化。

**配置键的完整说明在 [`configuration.md`](configuration.md)**——包括每个键的类型、代码默认值、生效位置、环境变量覆盖方式、值格式约定与三套可抄的示例。本文只重复上线时必须确认的几项。

## 0. 上线前必须确认的配置

| 键 | 必须设置成 | 不设的后果 |
| --- | --- | --- |
| `auth.jwt_secret` | 一段足够长的随机串（`KRATOS_JWT_SECRET`） | 每次重启随机生成，所有令牌与签名 URL 立即失效 |
| `auth.admin_password` | 明确的强口令（`KRATOS_ADMIN_PASSWORD`） | 随机生成并打印一次到启动日志，容易漏看 |
| `data.object_storage.endpoint` | 实际的对象存储地址 | 留空等于禁用对象存储，上传/下载返回 `NETDISK_UNAVAILABLE` |
| `data.object_storage.public_endpoint` | 浏览器可达的地址（与服务端不同时） | 预签名 URL 的主机名与签名不一致，浏览器直传/直下失败 |
| `data.database.source` | 持久卷内的路径（如 `/srv/nagisa/data/nagisa.db`） | 容器重建即丢数据 |
| `auth.password_private_key_file` | 持久卷内的路径 | 私钥丢失后，客户端用旧公钥加密的密码全部无法解密 |
| `server.http.timeout` | `600s` 量级 | kratos 默认 1s 会掐断任何真实传输 |
| `web.cors_origins` | 前端的确切源站 | 留空则不下发任何 CORS 头，浏览器前端无法调用 |
| `auth.allow_plain_password` | `false` | 退化为允许明文密码传输 |
| `server.grpc.addr` | 与对象存储的端口错开 | SeaweedFS 用 `8333`/`9333`/`8888`/`8080`，与默认的 `8000`/`9000` 都不冲突；若改过端口，别撞上这几个 |

以上每一项的细节与取值理由见 [`configuration.md`](configuration.md)。

## 1. 对象存储（默认 SeaweedFS）

Nagisa 只使用标准 S3 API，**任何 S3 兼容实现都可以**：换后端只改 `data.object_storage.endpoint` 与凭据，代码不用动（需要的能力见 1.7）。

默认推荐 **SeaweedFS**：Apache-2.0、单个二进制、维护活跃，`UploadPartCopy`、预签名 URL 与 `response-content-disposition` 都支持，正好覆盖分片上传、服务端组装与签名直链三条链路。

> **为什么不再推荐 MinIO**：开源版 MinIO Server、`mc`、KES 已归档，官方下载站对所有社区版 release 与 hotfix 返回 410，不再提供安全更新与安全公告，也不接收漏洞报告。商业版 AIStor 按容量与节点订阅——对一个自带存储的网盘后端没有必要。已部署的旧版本仍能运行，但不会再有任何修复。

### 1.1 启动 SeaweedFS

```bash
# 单机：一个进程同时提供 master、volume、filer 与 S3 API
weed server -s3 -dir=/srv/seaweedfs -master.dir=/srv/seaweedfs/meta -s3.config=/etc/seaweedfs/s3.json
```

Windows（PowerShell）等价写法，数据与凭据都放在仓库的 `data/seaweedfs`（该目录已被 `.gitignore` 忽略，不会误提交）：

```powershell
weed server -s3 -dir=.\data\seaweedfs -master.dir=.\data\seaweedfs\meta -s3.config=.\data\seaweedfs\s3.json
```

> ⚠️ **`-dir` 不能单独给。** 元数据目录的参数是 `-master.dir`，而它**不会**从 `-dir` 继承：`server.go` 的可写性检查跑在继承逻辑之前，于是启动会直接 Fatal：
>
> ```
> Check Meta Folder (-mdir="") Writable: Stat : The system cannot find the path specified.
> ```
>
> 两个目录都要显式写出（`-master.dir` 的目录会自动创建）。完全不给 `-dir` 时它才会用系统临时目录兜底——但那样数据会随临时目录被清理，不要这么跑。

还没有 `weed` 时，两种获取方式：从 [SeaweedFS Releases](https://github.com/seaweedfs/seaweedfs/releases) 下 `weed_windows_amd64.zip` 解压到 PATH，或直接 `go install github.com/seaweedfs/seaweedfs/weed@latest`（装到 `%GOPATH%\bin`）。

`-s3.config` 指向的凭据文件要自己创建，内容见 1.2；仓库里已经放了一份开发用的 `data/seaweedfs/s3.json`，与 `configs/config.yaml` 的默认凭据一致。

默认端口：S3 `8333`、master `9333`、filer `8888`、volume `8080`（另外还有 `8181`/`9101` 等辅助端口）。Nagisa 的 `server.http.addr`（`8000`）与 `server.grpc.addr`（`9000`）与它们都不冲突，默认值即可。

### 1.2 凭据

凭据文件（开发用示例见仓库里的 `data/seaweedfs/s3.json`，生产放 `/etc/seaweedfs/s3.json` 并 `chmod 600`）：

```json
{
  "identities": [
    {
      "name": "nagisa",
      "credentials": [{ "accessKey": "nagisa", "secretKey": "nagisa-secret" }],
      "actions": ["Read:nagisa", "Write:nagisa", "List:nagisa", "Tagging:nagisa"]
    }
  ]
}
```

`accessKey` / `secretKey` 必须与 `data.object_storage.access_key` / `secret_key` 一致。语法以 SeaweedFS 官方 [S3 Credentials](https://github.com/seaweedfs/seaweedfs/wiki/S3-Credentials) 为准，也可以用 `weed shell` 在线增删。

> ⚠️ **桶级动作不能建桶。** `CreateBucket` 走的是独立授权路径（见 [discussion #7604](https://github.com/seaweedfs/seaweedfs/discussions/7604)、[PR #11049](https://github.com/seaweedfs/seaweedfs/pull/11049)），所以上面这种只授权到桶的身份在 `auto_create_bucket: true` 时会拿到 `AccessDenied`。两种情况二选一：
>
> - **开发图省事**：`actions` 直接写 `["Admin"]`（仓库里那份开发凭据就是这么配的）。
> - **生产**：先用 `weed shell` 的 `s3.bucket.create -name nagisa` 建好桶（该命令走 master，不经 S3 授权），再把 `auto_create_bucket` 设为 `false`，凭据保持桶级最小权限。

### 1.3 建桶

| 方式 | 做法 | 适用 |
| --- | --- | --- |
| 交给服务端（推荐） | `data.object_storage.auto_create_bucket: true` | 不需要任何外部客户端。启动时探测桶（15s 超时），不存在就用配置的 `region` 创建。**要求凭据能建桶，见 1.2** |
| `weed shell` | `s3.bucket.create -name nagisa` | 想在建桶时一并设置配额、生命周期等 |
| 任意 S3 客户端 | `aws s3api create-bucket --bucket nagisa --endpoint-url http://127.0.0.1:8333` | 已有现成运维脚本 |

**桶保持私有**：下载一律走 Nagisa 的签名直链或流式接口，不要给桶开匿名读。

- 探测桶本身失败（连不上、凭据不对）会让启动直接报错，这是故意的：宁可起不来，也不要带病运行。
- `prefix` 让同一桶承载多个环境（例如 `netdisk-prod`、`netdisk-staging`），键不会互相干扰。

### 1.4 跑通验证

起好对象存储后，先用一个真实后端的往返测试确认整条链路（分片上传 → 服务端组装 → 跨分片读取 → 预签名下载校验字节与文件名 → 浏览器式预签名 PUT → 前缀删除）：

```powershell
$env:NAGISA_S3_SMOKE_ENDPOINT="127.0.0.1:8333"
go test -tags s3smoke ./internal/data/ -run LiveS3 -v
```

它默认用 `nagisa` / `nagisa-secret` / `nagisa-smoke` 桶，需要在 `data/seaweedfs/s3.json` 里对得上（凭据见 1.2）；桶名与凭据可用 `NAGISA_S3_SMOKE_ACCESS_KEY`、`NAGISA_S3_SMOKE_SECRET_KEY`、`NAGISA_S3_SMOKE_BUCKET` 覆盖。测试自己清理写入的对象。

服务侧的自检则是：

```bash
curl -s http://127.0.0.1:8000/v1/system/health   # 期望 checks.object_storage = ok
curl -s http://127.0.0.1:8000/v1/system/info     # 期望 storage_backend = s3
```

### 1.5 endpoint 与 public_endpoint

| 配置 | 谁用它 | 为什么需要分开 |
| --- | --- | --- |
| `endpoint` | 服务端进程访问对象存储 | 常常是内网地址（`seaweedfs:8333`）或 localhost |
| `public_endpoint` | 浏览器访问对象存储 | 浏览器可能完全到不了内网地址 |

**预签名 URL 的签名对象是主机名。** 服务端用 `public_endpoint` 建一个独立的 S3 客户端，所有交给客户端的预签名地址都由这个客户端签发：

| 场景 | 用哪个客户端 | 结果 |
| --- | --- | --- |
| `PresignPutObject`（分片直传） | `public_endpoint`（未配置时用 `endpoint`） | 浏览器拿到的 PUT 地址主机名与签名一致，可以成功上传 |
| `PresignGetObject`（下载直链） | 同上 | 浏览器可以直接下载 |
| `PutObject` / `GetObject` / `ComposeObject` / `RemovePrefix` | 始终 `endpoint` | 服务端自己的读写走内网，不受影响 |

只配 `endpoint`（不配 `public_endpoint`）而浏览器又到不了该地址时，最典型的表现是：`POST /v1/files/uploads/create` 正常返回 `parts[].uploadUrl`，但浏览器 PUT 该地址失败（DNS/连接超时）。此时要么补上 `public_endpoint`（例如反向代理出来的 `https://files.example.com`），要么把 `upload.default_mode` 改成 `proxy`。

另外两点容易被忽略：

- **浏览器直连对象存储时，跨域预检由对象存储自己回答。** 本服务从不修改桶的 CORS 配置，因此需要在对象存储侧为前端源站配置 CORS 规则（允许 `PUT`/`GET`、`Authorization` 等头；SeaweedFS 见官方 [S3 CORS](https://github.com/seaweedfs/seaweedfs/wiki/S3-CORS)）。也可以直接让前端走代理模式（`upload.default_mode: proxy`），分片经由 Nagisa 转发，完全不依赖对象存储的 CORS。
- **5 MiB 规则**：S3 multipart 要求除末片外每片至少 5 MiB。`upload.min_chunk_size` 低于 `5242880` 会导致 `CompleteUpload` 组装失败或返回 `NETDISK_UPLOAD_INCOMPLETE`。末片可以是任意大小（含 0 字节）。

### 1.6 取消 / 过期上传的资源回收

取消上传会删除 `uploads/<uploadId>/` 前缀；删除失败时对象键进入待删除队列，由 `purge_deletions` 重试。超过 `upload.session_ttl` 的会话由 `expire_uploads` 置为 `EXPIRED` 并排队清理。两者都应纳入定时维护（见第 5 节）。

### 1.7 换后端时的兼容性核对清单

客户端（`internal/data/storage.go`，基于 AWS SDK for Go v2）只用下面这些能力。换成别的 S3 实现前，逐条确认：

| 能力 | 用在哪 | 不支持的后果 |
| --- | --- | --- |
| `CreateMultipartUpload` / `UploadPart` / `CompleteMultipartUpload` | 分片上传（含浏览器直传） | 大文件完全传不了 |
| `UploadPartCopy` | `CompleteUpload` 的服务端组装（把 `uploads/<id>/parts/N` 拼成 `files/<owner>/<nodeId>`） | 组装失败，上传永远完不成 |
| SigV4 预签名（query 形式，path-style） | 分片直传与下载直链 | 默认上传模式失效，只能退回 `proxy` |
| 预签名 GET 上的 `response-content-disposition` / `response-content-type` | 下载时指定文件名与类型 | 下载文件名变成对象键 |
| `DeleteObjects`（批量删除） | 取消上传、回收站清理、维护任务 | 清理留残，前缀删除报错 |
| `ListObjectsV2`（`prefix`，无 `delimiter`） | 配额统计、孤儿回收 | 统计与 GC 失效 |
| `HeadObject` / `HeadBucket` | 健康检查、组装前的尺寸探测 | 启动自检与 `GetStorageStats` 异常 |

寻址方式是 **path-style**（`http://host:8333/<bucket>/<key>`），这是自建 S3 的通用形态，也是预签名 URL 出现在浏览器里的样子；因此请让后端保持在 path-style 下工作。

⚠️ **`UploadPartCopy` 单次最多 5 GiB**。这不是问题：S3 规定单个分片本身也不能超过 5 GiB，所以每个源对象一定在限制内，组装始终是「一个源对象 = 一个目标分片」。

已核对可用的实现：SeaweedFS（Apache-2.0）、Ceph RGW（LGPL-2.1）、以及托管服务 AWS S3 / Cloudflare R2 / Backblaze B2。**MinIO 的冻结版本仍然能用**（能力都齐），但按第 1 节开头所述不会再有修复，新部署不建议再选它。

## 2. SQLite 生产实践

### 2.1 必开的设置

| 设置 | 值 | 原因 |
| --- | --- | --- |
| `data.database.wal` | `true` | 读不阻塞写；崩溃恢复更快 |
| `data.database.busy_timeout` | `10s` 或更大 | 等锁而不是立刻报错 |
| `data.database.foreign_keys` | `true` | 真正执行外键约束 |
| `server.http.timeout` | `600s` | 默认 1s 会中断大文件传输 |

### 2.2 单写者与单实例

写事务由**进程内**互斥锁串行化（`internal/data/data.go` 的 `writeMu`），并配合 `_txlock=immediate` 让事务一开始就取写锁。这保证了「先查重再插入」「先查配额再写入」这类序列的正确性，但**这个互斥锁不能跨进程生效**：

- 不要把多个 Nagisa 实例指向同一个 SQLite 文件（会出现写锁竞争，重名检查与配额检查也不再互斥）。
- 需要横向扩展时改用 MySQL（见 2.4），此时并发写由数据库的锁与事务保证，代码无需改动。

### 2.3 备份

在线一致快照（推荐，脚本里可以用）：

```bash
sqlite3 ./data/nagisa.db ".backup '/backup/nagisa-$(date +%F-%H%M).db'"
```

停服务后复制文件：

```bash
systemctl stop nagisa
sqlite3 ./data/nagisa.db "PRAGMA wal_checkpoint(TRUNCATE);"   # 可选：把 WAL 合并回主库
cp ./data/nagisa.db /backup/
systemctl start nagisa
```

要点：

- 直接 `cp` 运行中的 SQLite 文件会得到不一致的快照，必须停进程或改用 `.backup`；WAL 模式下还要一并处理 `-wal` / `-shm`。
- 备份要同时覆盖**对象存储**与 `auth.password_private_key_file` 指向的私钥文件（数据库与内容缺一不可；私钥丢失会导致客户端缓存的公钥失效，必须在下次登录前重新拉取 `GET /v1/auth/config`）。
- 对象存储侧使用 `mc mirror` 或后端自带的桶复制能力，备份与数据库快照不必严格同一时刻，但应成对保留。

### 2.4 auto_migrate 与切换到 MySQL

`auto_migrate` 为 true 时启动会执行 `ent` 的 `Schema.Create`，按当前 schema 创建/更新表结构：

| 环境 | 建议 |
| --- | --- |
| 开发 | `true`，改 schema 后重启即可 |
| 生产 | **首次启动临时开启，确认表结构正确后改回 `false`**；本仓库没有独立的迁移命令与版本化迁移文件，schema 变更应由运维评审后在维护窗口执行 |

切换到 MySQL 只需改配置（`go-sql-driver/mysql` 已在代码里注册）：

```yaml
data:
  database:
    driver: mysql
    source: "nagisa:secret@tcp(127.0.0.1:3306)/nagisa?parseTime=true&charset=utf8mb4&loc=Local"
    auto_migrate: true
    max_open_conns: 32
    max_idle_conns: 16
```

注意：

- DSN 必须带 `parseTime=true`，模型里有 `time.Time` 字段。
- `wal` / `busy_timeout` / `foreign_keys` 三个键只对 SQLite 生效，MySQL 下会被忽略。
- 首次连接前要先建好库与账号；`auto_migrate` 负责建表。

## 3. 反向代理、TLS 与 CORS

### 3.1 nginx 示例

```nginx
upstream nagisa_api {
    server 127.0.0.1:8000;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name netdisk.example.com;

    ssl_certificate     /etc/letsencrypt/live/netdisk.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/netdisk.example.com/privkey.pem;

    # 内联上传（UploadSmallFile）与 proto JSON 请求体：留出余量
    client_max_body_size 64m;

    location / {
        proxy_pass http://nagisa_api;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-Id      $request_id;   # 会写入审计日志并回显

        # 流式下载与实时 zip 不能被缓冲，否则首字节永远等不到
        proxy_buffering off;
        # 大分片上传：不要在代理层把请求体攒完再转发
        proxy_request_buffering off;

        proxy_connect_timeout 10s;
        proxy_send_timeout    600s;
        proxy_read_timeout    600s;

        # 发布前端时开启：静态资源不发 Cookie/Authorization 也可
        # add_header Cache-Control "no-cache" always;
    }

    # 只暴露接口时把上面的 location / 换成：
    # location /v1/ { proxy_pass http://nagisa_api; ... }
    # location /docs/ { proxy_pass http://nagisa_api; ... }

    # 如果对象存储也经此代理（配合 public_endpoint 使用）
    location /storage/ {
        proxy_pass http://127.0.0.1:9000/;
        proxy_set_header Host $host;   # 关键：签名校验的主机名必须与 public_endpoint 一致
        proxy_buffering off;
    }
}
```

要点：

- **`proxy_buffering off` 与 `proxy_request_buffering off`**：前者保证 zip/流式下载边生成边下发，后者避免上传在代理层被完整缓存。
- **Host 头**：预签名 URL 的签名绑定主机名。若通过代理暴露对象存储，`public_endpoint` 必须写成浏览器看到的地址（如 `https://netdisk.example.com/storage` 对应的主机），并且代理转发时保持同一个 Host。
- TLS 在代理层终止；后端的 `web.public_base_url` 要写成 `https://...`，否则它会用 http 拼出签名地址，浏览器会因混合内容拦截。
- gRPC 端口（默认 9000）如果没有对外需求，不要暴露到公网。

### 3.2 public_base_url 的作用范围

| 使用点 | 影响 |
| --- | --- |
| `GetArchiveUrl` | 打包下载地址的前缀；留空时是相对路径 `/v1/files/...` |
| `SystemInfo.public_base_url` | 前端读取它来决定 API 与分享地址 |
| `Share.url` | 分享链接的绝对地址（`public_base_url` + `share_path_prefix` + `token`） |

### 3.3 CORS

CORS 过滤器只在 `web.enabled` 为 true 时挂载，行为由 `web.cors_*` 决定：

| 配置 | 行为 |
| --- | --- |
| `cors_origins` 命中请求 `Origin` | 回显该源站，并加 `Vary: Origin` |
| `cors_origins` 含 `*` 且 `cors_allow_credentials` 为 false | 回 `Access-Control-Allow-Origin: *` |
| `cors_origins` 含 `*` 但 `cors_allow_credentials` 为 true | **不回任何 Allow-Origin**（浏览器拒绝通配源 + 凭证的组合），请求会被浏览器拦下 |
| 预检 | `OPTIONS` 直接返回 204，并带上 `Access-Control-Max-Age: 600` |
| 暴露的响应头 | `Content-Disposition`、`Content-Length`、`Content-Range`、`ETag`、`X-Request-Id`（下载与断点续传要用） |

实践建议：

- **同源部署最简单**：前端由同一个端口提供（`web.enabled: true` + `web.root: ./web/dist`），完全不需要 CORS，把 `cors_origins` 留空即可。
- 前后端分离时把 `cors_origins` 精确列成前端源站（如 `https://netdisk.example.com`、`http://127.0.0.1:5173`），不要用 `*`。
- `cors_allow_credentials` 在令牌放在 `Authorization` 头的设计下不需要打开（浏览器不会因为没有它而拒绝 Bearer 请求），保持 false 最安全。
- 浏览器直连对象存储的跨域由对象存储负责，与本配置无关（见 1.5）。

## 4. 首次启动检查清单

| # | 检查项 | 期望值 / 动作 |
| --- | --- | --- |
| 1 | `auth.jwt_secret` | 显式设置一个高强度随机串。留空会每次启动随机生成，重启即让所有令牌与下载链接失效，并在日志里 WARN |
| 2 | `auth.admin_password` | 要么显式设置，要么在首次启动日志里抓取一次性生成的密码（WARN `bootstrap account created with a generated password`，`password` 字段）。登录后立即修改 |
| 3 | `data.object_storage.public_endpoint` | 指向浏览器可达的对象存储地址；只配 `endpoint` 时直传与直链在浏览器侧不可用 |
| 4 | `web.cors_origins` | 收窄到前端实际源站，或同源部署时留空 |
| 5 | `auth.allow_plain_password` | **必须为 false**（true 时允许明文密码字段，等于放弃传输层加密） |
| 6 | `server.http.timeout` / `proxy_*_timeout` | 放大到 600s 级别，否则大文件传输会被中断 |
| 7 | `storage.max_path_depth` / `max_name_length` | 与业务约定一致；改动会影响已存在数据的可操作性 |
| 8 | `upload.min_chunk_size` | 不低于 `5242880` |
| 9 | `data.database.auto_migrate` | 首次启动后改回 false |
| 10 | `auth.password_private_key_file` | 确认文件已生成且纳入备份；与数据库同生命周期 |
| 11 | 备份 | 数据库快照 + 对象存储 + 私钥文件三件套就位 |
| 12 | 定时维护 | 配置 `POST /v1/system/maintenance/run` 的 cron（第 5 节） |

## 5. 运行参数与维护任务

### 5.1 运行参数（SystemSetting）

`GET /v1/system/settings/list` 与 `PUT /v1/system/settings/update` 读写这些键（前者需要 `storage_manage`，后者需要 `system_manage`）。首次启动会种入默认值，已存在的键不会被覆盖。

| 键 | 类型 | 可写 | 种入的默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `guest.default_permissions` | int | 是 | `3` | 访客账号的默认权限位掩码 |
| `storage.default_quota_bytes` | int | 是 | `0` | 新建账号的默认配额（字节，0 表示不限） |
| `upload.max_inline_size` | int | 是 | 未种入 | 单请求内联上传的大小上限（字节） |
| `upload.default_chunk_size` | int | 是 | 未种入 | 默认分片大小（字节） |
| `upload.keep_versions` | bool | 是 | `true` | 覆盖文件时是否保留历史版本 |
| `upload.max_versions` | int | 是 | `20` | 每个文件保留的历史版本上限 |
| `share.allow_public` | bool | 是 | `true` | 是否允许匿名访问分享链接 |
| `trash.retention_days` | int | 是 | `30` | 回收站保留天数 |
| `system.maintenance_interval` | string | 是 | `1h` | 维护任务间隔，例如 `1h` |
| `system.version` | string | **否** | — | 当前构建版本（只读，写入返回 403） |

**重要提醒**：这些运行参数当前只被「读写与展示」——服务端没有任何逻辑回读它们来改变行为（上限、版本策略等仍由 `configs/config.yaml` 的 `upload.*`、`storage.*` 决定）。把它们当作待接入的运行时开关使用，不要指望改动立即生效。要改变实际行为，请改配置并重启。

### 5.2 维护任务

`POST /v1/system/maintenance/run`，需要 `storage_manage`。`tasks` 省略时只执行 `expire_uploads`、`purge_deletions`、`expire_shares`、`recount_usage`；`gc_orphans` 与 `purge_trash` 需要显式请求。

| 任务名 | 行为 | 报告字段 |
| --- | --- | --- |
| `expire_uploads` | 取最多 500 个已过 `expires_at` 且仍为 `PENDING`/`IN_PROGRESS` 的会话，置为 `EXPIRED`（`last_error = "expired by maintenance"`），删除分片行，并把 `uploads/<id>` 前缀排入待删除队列 | `expired_uploads` |
| `purge_deletions` | 取最多 500 条待删除记录，按前缀删除对象；成功删记录，失败则 `attempts + 1` 并记录 `last_error` 留待下轮 | `deleted_objects` |
| `gc_orphans` | 扫描 `uploads/` 与 `orphan/` 前缀，删除不属于任何**仍开放**上传会话的对象，回收被取消、已过期或未清理干净的暂存分片 | `orphan_objects` |
| `expire_shares` | 取最多 500 个已过期且状态仍为 `ACTIVE` 的分享，置为 `SHARE_STATUS_EXPIRED` | `expired_shares` |
| `recount_usage` | 按文件树重算每个账号的 `used_bytes` / `file_count` / `folder_count`，修复中断操作留下的偏差（`dry_run` 时不执行且返回 0） | `recounted_users` |
| `purge_trash` | 取最多 10000 个回收站节点，把 `trashed_at` 早于 `now - trash_retention_days` 的逐个彻底删除 | `purged_nodes` |

`dry_run: true` 时只统计不修改。未知任务名不会报错，而是写进 `warnings`（`unknown task: <name>`）；某个任务失败也会记入 `warnings` 并继续执行其余任务。

**`gc_orphans` 使用须知**（源码行为）：

- 它对每个仍处于 `PENDING` / `IN_PROGRESS` 且未过期的上传会话保留 `uploads/<upload_id>/` 前缀，因此**在途上传的分片不会被回收**；被回收的是已取消、已过期或已清理会话的残留。会话一旦被 `expire_uploads` 置为 `EXPIRED`，其分片进入可回收范围。
- 它只扫描 `uploads/` 与 `orphan/` 两个前缀，对 `files/...` 不做任何回收（那由待删除队列负责）；`orphan/` 前缀当前没有代码写入，留给运维手工归置待清理对象。
- 彻底删除（`PurgeNodes` / `purge_trash`）会把被删节点的内容键与历史版本键写入待删除队列，但对象是在**队列被处理时**才释放。若不定期运行 `purge_deletions`（维护任务），对象存储的实际占用会大于统计值，可用 `GetStorageStats` 的 `backend_used_bytes` 与桶的实际用量对比。

### 5.3 cron 示例

维护接口需要访问令牌，而访问令牌默认 2 小时过期、刷新令牌每次使用都会轮换，所以定时脚本应当保存**刷新令牌**，每次运行先换新令牌并写回：

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE=https://netdisk.example.com
STATE=/var/lib/nagisa/cron-token          # 只允许该脚本读写（0600）

# 1. 用上次保存的刷新令牌换一对新令牌（首次运行前手工登录一次并写入该文件）
REFRESH=$(cat "$STATE")
PAIR=$(curl -sf -X POST "$BASE/v1/auth/refresh" \
  -H 'Content-Type: application/json' \
  -d "{\"refresh_token\":\"$REFRESH\"}")
echo "$PAIR" | python3 -c 'import json,sys;print(json.load(sys.stdin)["refreshToken"])' > "$STATE"
TOKEN=$(echo "$PAIR" | python3 -c 'import json,sys;print(json.load(sys.stdin)["accessToken"])')

# 2. 执行维护（先 dry_run 验证过再打开实际执行）
curl -sf -X POST "$BASE/v1/system/maintenance/run" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"tasks":["expire_uploads","purge_deletions","expire_shares","recount_usage"],"trash_retention_days":30}' \
  >> /var/log/nagisa-maintenance.log 2>&1
```

```cron
# 每小时一次；建议先手工以 dry_run 验证一次
0 * * * * /usr/local/bin/nagisa-maintenance.sh
```

> 刷新令牌也会过期（默认 30 天）。脚本应监控失败并在必要时重新登录（`POST /v1/auth/login`，密码同样需要 RSA 加密，见 [`api-guide.md`](api-guide.md#认证流程)）。进程本身不内置调度器，`system.maintenance_interval` 目前只是一个配置项。

## 6. 前端部署

前端接入位的约定见 [`web/README.md`](../web/README.md)。后端负责托管，配置步骤如下：

1. **构建到 `web/dist`**：`npm run build` 之类，产物入口是 `web/dist/index.html`，带内容哈希的资源放 `web/dist/assets/`。仓库里现有的是占位页，构建后覆盖即可。
2. **打开托管**：`web.enabled: true`、`web.root: ./web/dist`（默认值）。目录不存在时启动只 WARN，接口仍可用，并在访问 `/` 时返回内置的提示页。
3. **SPA 回退**：`web.spa_fallback: true` 时未知路径返回 `index.html`，客户端路由可用。`/v1/` 与 `/docs/` 前缀**不会**回退，始终返回 404——接口打错字不会被 HTML 页面掩盖。
4. **路径前缀**：`web.path_prefix` 默认 `/`。挂在子路径（如 `/app`）时，前端构建的 `base` 也要相应调整，且 API 请求仍需指向 `/v1/`。
5. **缓存**：只有形如 `app.<8 位以上十六进制>.js` 的内容哈希资源会得到 `Cache-Control: public, max-age=<cache_max_age>`，其余文件（含 `index.html`）一律 `no-cache`，因此发版后刷新即可生效。
6. **接口文档**：`/docs/` 静态路由始终注册，提供 Swagger UI（`docs/index.html`）与 `docs/openapi.yaml`、`docs/swagger.yaml`、`docs/openapi.json`。它们由 `make docs`（`make all` 的一环）生成。
7. **本地联调**：把前端开发服务器源站（通常是 `http://127.0.0.1:5173`）加进 `web.cors_origins`，或在前端用代理把 `/v1` 转到 `http://127.0.0.1:8000`，后者可以完全避开 CORS。

## 7. 容器化

仓库自带的 `Dockerfile` 分两段：构建段执行 `make build`，运行段基于 `debian:stable-slim`，把 `bin/`、`configs/`、`docs/`、`web/` 复制进镜像并 `CMD ["./nagisa", "-conf", "/app/configs"]`。

- `make build` 现在只构建 `./cmd/...`，产物是 **`bin/nagisa`**，与上面的启动命令一致。在 Windows 上可以用 `.\scripts\build.ps1 build` 得到同样的 `bin\nagisa.exe`（镜像内是 Linux 二进制，所以镜像构建本身仍应在 Linux 或 `docker build` 里跑 `make`）。
- 镜像暴露 `EXPOSE 8000 9000`，并把 **`/app/data`** 声明为卷：数据库与 RSA 私钥默认落在该目录（`source: /app/data/nagisa.db`、`password_private_key_file: /app/data/password_key.pem`）。**务必挂载这个卷**，否则容器重建会同时丢掉数据与解密密码用的私钥。
- 容器内访问对象存储时，`endpoint` 用服务名加端口（例如 `seaweedfs:8333`），`public_endpoint` 仍要写浏览器可达的地址。
- 时区与证书：镜像已装 `ca-certificates` 与 `tzdata`；若 `public_base_url` / `endpoint` 用自签证书，开发环境可开 `data.object_storage.insecure_skip_verify`，生产不要开。

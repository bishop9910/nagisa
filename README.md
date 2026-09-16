# Nagisa 网盘后端

Nagisa 是一个可直接商用部署的轻量级网盘后端：用 Kratos v3 同时提供 HTTP 与 gRPC，用 SQLite（ent）保存元数据、用 SeaweedFS 或任意 S3 兼容存储保存内容，接口以 protobuf 为唯一来源并生成 OpenAPI 文档；文件夹与文件统一建模为「节点」，因此描述、可见范围、访问名单（ACL）与密码对两者同等生效。它自带分片断点续传、签名直链下载、服务端流式下载、实时 zip 打包、分享链接、回收站、历史版本、配额、审计与维护任务，并可以直接托管 `web/dist` 下的前端。

## 功能特性

**账号与权限**

- 层级角色 `admin` / `manager` / `user` / `guest`，权限用 12 位位掩码存储，角色只是默认模板。
- 严格等级序（`rank`）：只能管理等级严格低于自己的账号，只能授予自己拥有的权限。
- 免登录只读浏览：`auth.guest_auto_login` 打开时，前端启动即换取一个只读访客会话，打开网页就能浏览与下载对外可见的内容（`POST /v1/auth/guest`）。
- 内置管理员与内置访客账号锁死：角色、等级、权限集由配置决定，任何账号（含管理员）都无法修改或删除它们。
- 密码从不明文传输：客户端用 `GET /v1/auth/config` 返回的 RSA 公钥做 RSA-OAEP(SHA-256) 加密后 base64 提交。
- bcrypt(12) 存储账号密码与文件夹/分享密码；刷新令牌以 SHA-256 摘要入库并在每次使用时轮换。

**文件树**

- 文件夹即节点（`NODE_KIND_FOLDER`），与文件共用描述、可见范围、ACL、密码、元数据模型。
- 物化 `path` 列（`/<id>/<id>/`）：子树查询是一次前缀扫描，移动是一次前缀重写。
- 单层/递归列表、树展开、全树搜索、路径面包屑、子树统计。
- 批量移动/复制（含子树，事务内全有或全无）、重名策略 `FAIL` / `RENAME` / `OVERWRITE`。
- 回收站：移入、还原、彻底清除、清空；维护任务按保留期自动清理。
- 历史版本：覆盖写保留旧内容，可列出版本、还原、删除。

**上传**

- 分片断点续传，`UPLOAD_MODE_PRESIGNED`（客户端直传对象存储）与 `UPLOAD_MODE_PROXY`（经服务端中转）两种传输。
- 会话幂等：同目录同名同属主的未完成会话会被复用；`resumable_upload_id` 可显式续传。
- 会话创建时即预检配额、文件大小与片数，完成时再次校验片大小、总大小与 SHA-256。
- 小文件内联上传 `UploadSmallFile`（上限 `upload.max_inline_size`，默认 4 MiB）。

**下载**

- `GetDownloadUrl` 返回带签名的直链（预签名指向对象存储，字节不经过服务端）。
- `GET /v1/files/{node_id}/content` 由服务端流式转发，支持 `Range` 断点续传。
- `GET /v1/files/{node_id}/archive` 实时把文件夹打成 zip，不落地临时对象。
- `GetPreviewUrl` 为可预览的 MIME 返回内联签名地址。

**分享**

- 分享链接携带独立的能力集合（仅 `view` / `download` / `upload`）、可选密码、过期时间与下载额度。
- 匿名接口：`AccessShare`、`ListShareChildren`、`GetShareDownloadUrl` 无需账号。

**运维与审计**

- 审计日志覆盖每一次状态变更与失败调用，只增不改；无 `audit_read` 权限者只能看到自己的记录。
- 存储统计（含后端实际用量与最大占用账号）、可热改的运行参数、维护任务（过期上传、删除队列、孤儿对象、过期分享、用量重算、回收站清理）。
- `GET /v1/system/info` 与 `GET /v1/system/health` 无需令牌，供前端启动引导与探活。

**前端与文档**

- `web.enabled` 打开后，Kratos HTTP 服务直接托管 `web/dist`，支持 SPA 回退与资源缓存策略。
- `/docs/` 静态路由发布本目录文档与 OpenAPI 文档。

## 快速开始

### 前置条件

| 依赖 | 说明 |
| --- | --- |
| Go | 1.25 或更高（`go.mod` 声明 `go 1.25.7`，镜像使用 `golang:1.25`） |
| SeaweedFS 或任意 S3 兼容存储 | 分片上传、签名下载、打包下载都依赖它；仅跑数据库与接口时可以把 `data.object_storage.endpoint` 留空。换成别的实现只改配置，见 [`docs/deployment.md`](docs/deployment.md#17-换后端时的兼容性核对清单) |
| GNU Make | 生成与构建都通过 `Makefile` 驱动 |
| `wire` / `buf` | 由初始化命令安装（`make init` 或 `.\scripts\build.ps1 init`） |
| GNU Make | 仅 Linux/macOS 需要；Windows 用等价的 PowerShell 脚本，见下 |

### 安装、生成与构建

Linux / macOS（需要 `make`）：

```bash
make init     # 安装 wire 与 buf
make all      # make api + make config + make generate + make docs
make build    # 产物在 ./bin/
```

Windows（不需要安装 `make`，用仓库自带的 PowerShell 脚本）：

```powershell
.\scripts\build.ps1 init      # 安装 buf 与 wire
.\scripts\build.ps1 all       # api + config + generate + docs
.\scripts\build.ps1 build     # 产物在 .\bin\nagisa.exe
```

脚本与 Makefile 的目标一一对应，另外还有 `run`、`test`、`test-integration`、`fmt`、`vet`、`tidy`、`clean`，不带参数运行会打印全部用法。若 PowerShell 执行策略拦住了脚本，用 `powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 all`，或先执行一次 `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`。

两套入口做的事完全一样，都会重新生成 `*.pb.go`、`*_http.pb.go`、`internal/conf/conf.pb.go`、`wire_gen.go`、`openapi.yaml`，并由 `tools/openapi` 富化文档、把副本发布到 `docs/`。这些文件不要手改。生成命令的完整说明见 [`docs/architecture.md`](docs/architecture.md#72-生成命令)。

> 生成链需要 `buf`。若它不在 PATH 上，脚本会自动到 `%GOPATH%\bin` 里找；`init` 会把 `buf` 与 `wire` 装到那里。注意 `wire` 用 `go run github.com/google/wire/cmd/wire@v0.7.0` 调用，因为 v0.6.0 无法解析 `go 1.25` 的模块。

### 启动

```bash
go run ./cmd/nagisa -conf ./configs
```

默认监听：

| 端口 | 用途 | 配置键 |
| --- | --- | --- |
| `0.0.0.0:8000` | HTTP（REST + 前端 + `/docs/`） | `server.http.addr` |
| `0.0.0.0:9000` | gRPC | `server.grpc.addr` |

启动后先访问 `http://127.0.0.1:8000/v1/system/health` 确认进程与依赖状态。

> 注意：`configs/config.yaml` 里 `data.object_storage.endpoint` 的默认值是 `127.0.0.1:8333`（SeaweedFS 的 S3 端口），与服务端口不冲突。若你的对象存储跑在别的地址，用 `S3_ENDPOINT` 指向它（例如 `127.0.0.1:9010`）。

### 第一个管理员密码

初次启动时 `internal/data/seed.go` 会补齐内置账号与默认设置，已存在的账号不会被覆盖：

| 方式 | 做法 |
| --- | --- |
| 显式指定 | 设置 `auth.admin_password`（示例配置里可用环境变量 `ADMIN_PASSWORD`），首次启动即用该密码 |
| 留空自动生成 | `admin_password` 留空时，服务端生成一个 12 字节随机密码，**只在日志中打印一次**：先查 `auth` 配置下的 `admin_password`，再看首次启动日志里的 `bootstrap account created with a generated password` 这条 WARN，其 `password` 字段就是初始密码 |

内置访客账号同理（`auth.guest_username` / `auth.guest_password`，默认 `guest`，默认权限为只读的 `view|download`）。

## 5 分钟 API 漫游

示例使用 `curl` 与 `jq`（也可用 `python -m json.tool` 代替）。先设 `BASE`：

```bash
BASE=http://127.0.0.1:8000
```

**1. 读取部署能力与限额**（无需令牌）

```bash
curl -s $BASE/v1/system/info | jq '{name,version,apiVersion,features}'
```

回复是 `SystemInfo`：`features` 列出能力开关，`maxUploadSize` / `defaultChunkSize` / `minChunkSize` / `maxInlineSize` 给出上传限额，`uploadModes` 给出本部署支持的分片传输方式，`storageBackend` / `databaseBackend` 给出实际后端，`auth` 内嵌登录前的握手参数。

**2. 取得登录握手参数**（无需令牌）

```bash
curl -s $BASE/v1/auth/config | jq '{passwordKeyId,passwordEncoding,accessTokenTtlSeconds,refreshTokenTtlSeconds,plainPasswordAllowed}'
curl -s $BASE/v1/auth/config | jq -r .passwordPublicKey > pub.pem
```

`passwordEncoding` 固定为 `rsa-oaep-sha256`，`passwordPublicKey` 是 PEM 编码的 PKCS#8 RSA 公钥（3072 位）。`plainPasswordAllowed` 只在开发配置下为 true。

**3. 加密密码**

登录时 `password` 字段必须是 **RSA-OAEP(SHA-256) 密文的 base64**，服务端不会接受明文（`auth.allow_plain_password` 必须保持 false）：

```bash
ENC=$(printf '%s' 'Admin@12345' | openssl pkeyutl -encrypt -pubin -inkey pub.pem \
        -pkeyopt rsa_padding_mode:oaep \
        -pkeyopt rsa_oaep_md:sha256 \
        -pkeyopt rsa_mgf1_md:sha256 \
      | openssl base64 -A)
```

Go 客户端等价写法是两行：用 `x509.ParsePKIXPublicKey` 解析公钥，再 `rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, []byte(plain), nil)`，最后 `base64.StdEncoding.EncodeToString`（`test/integration/smoke_test.go` 就是这么做的）。

**4. 登录**（无需令牌）

```bash
LOGIN=$(curl -s -X POST $BASE/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$ENC\"}")
echo "$LOGIN" | jq '{tokenType,expiresIn,refreshExpiresIn,user:{id:.user.id,role:.user.role,rank:.user.rank,permissionsMask:.user.permissionsMask}}'
TOKEN=$(echo "$LOGIN" | jq -r .accessToken)
```

`LoginReply` 返回 `accessToken`、`refreshToken`、`tokenType`（`Bearer`）、`expiresIn`、`refreshExpiresIn` 与已登录账号 `user`。之后所有需要鉴权的请求都带 `Authorization: Bearer $TOKEN`。

**5. 新建文件夹**

```bash
FOLDER=$(curl -s -X POST $BASE/v1/nodes/folders/create \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"folder":{"name":"文档","description":"示例目录","visibility":"VISIBILITY_PRIVATE"},
       "conflict_policy":"CONFLICT_POLICY_FAIL"}')
echo "$FOLDER" | jq '{id,name,kind,displayPath,visibility,owned}'
FOLDER_ID=$(echo "$FOLDER" | jq -r .id)
```

回复是创建出来的 `Node`：`id`、`path`（物化祖先路径）、`displayPath`（可直接展示的绝对路径）、`effectivePermissions` 等。同名冲突且策略为 `FAIL` 时返回 HTTP 409 与 `reason=NETDISK_NAME_CONFLICT`。

**6. 列目录**

```bash
curl -s "$BASE/v1/nodes/list?parent_id=$FOLDER_ID&page_size=20" \
  -H "Authorization: Bearer $TOKEN" | jq '{totalSize,totalBytes,nodes:[.nodes[].name]}'
```

查询参数同时接受 `parent_id` 与 `parentId` 两种写法。回复 `NodeSet` 含 `nodes`、`totalSize`、`totalBytes` 与分页用的 `nextPageToken`。

**7. 开一个分片上传会话**

```bash
UP=$(curl -s -X POST $BASE/v1/files/uploads/create \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"parent_id\":\"$FOLDER_ID\",\"name\":\"report.pdf\",\"size\":12582912,
       \"mode\":\"UPLOAD_MODE_PRESIGNED\"}")
echo "$UP" | jq '{id,chunkSize,totalParts,status,mode,partUrls:[.parts[].uploadUrl]}'
```

回复 `UploadSession`：`chunkSize`（每片字节数）、`totalParts`、`status`、`mode`，以及 `parts` 数组——`UPLOAD_MODE_PRESIGNED` 下每一项都带 `uploadUrl`（直传对象存储的预签名 PUT 地址）、`urlExpiresAt` 与 `headers`。

接着对每一片：

```bash
# 预签名模式：把分片直接 PUT 到 uploadUrl，再把返回的 ETag 报回来
curl -X PUT --upload-file part-1.bin "<part1-upload-url>"
curl -s -X POST $BASE/v1/files/uploads/parts/confirm \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"upload_id":"<upload-id>","part_number":1,"etag":"<etag>"}'

# 最后提交，得到文件节点
curl -s -X POST $BASE/v1/files/uploads/complete \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"upload_id":"<upload-id>"}' | jq '{id,name,size,parentId,displayPath}'
```

代理模式（`UPLOAD_MODE_PROXY`）则把同样的字节 PUT 到 `PUT /v1/files/uploads/{upload_id}/parts/{part_number}/raw`（请求体即原始二进制），无需确认步骤。完整流程与续传见 [`docs/api-guide.md`](docs/api-guide.md#分片上传完整流程)。

**8. 取下载直链**

```bash
curl -s "$BASE/v1/files/$NODE_ID/download-url?expires_in_seconds=600" \
  -H "Authorization: Bearer $TOKEN" | jq '{url,method,expiresIn,fileName,mimeType,size}'
```

其余接口（账号、ACL、分享、审计、维护任务）见 [`docs/api-guide.md`](docs/api-guide.md)。

## 目录结构

```text
api/netdisk/v1/          protobuf 契约：common/auth/user/node/file/share/audit/system + error_reason
cmd/nagisa/              进程入口、Wire 注入器（wire.go / wire_gen.go）
configs/                 运行配置 config.yaml（不含密钥）
docs/                    本仓库文档与 OpenAPI 文档
internal/conf/           配置 proto 与生成代码
internal/server/         HTTP/gRPC 装配、编解码、静态托管、原始字节路由、中间件
internal/service/        传输适配层：DTO ↔ DO，一个资源一个文件
internal/biz/            领域层：DO、用例、仓储接口、权限与访问判定、错误
internal/data/           仓储实现、ent schema 与生成代码、对象存储客户端、种子数据
internal/pkg/            bcrypt/RSA/JWT/URL 签名等基础库
test/integration/        端到端冒烟测试（build tag: integration）
tools/openapi/           OpenAPI 文档富化工具（补充安全方案、错误响应与原始字节绑定）
web/                     前端接入位：README.md 说明约定，dist/ 目前只有占位 index.html
```

## 文档索引

| 文档 | 内容 |
| --- | --- |
| [`docs/README.md`](docs/README.md) | 文档导航与阅读顺序 |
| [`configs/README.md`](configs/README.md) | **`config.yaml` 逐条说明**：每个条目是干什么的（大白话版，先看这个） |
| [`docs/configuration.md`](docs/configuration.md) | 配置参考：加载与覆盖顺序、值格式约定、缺省时的实际默认值、环境变量注入、三套示例、启动失败排查 |
| [`docs/architecture.md`](docs/architecture.md) | 请求路径、DTO/DO/PO 分层、并发与原子性、SQLite 细节、对象键布局、扩展步骤 |
| [`docs/api-guide.md`](docs/api-guide.md) | 通用约定、认证流程、逐个 RPC 的字段与错误、上传/下载/分享完整流程、错误码表 |
| [`docs/permissions.md`](docs/permissions.md) | 角色与权限位表、等级与授权规则、节点访问判定顺序与示例、常见配置场景 |
| [`docs/deployment.md`](docs/deployment.md) | 上线落地：对象存储（SeaweedFS）、SQLite 生产建议、反向代理与 TLS、首次启动清单、运维任务、前端部署、容器化 |
| [`openapi.yaml`](openapi.yaml) | 生成的 OpenAPI 3.0.3 文档（已由 `tools/openapi` 富化，含 `bearerAuth` 等安全方案、共享错误响应与原始字节绑定） |
| [`web/README.md`](web/README.md) | 前端接入约定与本地联调方式 |

服务运行后，同一份文档也通过静态路由发布：`GET /docs/` 是 Swagger UI 页面（`docs/index.html`），`GET /docs/openapi.yaml`、`GET /docs/swagger.yaml`、`GET /docs/openapi.json` 是文档本体。`docs/openapi.yaml`、`docs/swagger.yaml` 与根目录的 `openapi.yaml` 当前是同一份文档的副本。

## 测试

单元测试：

```bash
go test ./...
```

端到端集成测试（需要先启动一个服务实例）：

```bash
go test -tags integration ./test/integration/ -v
```

集成测试通过环境变量定位目标：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `NETDISK_BASE_URL` | `http://127.0.0.1:18000` | 服务基地址；探测 `/v1/system/health` 失败时测试会 skip |
| `NETDISK_ADMIN` | `Admin@12345` | 管理员密码（明文，测试内部自行做 RSA 加密） |
| `NETDISK_GUEST` | `Guest@12345` | 访客密码 |

因此本机跑集成测试时，服务端的 `auth.admin_password` / `auth.guest_password` 要么与上述默认值一致，要么通过 `NETDISK_ADMIN` / `NETDISK_GUEST` 显式告知测试。

## 安全要点

- 密码永不明文传输：账号密码、文件夹密码、分享密码都要求 RSA-OAEP(SHA-256) + base64，服务端用 `auth.password_private_key` / `password_private_key_file` 解密；`auth.allow_plain_password` 仅供本地调试，生产必须为 false。
- 账号密码、文件夹密码、分享密码统一用 bcrypt（cost 12）哈希存储；比较用 `bcrypt.CompareHashAndPassword`。
- 刷新令牌以 SHA-256 摘要入库，使用一次即作废并签发新令牌（轮换）；改密码、禁用账号、删除账号都会吊销该账号的全部会话。
- 访问令牌是 HS256 签名的 JWT，密钥来自 `auth.jwt_secret`；未配置时每次启动随机生成，重启后所有令牌失效（并在日志中告警）。生产必须显式配置。
- 下载链接是带过期时间的签名 URL（绑定方法、路径、节点、处置方式与过期时刻），服务端在落地请求时再次校验签名；签名密钥同样来自 `auth.jwt_secret`。
- 审计日志记录每一次状态变更及其成功/失败与失败原因，失败调用同样入库；日志只增不改，接口不提供修改或删除。
- 节点访问遵循「账号权限上限 → 属主通行 → 显式拒绝 → 显式允许（含继承）→ 可见范围 → 文件夹密码」，ACL 只能携带节点级权限，无法把账号提升为管理员；分享链接最多只能传递 `view` / `download` / `upload`。

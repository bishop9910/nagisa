# 配置参考

本文是 Nagisa 网盘配置的完整参考：配置从哪里加载、怎么被环境变量覆盖、每个键的类型与默认值、以及三套可以直接抄的示例。

> 只想知道「`config.yaml` 里每条是干啥的」→ 看 [`../configs/README.md`](../configs/README.md)，那是逐条大白话说明。本文是完整的键参考。

配置的定义源头是 [`internal/conf/conf.proto`](../internal/conf/conf.proto)（`kratos.api.Bootstrap`），示例值是 [`configs/config.yaml`](../configs/config.yaml)。表中「代码默认」指**该键缺省或为零值时程序内部实际使用的值**，它不一定等于示例文件里写的值——判断行为请以这一列和「生效位置」为准。

---

## 1. 加载方式

```bash
go run ./cmd/nagisa -conf ./configs        # 目录：读 <dir>/config.yaml
go run ./cmd/nagisa -conf ./my.yaml        # 文件：读该文件
```

`-conf` 默认值就是 `../../configs`（相对于命令的源码目录）。

解析顺序（后加载的覆盖先加载的）：

1. `-conf` 指向的配置文件；
2. 环境变量源（前缀 `KRATOS`）。

两个来源合并后再做占位符解析，最后整体反序列化成 `Bootstrap`。

> **目录形态只读 `config.yaml`。** 早先的实现直接把目录交给 kratos 的文件源，而它会尝试解码目录里的**每一个文件**，遇到不认识的扩展名就 `panic: unsupported key: nagisa.db format: db`。现在 `-conf <dir>` 固定读 `<dir>/config.yaml`，所以日志、数据库、README 放在配置目录旁边不会再让进程起不来；要读别的文件名就直接把文件路径传给 `-conf`。

### 1.1 用环境变量覆盖（推荐给密钥）

环境变量名 = `KRATOS_` + 配置键路径。两种写法都有效：

**A. 点分路径，直接覆盖任意键**

```bash
KRATOS_server.http.addr=0.0.0.0:9000
KRATOS_auth.issuer=nagisa-prod
```

变量名就是完整的点分配置路径，优先级高于配置文件里的同名键。

**B. 平面名字，配合文件里的 `${...}` 占位符**

`config.yaml` 中写成 `${KEY:默认值}` 的值，会在解析阶段替换成**配置键 `KEY`** 的值；若该键不存在（即没有对应的 `KRATOS_KEY`）就使用冒号后的默认值。

```yaml
auth:
  jwt_secret: ${JWT_SECRET:}          # 被 KRATOS_JWT_SECRET 填充
  admin_password: ${ADMIN_PASSWORD:}  # 被 KRATOS_ADMIN_PASSWORD 填充
```

```bash
KRATOS_JWT_SECRET=... KRATOS_ADMIN_PASSWORD=... ./bin/nagisa -conf ./configs
```

三点容易踩：

- 占位符查的是**配置键**，不是同名 OS 变量。只设 `export JWT_SECRET=...` 是没用的，必须带 `KRATOS_` 前缀（或者用写法 A 直接写 `KRATOS_auth.jwt_secret`）。
- 默认值允许自带冒号，只按**第一个**冒号切分，所以 `${S3_ENDPOINT:127.0.0.1:8333}` 的默认值是 `127.0.0.1:8333`。
- 写成 `KRATOS_A_B_C` 这种下划线风格的变量名不会映射到 `a.b.c`，它只会变成一个与 `Bootstrap` 无关的顶层键，被静默忽略（不会报错，也不会生效）。

### 1.2 值格式约定

| 类型 | 写法 | 说明 |
| --- | --- | --- |
| `google.protobuf.Duration` | `"1800s"`、`"0.5s"`、`"0s"` | **只接受秒为单位的字符串**，因为这是 protobuf JSON 的标准 Duration 形式。`30m`、`2h`、`1d` 都会让启动直接 `panic: invalid google.protobuf.Duration value "2h"`。换算：`1h=3600s`、`1d=86400s`、`30d=2592000s`。 |
| 布尔 | `true` / `false` | YAML 原生布尔 |
| 整数 | `8388608` | 不支持 `8MiB` 这类字面量，用字节数 |
| 字符串 | `nagisa` / `"127.0.0.1:9000"` | 含冒号建议加引号 |
| 字符串列表 | `- GET` 逐行 | `cors_methods`、`cors_headers`、`cors_origins` |
| 占位符 | `${KEY:默认值}` | 见上文 |

### 1.3 改完配置要重启

进程在启动时把配置读进各用例的结构体字段，之后不再回读（没有注册 config watcher），**所有键都需要重启生效**。运行时可通过 API 修改的只有数据库里的「运行参数」，见第 6 节。

---

## 2. `server` — 监听与超时

| 键 | 类型 | 代码默认 | 示例 | 说明 |
| --- | --- | --- | --- | --- |
| `server.http.network` | string | 空 → `tcp` | `tcp` | 留空即可 |
| `server.http.addr` | string | 空 → kratos 默认 `:0` | `0.0.0.0:8000` | HTTP 监听地址 |
| `server.http.timeout` | Duration | 未设 → kratos 默认 `1s` | `600s` | 单请求超时。**必须放大**：1s 会掐断流式下载、实时 zip 与大分片上传 |
| `server.http.max_body_size` | int64 | `0` → 32 MiB | `67108864` | **只限制 `Content-Type` 含 `json` 的请求体**，防止一个请求让进程缓冲无上限的数据。二进制路由（`/content`、`/archive`、`/parts/{n}/raw`）不受它约束，它们由 `upload.*` 控制 |
| `server.grpc.network` | string | 空 → `tcp` | `tcp` | 同上 |
| `server.grpc.addr` | string | 空 → kratos 默认 `:0` | `0.0.0.0:9000` | gRPC 监听地址。**注意别和对象存储撞端口**（SeaweedFS 占 `8333`/`9333`/`8888`/`8080`） |
| `server.grpc.timeout` | Duration | 未设 → `1s` | `600s` | 同 HTTP |

生效位置：`internal/server/http.go`、`internal/server/grpc.go`。

---

## 3. `data` — 数据库与对象存储

### 3.1 `data.database`

| 键 | 类型 | 代码默认 | 示例 | 说明 |
| --- | --- | --- | --- | --- |
| `driver` | string | 空 → `sqlite` | `sqlite` | `sqlite`/`sqlite3`/`modernc` 都按 SQLite 处理；其他值走 `ent.Open(driver, source)`，例如 `mysql` |
| `source` | string | 空 → `file:nagisa.db?cache=shared` | `./data/nagisa.db` | SQLite 是文件路径（自动建父目录），MySQL 是 DSN。**不要提交真实凭据** |
| `debug` | bool | `false` | `false` | 打印每条 SQL，仅开发用 |
| `auto_migrate` | bool | `false` | `true` | 启动时按 ent schema 建表/改表。生产建议关闭并单独执行迁移 |
| `max_open_conns` | int32 | `0` → `8` | `8` | 连接池上限。SQLite 建议 4~8 |
| `max_idle_conns` | int32 | `0` → 等于 `max_open_conns` | `8` | 空闲连接数 |
| `conn_max_lifetime` | Duration | `0` → 不限制 | `0s` | 连接最长存活时间 |
| `wal` | bool | `false` | `true` | 加 `_pragma=journal_mode(WAL)`。**强烈建议开**：读不阻塞写 |
| `busy_timeout` | Duration | `0` → `10s` | `10s` | 加 `_pragma=busy_timeout(<毫秒>)`，写锁等待时长 |
| `foreign_keys` | bool | `false` | `true` | 加 `_pragma=foreign_keys(1)`。**`auto_migrate: true` 时必须同时设为 `true`**：ent 迁移要求 DSN 里带 `_fk=1`，否则直接 panic（见第 8 节） |

SQLite 的 DSN 会被自动追加 `_txlock=immediate`（除非你自己写了 `_txlock`）与 `_pragma=synchronous(NORMAL)`。原因见 [`architecture.md`](architecture.md#) 的并发一节：延迟事务「先读后写」需要升级锁，SQLite 会立刻返回 `SQLITE_BUSY` 而不等 `busy_timeout`，用 IMMEDIATE 可以消掉这一类 `database is locked`。

### 3.2 `data.object_storage`

| 键 | 类型 | 代码默认 | 示例 | 说明 |
| --- | --- | --- | --- | --- |
| `endpoint` | string | **空 = 不启用对象存储** | `127.0.0.1:8333` | S3 兼容地址，不要带 `http://` 前缀（带了也会被剥掉）。任何 S3 兼容实现都可以，默认值对应 SeaweedFS。留空时所有上传/下载接口返回 `NETDISK_UNAVAILABLE`，但账号、权限、文件夹、ACL 等元数据功能仍可完整使用 |
| `access_key` | string | 空 | `nagisa` | 访问密钥 |
| `secret_key` | string | 空 | `nagisa-secret` | 私密密钥，用 `KRATOS_S3_SECRET_KEY` 注入 |
| `bucket` | string | 空 → `netdisk` | `nagisa` | 桶名 |
| `region` | string | 空 | `us-east-1` | 留空用 SDK 默认 |
| `use_ssl` | bool | `false` | `true` | 用 HTTPS 连对象存储 |
| `public_endpoint` | string | 空 → 用 `endpoint` | `files.example.com` | **给浏览器用的地址**。预签名 URL 的签名绑定了 Host，浏览器可达的地址与服务端不同时必须设置，否则签名校验失败 |
| `auto_create_bucket` | bool | `false` | `true` | 启动时缺桶就建。开发方便，生产建议预先建好 |
| `prefix` | string | 空 → 无前缀 | `netdisk` | 所有对象键的公共前缀，便于一个桶放多个部署 |
| `presign_ttl` | Duration | `0` → `30m` | `1800s` | 分片预签名 PUT 与下载 URL 的默认有效期 |
| `insecure_skip_verify` | bool | `false` | `false` | 跳过 TLS 证书校验，仅开发自签证书时开 |

签名有效期上限：客户端在请求里指定的过期时间会被夹到 `4 × presign_ttl` 以内（分享下载另有 4 小时硬上限）。把 `presign_ttl` 调大等于放宽这个上限。

对象键布局：

```
<prefix>/files/<owner_id>/<node_id>          当前内容
<prefix>/uploads/<upload_id>/parts/<n>       进行中的分片
```

---

## 4. `auth` — 凭据与会话

| 键 | 类型 | 代码默认 | 示例 | 说明 |
| --- | --- | --- | --- | --- |
| `jwt_secret` | string | 空 → **每个进程随机生成并打 WARN** | `${JWT_SECRET:}` | 同时用于签发访问令牌/解锁令牌与 URL 签名。为空时重启会让所有令牌失效，**生产必须设置** |
| `issuer` | string | 空 → `nagisa-netdisk` | `nagisa-netdisk` | JWT 的 `iss` 声明 |
| `access_token_ttl` | Duration | `0` → `2h` | `7200s` | 访问令牌有效期 |
| `refresh_token_ttl` | Duration | `0` → `30d` | `2592000s` | 刷新令牌有效期（`remember_me` 时翻倍）。刷新令牌只存 SHA-256 摘要，且每次使用即轮换 |
| `password_private_key` | string | 空 | — | 内联 PEM 私钥，优先级高于文件 |
| `password_private_key_file` | string | 两者都空 → `./data/password_key.pem` | `./data/password_key.pem` | RSA 私钥路径（PKCS#8）。文件不存在时自动生成 3072 位密钥并以 `0600` 落盘。**这是解密客户端密码的唯一钥匙，容器化时必须挂载持久卷** |
| `allow_plain_password` | bool | `false` | `false` | 允许非 RSA 编码的密码。**只在本地临时调试时开**，相当于放弃「密码不明文传输」这一保证 |
| `admin_username` | string | 空 → `admin` | `admin` | 内置管理员用户名，首次启动时创建 |
| `admin_password` | string | 空 → 随机生成并在启动日志打印一次 | `${ADMIN_PASSWORD:}` | 内置管理员初始密码。**只在账号不存在时使用**，重启不会覆盖已有密码 |
| `guest_username` | string | 空 → `guest` | `guest` | 内置访客用户名。**它没有密码键**：访客身份只能通过 `POST /v1/auth/guest` 免密取得，所以账号里的口令哈希是一串谁也拿不到的随机值，用 `auth.login` 走口令登录永远失败 |
| `guest_auto_login` | bool | `false` | `true` | 允许不输口令直接换一个只读访客会话（`POST /v1/auth/guest`），前端启动时就会自动调用，从而实现「打开网页即只读浏览」。**这是访客唯一的入口**：关掉它就没有任何办法以访客身份登录。**无论开关如何，内置访客账号的角色、等级、权限集与状态都是锁死的**：没有任何账号（包括内置管理员）能修改或删除它，服务端每次启动还会把它们校正回来 |
| `node_token_ttl` | Duration | `0` → `30m` | `1800s` | `UnlockNode` 返回的文件夹解锁令牌有效期 |
| `min_password_length` | int32 | `0` → `8` | `8` | 账号密码最小长度（按字符计）。文件夹密码与分享密码同样受此下限约束 |

RSA 密钥对的轮换：换掉 `password_private_key_file` 指向的文件会让旧公钥立即失效，客户端需要重新调用 `GetAuthConfig`——不会影响已签发的访问令牌。

---

## 5. `storage` / `upload` / `web`

### 5.1 `storage` — 树与配额

| 键 | 类型 | 代码默认 | 示例 | 说明 |
| --- | --- | --- | --- | --- |
| `root_folder_name` | string | 空 → `我的网盘` | `我的网盘` | 虚拟根的名字，出现在 `displayPath` 的第一段 |
| `default_quota_bytes` | int64 | `0` = 不限 | `0` | 管理员建号时未指定配额则用此值 |
| `guest_quota_bytes` | int64 | `0` = 不限 | `1073741824` | **只在内置 guest 账号创建时使用**；之后改它不会影响已存在的账号，要改走 `UpdateUser` 的 `quota_bytes` |
| `case_insensitive_names` | bool | `false` | `true` | 同一目录下名字仅大小写不同视为重名 |
| `allow_public_share` | bool | `false` | `true` | **关掉会整体禁用分享功能**：`CreateShare` 直接返回 `NETDISK_UNSUPPORTED`，已有链接也无法解析 |
| `trash_retention` | Duration | `0` → `30d` | `2592000s` | 维护任务 `purge_trash` 未显式传 `trash_retention_days` 时用它折算天数 |
| `default_visibility` | string | 空或无法解析 → `private` | `private` | 新建文件夹未指定可见范围时用它；上传的文件跟随父目录。可选 `private` / `internal` / `public` |
| `max_path_depth` | int32 | `0` → `32` | `32` | 目录最大嵌套层数，超过时报 `INVALID_ARGUMENT` |
| `max_name_length` | int32 | `0` → `255` | `255` | 节点名最大字节数 |
| `default_role_preset` | string | 空或无法解析 → `guest` | `guest` | 建号时未指定角色所使用的预设，可选 `guest` / `user` / `manager`。默认 `guest` 意味着「没写清楚就是只读」 |

### 5.2 `upload` — 分片与版本

| 键 | 类型 | 代码默认 | 示例 | 说明 |
| --- | --- | --- | --- | --- |
| `default_chunk_size` | int64 | `0` → `8388608`（8 MiB） | `8388608` | 客户端没指定分片大小时使用；会被夹到 `[min, max]` 并向上对齐到 256 KiB |
| `min_chunk_size` | int64 | `0` → `5242880`（5 MiB） | `5242880` | **不要低于 5242880**：S3 要求除最后一片外每片至少 5 MiB，组装和完整性校验都按这个下限检查，调小会让大文件组装失败 |
| `max_chunk_size` | int64 | `0` → `5368709120`（5 GiB） | `5368709120` | 单分片上限 |
| `max_file_size` | int64 | `0` = 不限 | `0` | 单文件上限，`InitiateUpload` 时预检 |
| `max_inline_size` | int64 | `0` → `4194304`（4 MiB） | `4194304` | `UploadSmallFile` 的载荷上限。注意 base64 会膨胀约 1/3，它同时受 `server.http.max_body_size` 约束 |
| `session_ttl` | Duration | `0` → `24h` | `86400s` | 上传会话有效期，过期后 `CompleteUpload` 返回 `UPLOAD_EXPIRED` |
| `default_mode` | string | 空或无法解析 → `presigned` | `presigned` | 默认传输方式。`presigned`=客户端直传对象存储；`proxy`=经服务端中转（对象存储对浏览器不可达时用） |
| `max_parts` | int32 | `0` → `10000` | `10000` | 单次上传分片数上限 |
| `verify_checksum` | bool | `false` | `true` | 完成时校验客户端声明的 SHA-256 |
| `keep_versions` | bool | `false` | `true` | 覆盖写时把旧内容留为历史版本 |
| `max_versions` | int32 | `0` = 不限 | `20` | 每个文件保留的版本数，超出后从最旧的开始删除 |

### 5.3 `web` — 前端托管与 CORS

| 键 | 类型 | 代码默认 | 示例 | 说明 |
| --- | --- | --- | --- | --- |
| `enabled` | bool | `false` | `true` | 是否托管前端。关掉后 API 与 `/docs/` 仍正常，只是没有静态站点 |
| `root` | string | 空 → `./web/dist` | `./web/dist` | 构建产物目录；目录不存在只打 WARN，不影响启动 |
| `index` | string | 空 → `index.html` | `index.html` | 目录请求与 SPA 回退使用的文件 |
| `spa_fallback` | bool | `false` | `true` | 未知路径返回 index（前端路由需要）；关掉则返回 404。`/v1/`、`/docs/` 始终返回 404，不会被回退成 HTML |
| `public_base_url` | string | 空 → 只输出相对地址 | `https://netdisk.example.com` | 拼接分享链接与签名 URL 的绝对前缀 |
| `path_prefix` | string | 空 → `/` | `/` | SPA 挂载的路径前缀 |
| `cors_origins` | []string | 空 → 不下发任何 CORS 头 | `https://app.example.com` | 允许的源站，`*` 表示全部（与 `cors_allow_credentials` 互斥） |
| `cors_methods` | []string | 空 → `GET,POST,PUT,PATCH,DELETE,OPTIONS` | 同默认 | 预检回复的 `Access-Control-Allow-Methods` |
| `cors_headers` | []string | 空 → `Authorization,Content-Type,X-Node-Token,X-Share-Token,X-Request-Id,Accept,Origin` | 同默认 | 预检回复的 `Access-Control-Allow-Headers` |
| `cors_allow_credentials` | bool | `false` | `false` | 需要携带 Cookie 时才开；开着同时把 `cors_origins` 写成 `*` 浏览器会拒绝 |
| `cache_max_age` | Duration | `0` → 一律 `no-cache` | `3600s` | 仅对形如 `app.9f3a1c2d.js` 的哈希资源生效，其它文件仍 `no-cache` |
| `share_path_prefix` | string | 空 → `/s` | `/s` | 分享链接里 token 前面的路径段 |

CORS 过滤器**始终安装**，与 `enabled` 无关：前端部署在别处时也需要它。没有 `cors_origins` 时不下发任何 CORS 头，等于关闭跨域。

---

## 6. 运行参数（数据库内，预留）

`GET/PUT /v1/system/settings/*` 读写的是数据库 `settings` 表，与上面的 YAML 无关。键名与 YAML 同名（如 `upload.max_versions`），首次启动会种入默认值。

**这些值目前只存不读**：没有任何领域逻辑回读它们，改了对行为没有影响（`system.version` 标记为只读）。要调整行为请改 YAML 并重启。

| 键 | 类型 | 种入值 |
| --- | --- | --- |
| `guest.default_permissions` | int | guest 预设掩码 `3`（查看+下载） |
| `storage.default_quota_bytes` | int | `0` |
| `upload.keep_versions` | bool | `true` |
| `upload.max_versions` | int | `20` |
| `share.allow_public` | bool | `true` |
| `trash.retention_days` | int | `30` |
| `system.maintenance_interval` | string | `1h` |
| `system.version` | string | 只读，当前版本 |

---

## 7. 三套可抄的示例

### 7.1 最小可用（只跑接口，不接对象存储）

适合先验证权限模型、目录树与文档。

```yaml
server:
  http: { addr: 127.0.0.1:8000, timeout: 600s }
  grpc: { addr: 127.0.0.1:9000, timeout: 600s }
data:
  database:
    driver: sqlite
    source: ./data/nagisa.db
    auto_migrate: true
    wal: true
    busy_timeout: 10s
    foreign_keys: true
  object_storage:
    endpoint: ""          # 关键：留空即不启用
auth:
  jwt_secret: local-dev-only
  admin_password: Admin@12345
storage:
  root_folder_name: 我的网盘
web:
  enabled: false
```

上传/下载会返回 `NETDISK_UNAVAILABLE`，其余接口全部可用。

### 7.2 本地开发（SeaweedFS + 前端 dev server）

```yaml
server:
  http: { addr: 0.0.0.0:8000, timeout: 600s }
  grpc: { addr: 0.0.0.0:9000, timeout: 600s }
data:
  database:
    driver: sqlite
    source: ./data/nagisa.db
    auto_migrate: true
    debug: false
    wal: true
    busy_timeout: 10s
    foreign_keys: true
  object_storage:
    endpoint: 127.0.0.1:8333
    access_key: nagisa
    secret_key: nagisa-secret
    bucket: nagisa
    auto_create_bucket: true
    prefix: netdisk
    presign_ttl: 1800s
auth:
  jwt_secret: local-dev-only
  admin_password: Admin@12345
  allow_plain_password: false
upload:
  default_mode: presigned
  default_chunk_size: 8388608
  min_chunk_size: 5242880
web:
  enabled: true
  root: ./web/dist
  spa_fallback: true
  public_base_url: http://127.0.0.1:8000
  cors_origins:
    - http://127.0.0.1:5173
    - http://localhost:5173
```

### 7.3 生产（TLS 反代 + 固定密钥 + 严格 CORS）

```yaml
server:
  http: { addr: 127.0.0.1:8000, timeout: 1800s, max_body_size: 33554432 }
  grpc: { addr: 127.0.0.1:9100, timeout: 1800s }
data:
  database:
    driver: sqlite
    source: /srv/nagisa/data/nagisa.db
    auto_migrate: false        # 迁移单独执行
    debug: false
    max_open_conns: 8
    max_idle_conns: 8
    wal: true
    busy_timeout: 15s
    foreign_keys: true
  object_storage:
    endpoint: seaweedfs.internal:8333
    access_key: ${S3_ACCESS_KEY:}
    secret_key: ${S3_SECRET_KEY:}
    bucket: nagisa-prod
    use_ssl: true
    public_endpoint: files.example.com   # 浏览器走 CDN/反代
    auto_create_bucket: false
    prefix: prod
    presign_ttl: 900s
auth:
  jwt_secret: ${JWT_SECRET:}             # KRATOS_JWT_SECRET 注入
  issuer: nagisa-prod
  access_token_ttl: 3600s
  refresh_token_ttl: 1209600s
  password_private_key_file: /srv/nagisa/data/password_key.pem
  allow_plain_password: false
  admin_password: ${ADMIN_PASSWORD:}
  guest_username: guest
  node_token_ttl: 900s
  min_password_length: 12
storage:
  default_quota_bytes: 107374182400
  guest_quota_bytes: 1073741824
  case_insensitive_names: true
  allow_public_share: true
  trash_retention: 1296000s              # 15 天
  default_visibility: private
  max_path_depth: 32
  max_name_length: 255
  default_role_preset: guest
upload:
  default_chunk_size: 16777216
  min_chunk_size: 5242880
  max_chunk_size: 5368709120
  max_file_size: 0
  max_inline_size: 4194304
  session_ttl: 86400s
  default_mode: presigned
  verify_checksum: true
  keep_versions: true
  max_versions: 20
web:
  enabled: true
  root: /srv/nagisa/web/dist
  spa_fallback: true
  public_base_url: https://netdisk.example.com
  cors_origins:
    - https://netdisk.example.com
  cors_allow_credentials: false
  cache_max_age: 86400s
  share_path_prefix: /s
```

配套的环境变量（密钥不进仓库）：

```bash
KRATOS_JWT_SECRET=$(openssl rand -base64 48)
KRATOS_S3_ACCESS_KEY=...
KRATOS_S3_SECRET_KEY=...
KRATOS_ADMIN_PASSWORD=...
```

---

## 8. 启动即失败的常见原因

| 症状 | 原因 | 处理 |
| --- | --- | --- |
| `panic: invalid google.protobuf.Duration value "30m"` | Duration 写了 `30m`/`2h` 这类单位 | 改成秒字符串：`1800s`、`7200s` |
| `panic: unsupported key: xxx format: yyy` | 配置目录里有非配置文件的扩展名 | 已修复：目录形态只读 `config.yaml`。若你显式传了文件，请确认它是 YAML |
| `panic: config path ...: no such file` | `-conf` 路径不存在，或目录里没有 `config.yaml` | 检查路径；默认是 `./configs` |
| `panic: data: open database: ...` | SQLite 目录不可写，或 MySQL DSN 错误 | 检查 `source` 的父目录权限 |
| `panic: data: migrate schema: sqlite: foreign_keys pragma is off: missing "_fk=1" in the connection string` | 用 SQLite 且开了 `auto_migrate: true`，但 `data.database.foreign_keys` 还是代码默认的 `false` | ent 建表/改表要求连接串里有 `_fk=1`，把 `foreign_keys` 设成 `true`（两个开关应当同开同关；三个可抄示例都是这么配的） |
| `panic: data: probe bucket / create bucket` | 对象存储不可达或凭据错误，且 `auto_create_bucket: true` | 修好连通性，或先 `auto_create_bucket: false` 让进程起来 |
| 日志出现 `auth.jwt_secret is unset ...` | 未设置 `jwt_secret` | 设好它，否则重启后所有令牌失效 |
| `allow_plain_password: true` | 无 | 生产环境必须为 `false` |
| 老配置里还留着 `auth.guest_password` | 该键已移除，访客不再有口令 | **不会导致启动失败**：配置解码用的是 protojson 的 `DiscardUnknown`，多余键被静默丢弃，顺手删掉那一行即可 |

自检：

```bash
curl -s http://127.0.0.1:8000/v1/system/health   # 依赖探活
curl -s http://127.0.0.1:8000/v1/system/info     # 生效的存储后端、限额、上传模式
```

`/v1/system/info` 会把实际生效的 `storage_backend`、`database_backend`、各种上限与 `upload_modes` 报回来，是确认配置是否按预期生效的最快方式。

上传相关的几个上限（`default_chunk_size`、`min_chunk_size`、`max_inline_size`、`session_ttl`、签名 URL 的 TTL 与它的 4 倍上限）报的是**生效值**，即服务端补齐默认之后的值，而不是配置文件里写的原始值：不写 `upload` 段也会看到 `8388608` / `5242880` / `4194304` / `86400s` / `1800s` / `7200s`，不会是一串 `0`。

---

## 9. 相关文档

- [`deployment.md`](deployment.md) — 部署、对象存储（SeaweedFS）、反向代理、备份、维护任务
- [`architecture.md`](architecture.md) — 分层、并发与事务边界、对象键布局
- [`permissions.md`](permissions.md) — 角色、权限位、ACL 判定顺序
- [`api-guide.md`](api-guide.md) — 每个接口的请求与错误

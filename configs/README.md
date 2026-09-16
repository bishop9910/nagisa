# `config.yaml` 逐条说明

这份文档解释 [`config.yaml`](config.yaml) 里**每一个条目是干什么的**，用大白话写，不涉及代码细节。

- 想知道某个键**缺省时程序实际用什么值**、怎么用环境变量覆盖、上线怎么配 → 看 [`../docs/configuration.md`](../docs/configuration.md)
- 想知道**怎么部署起来**（对象存储、反代、备份、运维任务）→ 看 [`../docs/deployment.md`](../docs/deployment.md)

> 本目录里除了 `config.yaml` 可以有别的文件（比如这份 README）。启动时 `-conf <目录>` **只读 `<目录>/config.yaml`**，不会去解析目录里的其它文件。

配置一共 6 段、54 个条目。

---

## server —— 监听与超时

| 键 | 现在的值 | 干什么用的 |
| --- | --- | --- |
| `http.addr` | `0.0.0.0:8000` | REST 接口、`/docs/` 文档页、前端静态文件都从这个端口出 |
| `http.timeout` | `600s` | 单个 HTTP 请求的超时。**必须放大**：kratos 默认 1 秒，会把流式下载、打包 zip、大分片上传全掐断 |
| `grpc.addr` | `0.0.0.0:9000` | gRPC 端口，和 HTTP 是同一批接口 |
| `grpc.timeout` | `600s` | gRPC 请求超时，同上 |

## data —— 数据库和对象存储

### `database`：元数据存哪

| 键 | 现在的值 | 干什么用的 |
| --- | --- | --- |
| `driver` | `sqlite` | 用哪个数据库。改成 `mysql` 就走 MySQL |
| `source` | `./data/nagisa.db` | SQLite 就是数据库文件路径（父目录会自动建）；MySQL 则是 DSN |
| `debug` | `false` | 开了会把每条 SQL 打到日志，只适合本地排查 |
| `auto_migrate` | `true` | 启动时按代码里的表结构自动建表/加列。生产建议关掉、单独跑迁移 |
| `max_open_conns` | `8` | 数据库连接池上限 |
| `max_idle_conns` | `8` | 池里保持的空闲连接数 |
| `wal` | `true` | SQLite 的 WAL 日志模式。**保持开着**，否则读会被写阻塞 |
| `busy_timeout` | `10s` | 抢写锁时最多等多久 |
| `foreign_keys` | `true` | 开启外键约束检查 |

### `object_storage`：文件内容存哪

| 键 | 现在的值 | 干什么用的 |
| --- | --- | --- |
| `endpoint` | `127.0.0.1:8333` | S3 兼容地址（默认值对应 SeaweedFS 的 S3 端口）。**留空就等于不启用对象存储**：账号、权限、文件夹、ACL 照常可用，上传下载报 `NETDISK_UNAVAILABLE` |
| `access_key` / `secret_key` | `nagisa` | 对象存储凭据，必须与 SeaweedFS 的 `s3.json` 一致 |
| `bucket` | `nagisa` | 用哪个桶 |
| `region` | 空 | 一般是空，交给 SDK 默认 |
| `use_ssl` | `false` | 连对象存储是否走 HTTPS |
| `public_endpoint` | 空 | **给浏览器用的地址**。浏览器能直连的地址和服务端不一样时必须填（比如走 CDN/反代），否则预签名 URL 签名对不上，直传直下会失败 |
| `auto_create_bucket` | `true` | 启动时桶不存在就自动创建，开发方便 |
| `prefix` | `netdisk` | 所有对象的公共前缀，一个桶放多个部署时用来隔离 |
| `presign_ttl` | `1800s` | 分片上传的预签名 URL 和下载直链的默认有效期（客户端要更长会被夹到它的 4 倍以内） |
| `insecure_skip_verify` | `false` | 跳过 TLS 证书校验，只给自签证书的本地环境用 |

## auth —— 登录与凭据

| 键 | 现在的值 | 干什么用的 |
| --- | --- | --- |
| `jwt_secret` | 空 | 签发访问令牌、文件夹解锁令牌、URL 签名的密钥。**为空时每次启动随机生成**，重启后所有令牌失效，生产必填 |
| `issuer` | `nagisa-netdisk` | 令牌里的签发方标识，基本不用动 |
| `access_token_ttl` | `7200s` | 访问令牌有效期（2 小时） |
| `refresh_token_ttl` | `2592000s` | 刷新令牌有效期（30 天）。刷新令牌只存哈希，且用一次就换新的 |
| `password_private_key_file` | `./data/password_key.pem` | RSA 私钥路径，用来解开客户端 RSA 加密后的密码。文件不存在会自动生成。**这是密码体系的钥匙，容器化必须挂持久卷** |
| `allow_plain_password` | `false` | 允许客户端不加密直接发明文密码。只有本地临时调试才开 |
| `admin_username` / `admin_password` | `admin` / 空 | 内置超级管理员。密码留空则首次启动随机生成并往日志打一次；账号已存在时不会覆盖 |
| `guest_username` | `guest` | 内置访客账号，默认只读。**没有密码配置**——访客会话一律走 `POST /v1/auth/guest` 免密换取 |
| `guest_auto_login` | `true` | **打开网页不用登录就是访客**：前端启动时自动换取一个只读访问令牌。访客账号的角色、等级、权限与状态被锁死，任何人都改不了（和内置管理员一样），只有这里能关掉这个入口 |
| `node_token_ttl` | `1800s` | 输入文件夹密码后拿到的解锁令牌能用多久 |
| `min_password_length` | `8` | 账号密码、文件夹密码、分享密码的最短长度 |

## storage —— 文件树的规矩

| 键 | 现在的值 | 干什么用的 |
| --- | --- | --- |
| `root_folder_name` | `我的网盘` | 文件树虚拟根的名字，出现在返回的 `displayPath` 里 |
| `default_quota_bytes` | `0` | 管理员建号时没指定配额就用这个，0 = 不限 |
| `guest_quota_bytes` | `1073741824` | 内置 guest 账号的配额（1 GiB）。**只在建号那一刻生效**，之后改这里不会影响已有账号 |
| `case_insensitive_names` | `true` | 同一目录里 `A.txt` 和 `a.txt` 算不算重名 |
| `allow_public_share` | `true` | 是否允许分享功能。**关掉会让整个分享功能不可用**，不只是禁止新建 |
| `trash_retention` | `2592000s` | 回收站保留多久（30 天），维护任务按它清理 |
| `default_visibility` | `private` | 新建文件夹没写可见范围时用哪个（`private`/`internal`/`public`）；上传的文件跟随所在文件夹 |
| `max_path_depth` | `32` | 目录最多嵌套几层 |
| `max_name_length` | `255` | 文件/文件夹名最大长度（字节） |
| `default_role_preset` | `guest` | 建号没写角色时给什么权限模板。默认 `guest` 意味着「说不清就是只读」 |

## upload —— 分片与版本

| 键 | 现在的值 | 干什么用的 |
| --- | --- | --- |
| `default_chunk_size` | `8388608` | 客户端没指定分片大小时用 8 MiB |
| `min_chunk_size` | `5242880` | 最小分片 5 MiB。**不要调小**：S3 要求除最后一片外每片至少 5 MiB，调小会让大文件组装失败 |
| `max_chunk_size` | `5368709120` | 单分片上限 5 GiB |
| `max_file_size` | `0` | 单文件上限，0 = 不限 |
| `max_inline_size` | `4194304` | 「小文件一次传完」这个接口的体积上限 4 MiB |
| `session_ttl` | `86400s` | 上传会话多久过期，过期后不能再完成 |
| `default_mode` | `presigned` | 分片默认怎么传：`presigned` = 客户端直传对象存储（不经过服务端）；`proxy` = 流经服务端转发（对象存储对浏览器不可达时用） |
| `max_parts` | `10000` | 一次上传最多多少片 |
| `verify_checksum` | `true` | 完成时是否校验客户端声明的 SHA-256 |
| `keep_versions` | `true` | 覆盖写时是否把旧内容留成历史版本 |
| `max_versions` | `20` | 每个文件最多留几个版本，超了从最旧的删 |

## web —— 前端托管和跨域

| 键 | 现在的值 | 干什么用的 |
| --- | --- | --- |
| `enabled` | `true` | 是否由后端直接托管前端页面。关掉后接口和 `/docs/` 照常，只是没有网页 |
| `root` | `./web/dist` | 前端构建产物目录 |
| `index` | `index.html` | 入口文件，也是前端路由的回退页 |
| `spa_fallback` | `true` | 访问未知路径时返回入口页（前端路由需要）。`/v1/`、`/docs/` 例外，永远 404 |
| `public_base_url` | `http://127.0.0.1:8000` | 拼分享链接和签名 URL 用的绝对前缀，上线要改成真实域名 |
| `path_prefix` | `/` | 前端挂在哪个路径下 |
| `cors_origins` | `http://127.0.0.1:5173` 等 | 允许跨域的前端源站。**留空则不下发任何 CORS 头**，浏览器前端就调不通 |
| `cors_methods` | GET/POST/… | 预检回复里允许的 HTTP 方法 |
| `cors_headers` | Authorization、`X-Node-Token` 等 | 预检回复里允许携带的请求头 |
| `cors_allow_credentials` | `false` | 要带 Cookie 才开。开着同时把源站写成 `*`，浏览器会拒绝 |
| `cache_max_age` | `3600s` | 静态资源缓存时长，只对带内容哈希的文件名（`app.3f9a1c2b.js`）生效，其他一律不缓存 |
| `share_path_prefix` | `/s` | 分享链接里 token 前面的路径段，即 `域名/s/<token>` |

---

## 三个容易踩的点

1. **所有时间只能写秒**：`"1800s"`。写 `30m`、`2h` 会直接启动崩溃（`panic: invalid google.protobuf.Duration value "2h"`）。
2. **`${XXX:默认值}` 是被环境变量 `KRATOS_XXX` 填的**，不是被同名 OS 变量填的。比如 `${JWT_SECRET:}` 要设 `KRATOS_JWT_SECRET`。也可以绕开占位符，直接按路径覆盖：`KRATOS_server.http.addr=0.0.0.0:8080`。
3. **有三个默认值是坑**：
   - `jwt_secret` 空 → 重启后令牌全失效；
   - `object_storage.endpoint` 与服务 `grpc.addr` 别撞端口：默认值（`8333` 与 `9000`）是错开的，改过端口后要自己确认；
   - `public_endpoint` 空 → 浏览器直连地址和服务端不同时签名对不上。

## 上线前必须改的

| 键 | 改成 |
| --- | --- |
| `auth.jwt_secret` | 一段足够长的随机串（`KRATOS_JWT_SECRET`） |
| `auth.admin_password` | 明确的强口令（`KRATOS_ADMIN_PASSWORD`） |
| `auth.password_private_key_file` | 持久卷里的路径 |
| `data.database.source` | 持久卷里的路径 |
| `data.object_storage.endpoint` / `access_key` / `secret_key` / `bucket` | 真实的对象存储信息 |
| `data.object_storage.public_endpoint` | 浏览器可达的地址（与服务端不同时必填） |
| `data.object_storage.auto_create_bucket` | `false`（桶预先建好） |
| `data.database.auto_migrate` | `false`（迁移单独执行） |
| `server.grpc.addr` | 与对象存储端口错开（SeaweedFS 占 `8333`/`9333`/`8888`/`8080`） |
| `web.public_base_url` | 真实域名 |
| `web.cors_origins` | 真实前端源站 |
| `auth.allow_plain_password` | 保持 `false` |

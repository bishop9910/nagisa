# API 使用指南

本文是 Nagisa 网盘 HTTP API 的完整参考：通用约定、认证流程、逐个 RPC 的路径/字段/错误，以及上传、下载、分享的完整调用序列。字段的权威定义在 `api/netdisk/v1/*.proto`，机器可读版本见 `openapi.yaml` 与 `docs/openapi.yaml`。

服务同时提供 gRPC（同一批 proto 方法、同一个用例层），本文只描述 HTTP 绑定。

## 通用约定

### 基础地址

| 项 | 值 |
| --- | --- |
| HTTP 基地址 | `http://<host>:8000`（`server.http.addr`，示例配置 `0.0.0.0:8000`） |
| gRPC 地址 | `<host>:9000`（`server.grpc.addr`） |
| API 版本 | `v1`，所有路径都以 `/v1/` 开头 |
| OpenAPI 文档 | `GET /docs/`（Swagger UI）、`GET /docs/openapi.yaml`、`GET /docs/swagger.yaml`、`GET /docs/openapi.json` |
| 公开探活 | `GET /v1/system/health` |

### 请求头

| 请求头 | 何时需要 | 说明 |
| --- | --- | --- |
| `Authorization: Bearer <accessToken>` | 除公开接口外的全部接口 | 令牌由 `POST /v1/auth/login` 签发；也接受不带 `Bearer ` 前缀的裸令牌 |
| `X-Node-Token: <unlockToken>` | 访问受密码保护的文件夹及其子树时 | 由 `POST /v1/nodes/{node_id}/unlock` 签发；缺失或过期时返回 403 `NETDISK_NODE_LOCKED` |
| `X-Request-Id` | 可选 | 调用方自带的链路标识；服务端会写入响应头与审计日志，缺省时自行生成 |
| `Content-Type: application/json` | 有请求体时 | 服务端以 protobuf JSON 解析；`Content-Type` 不含 `json` 时回落到默认解码器 |
| `X-Forwarded-For` / `X-Real-Ip` | 可选（经反向代理时） | 用于记录客户端地址，`X-Forwarded-For` 取第一个地址 |
| `Range` | 下载时可选 | 仅 `GET /v1/files/{node_id}/content` 支持 |

公开接口（无需令牌）共 8 个，与 `internal/server/middleware/auth.go` 的清单一致：

`AuthService.GetAuthConfig`、`AuthService.Login`、`AuthService.RefreshToken`、`ShareService.AccessShare`、`ShareService.ListShareChildren`、`ShareService.GetShareDownloadUrl`、`SystemService.GetSystemInfo`、`SystemService.HealthCheck`。

### 数据传输格式

请求与响应使用 **protobuf 的规范 JSON 映射**（`protojson`），而不是默认的反射编码：

| 类型 | 线上形态 | 请求示例 | 响应示例 |
| --- | --- | --- | --- |
| 枚举 | 名称字符串（也接受数字） | `"visibility": "VISIBILITY_PRIVATE"` | `"role": "ROLE_MANAGER"` |
| 64 位整数 | 字符串（也接受 JSON 数字） | `"size": "12582912"` | `"totalSize": "42"` |
| `bytes` | base64 | `"content": "SGVsbG8="` | `"content": "SGVsbG8="` |
| `google.protobuf.Timestamp` | RFC 3339 字符串 | `"trashedBefore": "2026-02-01T09:30:00Z"` | `"createdAt": "2026-02-01T09:30:00Z"` |
| `google.protobuf.FieldMask` | 逗号分隔的驼峰路径字符串 | `"updateMask": "description,visibility"` | 同上 |
| `map<string,string>` | JSON 对象 | `"metadata": {"tag": "2026"}` | 同上 |

字段名同时接受文档中的 camelCase 与 proto 原始的下划线写法：`parentId` 与 `parent_id` 等价，查询参数同理。请求体中的未知字段会被忽略（`DiscardUnknown`），因此把刚读到的对象整体回传是安全的。

响应由 `protojson` 默认配置编码，**零值字段不会出现在响应里**（`size` 为 0、`totalSize` 为 0、`locked` 为 false 等字段可能缺席）。客户端应当把「字段缺失」当作零值处理。

### 分页

所有列表接口使用 AIP 分页：

| 参数 | 说明 |
| --- | --- |
| `page_size` | 可选。缺省 **20**；超过该接口上限时按上限截断 |
| `page_token` | 可选。上一次响应里的 `next_page_token` 原样回传 |
| `next_page_token` | 只在「本次返回条数 == 请求条数」时给出；为空表示已到末页 |
| `total_size` | 满足过滤条件的总数（忽略分页）；少数接口（如部分分享接口）返回的是本页条数 |

各接口的 `page_size` 上限：

| 接口 | 上限 |
| --- | --- |
| `ListNodes`、`SearchNodes`、`ListTrash`、`GetNodeTree`（每层） | 1000 |
| `ListUsers`、`ListUploads`、`ListShares`、`ListSharesByNode`、`ListShareChildren`、`ListNodeVersions` | 200 |
| `ListAuditLogs` | 500 |
| `ListSessions` | 1000 |

### 过滤与排序

`filter` 使用 AIP 标准语法（`go.einride.tech/aip/filtering`）：

- 字符串等值：`filter=status=1`、`filter=username="alice"`；
- 字符串匹配：`filter=name:"report"`、`filter=mime_type:"image/"`（proto 注释描述为前缀匹配）；
- 数值与时间比较：`filter=size>1048576`、`filter=rank<500`、`filter=created_at>"2026-01-01T00:00:00Z"`；
- 布尔：`filter=success`（为真）、`filter=NOT success`（为假）。AIP 语法里没有布尔字面量，因此 `success=true` / `success=false` 由服务端在解析前归一化成上面两种写法，两种形式等价；
- 组合：`AND` / `OR` / `NOT` 与括号。

未声明的标识符或语法错误返回 400 `NETDISK_INVALID_ARGUMENT`。`order_by` 是逗号分隔的字段列表，采用 AIP 的排序语法：升序直接写字段名，降序在字段名后加空格与 `desc`（如 `order_by=size desc,name`）。**注意不能写成 `-size`**：AIP 的解析器不接受 `-`，会直接返回 400；不在白名单里的字段同样返回 400。

各接口支持的字段（过滤字段用下划线，排序字段同左）：

| 接口 | `filter` 支持字段 | `order_by` 支持字段 |
| --- | --- | --- |
| `ListNodes`、`GetNodeTree` | `id`、`name`、`kind`、`mime_type`、`extension`、`size`、`owner_id`、`status`、`path`、`trashed_at`、`created_at`、`updated_at` | `name`、`size`、`kind`、`mime_type`、`extension`、`status`、`created_at`、`updated_at` |
| `ListTrash` | 同上（含 `trashed_at` 区间） | `name`、`size`、`trashed_at`、`updated_at` |
| `SearchNodes` | 无 `filter` 字段，改用请求里的 `query`、`kind`、`mime_type`、`owner_id`、`min_size`、`max_size`、`updated_after`、`updated_before`、`match_description` | `name`、`size`、`updated_at`、`created_at` |
| `ListUsers` | `username`、`nickname`、`email`、`role`、`status`、`rank`、`created_at`、`updated_at`、`last_login_at` | `username`、`nickname`、`role`、`rank`、`status`、`used_bytes`、`created_at`、`updated_at`、`last_login_at` |
| `ListShares` | `node_id`、`owner_id`、`name`、`status`、`token`、`created_at`、`expires_at` | `name`、`created_at`、`updated_at`、`expires_at`、`download_count`、`view_count` |
| `ListAuditLogs` | `actor_id`、`actor_name`、`action`、`target_type`、`target_id`、`success`、`ip`、`created_at` | `created_at`、`action`、`actor_name` |
| `ListUploads`、`ListSessions`、`ListSharesByNode`、`AccessShare`、`ListNodeVersions` | 不支持 `filter`（消息里没有该字段） | 不支持 `order_by`：`ListUploads`/`ListSessions` 只分页；`ListSharesByNode`/`AccessShare` 的排序字段被解析但不参与查询，`ListShareChildren` 的 `order_by` 同样不生效 |

`ListNodes` 的返回始终把文件夹排在文件之前；`SearchNodes` 实际是「先按范围取最多 5000 行，再在内存里匹配、过滤、分页」，因此 `total_size` 是匹配结果数而不是全库数。

### 错误信封

失败响应的结构固定为：

```json
{"code":403,"reason":"NETDISK_PERMISSION_DENIED","message":"permission denied"}
```

| 字段 | 说明 |
| --- | --- |
| `code` | HTTP 状态码，同一份数值也出现在响应状态行 |
| `reason` | `ErrorReason` 枚举名，**客户端应当按它分支**，不要解析 `message` |
| `message` | 人类可读的英文简短描述 |

注意 `HTTP 状态码 ↔ reason` 是「多对一」的：例如 `NETDISK_QUOTA_EXCEEDED`、`NETDISK_NODE_LOCKED`、`NETDISK_ACCOUNT_DISABLED`、`NETDISK_PERMISSION_DENIED` 都是 403。完整对照见文末「错误码表」。

### 原始字节绑定

以下三条路由直接收发二进制，无法用 proto JSON 表达，因此注册在生成的中间件链之外，各自负责鉴权（CORS 与 `X-Request-Id` 过滤器仍然生效）：

| 方法与路径 | 鉴权方式 | 请求体 | 响应 |
| --- | --- | --- | --- |
| `PUT /v1/files/uploads/{upload_id}/parts/{part_number}/raw` | `Authorization: Bearer`（会话属主） | 原始分片字节，必须有 `Content-Length` | `UploadPart`（JSON） |
| `GET /v1/files/{node_id}/content` | URL 签名（`exp` / `sig` / `sub` / `extra` 查询参数） | 无 | 原始内容，支持 `Range` |
| `GET /v1/files/{node_id}/archive` | URL 签名（`exp` / `sig` / `sub` / `extra`=归档名） | 无 | `application/zip` 流 |

## 认证流程

```text
① GET  /v1/auth/config            → passwordPublicKey / passwordEncoding / TTL
② 本地加密：base64( RSA-OAEP-SHA256( plainPassword ) )
③ POST /v1/auth/login             → accessToken + refreshToken + user
③' POST /v1/auth/guest            → accessToken（只读访客，无口令；开启即可跳过 ① ② ③）
④ 业务请求：Authorization: Bearer <accessToken>
⑤ POST /v1/auth/refresh           → 新的 accessToken + refreshToken（旧的立即失效）
⑥ POST /v1/auth/logout            → 吊销该 refreshToken，或吊销该账号全部会话
```

**① 取握手参数。** `GET /v1/auth/config` 返回 `passwordKeyId`（密钥标识，用于发现密钥轮换）、`passwordEncoding`（固定 `rsa-oaep-sha256`）、`passwordPublicKey`（PEM 编码的 PKCS#8 RSA 公钥，3072 位）、`accessTokenTtlSeconds`、`refreshTokenTtlSeconds`、`plainPasswordAllowed`、`guestLoginEnabled`（是否允许免登录的只读访客会话）、`serverTime`。

**② 加密密码。** 所有密码字段（登录、改密、建号、重置密码、文件夹密码、分享密码）都必须发送 **RSA-OAEP(SHA-256) 密文的 base64**。服务端只在 `auth.allow_plain_password` 为 true 时才接受明文，生产环境必须保持 false。

```bash
# openssl 1.1.1+
curl -s $BASE/v1/auth/config | jq -r .passwordPublicKey > pub.pem
ENC=$(printf '%s' 'Admin@12345' | openssl pkeyutl -encrypt -pubin -inkey pub.pem \
        -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256 \
      | openssl base64 -A)
```

Go 客户端的关键两步是 `x509.ParsePKIXPublicKey` 与 `rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, []byte(plain), nil)`，随后 `base64.StdEncoding.EncodeToString`；`test/integration/smoke_test.go` 就是完整可运行示例。

**③ 登录。** `POST /v1/auth/login` 的请求体是 `{"username": ..., "password": "<base64 密文>"}`，可选 `ip`、`user_agent`（未经过反向代理时可作为提示）与 `remember_me`。回复 `LoginReply`：

| 字段 | 说明 |
| --- | --- |
| `accessToken` | HS256 JWT，放进 `Authorization: Bearer` |
| `refreshToken` | 不透明随机串（18 字节熵的 URL-safe base64），服务端只存 SHA-256 摘要 |
| `tokenType` | 固定 `Bearer` |
| `expiresIn` | 访问令牌有效期（秒），默认 7200 |
| `refreshExpiresIn` | 刷新令牌有效期（秒），默认 2592000（`remember_me` 为 true 时翻倍） |
| `user` | 已登录账号，含 `permissions` / `permissionsMask` / `rank` / `manageable` 等 |

**④ 使用访问令牌。** 除公开接口外的所有接口都要带令牌；令牌里携带 `uname` / `role` / `rank` / `perms`，但每次请求都会重新从数据库读取账号，因此改权限、禁用账号立即生效。

**⑤ 刷新与轮换。** `POST /v1/auth/refresh` 用 `refresh_token` 换一对新令牌，**旧的刷新令牌在同一次调用里被吊销**（真正的轮换，不是复用）；过期、已吊销或未知的令牌返回 401 `NETDISK_UNAUTHENTICATED`。新会话继承原会话的剩余有效期。

**⑥ 退出。** `POST /v1/auth/logout` 带 `refresh_token` 吊销某一个会话；带 `revoke_all: true` 吊销该账号全部会话（需要有效访问令牌）。修改密码、重置他人密码、禁用账号、删除账号都会连带吊销相关会话。

**会话管理。** `GET /v1/auth/sessions/list` 列出自己的刷新会话（`includeInactive` 决定是否包含已过期/已吊销的），`POST /v1/auth/sessions/revoke` 按 `id` 吊销其中一个；他人的会话 id 会被拒绝（403）。

## AuthService

### `GetAuthConfig`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/auth/config` |
| 鉴权 | 公开 |
| 请求 | 无 |
| 回复 | `AuthConfig`：`passwordKeyId`、`passwordEncoding`、`passwordPublicKey`、`accessTokenTtlSeconds`、`refreshTokenTtlSeconds`、`plainPasswordAllowed`、`guestLoginEnabled`、`serverTime` |
| 错误 | — |

### `Login`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/auth/login` |
| 鉴权 | 公开 |
| 请求 | `username`（必填）、`password`（必填，RSA 密文 base64）、`ip`、`user_agent`、`remember_me` |
| 回复 | `LoginReply`：`accessToken`、`refreshToken`、`tokenType`、`expiresIn`、`refreshExpiresIn`、`user` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（用户名为空、密码为空或无法解密）、`NETDISK_UNAUTHENTICATED`（账号不存在或密码不匹配，两种情况返回同一个错误）、`NETDISK_ACCOUNT_DISABLED`（账号被禁用或已删除） |

### `GuestLogin`

免登录换取只读访客会话，前端「打开网页即可浏览」就是靠它。

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/auth/guest` |
| 鉴权 | 公开（不需要任何令牌，也不需要口令） |
| 请求 | 可选 `ip`、`user_agent` 提示 |
| 回复 | `LoginReply`，但 **`refreshToken` 为空**、`refreshExpiresIn` 为 0：访客没有需要轮换的服务端会话，令牌过期后重新调用本接口即可。`user` 是内置访客账号，权限只有 `view` 与 `download` |
| 错误 | `NETDISK_UNSUPPORTED`（部署未开启 `auth.guest_auto_login`）、`NETDISK_ACCOUNT_DISABLED`（访客账号被停用）、`NETDISK_UNAUTHENTICATED`（访客账号不存在） |

行为要点：

- 只有 `auth.guest_auto_login: true` 时可用；`GET /v1/auth/config` 的 `guestLoginEnabled` 会如实反映，客户端应据此决定要不要显示「以访客身份浏览」。
- **访客没有口令**：内置访客账号的密码哈希是一串谁也拿不到的随机值，用 `Login` 提交 `guest` 这个用户名一律返回 `NETDISK_UNAUTHENTICATED`。关掉 `auth.guest_auto_login` 就等于彻底关闭访客入口；需要一个有口令的只读账号，请自建账号并使用 `guest` 角色预设。
- **内置访客账号被锁死**：角色、等级、权限集、状态都由配置决定，没有任何账号（包括内置管理员）能通过 API 修改或删除它——`User.manageable` 与 `User.permissionsEditable` 对这两个内置账号恒为 false。服务端每次启动还会把它们校正回配置值。
- 访客能看到什么，完全由既有的可见范围与访问名单决定：`VisibilityInternal` / `VisibilityPublic` 的节点对已登录身份可见，访客会话因此也是「已登录」身份。给某个用户或角色加 ACL 条目同样生效。

### `RefreshToken`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/auth/refresh` |
| 鉴权 | 公开 |
| 请求 | `refresh_token`（必填） |
| 回复 | `LoginReply`（同 `Login`） |
| 错误 | `NETDISK_INVALID_ARGUMENT`（为空）、`NETDISK_UNAUTHENTICATED`（未知、已吊销或已过期）、`NETDISK_ACCOUNT_DISABLED` |

### `Logout`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/auth/logout` |
| 鉴权 | 需要令牌（`revoke_all` 时强制需要） |
| 请求 | `refresh_token`（可选）、`revoke_all`（可选） |
| 回复 | 空对象 |
| 错误 | `NETDISK_UNAUTHENTICATED`（未带令牌却请求吊销全部会话，或未带令牌又未提供 `refresh_token`，或 `refresh_token` 未知）、`NETDISK_PERMISSION_DENIED`（`refresh_token` 属于他人） |

### `GetCurrentUser`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/auth/me` |
| 鉴权 | 需要令牌 |
| 请求 | 无 |
| 回复 | `User`，其中 `manageable`、`permissionsEditable`、`maxGrantableRank` 是按调用方计算的 |
| 错误 | `NETDISK_UNAUTHENTICATED` |

### `ChangePassword`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/auth/password/change` |
| 鉴权 | 需要令牌 |
| 请求 | `current_password`（必填，RSA 密文）、`new_password`（必填，RSA 密文） |
| 回复 | 空对象 |
| 错误 | `NETDISK_INVALID_ARGUMENT`（字段为空、密文无法解密、新密码短于 `auth.min_password_length`）、`NETDISK_UNAUTHENTICATED`（当前密码不匹配） |

成功后账号的 `must_change_password` 被清除，并且**该账号其它所有刷新会话被吊销**。

### `ListSessions`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/auth/sessions/list` |
| 鉴权 | 需要令牌 |
| 请求 | `page_size`、`page_token`、`include_inactive` |
| 回复 | `SessionSet`：`sessions[]`（`id`、`ip`、`user_agent`、`created_at`、`last_used_at`、`expires_at`、`active`、`current`）、`next_page_token` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（分页令牌非法）、`NETDISK_UNAUTHENTICATED` |

只返回调用方自己的会话。

### `RevokeSession`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/auth/sessions/revoke` |
| 鉴权 | 需要令牌 |
| 请求 | `id`（必填） |
| 回复 | 空对象 |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED`（不是自己的会话） |

## UserService

账号管理的总则是：调用方必须持有 `PERMISSION_USER_MANAGE`；只能创建/修改/删除**等级严格低于自己**的账号；只能授予自己拥有的权限；内置超级管理员（`rank = 1000`）对任何人都不可管理；管理路径上不允许对自己操作。细节见 [`permissions.md`](permissions.md)。

### `CreateUser`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/users/create` |
| 鉴权 | 需要令牌 + `PERMISSION_USER_MANAGE` |
| 请求 | `user`（必填对象：`username`、`nickname`、`email`、`avatar_url`、`role`、`rank`、`permissions` 或 `permissions_mask`、`quota_bytes`、`remark`、`status`）、`password`（必填，RSA 密文）、`must_change_password` |
| 回复 | `User`（新建账号，含 `id`、`role`、`rank`、`permissionsMask`、`status`、`quotaBytes` 与按调用方计算的标志位） |
| 错误 | `NETDISK_INVALID_ARGUMENT`（`user` 为空、`username` 不匹配 `^[A-Za-z0-9][A-Za-z0-9_.-]{1,31}$`、角色不存在、密码为空或过短、密文无法解密）、`NETDISK_PERMISSION_DENIED`（缺少权限、`rank` ≥ 自己的等级、权限集合超出自己的权限、非超级管理员创建 `ROLE_ADMIN`）、`NETDISK_ALREADY_EXISTS`（用户名重复） |

未显式给出 `role` 时使用 `storage.default_role_preset`（解析失败则用访客）；未显式给出 `rank` 时使用角色预设等级；未显式给出权限集合时使用角色预设权限。`status` 省略或非 `USER_STATUS_DISABLED` 时账号可登录（`ACTIVE`）。

### `GetUser`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/users/{id}` |
| 鉴权 | 需要令牌；`id` 可以是字面量 `me` |
| 请求 | 路径参数 `id`（必填） |
| 回复 | `User` |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED`（既不是自己，又没有 `USER_MANAGE` 或无法管理对方） |

### `ListUsers`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/users/list` |
| 鉴权 | 需要令牌 + `PERMISSION_USER_MANAGE` |
| 请求 | `page_size`、`page_token`、`filter`、`order_by`、`include_deleted` |
| 回复 | `UserSet`：`users[]`、`next_page_token`、`total_size` |
| 错误 | `NETDISK_PERMISSION_DENIED`、`NETDISK_INVALID_ARGUMENT`（过滤、排序或分页令牌非法） |

### `UpdateUser`

| 项 | 内容 |
| --- | --- |
| HTTP | `PUT /v1/users/update` |
| 鉴权 | 需要令牌 + `PERMISSION_USER_MANAGE` |
| 请求 | `user`（必填，`user.id` 指定目标）、`update_mask`（必填）。支持的路径：`nickname`、`email`、`avatar_url`、`remark`、`role`、`rank`、`permissions_mask`、`status`、`quota_bytes` |
| 回复 | `User` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（未给出 mask 或路径不受支持、`status` 为未指定、角色不存在）、`NETDISK_PERMISSION_DENIED`（无法管理对方、`rank` ≥ 自己的等级、权限集合超出自己的权限、非超级管理员改为 `ROLE_ADMIN`）、`NETDISK_NOT_FOUND` |

被 mask 省略的字段保持原值。把 `status` 改为非 `ACTIVE`、或修改密码后，目标账号的全部会话会被吊销。`status` 不允许改成 `USER_STATUS_DELETED`（请用 `DeleteUser`）。

### `DeleteUser`

| 项 | 内容 |
| --- | --- |
| HTTP | `DELETE /v1/users/{id}` |
| 鉴权 | 需要令牌 + `PERMISSION_USER_MANAGE` |
| 请求 | 路径参数 `id`（必填）、查询参数 `trash_nodes`（可选，是否把该账号的节点移入回收站） |
| 回复 | 空对象 |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_PERMISSION_DENIED`（缺少权限、目标是自己、无法管理对方）、`NETDISK_NOT_FOUND` |

软删除：账号置为 `USER_STATUS_DELETED` 并从列表中隐藏（除非 `include_deleted`），会话全部吊销；账号的行保留，因此审计日志里的操作者仍可追溯，其名下节点会一直留在原地，除非把 `trash_nodes` 设为 `true`——那样会把该账号根级子树整体移入回收站（随后的清理走 `PurgeNodes` / `purge_trash`）。

### `SetUserPermissions`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/users/permissions/set` |
| 鉴权 | 需要令牌 + `PERMISSION_USER_MANAGE` |
| 请求 | `id`（必填）、`permissions[]` 或 `permissions_mask`（整体替换；两者都为空表示清空权限） |
| 回复 | `User` |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_PERMISSION_DENIED`（无法管理对方或权限集合超出自己的权限）、`NETDISK_NOT_FOUND` |

### `ResetUserPassword`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/users/password/reset` |
| 鉴权 | 需要令牌 + `PERMISSION_USER_MANAGE` |
| 请求 | `id`（必填）、`password`（必填，RSA 密文）、`must_change_password` |
| 回复 | 空对象 |
| 错误 | `NETDISK_INVALID_ARGUMENT`（密码为空、过短或密文无法解密）、`NETDISK_PERMISSION_DENIED`、`NETDISK_NOT_FOUND` |

目标账号的全部会话会被吊销。

### `ListRolePresets`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/users/roles/list` |
| 鉴权 | 需要令牌（任何已认证账号） |
| 请求 | 无 |
| 回复 | `RolePresetSet`：`presets[]`（`role`、`name`、`displayName`、`description`、`defaultRank`、`permissions`、`permissionsMask`）、`max_grantable_rank`（= 调用方 `rank - 1`）、`caller_rank` |
| 错误 | `NETDISK_UNAUTHENTICATED` |

### `ListPermissionCatalog`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/users/permissions/catalog` |
| 鉴权 | 需要令牌 |
| 请求 | 无 |
| 回复 | `PermissionCatalog`：`permissions[]`（每项含 `permission`、`name`、`displayName`、`description`、`category`）、`granted`、`granted_mask` |
| 错误 | `NETDISK_UNAUTHENTICATED` |

### `GetUserStats`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/users/stats/{id}` |
| 鉴权 | 需要令牌；读取他人统计需要 `PERMISSION_USER_MANAGE` |
| 请求 | 路径参数 `id`（必填，`me` 表示自己） |
| 回复 | `UserStats`：`user_id`、`username`、`quota_bytes`、`used_bytes`、`file_count`、`folder_count`、`trashed_bytes`、`trashed_count`、`usage_ratio` |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_PERMISSION_DENIED`、`NETDISK_NOT_FOUND`、`NETDISK_UNAUTHENTICATED` |

## NodeService

文件夹与文件都是节点，因此描述、可见范围、ACL 与密码对所有节点一致。判定一个节点上的权限时遵循固定顺序（账号权限上限 → 属主通行 → 显式拒绝 → 显式允许 → 可见范围 → 文件夹密码），逐条解释见 [`permissions.md`](permissions.md)。

### `ListNodes`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/list` |
| 鉴权 | 需要令牌；在目标文件夹上需 `view`（`parent_id` 为空时列自己的根目录） |
| 请求 | `parent_id`（空 = 根目录）、`page_size`、`page_token`、`filter`、`order_by`、`recursive`、`max_depth`、`include_trashed` |
| 回复 | `NodeSet`：`nodes[]`、`next_page_token`、`total_size`、`total_bytes`（本页文件字节数） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`（id/过滤/排序/分页非法）、`NETDISK_NOT_FOUND`（父节点不存在）、`NETDISK_NODE_LOCKED`（父文件夹未解锁）、`NETDISK_PERMISSION_DENIED`（无 `view`） |

不可见的子节点会被静默丢弃；`include_trashed` 需要 `PERMISSION_TRASH_MANAGE` 才生效；`recursive` 使用物化路径做一次前缀扫描，`max_depth` 限制相对深度。

### `SearchNodes`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/search` |
| 鉴权 | 需要令牌 |
| 请求 | `query`（子串，大小写不敏感，空串匹配全部）、`scope_node_id`、`kind`、`mime_type`（前缀）、`owner_id`、`min_size`、`max_size`、`updated_after`、`updated_before`、`match_description`、`page_size`、`page_token`、`order_by` |
| 回复 | `NodeSet` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`（范围节点不存在）、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED` |

`match_description` 为 true 时匹配描述而不是名称。结果仍然逐个做访问判定，锁定或无 `view` 的节点不会出现。

### `GetNodeTree`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/tree` |
| 鉴权 | 需要令牌 |
| 请求 | `root_id`（空 = 自己的根目录）、`depth`（缺省 1，上限 `storage.max_path_depth`）、`page_size`（缺省级 200）、`filter`（与 `ListNodes` 同语法，逐层生效） |
| 回复 | `NodeTree`：`node` + 递归的 `children[]`；`root_id` 为空时返回一个虚拟根节点（`id` 为空、名称为 `storage.root_folder_name`） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED` |

### `ListTrash`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/trash/list` |
| 鉴权 | 需要令牌；无 `PERMISSION_TRASH_MANAGE` 时只看到自己拥有的回收站条目 |
| 请求 | `page_size`、`page_token`、`filter`（同 `ListNodes`，另支持 `trashed_at` 区间）、`order_by`、`original_parent_id` |
| 回复 | `NodeSet` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT` |

**一次只列一层，层级关系保留**：不带 `original_parent_id` 时返回的是每棵被删子树的**顶端条目**（谁被删就显示谁），被删文件夹里的内容不会跟它平铺在同一级；带上某个回收站文件夹的 id 就列它里面的条目，客户端据此逐层走进回收站。判定方式是 `parent_id == original_parent_id`——整棵被删子树共享同一个 `original_parent_id`（删除时所在目录），只有顶端那一项还满足这个等式。

还原时若只还原里面的某个子项，目标目录取它自己的 `original_parent_id`（即当初那个文件夹被删时所在的目录），所以单独还原出来的条目落在正常目录里，而不会留在一个仍在回收站里的文件夹内。

### `GetNode`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/{id}` |
| 鉴权 | 需要令牌；节点上至少要有 `view` |
| 请求 | 路径参数 `id`（必填） |
| 回复 | `Node`（含 `displayPath`、`effectivePermissions`、`locked`、`owned`、`shared`、`shareCount`、`versionCount` 等计算字段） |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED`（在该节点上无任何权限） |

### `GetNodePath`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/{id}/path` |
| 鉴权 | 需要令牌 |
| 请求 | 路径参数 `id`（必填） |
| 回复 | `NodePath`：`ancestors[]`（从根到父）、`node`、`display_path` |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED` |

### `GetNodeStats`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/{id}/stats` |
| 鉴权 | 需要令牌；`id` 为空时统计调用方自己的整棵树 |
| 请求 | 路径参数 `id`（可为空） |
| 回复 | `NodeStats`：`node_id`、`file_count`、`folder_count`、`total_size`、`trashed_count`、`trashed_size`、`largest_file_size`、`largest_file_name`、`size_by_category` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED` |

### `GetNodeAcl`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/{id}/acl` |
| 鉴权 | 需要令牌；节点上至少要有 `view` |
| 请求 | 路径参数 `id`（必填） |
| 回复 | `NodeAcl`：`entries[]`（直接挂在节点上）、`inherited_entries[]`（来自祖先的拒绝条目与带 `inherit` 的允许条目）、`effective_permissions`、`effective_permissions_mask`、`owned`、`editable`（是否持有 `acl_manage`） |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED` |

### `CreateFolder`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/folders/create` |
| 鉴权 | 需要令牌 + `PERMISSION_UPLOAD`；在目标父目录上需 `upload` |
| 请求 | `folder`（必填；只读 `name`、`parent_id`、`description`、`visibility`、`password_hint`、`metadata`）、`conflict_policy`（缺省 `FAIL`）、`password`（可选，RSA 密文，用于设置文件夹密码）、`acl[]`（与创建同事务写入） |
| 回复 | `Node` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED`（缺少上传权限、父目录上无 `upload`、ACL 条目超出自身权限）、`NETDISK_INVALID_ARGUMENT`（名称为空/含 `/`、`\`、NUL/超长；超过 `storage.max_path_depth`；父节点不是文件夹或已在回收站；ACL 主体类型或效果非法）、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`（父文件夹未解锁）、`NETDISK_NAME_CONFLICT`（`FAIL` 策略下同名已存在） |

### `UpdateNode`

| 项 | 内容 |
| --- | --- |
| HTTP | `PUT /v1/nodes/update` |
| 鉴权 | 需要令牌；改内容字段（`name`/`description`/`metadata`/`parent_id`）需 `edit`，改 ACL 字段（`visibility`/`password`）需 `acl_manage` |
| 请求 | `node`（必填，`node.id` 指定目标）、`update_mask`（必填）。支持的路径：`name`、`description`、`visibility`、`metadata`、`password`、`parent_id`；另有 `password`（RSA 密文）与 `remove_password`（清除密码）、`conflict_policy` |
| 回复 | `Node` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（未给 mask、路径不支持、`password` 路径但没有给密码、名称非法、新父节点非法、超深度）、`NETDISK_PERMISSION_DENIED`（缺少 `edit` 或 `acl_manage`）、`NETDISK_NODE_LOCKED`、`NETDISK_NAME_CONFLICT`、`NETDISK_CYCLE_DETECTED`（移动到自身子树）、`NETDISK_NOT_FOUND` |

`parent_id` 路径用于移动；重命名与移动合成的名字冲突由 `conflict_policy` 决定（`OVERWRITE` 在重命名路径上被当作失败处理，避免静默覆盖兄弟节点）。

### `SetNodeAcl`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/{node_id}/acl` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_ACL_MANAGE` |
| 请求 | 路径参数 `node_id`（必填）、`entries[]`（**整体替换**，空数组表示清空 ACL）、`recursive`（可选，把同一组条目写进所有未覆盖的后代） |
| 回复 | `NodeAcl`（替换后的条目与调用方的有效权限） |
| 错误 | `NETDISK_INVALID_ARGUMENT`（主体类型未知、`SUBJECT_TYPE_USER`/`ROLE` 的 `subject_id` 为空、效果不是 allow/deny、权限超出节点级范围 `view`/`download`/`upload`/`edit`/`delete`/`trash_manage`/`share`/`acl_manage`）、`NETDISK_PERMISSION_DENIED`（无 `acl_manage`，或条目授予/拒绝了调用方自己不持有的权限）、`NETDISK_NOT_FOUND` |

条目字段：`subject_type`（`SUBJECT_TYPE_USER` / `SUBJECT_TYPE_ROLE` / `SUBJECT_TYPE_EVERYONE`）、`subject_id`（账号 id / 角色机器名 `admin`/`manager`/`user`/`guest` / 留空）、`effect`（`EFFECT_ALLOW` / `EFFECT_DENY`）、`permissions` 或 `permissions_mask`、`inherit`（允许条目是否向下继承；**拒绝条目无论 `inherit` 如何都会作用于整棵子树**）。服务端返回时会填充 `subject_name`。

### `UnlockNode`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/{node_id}/unlock` |
| 鉴权 | 需要令牌 |
| 请求 | 路径参数 `node_id`（必填）、`password`（必填，RSA 密文） |
| 回复 | `NodeUnlock`：`node_id`（实际被解锁的受保护祖先节点）、`unlock_token`、`expires_in`、`expires_at` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（id 非法、密文无法解密）、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`（未提供密码）、`NETDISK_PERMISSION_DENIED`（密码错误） |

密码校验针对**最外层**受保护的祖先：请求一个受保护文件夹内部的节点，返回的 `node_id` 是最外层受保护目录。令牌默认 30 分钟（`auth.node_token_ttl`），覆盖该节点及其整棵子树；后续请求把它放在 `X-Node-Token` 里即可。

### `MoveNodes`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/move` |
| 鉴权 | 需要令牌；每个节点上需 `edit`，目标目录上需 `upload` |
| 请求 | `ids[]`（必填）、`target_parent_id`（空 = 自己的根目录）、`conflict_policy`（缺省 `FAIL`）、`new_names[]`（可选，长度必须与 `ids` 一致，可顺带改名） |
| 回复 | `NodeSet`（移动后的节点） |
| 错误 | `NETDISK_INVALID_ARGUMENT`（`ids` 为空、`new_names` 长度不符、名称非法、超深度、目标不是文件夹或已在回收站）、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_CYCLE_DETECTED`（把文件夹移到自身或后代里）、`NETDISK_NAME_CONFLICT` |

批操作是原子的：任何一个节点被拒绝，整批回滚。

### `CopyNodes`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/copy` |
| 鉴权 | 需要令牌；源节点上需 `view` + `download`，目标目录上需 `upload` |
| 请求 | `ids[]`（必填）、`target_parent_id`、`conflict_policy`、`new_names[]` |
| 回复 | `NodeSet` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED`（缺少 `download` 或目标不可写）、`NETDISK_NAME_CONFLICT`、`NETDISK_QUOTA_EXCEEDED`、`NETDISK_UNSUPPORTED`（源节点没有内容对象）、`NETDISK_STORAGE_ERROR` / `NETDISK_UNAVAILABLE` |

文件夹会连同整棵子树复制；副本归属调用方，对象逐个流式复制到 `files/<新属主>/<新节点 id>`。

### `DeleteNodes`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/delete` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_DELETE`；`permanent: true` 还需 `PERMISSION_TRASH_MANAGE` |
| 请求 | `ids[]`（必填）、`permanent`（可选，跳过回收站直接彻底删除） |
| 回复 | `DeleteNodesReply`：`deleted_ids[]`、`affected_count`（含后代）、`reclaimed_bytes`（仅彻底删除时非零） |
| 错误 | `NETDISK_INVALID_ARGUMENT`（`ids` 为空，或 `permanent: true` 时节点不在回收站）、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED`（无 `delete`，或彻底删除时无 `trash_manage`） |

默认只把节点（含子树）移入回收站，行与对象都保留；回收站把整棵子树显示为被删的那一项（层级见 `ListTrash`），彻底删除的行为与限制见 `PurgeNodes`。

### `RestoreNodes`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/trash/restore` |
| 鉴权 | 需要令牌；属主可直接还原，否则需 `PERMISSION_TRASH_MANAGE` |
| 请求 | `ids[]`（必填）、`target_parent_id`（空 = 还原到 `original_parent_id`）、`conflict_policy` |
| 回复 | `NodeSet` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（节点不在回收站、名称非法、超深度）、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED`、`NETDISK_NAME_CONFLICT` |

原父目录已不存在时自动回退到根目录。

### `PurgeNodes`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/trash/purge` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_TRASH_MANAGE` |
| 请求 | `ids[]`（必填） |
| 回复 | `PurgeNodesReply`：`purged_ids[]`、`affected_count`、`reclaimed_bytes` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（节点不在回收站）、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED` |

整棵子树的行、ACL 与用量计数在一个事务内删除，`reclaimed_bytes` 是子树中文件字节总和。被删文件的当前内容键与全部历史版本键会在同一事务里写入待删除队列（`node_purge` / `node_purge_version`），由维护任务 `purge_deletions` 在提交后真正释放对象，因此对象存储的占用会在下一轮维护任务后下降。详见 [`architecture.md`](architecture.md#44-待删除队列与维护任务)。

### `EmptyTrash`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/trash/empty` |
| 鉴权 | 需要令牌 + `PERMISSION_TRASH_MANAGE`（否则只清空自己拥有的条目） |
| 请求 | `trashed_before`（可选，只清理早于该时刻入回收站的条目） |
| 回复 | `PurgeNodesReply` |
| 错误 | `NETDISK_PERMISSION_DENIED`、`NETDISK_INVALID_ARGUMENT` |

回收站为空时返回全零结果而不是错误。

### `ListNodeVersions`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/nodes/{node_id}/versions/list` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_VIEW` |
| 请求 | 路径参数 `node_id`（必填）、`page_size`、`page_token` |
| 回复 | `NodeVersionSet`：`versions[]`（`id`、`node_id`、`version`、`size`、`mime_type`、`etag`、`comment`、`current`、`created_by`、`created_at`）、`next_page_token` |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED` |

`current` 表示该版本内容就是节点当前内容。版本在覆盖写时产生（受 `upload.keep_versions` 与 `upload.max_versions` 控制）。

### `RestoreNodeVersion`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/nodes/versions/restore` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_EDIT` |
| 请求 | `node_id`（必填）、`version_id`（必填） |
| 回复 | `Node` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（版本不属于该节点）、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED`、`NETDISK_UNSUPPORTED`（节点不是文件）、`NETDISK_NODE_LOCKED` |

还原会把当前内容先存成一个新版本（`comment = "before restore"`），再把节点指向目标版本的内容与大小，并修正用量。

### `DeleteNodeVersion`

| 项 | 内容 |
| --- | --- |
| HTTP | `DELETE /v1/nodes/{node_id}/versions/{version_id}` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_DELETE` |
| 请求 | 路径参数 `node_id`、`version_id`（均必填） |
| 回复 | 空对象 |
| 错误 | `NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED`、`NETDISK_NODE_LOCKED` |

版本行与版本计数在事务内删除，对象键在提交后进入待删除队列。

## FileService

### `InitiateUpload`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/files/uploads/create` |
| 鉴权 | 需要令牌 + `PERMISSION_UPLOAD`；目标目录上需 `upload` |
| 请求 | `name`（必填）、`size`（必填，总字节数）、`parent_id`、`mime_type`（空则按扩展名推断）、`chunk_size`、`mode`、`conflict_policy`（缺省 `RENAME`）、`etag`（可选，整文件 SHA-256）、`description`、`metadata`、`resumable_upload_id` |
| 回复 | `UploadSession`：`id`、`parent_id`、`name`、`kind`、`size`、`mime_type`、`chunk_size`、`total_parts`、`uploaded_parts[]`、`received_bytes`、`status`、`mode`、`conflict_policy`、`parts[]`、`expires_at` 等 |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED`（缺少上传权限或目标目录不可写）、`NETDISK_INVALID_ARGUMENT`（名称为空/含 `/`、`\`/超长、`size` 为负、id 非法）、`NETDISK_TOO_LARGE`（超过 `upload.max_file_size` 或分片数超过 `upload.max_parts`）、`NETDISK_QUOTA_EXCEEDED`、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED` |

`chunk_size` 会被夹到 `[upload.min_chunk_size, upload.max_chunk_size]` 并对齐到 256 KiB；`size` 为 0 时按最小分片规划成 1 片。`mode` 缺省取 `upload.default_mode`。

同名会话复用规则：同一属主 + 同一父目录 + 同名（大小写按 `storage.case_insensitive_names`）且仍处于 `PENDING`/`IN_PROGRESS` 且未过期的会话会被**直接返回**，因此重试 `InitiateUpload` 是幂等的，不会产生第二个暂存前缀。

### `ListUploads`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/files/uploads/list` |
| 鉴权 | 需要令牌 |
| 请求 | `page_size`、`page_token`、`status`（按状态过滤）、`parent_id`（按目标目录过滤） |
| 回复 | `UploadSessionSet`：`uploads[]`、`next_page_token`、`total_size` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT` |

只返回调用方自己的会话；`parent_id` 过滤在服务层完成。

### `CompleteUpload`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/files/uploads/complete` |
| 鉴权 | 需要令牌（会话属主，或持有 `PERMISSION_STORAGE_MANAGE`） |
| 请求 | `upload_id`（必填）、`etag`（可选，`sha256:<hex>` 形式的最终摘要）、`comment`（可选，写入新版本的备注） |
| 回复 | `Node`（新建或被覆盖的文件节点） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_INVALID_ARGUMENT`（会话已完成、id 非法、目标目录非法、超深度）、`NETDISK_ABORTED`（会话已取消）、`NETDISK_UPLOAD_EXPIRED`、`NETDISK_UPLOAD_INCOMPLETE`、`NETDISK_PRECONDITION_FAILED`（摘要不符）、`NETDISK_NAME_CONFLICT`（`FAIL` 策略同名，或同名目标是文件夹）、`NETDISK_QUOTA_EXCEEDED`、`NETDISK_NOT_FOUND` |

校验规则与提交顺序见下文「CompleteUpload 校验规则」。

### `AbortUpload`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/files/uploads/abort` |
| 鉴权 | 需要令牌（会话属主，或 `PERMISSION_STORAGE_MANAGE`） |
| 请求 | `upload_id`（必填） |
| 回复 | 空对象 |
| 错误 | `NETDISK_INVALID_ARGUMENT`（会话已完成或 id 非法）、`NETDISK_PERMISSION_DENIED`、`NETDISK_NOT_FOUND` |

会话置为 `ABORTED`、分片行删除，随后删除 `uploads/<uploadId>` 前缀（失败则写入待删除队列）。

### `ConfirmUploadPart`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/files/uploads/parts/confirm` |
| 鉴权 | 需要令牌（会话属主，或 `PERMISSION_STORAGE_MANAGE`） |
| 请求 | `upload_id`（必填）、`part_number`（必填，从 1 开始）、`etag`（必填，对象存储返回的 ETag）、`size`（可选） |
| 回复 | `UploadPart`：`part_number`、`size`、`etag`、`created_at` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_INVALID_ARGUMENT`（片号越界、会话已完成）、`NETDISK_ABORTED`、`NETDISK_UPLOAD_EXPIRED`、`NETDISK_UPLOAD_INCOMPLETE`（对象不存在或为空）、`NETDISK_PRECONDITION_FAILED`（ETag 不符）、`NETDISK_NOT_FOUND` |

服务端会实际 `StatObject` 检查该分片，客户端无法声明一个从未上传的分片。`size` 由服务端从对象读取，请求里的 `size` 不参与判定。

### `UploadSmallFile`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/files/upload` |
| 鉴权 | 需要令牌 + `PERMISSION_UPLOAD` |
| 请求 | `name`（必填）、`content`（必填，base64）、`parent_id`、`mime_type`、`conflict_policy`、`description`、`metadata` |
| 回复 | `Node` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_TOO_LARGE`（超过 `upload.max_inline_size`）、`NETDISK_INVALID_ARGUMENT`（名称非法）、`NETDISK_QUOTA_EXCEEDED`、`NETDISK_NAME_CONFLICT`、`NETDISK_NODE_LOCKED` |

内容随请求体一起传输，不产生上传会话。注意请求体整体受 HTTP 服务器读限制，仅适合小文件。

### `GetUpload`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/files/uploads/{id}` |
| 鉴权 | 需要令牌（会话属主，或 `PERMISSION_STORAGE_MANAGE`） |
| 请求 | 路径参数 `id`（必填） |
| 回复 | `UploadSession`（含 `parts[]`；预签名模式下未完成的分片会带上**新签发的** `upload_url`） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND` |

### `ListUploadParts`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/files/uploads/{upload_id}/parts` |
| 鉴权 | 需要令牌（会话属主，或 `PERMISSION_STORAGE_MANAGE`） |
| 请求 | 路径参数 `upload_id`（必填）、查询参数 `from_part_number`（可选，只返回编号大于它的分片） |
| 回复 | `UploadPartSet`：`upload_id`、`parts[]` |
| 错误 | 同 `GetUpload` |

### `UploadChunk`

| 项 | 内容 |
| --- | --- |
| HTTP | `PUT /v1/files/uploads/{upload_id}/parts/{part_number}` |
| 鉴权 | 需要令牌（会话属主，或 `PERMISSION_STORAGE_MANAGE`） |
| 请求 | 路径参数 `upload_id`、`part_number`（必填，从 1 开始）、请求体 `content`（base64）与可选 `etag` |
| 回复 | `UploadPart` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_INVALID_ARGUMENT`（片号越界或小于 1、会话已完成）、`NETDISK_ABORTED`、`NETDISK_UPLOAD_EXPIRED`、`NETDISK_TOO_LARGE`（分片大于会话的分片大小）、`NETDISK_PRECONDITION_FAILED`（客户端 ETag 与存储返回的不符）、`NETDISK_NOT_FOUND` |

与之等价的二进制绑定是 `PUT /v1/files/uploads/{upload_id}/parts/{part_number}/raw`：请求体是原始字节，必须带 `Content-Length`，响应是同样的 `UploadPart`（仅 `part_number` 与 `size`）。浏览器应优先使用它，避免 base64 带来的 33% 膨胀。

### `GetDownloadUrl`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/files/{node_id}/download-url` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_DOWNLOAD` |
| 请求 | 路径参数 `node_id`（必填）、`expires_in_seconds`、`inline`、`file_name` |
| 回复 | `SignedUrl`：`url`、`method`（`GET`）、`expires_at`、`expires_in`、`node_id`、`size`、`file_name`、`mime_type`、`headers` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_UNSUPPORTED`（节点是文件夹，或没有内容对象）、`NETDISK_UNAVAILABLE`（未配置对象存储） |

返回的是对象存储的预签名地址：字节不经过 Nagisa 进程。`expires_in_seconds` 缺省为 `data.object_storage.presign_ttl`，上限为它的 4 倍。因此**浏览器必须能直连对象存储地址**，否则该 URL 不可用（`endpoint` 与 `public_endpoint` 的分工见 [`deployment.md`](deployment.md#15-endpoint-与-public_endpoint)）。

### `GetArchiveUrl`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/files/{node_id}/archive-url` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_DOWNLOAD` |
| 请求 | 路径参数 `node_id`（必填）、`expires_in_seconds`、`archive_name`（不含扩展名） |
| 回复 | `SignedUrl`：`url` 指向 `GET /v1/files/{node_id}/archive?...`，`file_name` 为 `<archive_name>.zip`，`mime_type` 为 `application/zip`，`size` 为 0（大小未知） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_UNSUPPORTED` |

文件与文件夹都支持；文件夹的 zip 由服务端即时生成，不落地临时对象。

### `GetPreviewUrl`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/files/{node_id}/preview-url` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_DOWNLOAD` |
| 请求 | 路径参数 `node_id`（必填）、`expires_in_seconds`、`thumbnail` |
| 回复 | `SignedUrl`（`headers`、`expires_at` 等），`Content-Disposition` 为 `inline` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_NOT_FOUND`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED`、`NETDISK_UNSUPPORTED` |

可预览的 MIME 前缀：`image/`、`video/`、`audio/`、`text/`，以及 `application/pdf`。请求里的 `thumbnail` 字段当前不参与实现（返回的是完整内容的 inline 地址）。

## ShareService

分享链接把节点发布给没有账号的访问者。链接自带能力集合（只能是 `view`/`download`/`upload`）、可选密码、过期时间与下载额度；三条匿名接口用**分享令牌**而不是账号令牌鉴权。

### `CreateShare`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/shares/create` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_SHARE`；要授予下载能力还需 `PERMISSION_DOWNLOAD`，授予上传能力需节点是文件夹且持有 `PERMISSION_UPLOAD` |
| 请求 | `node_id`（必填）、`name`、`description`、`permissions[]` 或 `permissions_mask`（缺省 `view`+`download`）、`password`（RSA 密文）、`password_hint`、`expires_at`（零值 = 永不过期）、`max_downloads`（0 = 不限）、`token`（可选，自定义令牌） |
| 回复 | `Share`：`id`、`token`、`node_id`、`owner_id`、`name`、`description`、`permissions`、`permissions_mask`、`password_protected`、`password_hint`、`expires_at`、`max_downloads`、`download_count`、`view_count`、`status`、`url`、`editable`、`created_at` 等 |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_UNSUPPORTED`（`storage.allow_public_share` 为 false）、`NETDISK_INVALID_ARGUMENT`（`node_id` 缺失、自定义令牌非法：长度 8–64 且只含 `[A-Za-z0-9_-]`）、`NETDISK_PERMISSION_DENIED`（节点上无 `share`、能力超出自身权限或超出节点类型限制）、`NETDISK_NOT_FOUND`、`NETDISK_ALREADY_EXISTS`（令牌已被占用） |

`url` 由 `web.public_base_url` + `web.share_path_prefix`（默认 `/s`）+ 令牌拼成。传入的能力会被裁剪到 `view`/`download`/`upload` 范围内，空集合回落到 `view|download`。

### `ListShares`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/shares/list` |
| 鉴权 | 需要令牌；无 `PERMISSION_USER_MANAGE` 时只看到自己的分享 |
| 请求 | `page_size`、`page_token`、`filter`、`order_by` |
| 回复 | `ShareSet`：`shares[]`、`next_page_token`、`total_size` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT` |

### `ListSharesByNode`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/shares/by-node/list` |
| 鉴权 | 需要令牌 + 节点上的 `PERMISSION_SHARE` |
| 请求 | `node_id`（必填）、`page_size`、`page_token` |
| 回复 | `ShareSet`（每条 `share` 内附带节点快照 `node`） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED` |

### `AccessShare`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/shares/access` |
| 鉴权 | **公开**（用分享令牌鉴权） |
| 请求 | `token`（必填）、`password`（可选，RSA 密文）、`access_token`（可选，之前拿到的访问令牌）、`node_id`（可选，解析分享内部某个文件夹）、`page_size`、`page_token` |
| 回复 | `ShareAccess`：`share`、`node`、`children[]`（共享节点为文件夹时的第一层）、`next_page_token`、`access_token`（仅带密码的分享在**首次用密码打开**时返回）、`expires_in`、`expires_at` |
| 错误 | `NETDISK_NOT_FOUND`（令牌为空、未知、已撤销或已过期）、`NETDISK_RESOURCE_EXHAUSTED`（下载额度已用尽）、`NETDISK_UNSUPPORTED`（`allow_public_share` 为 false）、`NETDISK_NODE_LOCKED`（需要密码但未提供，或提供的 `access_token` 无效）、`NETDISK_PERMISSION_DENIED`（密码错误）、`NETDISK_INVALID_ARGUMENT`（密文无法解密、`node_id` 非法） |

`node` 与 `children[]` 上的 `effectivePermissions` 就是这条链接传达的能力；受密码保护且尚未解锁时只保留 `view`。

### `ListShareChildren`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/shares/children/list` |
| 鉴权 | **公开** |
| 请求 | `token`（必填）、`node_id`（可选，要浏览的文件夹）、`access_token`（受密码保护时必须）、`page_size`、`page_token`、`order_by` |
| 回复 | `NodeSet`：`nodes[]`、`next_page_token` |
| 错误 | `NETDISK_NOT_FOUND`、`NETDISK_RESOURCE_EXHAUSTED`、`NETDISK_NODE_LOCKED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_UNSUPPORTED` |

该消息没有密码字段，所以受保护的分享只能先由 `AccessShare` 换取 `access_token`。`order_by` 被解析但不参与查询。

### `GetShareDownloadUrl`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/shares/download-url` |
| 鉴权 | **公开**（分享能力集合需含 `download`） |
| 请求 | `token`（必填）、`node_id`（可选）、`access_token`（受保护时必须）、`expires_in_seconds`、`inline` |
| 回复 | `SignedUrl`（指向对象存储的预签名地址） |
| 错误 | `NETDISK_NOT_FOUND`、`NETDISK_RESOURCE_EXHAUSTED`、`NETDISK_NODE_LOCKED`、`NETDISK_PERMISSION_DENIED`（链接没有下载能力）、`NETDISK_INVALID_ARGUMENT`、`NETDISK_UNSUPPORTED`（目标不是文件，或没有内容对象） |

成功调用会把 `download_count` 加一；`max_downloads` 用尽后本接口返回 429 `NETDISK_RESOURCE_EXHAUSTED`。**当前实现只支持文件**：对文件夹调用会返回 `NETDISK_UNSUPPORTED`（`Share` 的文档注释提到「文件夹解析为打包地址」，代码尚未实现）。

### `GetShare`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/shares/{id}` |
| 鉴权 | 需要令牌；分享属主或 `PERMISSION_USER_MANAGE` |
| 请求 | 路径参数 `id`（必填） |
| 回复 | `Share` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED` |

### `UpdateShare`

| 项 | 内容 |
| --- | --- |
| HTTP | `PUT /v1/shares/update` |
| 鉴权 | 需要令牌；分享属主或 `PERMISSION_USER_MANAGE` |
| 请求 | `share`（必填，`share.id` 指定目标）、`update_mask`（必填）。支持的路径：`name`、`description`、`permissions`、`permissions_mask`、`password`、`password_hint`、`expires_at`、`max_downloads`、`status`；另有 `password`（RSA 密文）与 `remove_password` |
| 回复 | `Share` |
| 错误 | `NETDISK_INVALID_ARGUMENT`（未给 mask、路径不支持、`password` 路径没给密码、能力字段非法、`expires_at` 为零值表示清除过期时间）、`NETDISK_PERMISSION_DENIED`（非属主、能力超出自身权限）、`NETDISK_NOT_FOUND` |

### `DeleteShare`

| 项 | 内容 |
| --- | --- |
| HTTP | `DELETE /v1/shares/{id}` |
| 鉴权 | 需要令牌；分享属主或 `PERMISSION_USER_MANAGE` |
| 请求 | 路径参数 `id`（必填） |
| 回复 | 空对象 |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED` |

删除后分享行即消失，原令牌立刻失效（再次访问会得到 `NETDISK_NOT_FOUND`）。

## AuditService

审计日志记录每一次状态变更与失败调用（读取类调用不记录）。标记为取消/失败的调用同样入库，并带上 `error_reason`。没有 `PERMISSION_AUDIT_READ` 的调用方只能看到自己作为 `actor` 的记录。

记录的动作名（`action`，与 `actionDisplay` 的中文标签）：`auth.login`、`auth.logout`、`auth.refresh`、`auth.password_change`、`user.create`、`user.update`、`user.delete`、`user.permissions_set`、`user.password_reset`、`node.create_folder`、`node.update`、`node.move`、`node.copy`、`node.delete`、`node.restore`、`node.purge`、`node.trash_empty`、`node.acl_set`、`node.unlock`、`node.version_restore`、`node.version_delete`、`file.upload_initiate`、`file.upload_complete`、`file.upload_abort`、`file.upload_part`、`file.upload_inline`、`file.download`、`file.archive`、`share.create`、`share.update`、`share.delete`、`share.access`、`share.download`、`system.maintenance`、`system.settings_update`。

### `ListAuditLogs`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/audit/logs/list` |
| 鉴权 | 需要令牌；无 `PERMISSION_AUDIT_READ` 时只在返回集里保留自己的记录（`total_size` 一并收窄） |
| 请求 | `page_size`、`page_token`、`filter`、`order_by` |
| 回复 | `AuditLogSet`：`logs[]`（`id`、`actor_id`、`actor_name`、`action`、`action_display`、`target_type`、`target_id`、`target_name`、`success`、`error_reason`、`detail`、`ip`、`user_agent`、`request_id`、`created_at`）、`next_page_token`、`total_size` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT` |

### `GetAuditLog`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/audit/logs/{id}` |
| 鉴权 | 需要令牌；无 `PERMISSION_AUDIT_READ` 时只能读自己的记录 |
| 请求 | 路径参数 `id`（必填） |
| 回复 | `AuditLog` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_INVALID_ARGUMENT`、`NETDISK_NOT_FOUND`、`NETDISK_PERMISSION_DENIED` |

### `GetAuditSummary`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/audit/summary` |
| 鉴权 | 需要令牌 + `PERMISSION_AUDIT_READ` |
| 请求 | `from`（缺省为 `to - 7 天`）、`to`（缺省为当前时间）、`action_prefix`（按动作名前缀过滤） |
| 回复 | `AuditSummary`：`from`、`to`、`total`、`failure_count`、`by_action[]`（最多 50 项）、`by_actor[]`（最多 20 项）、`by_day`（ISO-8601 日期 → 次数） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED` |

## SystemService

### `GetSystemInfo`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/system/info` |
| 鉴权 | 公开 |
| 请求 | 无 |
| 回复 | `SystemInfo`：`name`、`version`、`api_version`、`features[]`、`max_upload_size`、`default_chunk_size`、`min_chunk_size`、`max_inline_size`、`upload_session_ttl_seconds`、`signed_url_ttl_seconds`、`signed_url_max_ttl_seconds`、`upload_modes[]`、`storage_backend`、`database_backend`、`default_visibility`、`registration_enabled`（本部署恒为 false，账号只能由管理员创建）、`auth`（同 `AuthConfig`）、`server_time`、`public_base_url` |
| 错误 | — |

### `HealthCheck`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/system/health` |
| 鉴权 | 公开 |
| 请求 | `deep`（可选；为 true 时真正探测数据库与对象存储） |
| 回复 | `HealthStatus`：`status`（`ok` / `degraded` / `down`）、`checks`（如 `{"database":"ok","object_storage":"ok"}`）、`uptime_seconds`、`server_time` |
| 错误 | — |

不传 `deep` 时只报告进程存活，并返回 `database=ok`、`object_storage=ok`。

### `GetStorageStats`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/system/storage/stats` |
| 鉴权 | 需要令牌 + `PERMISSION_STORAGE_MANAGE` |
| 请求 | `top_owners`（可选，缺省 10，上限 100） |
| 回复 | `StorageStats`：`total_bytes`、`total_files`、`total_folders`、`total_users`、`active_users`、`disabled_users`、`trashed_bytes`、`trashed_nodes`、`uploads_in_progress`、`active_shares`、`versions_bytes`、`size_by_category`、`top_owners[]`、`backend_used_bytes` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED` |

### `RunMaintenance`

| 项 | 内容 |
| --- | --- |
| HTTP | `POST /v1/system/maintenance/run` |
| 鉴权 | 需要令牌 + `PERMISSION_STORAGE_MANAGE` |
| 请求 | `dry_run`（可选，只报告不修改）、`tasks[]`（可选；支持 `expire_uploads`、`purge_deletions`、`gc_orphans`、`expire_shares`、`recount_usage`、`purge_trash`；**省略时默认执行 `expire_uploads`、`purge_deletions`、`expire_shares`、`recount_usage`**）、`trash_retention_days`（可选，缺省取 `storage.trash_retention` 的天数） |
| 回复 | `MaintenanceReport`：`dry_run`、`tasks[]`、`expired_uploads`、`deleted_objects`、`orphan_objects`、`expired_shares`、`recounted_users`、`purged_nodes`、`warnings[]`、`duration_ms`、`finished_at` |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED` |

未知任务名不会报错，而是写进 `warnings`（`unknown task: <name>`）；某个任务失败也会记入 `warnings` 并继续执行其余任务。

### `ListSystemSettings`

| 项 | 内容 |
| --- | --- |
| HTTP | `GET /v1/system/settings/list` |
| 鉴权 | 需要令牌 + `PERMISSION_STORAGE_MANAGE` |
| 请求 | 无 |
| 回复 | `SystemSettingSet`：`settings[]`（`key`、`value`、`type`、`description`、`writable`） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED` |

### `UpdateSystemSettings`

| 项 | 内容 |
| --- | --- |
| HTTP | `PUT /v1/system/settings/update` |
| 鉴权 | 需要令牌 + `PERMISSION_SYSTEM_MANAGE` |
| 请求 | `settings[]`（每项 `key` 与 `value`；`type` / `description` / `writable` 只用于回显） |
| 回复 | `SystemSettingSet`（写入后的完整列表） |
| 错误 | `NETDISK_UNAUTHENTICATED`、`NETDISK_PERMISSION_DENIED`（缺少 `SYSTEM_MANAGE`，或键为只读）、`NETDISK_INVALID_ARGUMENT`（键不存在，或值与声明的类型不符） |

可写键与类型见 [`deployment.md`](deployment.md#5-运行参数与维护任务)；`system.version` 是只读键，写入返回 403。注意这些运行参数目前只被读写与展示，没有任何逻辑回读它们来改变行为——实际生效的上限与策略来自 `configs/config.yaml`。

## 分片上传完整流程

两种传输方式的选择：客户端能直连对象存储就用 `UPLOAD_MODE_PRESIGNED`（字节不经过 Nagisa，吞吐最好）；不能直连就用 `UPLOAD_MODE_PROXY`（字节经服务端转发）。缺省值由 `upload.default_mode` 决定。

### 预签名模式（`UPLOAD_MODE_PRESIGNED`）

1. `POST /v1/files/uploads/create`，请求体至少包含 `name`、`size`，并显式声明 `mode: "UPLOAD_MODE_PRESIGNED"`（或依赖缺省值）。
   服务端在此刻完成：认证与上传权限检查、目标目录可写检查、名称合法性检查、`size` 与分片上限检查、**配额预检**，然后落库一个 `PENDING` 会话。
   回复中的 `chunk_size`、`total_parts` 是权威的分片规划；`parts[]` 为 `1..total_parts` 每一项都带新签发的 `upload_url`、`url_expires_at` 与 `headers`。
2. 对每个分片：`PUT <upload_url>`，请求体是这一片的原始字节。**不要**附加 `Content-Type`（预签名 PUT 的签名不含该头，附加会导致 S3 校验失败）；按需带上 `upload_url` 附带的 `headers`。从响应头取 `ETag`（带引号，可原样回传）。
3. 对每个分片：`POST /v1/files/uploads/parts/confirm`，请求体 `{"upload_id": "...", "part_number": N, "etag": "<ETag>"}`。
   服务端会用 `StatObject` 确认该对象确实存在且非空，因此这一步无法伪造；ETag 与服务端读到的不一致返回 400 `NETDISK_PRECONDITION_FAILED`。分片行写库后会话状态变为 `IN_PROGRESS`，`received_bytes` 与 `uploaded_parts` 同步更新。
4. 重复 2–3 直到所有分片确认完毕（可用 `uploaded_parts` 判断还缺哪几片）。
5. `POST /v1/files/uploads/complete`，请求体 `{"upload_id": "..."}`，可选 `etag` 与 `comment`。返回新建或被覆盖的 `Node`。
6. 服务端在成功后清理 `uploads/<uploadId>` 前缀并删除会话行；清理失败会写入待删除队列，由维护任务重试，**不影响**已经成功的上传。

预签名地址会过期（默认 `presign_ttl`，可被 `expires_in_seconds` 缩短但不能超过 4 倍基准）。过期后重新调用 `GetUpload` 或 `ListUploadParts` 会为**尚未完成**的分片重新签发地址，已确认的分片不会再返回 `upload_url`。

### 代理模式（`UPLOAD_MODE_PROXY`）

1. `POST /v1/files/uploads/create`，`mode: "UPLOAD_MODE_PROXY"`。回复的 `parts[]` 不包含 `upload_url`。
2. 对每个分片二选一：
   - 二进制绑定（推荐，浏览器使用）：`PUT /v1/files/uploads/{upload_id}/parts/{part_number}/raw`，`Authorization: Bearer <accessToken>`，请求体是原始字节，**必须带 `Content-Length`**（缺失返回 400 `NETDISK_INVALID_ARGUMENT`）；响应是 `UploadPart`。
   - proto JSON：`PUT /v1/files/uploads/{upload_id}/parts/{part_number}`，请求体 `{"content": "<base64>"}`（可选 `etag`）。
   两种方式都会把分片写到 `uploads/<uploadId>/parts/<n>`；片号必须在 `1..total_parts` 内，且分片大小不能超过会话的 `chunk_size`（末片按其实际长度）。
3. 无需 `ConfirmUploadPart`。若客户端在 `UploadChunk` 里提供了 ETag 而服务端读到的对象 ETag 不同，会删除刚写入的对象并返回 `NETDISK_PRECONDITION_FAILED`。
4. `POST /v1/files/uploads/complete`。

### 断点与续传

| 场景 | 做法 |
| --- | --- |
| 客户端记得 `upload_id` | `GET /v1/files/uploads/{id}` 查看 `status`、`uploaded_parts`、`received_bytes`，然后从缺失的分片继续 |
| 只记得文件名与目录 | 再次 `POST /v1/files/uploads/create`：服务端会返回仍可用的同名会话（幂等复用），并给出完整分片视图 |
| 想把一个旧会话显式续上 | 在 `InitiateUpload` 里带 `resumable_upload_id`，会更新该会话的 `size`/`chunk_size`/`total_parts` 并把 `EXPIRED`/`ABORTED` 的会话复位为 `PENDING`（仅在未过期且未完成时） |
| 只想取增量分片列表 | `GET /v1/files/uploads/{upload_id}/parts?from_part_number=N`，只返回编号大于 N 的分片，握手体积更小 |
| 主动放弃 | `POST /v1/files/uploads/abort` |

### `CompleteUpload` 校验规则

按代码执行顺序：

1. 会话存在且属于调用方（或调用方持有 `PERMISSION_STORAGE_MANAGE`），否则 `NETDISK_NOT_FOUND` / `NETDISK_PERMISSION_DENIED`。
2. 会话状态可提交：`ABORTED` → `NETDISK_ABORTED`；`COMPLETED` → `NETDISK_INVALID_ARGUMENT`；`EXPIRED` 或当前时间晚于 `expires_at` → `NETDISK_UPLOAD_EXPIRED`。
3. 已存分片数必须等于 `total_parts`，且 `1..total_parts` 每一片都要存在，否则 `NETDISK_UPLOAD_INCOMPLETE`。
4. 每个**非末片**大小 ≥ `upload.min_chunk_size`（S3 的 5 MiB 规则），否则 `NETDISK_UPLOAD_INCOMPLETE`。
5. `size > 0` 时，所有分片大小之和必须等于声明的 `size`，否则 `NETDISK_UPLOAD_INCOMPLETE`。
6. 按分片顺序 `ComposeObject` 组装成 `files/<owner>/<newNodeId>`。
7. 摘要校验：请求里的 `etag` 为空时取会话上记录的 `etag`；当 `upload.verify_checksum` 为真且该值带 `sha256:` 前缀时，与组装结果的 ETag 比对，不符则删除刚组装的对象并返回 `NETDISK_PRECONDITION_FAILED`。
8. 单个数据库事务内提交：
   - 目标目录仍然可写（存在、是文件夹、未删除、调用方仍有 `upload`）、未超过 `storage.max_path_depth`；
   - 按 `conflict_policy` 处理同名（会话创建时的 `conflict_policy`，缺省 `RENAME`）：`FAIL` → `NETDISK_NAME_CONFLICT`；同名目标是文件夹 → `NETDISK_NAME_CONFLICT`；`RENAME` 取 `base (1).ext`；`OVERWRITE` 走覆盖分支且要求调用方在同名节点上有 `edit`；
   - 配额复查（新建按 `size` 计在调用方账号上，覆盖按 `size - 旧 size` 计在**被覆盖文件属主**的账号上），超限 → `NETDISK_QUOTA_EXCEEDED`；
   - 覆盖且 `upload.keep_versions` 为真时，把旧内容登记为新版本；超过 `upload.max_versions` 时裁剪最旧的版本并把它们的对象键排入待删除队列；
   - 更新节点行与账号用量计数。
9. 事务失败时删除刚组装的对象，返回错误；事务成功后清理暂存分片与会话行。

## 下载

| 方式 | 接口 | 数据路径 | 适用 |
| --- | --- | --- | --- |
| 预签名直链 | `GET /v1/files/{node_id}/download-url` | 浏览器 → 对象存储（不经过 Nagisa） | 默认方式，要求客户端能访问 `data.object_storage.public_endpoint` |
| 服务端流式 | `GET /v1/files/{node_id}/content` | 浏览器 → Nagisa → 对象存储 | 支持 `Range` 断点续传；当前需要签名参数 |
| 打包 | `GET /v1/files/{node_id}/archive`（由 `GetArchiveUrl` 签发） | 浏览器 → Nagisa（即时压缩） | 文件夹或单文件下载为 zip |
| 预览 | `GET /v1/files/{node_id}/preview-url` | 浏览器 → 对象存储 | 可预览 MIME 的内联展示 |

### 预签名直链 vs 服务端流式

- `GetDownloadUrl` 只对**文件**有效：文件夹或没有内容对象的节点返回 `NETDISK_UNSUPPORTED`。它返回的 URL 指向对象存储，`expires_in_seconds` 缺省 `presign_ttl`、上限 4 倍。URL 自带 `response-content-disposition` 与 `response-content-type`，因此可以直接交给浏览器或下载工具。
- `GET /v1/files/{node_id}/content` 由服务端读取对象再转发，因而能配合 `Range` 做断点续传：

| 请求头 | 行为 |
| --- | --- |
| 无 `Range` | 200，`Content-Length` 为节点大小，`Accept-Ranges: bytes` |
| `Range: bytes=100-199` | 206，`Content-Range: bytes 100-199/<total>`，`Content-Length: 100` |
| `Range: bytes=100-` | 206，从 100 到结尾 |
| `Range: bytes=-500` | 206，最后 500 字节 |
| 多段 `Range: bytes=0-9,20-29` | 忽略 `Range`，按完整内容返回（不支持多段响应） |

响应头还包含 `Content-Type`（节点 MIME）、`Content-Disposition`（`extra=inline` 时为 `inline`，否则 `attachment`，带 RFC 6266 的 UTF-8 文件名）与 `ETag`。

该路由的鉴权是 URL 签名：`exp`（Unix 秒）、`sig`（HMAC-SHA256 的 base64url）、`sub`（账号 id）、`extra`（处置方式）。签名由 `internal/pkg/urlsign` 用 `auth.jwt_secret` 生成，绑定方法、路径、节点与过期时刻。**当前没有 RPC 返回指向 `/content` 的签名地址**（`biz.FileUsecase.ContentURL` 尚未被 service 层暴露），因此实际部署里经服务端的下载走打包接口，或者让浏览器直连对象存储。

### 文件夹打包

`GetArchiveUrl` 返回的地址形如 `<public_base_url>/v1/files/<node_id>/archive?extra=<归档名>&exp=...&sig=...&sub=...`，响应是 `application/zip`，`Content-Disposition` 为 `<归档名>.zip`：

- 目标是文件夹时递归展开整棵子树，目录写成目录条目，文件按相对路径（基于 `displayPath`）写入，压缩方式为 deflate；
- 目标是文件时，zip 内只含这一个文件；
- 归档即时生成并流式输出，不在对象存储里留下临时对象，因此 `SignedUrl.size` 为 0。

`public_base_url` 配置为空时签名地址是相对路径，由浏览器按当前站点补全。

## 文件夹描述 / 可见范围 / 访问名单 / 密码

这四项对文件和文件夹都可用（同一张表、同一套判定），前端把它们统称为「文件夹设置」。

| 设置 | 创建时 | 修改时 | 读取 |
| --- | --- | --- | --- |
| 描述 | `CreateFolder.folder.description` | `UpdateNode` + `update_mask: "description"` | `Node.description` |
| 可见范围 | `CreateFolder.folder.visibility`（缺省 `VISIBILITY_PRIVATE`） | `UpdateNode` + `update_mask: "visibility"` | `Node.visibility`（祖先中最严格的有效值不体现在该字段上，判定时单独计算） |
| 访问名单 | `CreateFolder.acl[]`（与建目录同事务） | `SetNodeAcl`（整体替换；`recursive` 可写入整棵子树） | `GetNodeAcl` 的 `entries` / `inherited_entries` |
| 密码 | `CreateFolder.password`（RSA 密文） | `UpdateNode` + `update_mask: "password"`，配 `password` 或 `remove_password: true` | `Node.passwordProtected`、`Node.passwordHint`、`Node.locked` |

可见范围取值的语义：

| 值 | 认证访问者 | 匿名访问者 |
| --- | --- | --- |
| `VISIBILITY_PRIVATE` | 仅属主与被显式允许的主体 | 无 |
| `VISIBILITY_INTERNAL` | 任何已认证账号都会获得 `view|download`（仍受账号权限与拒绝条目限制） | 无 |
| `VISIBILITY_PUBLIC` | 同上 | 也会获得 `view|download` |

祖先链取「最严格」：私有父目录中的公开子节点依然不可公开访问。

### 密码解锁的完整流程

1. 创建或更新文件夹时提交 `password`（RSA-OAEP(SHA-256) + base64）。服务端解密后校验长度策略，再用 bcrypt 存储；`password_hint` 是明文提示，会随节点返回。
2. 未解锁时访问该文件夹或其任一后代都会失败：`ListNodes` 返回 403 `NETDISK_NODE_LOCKED`，`Node.locked` 为 true。受密码保护时调用方即使被 ACL 允许，也只能保留 `view`（用于展示目录本身）。
3. `POST /v1/nodes/{node_id}/unlock`，请求体 `{"password": "<RSA 密文>"}`。回复 `NodeUnlock`：`unlock_token`、`expires_in`、`expires_at`，其中 `node_id` 是最外层受密码保护的祖先——用子节点 id 调用同样能拿到那层目录的令牌。
4. 后续请求带上 `X-Node-Token: <unlock_token>`，即可访问该节点及其整棵子树；令牌默认 30 分钟（`auth.node_token_ttl`）。
5. 令牌缺失、过期或不属于该节点时，服务端重新按未解锁处理（403 `NETDISK_NODE_LOCKED`）。密码错误返回 403 `NETDISK_PERMISSION_DENIED`，未提供密码返回 403 `NETDISK_NODE_LOCKED`。

属主访问自己的受保护目录不需要解锁（属主通行），而账号级权限仍然是最外层上限。

## 分享链接

### 完整的匿名访问流程

```text
① POST /v1/shares/create            （需要令牌）→ share.token / share.url
② POST /v1/shares/access            （匿名）    → share / node / children / accessToken
③ POST /v1/shares/children/list     （匿名）    → 逐层浏览，带 accessToken
④ POST /v1/shares/download-url      （匿名）    → 预签名下载地址
```

**① 创建。** `POST /v1/shares/create` 需要账号令牌，并且调用方在目标节点上持有 `share`（授予 `download` 能力还需 `download`，授予 `upload` 能力要求节点是文件夹且持有 `upload`）。回复中的 `token` 是不透明令牌，`url` 是拼好的分享地址（`web.public_base_url` + `web.share_path_prefix`，缺省 `/s`）。可同时设置密码（`password`）、提示（`password_hint`）、过期时间（`expires_at`）与下载额度（`max_downloads`）。

**② 打开。** 匿名调用 `POST /v1/shares/access`：

- 无密码链接：请求体只需 `{"token": "..."}`，`node` 是共享节点，`children[]` 是文件夹的第一层，`access_token` 为空。
- 有密码链接：先不带密码调用会得到 403 `NETDISK_NODE_LOCKED`（响应里同一份 `Share` 含 `password_hint`），带上 `{"token": "...", "password": "<RSA 密文>"}` 才能打开。**首次用密码打开的那次响应里会返回 `access_token`**（默认 2 小时有效），后续调用把它放在请求体的 `access_token` 字段里复用，不需要重发密码。
- 令牌为空、未知、已撤销或已过期都返回 404 `NETDISK_NOT_FOUND`；下载额度用尽返回 429 `NETDISK_RESOURCE_EXHAUSTED`。

**③ 浏览。** `POST /v1/shares/children/list`，请求体 `{"token": "...", "node_id": "<文件夹 id>", "access_token": "<上一步的令牌>"}`。`node_id` 为空表示共享节点本身。服务端会校验目标节点确实位于共享子树内（用物化路径前缀判断），越界返回 403 `NETDISK_PERMISSION_DENIED`。返回的每个节点带 `effectivePermissions`，即这条链接传达的能力。

**④ 下载。** `POST /v1/shares/download-url`，请求体 `{"token": "...", "node_id": "<文件 id>", "access_token": "..."}`，返回对象存储的预签名地址，并把 `share.download_count` 加一。链接没有 `download` 能力时返回 403 `NETDISK_PERMISSION_DENIED`；目标是文件夹时返回 400 `NETDISK_UNSUPPORTED`。

### 能力集合与生命周期

| 项 | 说明 |
| --- | --- |
| 能力 | 只会保留 `view` / `download` / `upload` 三个中的一个或多个；管理员类权限（`user_manage`、`audit_read`、`storage_manage`、`system_manage`）与 ACL 类权限（`acl_manage`）永远不会通过链接传递 |
| 密码 | 可选。`Share.password_protected` 告知前端是否需要密码输入框；密码以 bcrypt 存储 |
| 过期 | `expires_at` 为零值表示永不过期。过期后所有匿名接口都按 404 处理；维护任务 `expire_shares` 会把状态刷成 `SHARE_STATUS_EXPIRED` |
| 下载额度 | `max_downloads` 为 0 表示不限；每次成功取地址都会加一，用尽后返回 429 |
| 状态 | `SHARE_STATUS_ACTIVE` / `EXPIRED` / `REVOKED`。`UpdateShare` 可显式改写；`DeleteShare` 直接删行 |
| 统计 | `view_count` 在 `AccessShare` 成功时加一，`download_count` 在 `GetShareDownloadUrl` 成功时加一 |

## 错误码表

`reason` 取自 `api/netdisk/v1/error_reason.proto`，HTTP 状态由 `internal/biz/model.go` 里的错误构造器决定。

| `reason` | HTTP | 含义 |
| --- | --- | --- |
| `NETDISK_NOT_FOUND` | 404 | 资源不存在，或对调用方不可见（未授权访问受保护分享同样返回它，避免泄露存在性） |
| `NETDISK_INVALID_ARGUMENT` | 400 | 请求体或字段不合法：名称非法、id 未提供或格式错误、`filter`/`order_by`/`page_token` 非法、密码字段无法解密或被 mask 指定的字段不支持 |
| `NETDISK_UNAUTHENTICATED` | 401 | 未携带令牌，或令牌无效、过期；登录时账号不存在或密码不匹配也返回它 |
| `NETDISK_PERMISSION_DENIED` | 403 | 已认证但缺少所需权限，或调用方无法管理目标；也用于「文件夹/分享密码错误」 |
| `NETDISK_CONFLICT` | 409 | 与当前状态冲突（通用冲突，另有更具体的同名冲突） |
| `NETDISK_INTERNAL` | 500 | 未预期的服务端错误 |
| `NETDISK_UNAVAILABLE` | 503 | 依赖不可用：未配置对象存储或对象存储不可达 |
| `NETDISK_QUOTA_EXCEEDED` | 403 | 操作会超出账号配额（配置 `quota_bytes = 0` 表示不限） |
| `NETDISK_STORAGE_ERROR` | 500 | 对象存储拒绝了操作（写入、组装、读取失败） |
| `NETDISK_NODE_LOCKED` | 403 | 节点受密码保护且尚未解锁；分享需要密码而未提供时也返回它 |
| `NETDISK_NAME_CONFLICT` | 409 | 目标目录已有同名兄弟节点，或同名目标是文件夹 |
| `NETDISK_CYCLE_DETECTED` | 400 | 把文件夹移动到它自身或它的后代里 |
| `NETDISK_UPLOAD_INCOMPLETE` | 400 | 上传会话还有缺失分片、分片过小、或分片总大小与声明不符；`ConfirmUploadPart` 时对象不存在也返回它 |
| `NETDISK_UPLOAD_EXPIRED` | 400 | 上传会话已过 `expires_at` 或状态为 `EXPIRED` |
| `NETDISK_TOO_LARGE` | 400 | 负载或声明大小超过配置上限（`upload.max_file_size`、`upload.max_parts`、`upload.max_inline_size`、单分片超过 `chunk_size`） |
| `NETDISK_UNSUPPORTED` | 400 | 该资源类型不支持此操作：对文件夹取文件下载地址、预览不可预览的 MIME、分享下载指向文件夹、未开启匿名分享 |
| `NETDISK_RESOURCE_EXHAUSTED` | 429 | 分享下载额度用尽等限流/额度耗尽情形 |
| `NETDISK_PRECONDITION_FAILED` | 400 | 条件不满足：分片或整文件的摘要/ETag 与声明不符 |
| `NETDISK_ALREADY_EXISTS` | 409 | 资源已存在且冲突策略为 `FAIL`（例如重复的用户名、已被占用的分享令牌） |
| `NETDISK_ABORTED` | 400 | 操作被取消：对 `ABORTED` 的上传会话继续写入 |
| `NETDISK_ACCOUNT_DISABLED` | 403 | 账号被禁用或已删除，不能登录或刷新令牌 |
| `NETDISK_UNSPECIFIED` | — | 枚举的零值，服务端不会主动返回 |

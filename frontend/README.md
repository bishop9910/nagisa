# Nagisa 网盘前端

Vue 3 + Pinia + Vue Router + TypeScript 实现的前端，覆盖 `api/netdisk/v1/*.proto` 里
全部 HTTP 能力：认证与会话、文件树、分片上传、下载与预览、回收站、分享链接（含匿名页）、
搜索、账号安全，以及 `/manage` 开头的管理后台。

- **零第三方运行时依赖**：HTTP、加密、图标、组件、样式全部自研，`pnpm install` 之后
  完全离线可用（新增依赖需要联网，本机 npm registry 不可达）。
- **口令不落明文**：所有口令字段都在浏览器内用服务端公钥做 RSA-OAEP(SHA-256) 加密
  （`auth.allow_plain_password: false` 的部署照常可用）。
- 界面为浅色为主 + 完整深色主题，所有颜色/间距/圆角/阴影走语义令牌，支持键盘操作、
  焦点环与 `prefers-reduced-motion`。

## 1. 快速开始

```bash
cd frontend
pnpm install          # 已安装可跳过
pnpm dev              # http://localhost:5173
```

开发服务器把 `/v1` 与 `/docs` 代理到后端，**浏览器里只存在一个源站**，因此不需要后端
配置 `web.cors_origins`。代理目标按以下顺序解析：

| 优先级 | 来源 | 说明 |
| --- | --- | --- |
| 1 | `VITE_DEV_API_TARGET` 环境变量 | `VITE_DEV_API_TARGET=http://127.0.0.1:9000 pnpm dev` |
| 2 | `frontend/.env.development.local` | 已在本地指向 `http://127.0.0.1:3010`（该文件被 `.gitignore` 的 `*.local` 忽略） |
| 3 | 默认值 | `http://127.0.0.1:8000`，与 `configs/config.yaml` 的 `server.http.addr` 一致 |

登录账号：内置管理员 `admin`，口令取自后端首次初始化数据库时写入的值（`auth.admin_password` 只在建库那一次生效，之后改配置不会覆盖旧口令）。
只读访客 `guest` **不需要登录**：前端启动时调用公开的 `POST /v1/auth/guest` 直接换取只读令牌，前提是服务端 `auth.guest_auto_login: true`。
本地躺着一对已经失效的令牌（令牌过期、服务端换过签名密钥、账号被改密或停用）时，前端也会落到同一个访客态，
只在顶部提示一次「当前以只读访客身份浏览」——只有访客入口关着、或换取访客会话失败，才会真的跳到登录页。

## 2. 构建与部署

```bash
pnpm build            # vue-tsc 类型检查 + vite build，产物输出到 ../web/dist
pnpm preview          # 本地预览构建产物
pnpm test:unit        # Vitest（37 个用例）
```

| 部署方式 | 做法 | 说明 |
| --- | --- | --- |
| 后端同源托管（推荐） | `web.enabled: true`、`web.root: ./web/dist`，然后 `make build`/`build.ps1 build` 后启动服务 | 访问 `http://<host>:<port>/`，无需 CORS；哈希资源自动长缓存，`index.html` 不缓存 |
| 前后端分离 | 构建时给 `VITE_API_BASE=https://api.example.com pnpm build`，并把前端源站写进 `web.cors_origins` | 需要后端下发 CORS 头，且源站要精确列出，不要用 `*` |
| 挂在子路径 | 同时调整 `web.path_prefix` 与构建的 `base` | API 仍须是 `/v1/` |

> 当前本地配置 `web.enabled: false`，所以 `web/dist` 不会生效，请用 `pnpm dev`。
> 想直接用后端托管，把 `web.enabled` 打开即可。

## 3. 目录结构

```
src/
  api/                 接口层：契约类型镜像 + 单一 HTTP 客户端
    types.ts           与 proto 一一对应的 TS 类型（枚举名、Int64 字符串、RFC3339）
    http.ts            fetch 封装：鉴权头、X-Node-Token、401 单飞续期、错误信封
    auth|user|node|file|share|audit|system.ts   每个 Service 一个文件
  stores/              Pinia：auth（会话/权限/解封令牌）、system、ui（主题/提示/确认）、
                       files（目录浏览）、upload（上传队列）
  utils/               格式化、AIP 过滤与排序、权限位掩码、文件类型、下载、口令加密
  components/ui/       自研组件：按钮/输入/选择/开关/复选/分段/对话框/抽屉/菜单/标签页/
                       分页/徽标/进度/头像/提示/骨架/空态/图标（内置 90+ 线性图标）
  components/layout/   侧栏、顶栏、用户菜单、整体框架
  components/files/    文件列表与网格、工具栏、面包屑、上传面板、预览、移动/复制、
                       分享、详情与设置抽屉、访问名单编辑器、历史版本
  views/               页面；views/manage/ 为管理后台
  assets/styles/       设计令牌（tokens.css）与基础样式（base.css）
```

## 4. 路由

| 路径 | 名称 | 页面 | 所需能力 |
| --- | --- | --- | --- |
| `/login` | login | 登录（公开） | — |
| `/s/:token` | share-public | 匿名分享页（公开） | — |
| `/files/:folderId?` | files | 我的文件 | 会话（账号或只读访客） |
| `/search` | search | 全文搜索 | 会话（账号或只读访客） |
| `/shares` | shares | 我的分享 | 会话（账号或只读访客） |
| `/trash` | trash | 回收站 | 会话（账号或只读访客） |
| `/uploads` | uploads | 传输列表 | 会话（账号或只读访客） |
| `/account` | account | 账号设置 | 会话（账号或只读访客） |
| `/manage`（别名 `/@manage`） | manage-overview | 系统概览 | 任一管理权限 |
| `/manage/users`（`/@manage/users`） | manage-users | 账号管理 | `user_manage` |
| `/manage/audit` | manage-audit | 审计日志 | `audit_read` |
| `/manage/storage` | manage-storage | 存储与维护 | `storage_manage` |
| `/manage/permissions` | manage-permissions | 角色与权限 | `user_manage` |
| `/manage/settings` | manage-settings | 系统设置 | `system_manage` |

管理后台同时接受 `/manage/...` 与 `/@manage/...` 两种写法（后者是别名），菜单里生成的
链接一律是 `/manage/...`。缺少对应能力的路由会被守卫拦回概览页并给出提示。

`requiresAuth` 要的是一个会话，不一定是正式账号：守卫先 `bootstrap`，没有会话时由
`stores/auth.ts` 换成免登录的只读访客；访客态下管理后台、上传、分享这些入口由权限位
自己挡住，守卫不需要为访客单开一条分支。

回收站（`/trash`）按层级浏览：顶层只列**被删的那一项**，被删文件夹里的内容不会和它平铺
在同一级；点名称或「打开」进入该文件夹，再用面包屑回到上层。实现上就是给 `ListTrash`
带上 `original_parent_id`（当前所在的那个被删文件夹），逐层取。单独还原里面的某个子项时，
后端会把它放回它被删时所在的目录，所以不会还原到一个仍在回收站里的文件夹内。

## 5. 页面与后端接口对照

| 页面能力 | 接口 |
| --- | --- |
| 登录 / 续期 / 退出 | `GetAuthConfig`、`Login`、`RefreshToken`、`Logout`、`GetCurrentUser` |
| 修改密码 / 会话管理 | `ChangePassword`、`ListSessions`、`RevokeSession` |
| 目录浏览 / 面包屑 / 树 | `ListNodes`、`GetNodePath`、`GetNodeTree`、`GetNode` |
| 搜索 | `SearchNodes`（关键词、类型、MIME、所有者、体积与时间区间、描述匹配、排序） |
| 新建 / 重命名 / 移动 / 复制 / 删除 | `CreateFolder`、`UpdateNode`、`MoveNodes`、`CopyNodes`、`DeleteNodes` |
| 回收站 | `ListTrash`、`RestoreNodes`、`PurgeNodes`、`EmptyTrash` |
| 详情与设置 | `GetNodeStats`、`UpdateNode`（描述/可见范围/密码） |
| 访问名单 | `GetNodeAcl`、`SetNodeAcl`（含递归写入、继承条目展示） |
| 历史版本 | `ListNodeVersions`、`RestoreNodeVersion`、`DeleteNodeVersion` |
| 上传 | `InitiateUpload`、`UploadChunk`、`UploadPart...`、`CompleteUpload`、`AbortUpload`、`UploadSmallFile`、`ListUploads`、`GetUpload`、`ListUploadParts` |
| 上传分片字节（代理模式） | `PUT /v1/files/uploads/{id}/parts/{n}/raw`（原始字节，XHR 带进度） |
| 下载 | `GetDownloadUrl`（文件直链）、`GetArchiveUrl`（文件夹打包 zip）、`GetPreviewUrl`（内联预览） |
| 分享管理 | `CreateShare`、`ListShares`、`ListSharesByNode`、`GetShare`、`UpdateShare`、`DeleteShare` |
| 匿名分享 | `AccessShare`、`ListShareChildren`、`GetShareDownloadUrl` |
| 账号管理 | `CreateUser`、`GetUser`、`UpdateUser`、`DeleteUser`、`SetUserPermissions`、`ResetUserPassword`、`GetUserStats` |
| 角色与权限 | `ListRolePresets`、`ListPermissionCatalog` |
| 审计 | `ListAuditLogs`、`GetAuditLog`、`GetAuditSummary` |
| 系统 | `GetSystemInfo`、`HealthCheck`、`GetStorageStats`、`RunMaintenance`、`ListSystemSettings`、`UpdateSystemSettings` |

## 6. 实现要点（踩过的坑都记在这里）

1. **口令加密**：`GET /v1/auth/config` 拿到 PEM 公钥后，用 WebCrypto 做
   RSA-OAEP(SHA-256) 再 base64，与 Go 侧 `rsa.EncryptOAEP(sha256.New(), ..., nil)` 等价。
   公钥按内容缓存，密钥轮换（`passwordKeyId` 变化）会自动重新导入。
   **但 `crypto.subtle` 只在安全上下文里存在**：`https://` 和 `http://localhost` 有，
   局域网按 IP 走的 `http://192.168.x.x` 没有——那种情况下浏览器报的不是「浏览器太老」，
   而是把整个 `subtle` 藏了起来（`navigator.clipboard` 同时失效）。所以
   `utils/crypto.ts` 在检测不到 `crypto.subtle` 时会退回到 `utils/rsa.ts` 的内置实现
   （自带的 SHA-256 + MGF1 + OAEP + BigInt 模幂，密文格式与 WebCrypto 完全一致），
   登录页会显示一条黄色提示。这条兜底让纯 HTTP 的局域网也能登录，但没有 TLS 就
   无法校验服务端身份，正式做法是给站点配 HTTPS：见 `docs/deployment.md` 3.4 节。
2. **令牌与 401**：访问令牌过期时用刷新令牌换新的一对并**重放原请求**；并发的多个 401
   共用同一次续期，避免并发轮换互相吊销。续期失败才清空会话并跳登录。
3. **解封令牌**：`UnlockNode` 返回的是**最外层**受保护祖先的令牌，因此前端按
   `nodeId → token` 保存，并在访问任意节点时沿当前面包屑链挑选可用的令牌放进
   `X-Node-Token`；命中 `NETDISK_NODE_LOCKED` 时弹解锁框（错误信封里的
   `metadata.password_hint` 会显示为密码提示）。
4. **上传**：默认走代理模式（`UPLOAD_MODE_PROXY`，字节经服务端转发，不依赖对象存储的
   CORS）；部署只提供预签名时自动切换为直传 + `ConfirmUploadPart`，预签名地址过期会用
   `ListUploadParts` 重新签发。小文件自动改走 `UploadSmallFile` 内联上传。支持暂停、
   继续、取消、失败重试；同名会话由服务端幂等复用，所以续传就是再调一次 `InitiateUpload`。
5. **AIP 语法**：`filter` 字段名用**下划线**（`mime_type`、`created_at`），排序用
   `order_by=field` / `order_by=field desc`（**不是** `-field`，`-` 会被判为非法字符）；
   `updateMask` 必须用 **camelCase**（`quotaBytes`、`maxDownloads`），写成下划线会
   400/500。布尔字段用裸标识符 `success` / `NOT success`，服务端也会把
   `success=true` / `success=false` 归一化成这两种写法。
6. **响应缺失字段**：protojson 不输出零值，因此「字段缺失」一律当零值处理（`toInt()`）。
7. **下载**：文件用 `GetDownloadUrl` 的预签名地址，文件夹用 `GetArchiveUrl` 的
   `/archive` 签名地址，都通过隐藏 `<a>` 触发导航（不会被弹窗拦截）；预览用
   `GetPreviewUrl`，文本类只取前 512 KiB。
8. **状态完备**：每个列表都有加载骨架、空态、错误态与重试；对象存储未配置时侧栏会给出
   「上传与下载不可用」的提示，而不是让人对着失败一头雾水。
9. **请求体只能写 proto 的字段名**：接口层不能把本地对象（例如 `{ contentBase64 }`）直接当
   请求体发出去。服务端解码时 `DiscardUnknown: true` 会悄悄丢掉不认识的字段，随后 AIP 的
   `field_behavior` 校验只报一句 `missing required field: content`（`reason: VALIDATOR`），
   真因完全看不出来。`api/files.ts` 的 `uploadSmallFile` 就是这么踩过一次：979 KB 的文件走内联
   上传必然失败，而大于 4 MiB 走分片（`uploadChunk` 字段名是对的）反而正常。
   `src/__tests__/files.spec.ts` 现在盯着这几个上传接口的请求体字段名。

## 7. 测试

```bash
pnpm test:unit                                  # 65 个用例：工具函数 / HTTP 客户端 / 口令加密（含兜底）/ UI 组件
pnpm build                                      # vue-tsc 类型检查 + 构建

# 对接真实后端的接口冒烟（与浏览器行为一致：RSA 加密口令、camelCase 参数、X-Node-Token）
node ../scripts/frontend-smoke.mjs --base http://127.0.0.1:3010
node ../scripts/frontend-smoke.mjs --base http://127.0.0.1:8000 --user admin --password 'Admin@12345'
node ../scripts/frontend-smoke.mjs --base http://127.0.0.1:3010 --skip-share
```

冒烟脚本会打印 `PASS/FAIL/KNOWN` 统计并以非零状态码结束，`KNOWN` 表示部署或后端自身的
已知限制（见下一节），不算前端失败。

## 8. 后端联调记录（问题已修）

下面几条是前端联调时暴露出来的后端问题，**已在后端修掉并重新生成/构建**（改动见
`api/netdisk/v1/user.proto`、`api/netdisk/v1/audit.proto`、`api/netdisk/v1/node.proto`、
`internal/service/util.go`、`internal/service/audit.go`、`cmd/nagisa/providers.go`）；
如果运行的是修复前编译的二进制，请重新 `.\scripts\build.ps1 build` 并重启。

| 现象 | 原因 | 修复 |
| --- | --- | --- |
| 账号管理页报 `NETDISK_INVALID_ARGUMENT`，`GET /v1/users/list` 返回 400 | kratos HTTP 路由按 proto 声明顺序注册，`GET /v1/users/{id}` 声明在 `GET /v1/users/list` 之前，`list` 被当成非法 id 交给 `GetUser` | 把 `user.proto` 里的 `rpc ListUsers` 移到 `rpc GetUser` 之前并重新生成；`/v1/users/list` 现返回 200（页面仍保留针对性提示，便于识别旧二进制） |
| 审计日志 `filter=success=true` / `success=false` 返回 400 | AIP 的语法里没有布尔字面量（`true`/`false` 会当成未声明的标识符），而 `ents` 适配器又会把这种标识符当列名 | 服务端在解析前把布尔字面量归一化成裸标识符形式（`success` / `NOT success`），两种写法等价，`internal/service/util_test.go` 覆盖 |
| 分享相关接口返回 `NETDISK_UNSUPPORTED` | 部署没有打开匿名分享 | `SystemInfo.features` 现在跟随配置：`storage.allow_public_share` 为假时不再声明 `public_share`，未配置对象存储时也不再声明传输类能力；分享对话框会据此提前给出提示。要开匿名分享请在 `configs/config.yaml` 设 `storage.allow_public_share: true` |
| 上传报上限/会话立刻过期等异常 | 配置缺 `upload:` 段，`session_ttl`、`min_chunk_size`、`max_inline_size`、`default_mode` 都取零值 | 属于启动配置问题：照 `configs/config.yaml` 补齐 `upload` 段（分片至少 5 MiB 才能过对象存储的规则） |
| 浏览器直连下载地址失败 | `data.object_storage.public_endpoint` 为空，预签名地址指向内网地址；直传分片还要求对象存储允许跨域 | 配好 `public_endpoint` 并在对象存储侧配置 CORS（SeaweedFS 见其 S3 CORS 文档）；或让上传走代理模式（前端的默认选择） |

另外两处是文档与实际不符，已同步修正：`docs/api-guide.md` 里 `order_by` 的降序写法
（实际是 `size desc`，不是 `-size`）与 `PurgeNodes` 的说明（对象键其实会进入待删除队列，
由 `purge_deletions` 释放）；`node.proto` / `audit.proto` 里 `page_size` 的默认值也从
50 改成实际的 20。

## 9. 无障碍与体验细节

- 仅图标按钮都带 `aria-label`；对话框锁滚动、Esc 关闭、打开时自动聚焦首个控件；
  抽屉与弹层的遮罩点击可关闭。
- 深浅两套主题都独立校验过对比度；`prefers-reduced-motion` 下动画降级。
- 列表支持键盘进入（网格卡片 `Tab` + `Enter`）、批选、批量移动/复制/删除。
- 小屏：侧栏变抽屉，表格隐藏次要列，触控目标放大到 40px 以上。

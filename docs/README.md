# Nagisa 文档

本目录是 Nagisa 网盘后端的完整文档。所有描述都以仓库内的代码与 protobuf 契约为准：文档解释「为什么」与「怎么用」，接口字段的权威定义始终在 `api/netdisk/v1/*.proto`。

## 阅读顺序

| 顺序 | 文档 | 什么时候读 |
| --- | --- | --- |
| 1 | [`../README.md`](../README.md) | 第一次接触项目：功能范围、启动方式、5 分钟 API 漫游 |
| 2 | [`architecture.md`](architecture.md) | 要改代码：请求路径、分层约束、事务与原子性、对象存储布局、加资源的步骤 |
| 3 | [`api-guide.md`](api-guide.md) | 要对接接口：通用约定、认证流程、逐个 RPC 的字段与错误、上传/下载/分享完整流程 |
| 4 | [`permissions.md`](permissions.md) | 要设计权限：角色与权限位、等级规则、节点访问判定顺序与示例 |
| 5 | [`../configs/README.md`](../configs/README.md) | 只想知道 `config.yaml` 里每个条目是干啥的：逐条大白话说明 |
| 6 | [`configuration.md`](configuration.md) | 要改配置：加载与覆盖顺序、值格式约定、缺省时的实际默认值、环境变量注入、三套示例 |
| 7 | [`deployment.md`](deployment.md) | 要上线：对象存储（SeaweedFS）、SQLite 生产建议、反向代理与 TLS、首启清单、运维任务、前端部署、容器化 |

## 事实来源

文档中的每一处行为都可以在这些文件里读到实现：

| 内容 | 位置 |
| --- | --- |
| 接口、字段、枚举、错误码 | `api/netdisk/v1/{common,auth,user,node,file,share,audit,system,error_reason}.proto` |
| 权限位、角色预设、权限中文目录 | `internal/biz/model.go` |
| 节点访问判定顺序 | `internal/biz/access.go`、`internal/biz/node.go` |
| 上传、下载、版本、待删除队列 | `internal/biz/file.go` |
| 分享链接与匿名访问 | `internal/biz/share.go`、`internal/service/share.go` |
| 账号等级与授权规则 | `internal/biz/user.go`、`internal/biz/auth.go` |
| 维护任务与运行参数 | `internal/biz/system.go` |
| 免令牌接口清单、`X-Node-Token` | `internal/server/middleware/auth.go`、`internal/server/grpc.go` |
| 线上传输格式（protobuf JSON） | `internal/server/codec.go` |
| 原始字节路由、静态托管、CORS | `internal/server/{http,stream,web}.go`、`internal/server/middleware/cors.go` |
| 事务、SQLite pragma、写入串行化 | `internal/data/data.go` |
| 对象存储键布局、预签名、ComposeObject | `internal/data/storage.go`、`internal/biz/file.go` |
| 内置账号与默认设置 | `internal/data/seed.go` |
| 全部配置键与注释 | `internal/conf/conf.proto`、`configs/config.yaml` |
| 端到端用法示例 | `test/integration/smoke_test.go` |
| 机器可读契约 | `openapi.yaml`、`docs/openapi.yaml` |

## 生成与发布

Linux / macOS 用 `make`，Windows 用等价的 `scripts/build.ps1`（目标名一一对应）：

```bash
make init       # 安装 wire 与 buf
make api        # buf 生成 Go 绑定与 openapi.yaml（protoc-gen-openapi）
make config     # 生成 internal/conf/conf.pb.go
make generate   # go generate ./...（ent）并 go mod tidy
make docs       # 用 tools/openapi 富化 openapi.yaml 并发布到 docs/
make all        # 依次执行 api、config、generate、docs
make build      # 构建到 ./bin/
make test       # go test ./...
make test-integration  # go test -tags integration ./test/integration/ -v
```

```powershell
.\scripts\build.ps1 init
.\scripts\build.ps1 api
.\scripts\build.ps1 config
.\scripts\build.ps1 generate
.\scripts\build.ps1 docs
.\scripts\build.ps1 all
.\scripts\build.ps1 build            # -> .\bin\nagisa.exe
.\scripts\build.ps1 test
.\scripts\build.ps1 test-integration
```

脚本另外提供 `run`（用 `./configs` 启动服务）、`fmt`（`-Check` 只检查不修改）、`vet`、`tidy`、`clean`，不带参数运行会打印全部用法。它不依赖 GNU make，只要求 Go 工具链；`buf` 若不在 PATH 上会自动到 `%GOPATH%\bin` 查找。

`make api` 产出的 `openapi.yaml` 是「原始」文档，安全方案（`bearerAuth` / `nodeToken` / `shareToken` / `requestId`）、共享错误响应与原始字节绑定由 `make docs` 补齐；富化工具拒绝在已富化的文件上重复运行，所以顺序必须是先 `make api` 再 `make docs`：

```bash
go run ./tools/openapi -in openapi.yaml -out docs
```

该命令会就地富化 `openapi.yaml`，并输出 `docs/openapi.yaml`、`docs/swagger.yaml`、`docs/openapi.json` 与 Swagger UI 页面 `docs/index.html`。三者中的 YAML 副本内容一致；服务开启静态路由后，`/docs/` 就是该目录的文件服务，访问 `/docs/` 看到的是 Swagger UI。

## 术语

| 术语 | 含义 |
| --- | --- |
| 节点（node） | 文件树中的一个条目。文件夹是 `NODE_KIND_FOLDER`，文件是 `NODE_KIND_FILE`，两者共用一张表与同一套访问模型 |
| 物化路径（`path`） | 由祖先节点 id 拼成的字符串，形如 `/<id1>/<id2>/`。子树查询退化为一次前缀扫描，移动退化为一次前缀重写 |
| DO / PO / DTO | 领域对象（`internal/biz`）/ 持久化对象（`internal/data`）/ 传输对象（proto 消息） |
| 权限位（bitmask） | 12 个权限的位掩码，`1 << (枚举值 - 1)`，在接口上表现为 `permissionsMask` |
| 等级（`rank`） | 账号的权威级别，默认 `admin=1000`、`manager=500`、`user=100`、`guest=10`；调用方只能管理等级严格低于自己的账号 |
| 可见范围（`visibility`） | 节点级粗粒度策略：`private` / `internal` / `public`，取值越小越严格，祖先中最严格的决定整条链 |
| 访问名单（ACL） | 节点上的允许/拒绝条目，主体可以是某账号、某角色或所有人 |
| 解锁令牌 | `UnlockNode` 为受密码保护的文件夹签发的短时效 JWT，通过 `X-Node-Token` 请求头传递 |
| 上传会话 | 一次分片上传的状态：`PENDING` → `IN_PROGRESS` → `COMPLETED`，或被 `ABORTED` / `EXPIRED` 终止 |
| 待删除队列 | 数据库提交后需要释放的对象键队列，由 `purge_deletions` 任务消费 |

## 约定

- 代码标识符、文件路径、HTTP 路径、字段名、配置键一律保持英文，正文使用简体中文。
- 请求/响应使用 protobuf 规范 JSON 映射：枚举是名称字符串，64 位整数是字符串，`bytes` 是 base64，时间戳是 RFC 3339，`FieldMask` 是逗号分隔的驼峰路径字符串。字段名同时接受 camelCase 与原始下划线写法。
- 本文档中的默认值来自代码在配置缺省时的兜底逻辑；`configs/config.yaml` 中的示例值可能与之不同，`configuration.md` 会分别列出。
- 端口：HTTP `8000`、gRPC `9000`；集成测试默认打 `http://127.0.0.1:18000`。

## 改代码时同步更新文档

| 改动 | 需要更新的文档 |
| --- | --- |
| 增删 RPC 或字段 | `api-guide.md` 对应小节；`error_reason.proto` 增删错误码时同步「错误码表」 |
| 增删配置键 | `internal/conf/conf.proto` 与 `configs/config.yaml`，说明见 `configuration.md` |
| 调整权限位或角色预设 | `permissions.md` 的权限目录与角色表 |
| 调整访问判定顺序或 ACL 语义 | `permissions.md` 与 `api-guide.md` 的节点权限小节 |
| 调整对象键布局、事务边界、维护任务 | `architecture.md` |

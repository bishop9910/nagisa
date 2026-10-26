# AGENTS.md

面向在本仓库工作的 AI agent 与人类贡献者的开发规范。约定不是偏好：每一条都对应现有代码或文档里已经存在的做法，改代码前先按这里的顺序读文档。

## 1. 项目与栈

| 项 | 值 |
| --- | --- |
| Go module | `nagisa`（Go 1.25.7） |
| 框架 | Kratos v3（`github.com/go-kratos/kratos/v3`）、Wire 依赖注入 |
| 持久化 | ent v0.14（SQLite 默认，可选 MySQL） |
| 接口 | proto3 + buf（`api/netdisk/v1/`），HTTP/JSON + gRPC 同源 |
| 列表语义 | AIP：`go.einride.tech/aip` 的 fieldmask / filtering，`aip-go/ents` 下推 SQL |
| 前端 | Vue 3.5 + TypeScript + Vite + Pinia，产物到 `web/dist` |
| 对象存储 | S3 协议（默认 SeaweedFS），未配置时回落本地目录 |

## 2. 目录职责

| 路径 | 职责 | 不要做 |
| --- | --- | --- |
| `api/netdisk/v1/` | proto 契约与生成代码（`*.pb.go` 由 buf 生成） | 手改生成文件 |
| `internal/service/` | transport 适配：请求解析、字段掩码、分页、错误透传 | 写权限判定、写业务规则 |
| `internal/biz/` | 领域逻辑、权限模型、用例；仓储接口在此声明 | import `service`/`server` |
| `internal/data/` | ent 仓储实现、事务、SQL 下推 | 泄漏 ent 类型到 biz |
| `internal/server/` | HTTP/gRPC 装配、中间件、静态 `web/dist` 与 `/docs` | 业务逻辑 |
| `internal/pkg/` | 通用件（`crypt`、`urlsign`） | 放业务代码 |
| `cmd/nagisa/` | 启动、Wire provider、健康检查 | 业务逻辑 |
| `frontend/src/` | 前端源码（`api/` `components/` `views/` `stores/` `utils/`） | 直接 `fetch`，绕过 `api/http.ts` |
| `web/dist/` | 前端构建产物，被 `.gitignore` 忽略；缺失时服务端返回占位页 | 手工编辑 |
| `configs/` | `config.yaml` 与配置说明 | 提交本地凭据 |
| `deploy/` | Compose 整套部署与 Caddyfile | 提交 `.env` |
| `docs/` | 中文文档 + 生成的 OpenAPI 副本，按 `/docs` 静态发布 | 手写 OpenAPI |
| `test/integration/` | 端到端冒烟（`//go:build integration`） | 在单测里跑 |
| `data/` | 运行期状态；仅 `README.md` 与 `seaweedfs/s3.json` 入库 | 提交数据库/密钥/上传文件 |
| `tools/openapi/` | OpenAPI 富化与分发（yaml→json、swagger） | — |

## 3. 常用命令

```bash
make api            # buf 生成 api/netdisk/v1/*.pb.go 与根 openapi.yaml
make config         # buf 生成 internal/conf 的配置结构体
make docs           # go run ./tools/openapi -in openapi.yaml -out docs
make build          # 构建 ./bin/nagisa（带 git 版本号）
make test           # go test ./...
make test-integration  # go test -tags integration ./test/integration/ -v
make generate       # go generate ./... && go mod tidy
make all            # init + api + config + generate + docs
go build ./... && go vet ./...   # make 里没有聚合检查目标，单独跑
```

前端（在 `frontend/`）：`npm run dev`、`npm run build`（含 type-check）、`npm run type-check`、`npm run test:unit`。

本机（Windows）补充：`buf` 在 `C:\Users\bisho\go\bin\buf.exe`，不在 PATH；沙箱下临时文件只能落在工作区内，跑 Go 工具链时用工作区内的 GOCACHE（例如 `.tmp/gocache`），否则会因清理被拦而失败。命令行不可写 `api/`、`bin/` 时，用编辑工具直接改文件。

## 4. 契约优先：proto 是唯一接口源

1. 改接口：先改 `api/netdisk/v1/*.proto`（含注释，注释会进 OpenAPI 与 `/docs`），再 `make api`。
2. 生成物必须与源一起提交：`api/netdisk/v1/*.pb.go`、根 `openapi.yaml`、`docs/openapi.yaml`、`docs/openapi.json`、`docs/swagger.yaml`。
3. `docs/swagger.yaml` 与 `docs/openapi.json` 由 `make docs` 从根 `openapi.yaml` 派生；三者内容必须同源。改完用 `node -e "JSON.parse(require('fs').readFileSync('docs/openapi.json','utf8'))"` 确认 JSON 仍合法（控制台显示中文乱码不代表文件损坏）。
4. 生成器不可用时，手改生成物必须先在同参数下产出等价文件并 `Get-FileHash` 比对；顺序、缩进、注释位置都要与生成结果逐字节一致。
5. 新增错误码：改 `ErrorReason` 枚举 → `make api` → 在 `internal/biz/model.go` 加哨兵变量。

## 5. Go 代码规范

- 分层依赖单向：`service → biz → data`；`biz` 只暴露接口与值对象，仓储实现在 `data`。
- 错误：统一用 `internal/biz/model.go` 里的 Kratos 错误（`errors.NotFound(...)`、`ErrPermissionDenied` 等），reason 取 `v1.ErrorReason_*` 枚举，方便客户端按 reason 分支。包装必须 `%w` 保留哨兵，不要 `fmt.Errorf` 制造无 code 的错误。
- 校验失败用 `InvalidArgument("field: rule")`，消息里写清字段与规则。
- 注释用英文、完整句子；导出标识符必须有注释。文档与提交说明用中文/英文按第 8、10 节。
- 测试用表驱动、同包 `_test.go`；纯逻辑优先测（如 `internal/biz/access_test.go`），涉及 HTTP 的放 `test/integration`。
- 新增依赖要能离线构建（vendor 不便时至少确认 `go mod tidy` 后的差异最小），不要引入 linter 之外的脚手架。

## 6. 权限模型规范（改动前必读）

判定顺序的唯一实现与唯一说明分别是 `internal/biz/access.go` 的 `EvaluateNodeAccess` 和 `docs/permissions.md` 第 4 节。两处必须同步修改。

- **第 0 步：超级管理员通行**。`Caller.IsSuperAdmin()`（只认 `role == ROLE_ADMIN`，不认 rank）直接得到 `PermNodeScope ∩ 账号权限`、`locked = false`、`owned = false`。可见范围、访问名单、文件夹密码不参与判定；账号权限集仍是上限。这是全站内容管理与跨账号处置违规文件的唯一手段。
- 超级管理员的通行只给节点权限，不给账号权限：`PermUserManage` 等仍由账号掩码决定。
- 属主捷径、拒绝优先、允许需 `inherit`、锁定最后收敛——这些顺序不能调换，改一处就要更新 `access_test.go` 与 `docs/permissions.md` 的示例表。
- 任何新接口都必须走 `RequireNodeAccess`/`EvaluateNodeAccess` 或 `Caller.Require`，禁止在 `service` 层手写 `if role == ...`。
- 回收站是权限模型的一部分：跨账号可见/可清只给 `ROLE_ADMIN`；`trash_manage` 只覆盖自己拥有的节点与被显式授权的节点。`EmptyTrash` 只清每个已删除子树的顶端（`TrashRoots`），不要退化成遍历全部 TRASHED 行。
- 新增节点级权限位：`internal/biz/model.go` 的 `PermMask` 常量与 `PermNodeScope` → proto 的 `Permission` 枚举 → 前端 `frontend/src/utils/constants.ts` → `docs/permissions.md` 第 2 节表格，四处一起改。

## 7. 前端规范

- 组件用 `<script setup lang="ts">`；状态进 Pinia（`src/stores/`），可复用逻辑进 `src/utils/`。
- 所有请求走 `src/api/*.ts`（`http.ts` 统一 baseURL、错误与鉴权），不要在组件里直接 `fetch`。
- 节点上的按钮一律按**节点**的有效权限判定：`node.effectivePermissionsMask` 的位 + `node.owned`（见 `components/files/NodeActions.vue`）。`auth.has(...)` 只用于页面级入口（侧栏、管理台），不能代替节点权限。
- 权限常量与后端枚举一一对应（`utils/constants.ts` 的 `PERMISSION_*`）；新增位要同时更新 `maskFromPermissions`。
- 文案用中文；主题需同时检查亮/暗两套对比度；下拉、弹层注意父容器的 `overflow` 裁剪。
- 单测用 vitest，`frontend/src/__tests__/*.spec.ts`，断言行为与权限矩阵，不测样式实现。

## 8. 文档规范

- 正文简体中文；标识符、路径、HTTP 路径、字段名、配置键保持英文。Markdown 用表格代替长列表，代码块带语言标记，陈述必须能追溯到已读代码。
- 文档里出现的每个仓库路径都必须真实存在。新增文件后再写文档，或先写文档再补文件——两者不能只做一半。本次修复的就是这类事故：`docs/deployment.md` 声称提供 `data/seaweedfs/s3.json`，仓库里没有该文件。
- 改配置项：`configs/config.yaml` 注释、`configs/README.md`、`docs/configuration.md` 三处同步。
- 改接口：proto 注释 + `docs/api-guide.md`；涉及权限的再加 `docs/permissions.md`。
- 新增权限、角色、判定步骤：`docs/permissions.md` 是唯一权威描述，代码注释只做指向。

## 9. 数据与运行期文件

- `data/` 只提交 `README.md` 与 `seaweedfs/s3.json`（开发用凭据，与 `configs/config.yaml` 默认值一致）。数据库、RSA 密钥、上传内容、SeaweedFS 数据目录都在 `.gitignore` 里，绝不入库。
- 生产凭据自行生成并 `chmod 600`，不要复用示例。

## 10. 提交与变更集

- 提交信息：短、英文、祈使句，一个主题一条（现有历史即风格：`add owner`、`fix permission bug`、`fix docker`）。
- 生成物与源同一次提交；不要出现「proto 改了但 `.pb.go` 没跟」的提交。
- 不提交 `.tmp/`、`.graphflow/`、`graphflow-out/`、`bin/`、`web/dist/`、`deploy/compose/.env`。
- 变更涉及行为时，在提交正文（或 PR 描述）里写清：改了什么判定/接口、为什么、如何验证。

## 11. 禁止事项

- 手改 `api/netdisk/v1/*.pb.go`、`openapi.yaml`、`docs/openapi.*`（除第 4 节的等价核对流程）。
- 在 service/前端绕过权限判定，或用 `auth.has` 冒充节点权限。
- 把运行期数据、密钥、`.env`、本地缓存提交进仓库。
- 在 `internal/data` 使用 `Object` 类型字段；在前端用 `as any` 规避类型。
- 无声修改默认配置或默认权限预设；预设属于契约，改动必须配文档与测试。

## 12. 交付前自检

1. `go build ./...`、`go vet ./...`、`go test ./...` 全绿。
2. 动过权限：`internal/biz/access_test.go` 有新用例，且 `docs/permissions.md` 的步骤表/示例表同步。
3. 动过接口：`make api && make docs` 后 `git diff` 只有预期改动；`docs/index.html` 与生成结果一致；JSON 可解析。
4. 动过前端：`npm run type-check`、`npm run test:unit` 通过。
5. 动过文档：文中每个反引号路径都能在实际仓库中找到。

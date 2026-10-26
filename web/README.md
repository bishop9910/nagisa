# 前端接入位（web）

后端会把构建好的单页应用直接托管出去，这里就是它的位置。

## 目录约定

```
frontend/          前端源码（Vue 3 + Vite + TypeScript），随仓库提供
web/
  dist/            构建产物，Kratos HTTP 服务器从这里读取静态文件（不提交）
    index.html     入口文档，也是 SPA 回退页
    assets/        带内容哈希的静态资源
```

`web/dist` **不在版本库里**（`.gitignore` 忽略它），由 `frontend/` 构建产生：`pnpm run build`
的输出目录就是 `../web/dist`。没有它时 `/` 会由 `internal/server/web.go` 里的占位页回答
（提示前端尚未构建），接口与 `/docs/` 都不受影响，可以先把后端跑起来。

## 后端提供了什么

| 能力 | 位置 |
| --- | --- |
| 静态托管 + SPA 回退 | `internal/server/web.go`，由 `web.enabled` 控制 |
| 接口文档（Swagger UI） | `/docs/`，文档文件在 `docs/openapi.yaml` |
| 机器可读契约 | `docs/openapi.yaml`、`docs/swagger.yaml`、`docs/openapi.json` |
| 预检与跨域 | `internal/server/middleware/cors.go`，由 `web.cors_*` 配置 |

## 与后端对接时需要知道的约定

1. **密码不明文传输。** 先 `GET /v1/auth/config` 拿到 `passwordPublicKey`，
   用 RSA-OAEP(SHA-256) 加密后再 base64 编码，登录、改密码、设置文件夹密码、
   设置分享密码都按这个规则发送。

2. **令牌。** 登录返回 `accessToken`（放进 `Authorization: Bearer`）与 `refreshToken`。
   访问令牌过期后用 `POST /v1/auth/refresh` 换取新的一对，旧刷新令牌立即失效。

3. **受密码保护的文件夹。** `POST /v1/nodes/{node_id}/unlock` 返回 `unlockToken`，
   之后访问该文件夹及其子节点都要带上 `X-Node-Token` 请求头。

4. **字段命名。** 请求与响应使用 protobuf 的规范 JSON 映射：枚举是名称字符串
   （`"PERMISSION_UPLOAD"`）、64 位整数是字符串（`"size": "1048576"`）、
   时间是 RFC 3339、FieldMask 是逗号分隔的驼峰路径字符串。
   字段名两种写法都接受：文档中的 `pageSize` 与下划线的 `page_size` 等价。

5. **大文件。** 推荐走 `UPLOAD_MODE_PRESIGNED`：`POST /v1/files/uploads/create`
   会返回每个分片的预签名 PUT 地址，浏览器直接传给对象存储，之后
   `POST /v1/files/uploads/parts/confirm` 回报 ETag，最后 `CompleteUpload` 提交。
   如果对象存储对浏览器不可达，改用代理模式：分片 PUT 到
   `/v1/files/uploads/{upload_id}/parts/{part_number}/raw`（原始字节请求体）。

6. **下载。** `GET /v1/files/{node_id}/download-url` 返回签名地址；
   文件夹用 `GET /v1/files/{node_id}/archive-url` 拿打包下载地址。
   签名地址自带时效，不需要再带令牌。

7. **权限开关。** 每个节点返回的 `effectivePermissions` / `effectivePermissionsMask`
   是当前调用者在该节点上的实际权限，界面应当据此决定按钮是否可用，
   而不是根据账号角色猜测。账号级权限用 `GET /v1/auth/me` 或
   `GET /v1/users/permissions/catalog` 读取。

## 本地开发

前端在 `frontend/`，Vite 开发服务器固定 `http://127.0.0.1:5173`，并把 `/v1` 与 `/docs`
代理到 `VITE_DEV_API_TARGET`（默认 `http://127.0.0.1:8000`，见 `frontend/vite.config.ts`）。
因为浏览器里只有一个源站，联调**不需要**改后端的 `web.cors_origins`：

```bash
# 后端
go run ./cmd/nagisa -conf ./configs

# 前端（另开一个终端）
cd frontend
pnpm install
pnpm dev          # http://127.0.0.1:5173 上已经带好 /v1 与 /docs 代理
```

生产环境直接构建到 `web/dist`，后端会连同接口一起提供：

```bash
cd frontend && pnpm build   # = vue-tsc --build + vite build，输出到 ../web/dist
go build -o bin/nagisa ./cmd/nagisa
```

用 Docker 时这一步由镜像自己做：`Dockerfile` 的 `frontend` 阶段跑 `pnpm install --frozen-lockfile && pnpm run build`，产物直接落到镜像内的 `/app/web/dist`（对应配置的 `WEB_ROOT`）。所以 `docker compose up -d --build`（见 [`deploy/compose/README.md`](../deploy/compose/README.md)）出来的镜像打开就是网盘界面，不需要先在宿主机构建前端。

Windows 上同样的两件事可以用仓库脚本完成（不需要 `make`）：

```powershell
cd frontend; pnpm build      # 输出到 ..\web\dist
.\scripts\build.ps1 build    # 产出 .\bin\nagisa.exe
.\scripts\build.ps1 run      # 用 ./configs 启动后端
```


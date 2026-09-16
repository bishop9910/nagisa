# 前端接入位（web）

后端会把构建好的单页应用直接托管出去，这里就是它的位置。

## 目录约定

```
web/
  dist/            构建产物，Kratos HTTP 服务器从这里读取静态文件
    index.html     入口文档，也是 SPA 回退页
    assets/        带内容哈希的静态资源
  src/             前端源码（本仓库不提供，自行初始化）
```

`web/dist` 里目前只有一个占位 `index.html`：当前端还没构建时，访问 `/` 会看到它，
接口本身不受影响，可以先把后端跑起来。

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

前端开发服务器通常在 `http://127.0.0.1:5173`。把它的源站加进
`configs/config.yaml` 的 `web.cors_origins`，然后：

```bash
# 后端
go run ./cmd/nagisa -conf ./configs

# 前端（使用你自己的工具链）
npm run dev -- --proxy /v1=http://127.0.0.1:8000
```

生产环境直接构建到 `web/dist`，后端会连同接口一起提供：

```bash
npm run build   # 输出到 web/dist
go build -o bin/nagisa ./cmd/nagisa
```

Windows 上同样的两件事可以用仓库脚本完成（不需要 `make`）：

```powershell
npm run build                # 输出到 web\dist
.\scripts\build.ps1 build    # 产出 .\bin\nagisa.exe
.\scripts\build.ps1 run      # 用 ./configs 启动后端
```


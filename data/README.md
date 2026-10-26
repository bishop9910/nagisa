# data/

运行期状态目录。`configs/config.yaml` 的默认值把数据库、密码密钥与本地对象存储都指向这里：

| 路径 | 内容 | 来源配置 |
| --- | --- | --- |
| `data/nagisa.db` | SQLite 数据库（含 WAL 文件） | `data.database.source` |
| `data/password_key.pem` | RSA 私钥，用于密码字段的传输加密 | `data.password_key_file` |
| `data/uploads/` | 未接对象存储时的本地对象目录 | `data.object_storage.directory` |
| `data/seaweedfs/` | 本地 SeaweedFS 的 S3 数据与元数据目录 | 见 [docs/deployment.md](../docs/deployment.md) 第 1 节 |

## 提交策略

除本文件与 `data/seaweedfs/s3.json` 外，`data/` 下的内容都是运行期状态，被 `.gitignore` 忽略：它们包含数据库、密钥和用户上传的文件，不应该进版本库。首次启动时这些文件会自动创建，不需要手工准备。

`data/seaweedfs/s3.json` 是**开发用**的 SeaweedFS 身份文件，与 `configs/config.yaml` 的默认凭据（`nagisa` / `nagisa-secret`）一致，仅用于本机 `weed server -s3` 的联调。生产部署不要使用它，按 `docs/deployment.md` 1.2 节生成自己的凭据并设置 `chmod 600`。

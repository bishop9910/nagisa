# 权限模型

Nagisa 的授权分三层：**账号级**（角色 + 等级 + 权限位掩码）、**节点级**（可见范围 + 访问名单 + 密码）、**链接级**（分享能力集合）。三层按固定顺序叠加，任何一层都不能把调用方提升到超出它自身账号权限的范围。

## 1. 角色与默认等级

角色是粗粒度的账号类别，它只决定默认的权限模板与展示标签；实际权威由 `rank` 决定，所以被提拔的 `manager` 可以压过另一位 `manager`。

| 角色 | 机器名 | 默认等级 | 中文名 | 默认权限集合 | 位掩码 | 说明 |
| --- | --- | --- | --- | --- | --- | --- |
| `ROLE_ADMIN` | `admin` | 1000（固定） | 超级管理员 | 全部 12 项 | 4095 | 内置最高权限账号，可管理所有账号与系统信息；任何其他账号都无法管理它，它自己也改不了自己的资料 |
| `ROLE_MANAGER` | `manager` | 500 | 管理员 | `view` `download` `upload` `edit` `delete` `trash_manage` `share` `acl_manage` `user_manage` `audit_read` `storage_manage` | 2047 | 由上级管理员提拔，可管理权限低于自己的账号；默认不含 `system_manage` |
| `ROLE_USER` | `user` | 100 | 普通用户 | `view` `download` `upload` `edit` `delete` `trash_manage` `share` `acl_manage` | 255 | 管理自己的文件，不能管理其他账号 |
| `ROLE_GUEST` | `guest` | 10 | 访客 | `view` `download` | 3 | 默认只读，仅可浏览与下载；**内置访客账号锁死**，见第 3 节与 7.3 |

等级常量：`RankAdmin = 1000`、`RankManager = 500`、`RankUser = 100`、`RankGuest = 10`。创建账号时若未指定 `rank`，使用角色预设等级；若指定了 `rank`，必须严格小于创建者的等级。

三个派生的权限集合是判定时的硬边界：

| 集合 | 内容 | 用途 |
| --- | --- | --- |
| `PermNodeScope` | `view` \| `download` \| `upload` \| `edit` \| `delete` \| `trash_manage` \| `share` \| `acl_manage` = 255 | 节点 ACL 能携带的全部权限；管理类权限被刻意排除，ACL 永远无法把账号提升为管理员 |
| `PermShareScope` | `view` \| `download` \| `upload` = 7 | 分享链接能传达的全部能力；`edit`、`delete`、`acl_manage` 与所有管理权限都无法通过链接传递 |
| `PermAll` | 12 位全 1 = 4095 | 内置管理员与「权限目录」满配的参照值 |

## 2. 权限目录

权限在线上与存储里都是位掩码：`枚举值 i` 对应 `1 << (i - 1)`。接口上的 `permissionsMask` 就是这个掩码，`permissions` 是同一集合的枚举名数组。

| 枚举 | 机器名 | 位值 | 中文名 | 说明 | 分组 |
| --- | --- | --- | --- | --- | --- |
| `PERMISSION_VIEW` | `view` | 1 (1<<0) | 查看 | 浏览目录、读取文件与文件夹的元信息 | `content` |
| `PERMISSION_DOWNLOAD` | `download` | 2 (1<<1) | 下载 | 下载原始文件与预览内容 | `content` |
| `PERMISSION_UPLOAD` | `upload` | 4 (1<<2) | 上传 | 上传文件与新建文件夹 | `content` |
| `PERMISSION_EDIT` | `edit` | 8 (1<<3) | 编辑 | 重命名、移动、修改描述与元信息 | `content` |
| `PERMISSION_DELETE` | `delete` | 16 (1<<4) | 删除 | 将文件或文件夹移入回收站 | `content` |
| `PERMISSION_TRASH_MANAGE` | `trash_manage` | 32 (1<<5) | 回收站管理 | 还原或彻底清除回收站内容 | `content` |
| `PERMISSION_SHARE` | `share` | 64 (1<<6) | 分享 | 创建与管理分享链接 | `content` |
| `PERMISSION_ACL_MANAGE` | `acl_manage` | 128 (1<<7) | 权限管理 | 设置文件夹描述、可见范围、访问名单与密码 | `content` |
| `PERMISSION_USER_MANAGE` | `user_manage` | 256 (1<<8) | 账号管理 | 创建、修改、禁用、删除账号并分配权限 | `admin` |
| `PERMISSION_AUDIT_READ` | `audit_read` | 512 (1<<9) | 审计日志 | 查看操作审计记录 | `admin` |
| `PERMISSION_STORAGE_MANAGE` | `storage_manage` | 1024 (1<<10) | 存储管理 | 查看全局存储统计并执行维护任务 | `admin` |
| `PERMISSION_SYSTEM_MANAGE` | `system_manage` | 2048 (1<<11) | 系统信息 | 查看系统信息、运行状态与运行参数 | `admin` |

常见组合值：只读 `3`、可上传的访客 `7`、普通用户满配 `255`、管理员默认 `2047`、全量 `4095`。

权限的自助查询接口：`GET /v1/users/permissions/catalog` 返回全部条目与调用方当前的 `granted` / `grantedMask`；`GET /v1/users/roles/list` 返回四个角色预设与 `max_grantable_rank`。

## 3. 账号授权规则

这四条规则在 `internal/biz/user.go` 中实现，任何一条不满足都会中止操作。

| 规则 | 精确表述 | 违反时的错误 |
| --- | --- | --- |
| 严格等级序 | 调用方只能创建、读取、修改、删除 `rank` **严格小于**自己的账号（`target.rank >= actor.rank` 一律拒绝）。`rank` 的赋值同样受此约束：新建与改级都必须 `rank < actor.rank` 且 `rank > 0` | `NETDISK_PERMISSION_DENIED` |
| 只能授予自己拥有的权限 | 新建账号、改权限、改角色时提供的权限集合必须是调用方自身权限的**子集**（`actor.perms.Has(new)`）；`CreateUser` 未显式给出集合时使用角色预设 | `NETDISK_PERMISSION_DENIED` |
| 内置管理员不可管理 | 内置超级管理员 `rank = 1000`，没有任何账号的等级严格高于它，因此它对所有人都是不可管理的（受上一条规则自动覆盖） | `NETDISK_PERMISSION_DENIED` |
| 内置访客不可管理 | 内置访客账号同样被锁死：`UpdateUser`、`SetUserPermissions`、`DeleteUser` 一旦发现目标账号的用户名等于 `auth.admin_username` / `auth.guest_username`（忽略大小写），无论调用方等级多高都直接拒绝。它的角色、等级、权限集与状态由配置决定，服务端每次启动还会校正回来（`internal/data/seed.go`） | `NETDISK_PERMISSION_DENIED` |
| 管理路径拒绝自我管理 | `CanManageUser` 在 `target.id == actor.id` 时直接拒绝；`DeleteUser` 也显式拒绝自我删除。管理类接口（`UpdateUser`、`SetUserPermissions`、`ResetUserPassword`、`DeleteUser`）都不能作用于自己 | `NETDISK_PERMISSION_DENIED` |

补充约束：

- 创建账号额外要求 `PERMISSION_USER_MANAGE`（`CreateUser`、`ListUsers`、`DeleteUser`、`SetUserPermissions`、`ResetUserPassword` 同）；`GetUser`、`GetUserStats` 允许读取自己。
- `ROLE_ADMIN` 只能由 `ROLE_ADMIN` 的调用方创建或指派（`internal/biz/user.go` 显式检查 `in.Role == RoleAdmin && actor.Role != RoleAdmin`）。
- 账号的 `rank` 不会被角色的默认等级覆盖：`UpdateUser` 改角色不会自动改等级，两者都要显式提交。
- 目标账号的会话在以下情况被全部吊销：修改密码、重置密码、`status` 改为非 `ACTIVE`、删除账号。
- 禁用与删除的区别：`USER_STATUS_DISABLED` 保留账号但不能登录（登录返回 `NETDISK_ACCOUNT_DISABLED`）；`USER_STATUS_DELETED` 是软删除，从列表隐藏。
- 修改 `status` 为 `USER_STATUS_DELETED` 会被拒绝（`NETDISK_INVALID_ARGUMENT`），删除必须走 `DeleteUser`。
- 自助修改资料当前没有对应 RPC：管理路径拒绝自我管理，而 `service` 层没有暴露别的入口。

## 4. 节点访问判定顺序

`internal/biz/access.go` 的 `EvaluateNodeAccess` 按下面的顺序计算一个节点上的有效权限；顺序本身就是语义，不能调换。

| 步骤 | 规则 | 实现要点 |
| --- | --- | --- |
| 0. 可见范围折叠 | 沿祖先链取**最严格**的可见范围（`private` < `internal` < `public`，未指定按 `private`），私有父目录中的公开子节点依然不公开 | `effectiveVisibility` |
| 1. 锁定检查 | 祖先链上第一个设有密码且未被本调用方解锁的节点，或节点自身设有密码且未解锁时，`locked = true`；**属主例外**（属主不会被自己设的密码挡住） | 祖先自外向内查找，记录最外层的 `lockedBy` |
| 2. 属主捷径 | 调用方是节点属主时：**立即返回**，权限 = `PermNodeScope ∩ 账号权限`，并清除 `locked` | 账号权限仍是上限：账号被收回 `upload` 后，它自己的文件夹也不能上传 |
| 3. 可见范围授予 | `public`：已认证调用方与匿名调用方都得到 `view\|download`；`internal`：仅已认证调用方得到 `view\|download`；`private`：不给任何额外授予 | 匿名调用方的上限是访客预设（`view\|download`） |
| 4. 显式拒绝 | 节点自身的条目 → 全部生效；祖先条目 → **无论 `inherit` 与否都作用于整棵子树**。所有命中的拒绝位取并集 | `SUBJECT_TYPE_EVERYONE` 匹配任何调用方（含匿名） |
| 5. 显式允许 | 节点自身的条目全部生效；祖先条目仅当 `inherit = true` 时生效。所有命中的允许位取并集 | `SUBJECT_TYPE_ROLE` 用角色机器名做大小写不敏感比较 |
| 6. 合并 | `perms = (可见范围授予 \| 显式允许) &^ 显式拒绝` | 拒绝优先于允许 |
| 7. 账号权限上限 | `perms = perms ∩ PermNodeScope ∩ 账号权限`；匿名调用方以访客预设为上限 | 这一步保证节点权限永远无法超越账号权限 |
| 8. 锁定收敛 | `locked` 为真时 `perms = perms ∩ view` | 未解锁时只保留 `view`，用于展示目录本身；`locked = false` 但权限为零的节点在列表中会被丢弃 |

`RequireAccess` 在判定之后再检查节点状态：回收站中的节点对没有 `trash_manage` 的调用方表现为 `NETDISK_NOT_FOUND`；`locked` 为真返回 `NETDISK_NODE_LOCKED`；权限不足返回 `NETDISK_PERMISSION_DENIED`。

### 4.1 判定示例

示例文件夹 `F`：`visibility = private`、设有密码、属主是内置管理员，访问名单两条（与 `test/integration/smoke_test.go` 中的用例一致）：

| 条目 | 主体 | 效果 | 权限 | `inherit` |
| --- | --- | --- | --- | --- |
| ① | `SUBJECT_TYPE_USER` = alice（`ROLE_MANAGER`，掩码 2047） | `EFFECT_ALLOW` | `view` `download` | true |
| ② | `SUBJECT_TYPE_EVERYONE` | `EFFECT_DENY` | `upload` | true |

**F 保持 `private` 时：**

| 调用方 | 命中条目 | 计算 | 解锁前 | 解锁后 |
| --- | --- | --- | --- | --- |
| 管理员（属主） | 属主捷径，跳过 ①② | `PermNodeScope ∩ 4095` = 255 | 255（属主不受密码限制） | 255 |
| alice | ① 允许 `view\|download`，② 拒绝 `upload` | `(0 \| 3) &^ 4` = 3，再 `∩ 255 ∩ 2047` = 3；解锁前再 `∩ view` | 1（仅 `view`） | 3（`view` `download`） |
| bob（`ROLE_USER`，掩码 255） | 仅② | `(0 \| 0) &^ 4` = 0 | 0 | 0 |
| guest（`ROLE_GUEST`，掩码 3） | 仅② | 0 | 0 | 0 |
| 匿名 | ② 命中（`EVERYONE` 匹配任何人），但不参与上限以外的东西 | `0`；上限为访客预设 3 | 0 | 0 |

结论：`private` 下只有属主与 alice 能看到 `F`，bob、访客与匿名调用方在列表里看不到它。直接按 id 访问（`GetNode`）得到 `NETDISK_PERMISSION_DENIED`；而以 `F` 作为父目录调用 `ListNodes` 时，若 `F` 未解锁会先得到 `NETDISK_NODE_LOCKED`（父目录的锁定检查发生在权限检查之前）。

**把同一个文件夹改成 `visibility = internal`（名单不变）后：**

| 调用方 | 可见范围授予 | 显式允许 | 显式拒绝 | 结果 |
| --- | --- | --- | --- | --- |
| alice | `view\|download` | `view\|download` | `upload` | 3 |
| bob | `view\|download` | 无 | `upload` | 3（只读） |
| guest | `view\|download` | 无 | `upload` | 3（只读） |
| 匿名 | 无（`internal` 不授予匿名） | 无 | `upload` | 0 |

**`F` 的子节点 `C`（自身无任何 ACL，`F` 也没有密码）展示了继承规则：**

| 调用方 | 结果 | 原因 |
| --- | --- | --- |
| alice | `view\|download` | ① 的 `inherit = true`，作为祖先允许条目向下继承 |
| bob | 在 `internal` 下得到 `view\|download`；在 `private` 下得到 0 | ② 的 `EFFECT_DENY` 与 `inherit` 无关，始终作用于整棵子树，因此子树里永远不可能出现 `upload` |

要点回顾：

- **拒绝无条件是子树级的**，允许只有在标记 `inherit` 时才向下传播；
- 节点自身的条目总是生效，与 `inherit` 无关；
- 属主捷径在拒绝之前执行，所以属主不会被自己节点上的拒绝条目挡住（但会被账号权限上限挡住）；
- 锁定收敛发生在最后一步，且只在未解锁时生效。

## 5. 文件夹即节点

文件夹与文件是同一种资源（`NODE_KIND_FOLDER` / `NODE_KIND_FILE`），共用一张表、一棵树、一套访问模型，因此下面四项对文件和文件夹完全等价：

| 能力 | 文件夹 | 文件 | 接口 |
| --- | --- | --- | --- |
| 描述 | `description` 常用来记录用途 | 同样可写 | `CreateFolder.folder.description`、`UpdateNode`（`description` 路径） |
| 可见范围 | 常用（公开目录） | 也支持（单文件公开） | `folder.visibility`、`UpdateNode`（`visibility` 路径） |
| 访问名单 | 常用（名单继承到子树） | 也支持（单文件授权） | `CreateFolder.acl`、`SetNodeAcl`、`GetNodeAcl` |
| 密码 | 常用（加密目录，解锁作用于整棵子树） | 也支持（加密单文件） | `CreateFolder.password`、`UpdateNode`（`password` 路径）、`UnlockNode` |

差别只在行为层面：文件夹有子节点、参与 `move`/`copy` 的子树语义、能被打包下载，并且 `size` 恒为 0；文件有内容对象、历史版本与预览。

关于 `acl_manage` 的判定细节：`SetNodeAcl` 要求调用方在目标节点上持有 `acl_manage`，并且每一条即将写入的条目都必须是调用方自身权限的子集（这条限制**对拒绝条目同样成立**：你不能拒绝一个你自己都没有的权限），条目的权限还必须落在 `PermNodeScope` 内。`recursive` 会把同一组条目复制到子树中每个后代节点上。

## 6. 分享链接的能力边界

分享链接是一个独立的、以令牌鉴权的能力载体，它不读取分享创建者的账号权限：

| 规则 | 说明 |
| --- | --- |
| 只能传达 `view` / `download` / `upload` | 任何超出 `PermShareScope` 的权限都会被裁剪掉；`edit`、`delete`、`trash_manage`、`acl_manage` 与四个管理权限永远无法通过链接传递 |
| 创建时双重校验 | 调用方在节点上要有 `share`；要授予 `download` 还需调用方在节点上有 `download`；要授予 `upload` 则节点必须是文件夹且调用方有 `upload`；最后还要求调用方账号本身持有这些权限 |
| 空集合回落到只读 | 未指定 `permissions` / `permissions_mask` 时按 `view\|download` 处理 |
| 密码只收敛到 `view` | 链接设有密码且未解锁时，有效权限被收敛为 `view`，其余能力在解锁后才生效 |
| 匿名调用方不享受任何账号权限 | 访问者通过链接得到的权限完全来自链接能力集合，与它（可能存在的）账号令牌无关 |
| 链接令牌与账号令牌互不通用 | 链接访问令牌（`access_token`）只承载分享 id，只能用于三个匿名接口 |

## 7. 常见配置示例

### 7.1 让某个人只能看某个文件夹

目标：新建一个只读账号，它登录后只能看到一个指定目录，其他内容都不可见。

1. 用管理员创建账号，角色选访客（或普通用户），权限保持预设只读：`POST /v1/users/create`，`user.role = "ROLE_GUEST"`、`user.rank` 留空（默认 10）或显式给一个低于自己等级的值。
2. 目录保持 `VISIBILITY_PRIVATE`，在它上面写一条允许条目：

```json
POST /v1/nodes/{folder_id}/acl
{
  "entries": [
    {
      "subject_type": "SUBJECT_TYPE_USER",
      "subject_id": "<该账号 id>",
      "effect": "EFFECT_ALLOW",
      "permissions": ["PERMISSION_VIEW", "PERMISSION_DOWNLOAD"],
      "inherit": true
    }
  ]
}
```

3. 结果：该账号只能看到这个目录（目录列表本身要求 `view`），子树通过 `inherit` 一并可见，其他节点因为 `private` 可见范围 + 没有任何允许条目而完全不可见。**注意**：账号仍然能看到并管理自己拥有的节点（属主捷径），所以不要给它别的内容；如果希望它对其他目录只读不可写，保持它的权限为 `3` 即可。
4. 需要连"进都进不去、只在分享里看到"的效果时，不要给账号，改用分享链接（见 7.4）。

### 7.2 让一个组不能上传

目标：在一棵共享目录里禁止某一类账号上传新文件，即使它们账号上有 `upload`。

```json
POST /v1/nodes/{folder_id}/acl
{
  "entries": [
    {
      "subject_type": "SUBJECT_TYPE_ROLE",
      "subject_id": "user",
      "effect": "EFFECT_DENY",
      "permissions": ["PERMISSION_UPLOAD"],
      "inherit": true
    },
    {
      "subject_type": "SUBJECT_TYPE_EVERYONE",
      "effect": "EFFECT_DENY",
      "permissions": ["PERMISSION_UPLOAD"],
      "inherit": true
    }
  ]
}
```

要点：

- `SUBJECT_TYPE_ROLE` 的 `subject_id` 用角色机器名：`admin` / `manager` / `user` / `guest`（大小写不敏感）。
- `EFFECT_DENY` 无论 `inherit` 如何都作用于整棵子树，因此不必为每个子目录重复设置。
- 拒绝条目同样要求调用方自己持有该权限，否则 `SetNodeAcl` 返回 `NETDISK_PERMISSION_DENIED`——上例需要调用方有 `upload` 与 `acl_manage`。
- 属主不受影响：目录属主在自己的目录里仍然可以上传（属主捷径先于拒绝条目执行）。若要让属主也受限，只能从账号权限入手（`POST /v1/users/permissions/set`）。

### 7.3 为什么不能给访客提权（要可上传的共享账号怎么办）

内置访客账号是**锁死**的：`POST /v1/users/permissions/set` 对它一律返回 `NETDISK_PERMISSION_DENIED`，`PUT /v1/users/update`、`DELETE /v1/users/delete` 同样如此——即使调用方是内置管理员。原因很直接：`auth.guest_auto_login` 让任何人不用口令就能拿到这个身份，它的权限一旦可改，等于把「可上传/可删除」的能力开放给所有匿名访问者。

它同时被 `User.manageable` 与 `User.permissionsEditable` 标记为不可管理，所以管理后台里这一行的所有操作都是灰的。

需要「可上传的共享账号」时，建一个自己的账号，别动访客：

```json
POST /v1/users/create
{
  "username": "uploader",
  "password": "<RSA 密文 base64>",
  "role": "ROLE_USER",
  "permissions": ["PERMISSION_VIEW", "PERMISSION_DOWNLOAD", "PERMISSION_UPLOAD", "PERMISSION_EDIT"]
}
```

要点：

- 调用方必须持有目标集合中的每一个权限（内置管理员天然满足）。
- 想关掉免登录访客入口：把配置里的 `auth.guest_auto_login` 改成 `false` 并重启，`GET /v1/auth/config` 的 `guestLoginEnabled` 会同步变 false。
- 访客的默认权限同时受 `storage.guest_quota_bytes` 与账号 `quota_bytes` 约束。
- `GET /v1/users/permissions/catalog` 可以确认改动结果（`granted` / `grantedMask` 是调用方自己的，目标账号要读 `GET /v1/users/{id}`）。

### 7.4 把一个用户提拔为管理员，但不超过自己

目标：让 alice（普通用户）成为可以管理其他账号的 `manager`，但永远低于自己。

```json
PUT /v1/users/update
{
  "user": {
    "id": "<alice id>",
    "role": "ROLE_MANAGER",
    "rank": 500,
    "permissions_mask": "2047"
  },
  "updateMask": "role,rank,permissions_mask"
}
```

要点：

- `rank` 必须严格小于调用方自己的等级；`GET /v1/users/roles/list` 返回的 `maxGrantableRank` 就是可赋的最大值（调用方等级 − 1），界面上可以直接用它做上限。
- 权限集合必须是调用方权限的子集；`2047` 是管理员预设（不含 `system_manage`），如果调用方本身没有某些位，会被拒绝。
- `ROLE_ADMIN` 是例外：只有 `ROLE_ADMIN` 的调用方才能指派，且内置管理员（`rank = 1000`）任何人都不能修改——所以"提拔到与内置管理员同级"无法实现，`rank` 最多给到 999（当调用方是内置管理员时）。
- 被提拔者随即可以管理所有 `rank < 500` 的账号，因此不要把它与目标账号的等级顺序搞反：等级相同或更高的账号它一律管理不了。
- 逆向操作（降权）同样是 `PUT /v1/users/update`，把 `role` / `rank` / `permissions_mask` 改回较低的值；降低等级不会自动吊销会话，但可以显式改 `status` 或重置密码来强制下线。

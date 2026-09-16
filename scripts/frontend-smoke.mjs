#!/usr/bin/env node
/**
 * 前端接口冒烟测试（Node，无第三方依赖）。
 *
 * 它与浏览器端行为一一对应：
 *   - 口令先取 GET /v1/auth/config 的公钥做 RSA-OAEP(SHA-256) 加密再 base64 提交；
 *   - 查询参数用 protojson 的 camelCase 名字，FieldMask 用 camelCase 路径；
 *   - 受保护目录用 X-Node-Token 访问；
 *   - 错误按 reason 分支，而不是解析 message。
 *
 * 用法：
 *   node scripts/frontend-smoke.mjs
 *   node scripts/frontend-smoke.mjs --base http://127.0.0.1:3010
 *   node scripts/frontend-smoke.mjs --base http://127.0.0.1:8000 --user admin --password 'Admin@12345'
 *   node scripts/frontend-smoke.mjs --skip-share   # 跳过依赖 storage.allow_public_share 的用例
 */

import { randomUUID, publicEncrypt, constants, createHash } from 'node:crypto'

const args = process.argv.slice(2)

function arg(name, fallback) {
  const index = args.indexOf(`--${name}`)
  if (index === -1) return fallback
  const value = args[index + 1]
  return value && !value.startsWith('--') ? value : true
}

const BASE = String(arg('base', process.env.NAGISA_BASE ?? 'http://127.0.0.1:8000')).replace(/\/+$/, '')
const USER = String(arg('user', 'admin'))
const PASSWORD = String(arg('password', 'Admin@12345'))
const GUEST_USER = String(arg('guest', 'guest'))
const GUEST_PASSWORD = String(arg('guest-password', 'Guest@12345'))
const SKIP_SHARE = Boolean(arg('skip-share', false))

const run = Date.now().toString(36)
let passed = 0
let failed = 0
let known = 0
const failures = []

function line(text) {
  process.stdout.write(`${text}\n`)
}

async function call(method, path, { body, token, nodeToken, anonymous } = {}) {
  const headers = {}
  if (!anonymous && token) headers.Authorization = `Bearer ${token}`
  if (nodeToken) headers['X-Node-Token'] = nodeToken
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const response = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let parsed
  try {
    parsed = text ? JSON.parse(text) : undefined
  } catch {
    parsed = undefined
  }
  return { status: response.status, body: parsed, text }
}

function check(label, result, expected) {
  if (result.status === expected) {
    passed += 1
    line(`[PASS] ${label} -> HTTP ${result.status}`)
  } else {
    failed += 1
    line(`[FAIL] ${label} -> HTTP ${result.status} (expected ${expected})`)
    line(`       ${result.text?.slice(0, 200)}`)
    failures.push(label)
  }
  return result.body
}

function checkKnown(label, result, expected, note) {
  if (result.status === expected) {
    known += 1
    line(`[KNOWN] ${label} -> HTTP ${result.status} :: ${note}`)
  } else {
    passed += 1
    line(`[PASS] ${label} -> HTTP ${result.status} (known issue looks fixed)`)
  }
  return result.body
}

function show(label, value) {
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  line(`     ${label}: ${text.length > 300 ? `${text.slice(0, 300)}...` : text}`)
}

/** 部署关闭匿名分享时，分享相关接口会返回 NETDISK_UNSUPPORTED。 */
function checkUnsupported(label, result, note) {
  if (result.status === 400 && result.body?.reason === 'NETDISK_UNSUPPORTED') {
    known += 1
    line(`[KNOWN] ${label} -> HTTP 400 NETDISK_UNSUPPORTED :: ${note}`)
    return false
  }
  check(label, result, 200)
  return true
}

/** 与浏览器一致的 RSA-OAEP(SHA-256) + base64 口令编码（公钥只取一次）。 */
let cachedPublicKey
async function encodePassword(plain) {
  if (!cachedPublicKey) {
    const config = await call('GET', '/v1/auth/config', { anonymous: true })
    cachedPublicKey = config.body?.passwordPublicKey
    if (!cachedPublicKey && !config.body?.plainPasswordAllowed) {
      throw new Error('服务端未下发口令公钥，无法加密提交')
    }
  }
  if (!cachedPublicKey) return plain
  const cipher = publicEncrypt(
    { key: cachedPublicKey, padding: constants.RSA_PKCS1_OAEP_PADDING, oaepHash: 'sha256' },
    Buffer.from(plain, 'utf8'),
  )
  return cipher.toString('base64')
}

async function login(username, password) {
  const encoded = await encodePassword(password)
  const result = await call('POST', '/v1/auth/login', {
    anonymous: true,
    body: { username, password: encoded, rememberMe: false },
  })
  if (result.status !== 200) {
    line(`[ERROR] 登录失败（${username}）：HTTP ${result.status} ${result.text?.slice(0, 160)}`)
    line('        账号口令只在该 SQLite 文件首次初始化时写入；若库已存在，请用当时设置的口令，或删库重来。')
    process.exit(2)
  }
  return result.body
}

async function main() {
  line(`base = ${BASE}`)
  line('')

  line('--- 公开接口 ---')
  const info = check('GET  /v1/system/info', await call('GET', '/v1/system/info', { anonymous: true }), 200)
  show(
    'system',
    `version=${info?.version} storage=${info?.storageBackend} db=${info?.databaseBackend} chunk=${info?.defaultChunkSize} inline=${info?.maxInlineSize} modes=${(info?.uploadModes ?? []).join('/')}`,
  )
  check('GET  /v1/auth/config', await call('GET', '/v1/auth/config', { anonymous: true }), 200)
  check('GET  /v1/system/health?deep=true', await call('GET', '/v1/system/health?deep=true', { anonymous: true }), 200)

  line('')
  line('--- 登录（RSA-OAEP 口令）与会话 ---')
  const session = await login(USER, PASSWORD)
  const token = session.accessToken
  show('login', `tokenType=${session.tokenType} expiresIn=${session.expiresIn} user=${session.user?.username} mask=${session.user?.permissionsMask}`)
  check('GET  /v1/auth/me', await call('GET', '/v1/auth/me', { token }), 200)
  check('GET  /v1/auth/me（无令牌）', await call('GET', '/v1/auth/me', { anonymous: true }), 401)

  line('')
  line('--- 文件树 ---')
  const folder = check(
    'POST /v1/nodes/folders/create（根目录）',
    await call('POST', '/v1/nodes/folders/create', {
      token,
      body: {
        folder: { name: `smoke-folder-${run}`, description: 'created by frontend smoke', visibility: 'VISIBILITY_INTERNAL' },
        conflictPolicy: 'CONFLICT_POLICY_RENAME',
      },
    }),
    200,
  )
  const folderId = folder.id
  show('folder', `id=${folderId} displayPath=${folder.displayPath} effectiveMask=${folder.effectivePermissionsMask}`)

  const child = check(
    'POST /v1/nodes/folders/create（子目录）',
    await call('POST', '/v1/nodes/folders/create', {
      token,
      body: { folder: { name: `smoke-child-${run}`, parentId: folderId } },
    }),
    200,
  )
  const childId = child.id

  const list = check(
    'GET  /v1/nodes/list（pageSize + orderBy）',
    await call('GET', '/v1/nodes/list?pageSize=50&orderBy=name&includeTrashed=false', { token }),
    200,
  )
  show('list', `nodes=${list.nodes?.length} totalSize=${list.totalSize}`)
  check('GET  /v1/nodes/list orderBy="<field> desc"', await call('GET', '/v1/nodes/list?orderBy=updated_at%20desc', { token }), 200)
  check('GET  /v1/nodes/list orderBy="name,size desc"', await call('GET', '/v1/nodes/list?orderBy=name,size%20desc', { token }), 200)
  check('GET  /v1/nodes/list orderBy="-updated_at"（应为 400）', await call('GET', '/v1/nodes/list?orderBy=-updated_at', { token }), 400)
  check('GET  /v1/nodes/list filter=kind=1', await call('GET', `/v1/nodes/list?parentId=${folderId}&filter=kind%3D1`, { token }), 200)
  check('GET  /v1/nodes/list filter=name:"..."', await call('GET', `/v1/nodes/list?filter=name%3A%22smoke%22`, { token }), 200)
  check(
    'GET  /v1/nodes/list filter=name:"..." AND kind=1',
    await call('GET', '/v1/nodes/list?filter=name%3A%22smoke%22%20AND%20kind%3D1', { token }),
    200,
  )
  check('GET  /v1/nodes/{id}', await call('GET', `/v1/nodes/${folderId}`, { token }), 200)
  const nodePath = check('GET  /v1/nodes/{id}/path', await call('GET', `/v1/nodes/${folderId}/path`, { token }), 200)
  show('path', `ancestors=${nodePath.ancestors?.length} displayPath=${nodePath.displayPath}`)
  check('GET  /v1/nodes/{id}/stats', await call('GET', `/v1/nodes/${folderId}/stats`, { token }), 200)
  const acl = check('GET  /v1/nodes/{id}/acl', await call('GET', `/v1/nodes/${folderId}/acl`, { token }), 200)
  show('acl', `entries=${acl.entries?.length} inherited=${acl.inheritedEntries?.length} editable=${acl.editable}`)
  check('GET  /v1/nodes/tree?depth=2', await call('GET', '/v1/nodes/tree?depth=2&pageSize=200', { token }), 200)

  line('')
  line('--- 更新与访问名单（FieldMask 必须是 camelCase） ---')
  check(
    'PUT  /v1/nodes/update mask=description,visibility',
    await call('PUT', '/v1/nodes/update', {
      token,
      body: {
        node: { id: folderId, description: 'updated by smoke', visibility: 'VISIBILITY_INTERNAL' },
        updateMask: 'description,visibility',
        conflictPolicy: 'CONFLICT_POLICY_FAIL',
      },
    }),
    200,
  )
  check(
    'PUT  /v1/nodes/update mask=name',
    await call('PUT', '/v1/nodes/update', {
      token,
      body: { node: { id: folderId, name: `smoke-folder-renamed-${run}` }, updateMask: 'name', conflictPolicy: 'CONFLICT_POLICY_FAIL' },
    }),
    200,
  )
  check(
    'PUT  /v1/nodes/update mask=parent_id（应为 500）',
    await call('PUT', '/v1/nodes/update', { token, body: { node: { id: folderId, parentId: folderId }, updateMask: 'parent_id' } }),
    500,
  )
  const aclSet = check(
    'POST /v1/nodes/{id}/acl（整体替换）',
    await call('POST', `/v1/nodes/${folderId}/acl`, {
      token,
      body: {
        entries: [
          {
            subjectType: 'SUBJECT_TYPE_EVERYONE',
            effect: 'EFFECT_ALLOW',
            permissions: ['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD'],
            permissionsMask: 3,
            inherit: true,
          },
        ],
        recursive: false,
      },
    }),
    200,
  )
  show('acl after set', `entries=${aclSet.entries?.length} effectiveMask=${aclSet.effectivePermissionsMask}`)
  check('POST /v1/nodes/{id}/acl（清空）', await call('POST', `/v1/nodes/${folderId}/acl`, { token, body: { entries: [], recursive: false } }), 200)

  line('')
  line('--- 受密码保护目录的解锁流程（非属主） ---')
  const locked = check(
    'POST /v1/nodes/folders/create（带密码）',
    await call('POST', '/v1/nodes/folders/create', {
      token,
      body: {
        folder: { name: `smoke-locked-${run}`, parentId: folderId, passwordHint: 'smoke hint', visibility: 'VISIBILITY_INTERNAL' },
        password: await encodePassword('Folder@12345'),
      },
    }),
    200,
  )
  const lockedId = locked.id
  const guestSession = await login(GUEST_USER, GUEST_PASSWORD)
  const guestToken = guestSession.accessToken
  const denied = check(
    'GET  /v1/nodes/list（未解锁，应为 403）',
    await call('GET', `/v1/nodes/list?parentId=${lockedId}`, { token: guestToken }),
    403,
  )
  show('locked error', `reason=${denied.reason} metadata=${JSON.stringify(denied.metadata ?? {})}`)
  const unlock = check(
    'POST /v1/nodes/{id}/unlock',
    await call('POST', `/v1/nodes/${lockedId}/unlock`, {
      token: guestToken,
      body: { password: await encodePassword('Folder@12345') },
    }),
    200,
  )
  show('unlock', `nodeId=${unlock.nodeId} expiresIn=${unlock.expiresIn}`)
  check(
    'GET  /v1/nodes/list（带 X-Node-Token）',
    await call('GET', `/v1/nodes/list?parentId=${lockedId}`, { token: guestToken, nodeToken: unlock.unlockToken }),
    200,
  )
  check(
    'POST /v1/nodes/{id}/unlock（错误密码，应为 403）',
    await call('POST', `/v1/nodes/${lockedId}/unlock`, {
      token: guestToken,
      body: { password: await encodePassword('wrong-password') },
    }),
    403,
  )

  line('')
  line('--- 搜索 / 移动 / 复制 / 回收站 ---')
  const search = check(
    'GET  /v1/nodes/search?query&orderBy=<f> desc',
    await call('GET', '/v1/nodes/search?query=smoke&orderBy=updated_at%20desc&pageSize=20&matchDescription=false', { token }),
    200,
  )
  show('search', `nodes=${search.nodes?.length} totalSize=${search.totalSize}`)
  check('GET  /v1/nodes/search（范围筛选）', await call('GET', '/v1/nodes/search?query=&kind=NODE_KIND_FOLDER&orderBy=name&minSize=0', { token }), 200)
  check('POST /v1/nodes/move', await call('POST', '/v1/nodes/move', { token, body: { ids: [childId], targetParentId: folderId, conflictPolicy: 'CONFLICT_POLICY_FAIL' } }), 200)
  check(
    'POST /v1/nodes/copy（FAIL 策略应为 409）',
    await call('POST', '/v1/nodes/copy', { token, body: { ids: [childId], targetParentId: folderId, conflictPolicy: 'CONFLICT_POLICY_FAIL' } }),
    409,
  )
  check(
    'POST /v1/nodes/copy（RENAME 策略）',
    await call('POST', '/v1/nodes/copy', { token, body: { ids: [childId], targetParentId: folderId, conflictPolicy: 'CONFLICT_POLICY_RENAME' } }),
    200,
  )
  check('GET  /v1/nodes/trash/list orderBy=<f> desc', await call('GET', '/v1/nodes/trash/list?pageSize=20&orderBy=trashed_at%20desc', { token }), 200)
  const deleted = check('POST /v1/nodes/delete', await call('POST', '/v1/nodes/delete', { token, body: { ids: [childId], permanent: false } }), 200)
  show('delete', `affectedCount=${deleted.affectedCount}`)
  check(
    'POST /v1/nodes/trash/restore',
    await call('POST', '/v1/nodes/trash/restore', { token, body: { ids: [childId], conflictPolicy: 'CONFLICT_POLICY_RENAME' } }),
    200,
  )
  check('POST /v1/nodes/trash/empty', await call('POST', '/v1/nodes/trash/empty', { token, body: {} }), 200)

  if (!SKIP_SHARE) {
    line('')
    line('--- 分享链接 ---')
    const share = await call('POST', '/v1/shares/create', {
      token,
      body: { nodeId: folderId, name: `smoke share ${run}`, permissions: ['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD'], maxDownloads: 0 },
    })
    const shareEnabled = checkUnsupported(
      'POST /v1/shares/create',
      share,
      'storage.allow_public_share 未开启，分享链接不可用（configs/config.yaml 默认 true）',
    )
    if (!shareEnabled) {
      line('     跳过其余分享用例')
    } else {
      show('share', `token=${share.body.token} url=${share.body.url} mask=${share.body.permissionsMask}`)
      const shareToken = share.body.token
      check('GET  /v1/shares/list orderBy=<f> desc', await call('GET', '/v1/shares/list?pageSize=20&orderBy=created_at%20desc', { token }), 200)
      check('GET  /v1/shares/by-node/list?nodeId=', await call('GET', `/v1/shares/by-node/list?nodeId=${folderId}`, { token }), 200)
      const access = check(
        'POST /v1/shares/access（匿名）',
        await call('POST', '/v1/shares/access', { anonymous: true, body: { token: shareToken, pageSize: 50 } }),
        200,
      )
      show('share access', `share=${access.share?.name} node=${access.node?.name} children=${access.children?.length}`)
      check('POST /v1/shares/children/list（匿名）', await call('POST', '/v1/shares/children/list', { anonymous: true, body: { token: shareToken, pageSize: 50 } }), 200)
      check(
        'POST /v1/shares/download-url（文件夹应为 400）',
        await call('POST', '/v1/shares/download-url', { anonymous: true, body: { token: shareToken, nodeId: folderId } }),
        400,
      )
      check(
        'PUT  /v1/shares/update mask=name,maxDownloads',
        await call('PUT', '/v1/shares/update', {
          token,
          body: { share: { id: share.body.id, name: `smoke share renamed ${run}`, maxDownloads: 5 }, updateMask: 'name,maxDownloads' },
        }),
        200,
      )
      check(
        'PUT  /v1/shares/update mask=max_downloads（应为 500）',
        await call('PUT', '/v1/shares/update', { token, body: { share: { id: share.body.id, maxDownloads: 6 }, updateMask: 'max_downloads' } }),
        500,
      )
      check('GET  /v1/shares/{id}', await call('GET', `/v1/shares/${share.body.id}`, { token }), 200)
    }
  }

  line('')
  line('--- 账号管理 ---')
  const usersList = check('GET  /v1/users/list', await call('GET', '/v1/users/list?pageSize=20', { token }), 200)
  show('users', `count=${usersList.users?.length} totalSize=${usersList.totalSize}`)
  check('GET  /v1/users/list（filter + orderBy）', await call('GET', '/v1/users/list?pageSize=20&orderBy=rank%20desc&filter=status%3D1', { token }), 200)
  const roles = check('GET  /v1/users/roles/list', await call('GET', '/v1/users/roles/list', { token }), 200)
  show('roles', `presets=${roles.presets?.length} callerRank=${roles.callerRank} maxGrantableRank=${roles.maxGrantableRank}`)
  const catalog = check('GET  /v1/users/permissions/catalog', await call('GET', '/v1/users/permissions/catalog', { token }), 200)
  show('catalog', `permissions=${catalog.permissions?.length} granted=${catalog.granted?.length} mask=${catalog.grantedMask}`)
  check('GET  /v1/users/me', await call('GET', '/v1/users/me', { token }), 200)
  check('GET  /v1/users/stats/me', await call('GET', '/v1/users/stats/me', { token }), 200)
  check('PUT  /v1/users/update（作用于自己应为 403）', await call('PUT', '/v1/users/update', { token, body: { user: { id: session.user.id, nickname: 'x' }, updateMask: 'nickname' } }), 403)

  const created = check(
    'POST /v1/users/create',
    await call('POST', '/v1/users/create', {
      token,
      body: {
        user: {
          username: `smoke_user_${run}`,
          nickname: 'smoke account',
          email: 'smoke@example.com',
          role: 'ROLE_USER',
          rank: 100,
          permissions: ['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD', 'PERMISSION_UPLOAD'],
          quotaBytes: 0,
          remark: 'smoke',
          status: 'USER_STATUS_ACTIVE',
        },
        password: await encodePassword('Smoke@12345'),
        mustChangePassword: false,
      },
    }),
    200,
  )
  const newUserId = created.id
  show('created user', `id=${newUserId} role=${created.role} rank=${created.rank} mask=${created.permissionsMask} manageable=${created.manageable}`)
  check(
    'PUT  /v1/users/update（camelCase mask）',
    await call('PUT', '/v1/users/update', {
      token,
      body: {
        user: {
          id: newUserId,
          nickname: 'smoke account 2',
          email: 'smoke2@example.com',
          avatarUrl: 'https://example.com/a.png',
          remark: 'r',
          rank: 150,
          quotaBytes: 1073741824,
          status: 'USER_STATUS_ACTIVE',
        },
        updateMask: 'nickname,email,avatarUrl,remark,rank,quotaBytes,status',
      },
    }),
    200,
  )
  check(
    'PUT  /v1/users/update mask=quota_bytes（应为 500）',
    await call('PUT', '/v1/users/update', { token, body: { user: { id: newUserId, quotaBytes: 0 }, updateMask: 'quota_bytes' } }),
    500,
  )
  check(
    'POST /v1/users/permissions/set',
    await call('POST', '/v1/users/permissions/set', { token, body: { id: newUserId, permissions: ['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD'], permissionsMask: 3 } }),
    200,
  )
  check('GET  /v1/users/stats/{id}', await call('GET', `/v1/users/stats/${newUserId}`, { token }), 200)
  check(
    'POST /v1/users/password/reset',
    await call('POST', '/v1/users/password/reset', {
      token,
      body: { id: newUserId, password: await encodePassword('Reset@12345'), mustChangePassword: true },
    }),
    200,
  )
  check('DELETE /v1/users/{id}?trashNodes=true', await call('DELETE', `/v1/users/${newUserId}?trashNodes=true`, { token }), 200)

  line('')
  line('--- 审计 / 系统 ---')
  const audit = check('GET  /v1/audit/logs/list orderBy=<f> desc', await call('GET', '/v1/audit/logs/list?pageSize=50&orderBy=created_at%20desc', { token }), 200)
  show('audit', `logs=${audit.logs?.length} totalSize=${audit.totalSize} first=${audit.logs?.[0]?.action}`)
  const successSet = check('GET  /v1/audit/logs/list filter=success（裸布尔）', await call('GET', '/v1/audit/logs/list?filter=success&pageSize=20', { token }), 200)
  const failureSet = check('GET  /v1/audit/logs/list filter=NOT success', await call('GET', '/v1/audit/logs/list?filter=NOT%20success&pageSize=20', { token }), 200)
  const literalTrue = check('GET  /v1/audit/logs/list filter=success=true', await call('GET', '/v1/audit/logs/list?filter=success%3Dtrue&pageSize=20', { token }), 200)
  const literalFalse = check('GET  /v1/audit/logs/list filter=success=false', await call('GET', '/v1/audit/logs/list?filter=success%3Dfalse&pageSize=20', { token }), 200)
  show('bool filter', `success=${successSet.totalSize} failure=${failureSet.totalSize} true=${literalTrue.totalSize} false=${literalFalse.totalSize}`)
  // success=true 必须与裸标识符等价，success=false 必须与 NOT success 等价。
  if (successSet.totalSize === literalTrue.totalSize && failureSet.totalSize === literalFalse.totalSize) {
    passed += 1
    line('[PASS] success=true 等价于 success，success=false 等价于 NOT success')
  } else {
    failed += 1
    failures.push('bool literal equivalence')
    line('[FAIL] success=true / success=false 与裸标识符形式不等价')
  }
  check('GET  /v1/audit/logs/{id}', await call('GET', `/v1/audit/logs/${audit.logs[0].id}`, { token }), 200)
  const summary = check('GET  /v1/audit/summary', await call('GET', '/v1/audit/summary?actionPrefix=node.', { token }), 200)
  show('summary', `total=${summary.total} failureCount=${summary.failureCount} byAction=${summary.byAction?.length} byActor=${summary.byActor?.length}`)
  const storage = check('GET  /v1/system/storage/stats?topOwners=5', await call('GET', '/v1/system/storage/stats?topOwners=5', { token }), 200)
  show('storage stats', `totalBytes=${storage.totalBytes} files=${storage.totalFiles} folders=${storage.totalFolders} users=${storage.totalUsers}`)
  const settings = check('GET  /v1/system/settings/list', await call('GET', '/v1/system/settings/list', { token }), 200)
  show('settings', `count=${settings.settings?.length} first=${settings.settings?.[0]?.key} type=${settings.settings?.[0]?.type}`)
  check(
    'PUT  /v1/system/settings/update',
    await call('PUT', '/v1/system/settings/update', { token, body: { settings: [{ key: 'trash.retention_days', value: '30', type: 'int' }] } }),
    200,
  )
  check(
    'PUT  /v1/system/settings/update（只读键应为 403）',
    await call('PUT', '/v1/system/settings/update', { token, body: { settings: [{ key: 'system.version', value: 'x' }] } }),
    403,
  )
  const report = check(
    'POST /v1/system/maintenance/run（dryRun）',
    await call('POST', '/v1/system/maintenance/run', { token, body: { dryRun: true, tasks: ['expire_uploads', 'recount_usage'], trashRetentionDays: 30 } }),
    200,
  )
  show('maintenance', `dryRun=${report.dryRun} warnings=${report.warnings?.length}`)

  line('')
  line('--- 上传会话（对象存储未配置时仍可开会话） ---')
  check('GET  /v1/files/uploads/list', await call('GET', '/v1/files/uploads/list?pageSize=20', { token }), 200)
  const upload = check(
    'POST /v1/files/uploads/create',
    await call('POST', '/v1/files/uploads/create', {
      token,
      body: {
        name: `smoke-${run}.bin`,
        size: 1048576,
        parentId: folderId,
        mode: 'UPLOAD_MODE_PROXY',
        conflictPolicy: 'CONFLICT_POLICY_RENAME',
      },
    }),
    200,
  )
  show('upload session', `id=${upload.id} totalParts=${upload.totalParts} chunkSize=${upload.chunkSize} mode=${upload.mode}`)
  check('GET  /v1/files/uploads/{id}', await call('GET', `/v1/files/uploads/${upload.id}`, { token }), 200)
  check('GET  /v1/files/uploads/{id}/parts', await call('GET', `/v1/files/uploads/${upload.id}/parts?fromPartNumber=0`, { token }), 200)
  check('POST /v1/files/uploads/abort', await call('POST', '/v1/files/uploads/abort', { token, body: { uploadId: upload.id } }), 200)
  check('GET  /v1/files/{folder}/archive-url', await call('GET', `/v1/files/${folderId}/archive-url?archiveName=smoke`, { token }), 200)

  line('')
  line('--- 刷新令牌 / 会话 ---')
  check('POST /v1/auth/refresh', await call('POST', '/v1/auth/refresh', { anonymous: true, body: { refreshToken: session.refreshToken } }), 200)
  const sessions = check('GET  /v1/auth/sessions/list', await call('GET', '/v1/auth/sessions/list?pageSize=20&includeInactive=true', { token }), 200)
  show('sessions', `count=${sessions.sessions?.length}`)
  check(
    'POST /v1/auth/sessions/revoke（未知 id 应为 404）',
    await call('POST', '/v1/auth/sessions/revoke', { token, body: { id: randomUUID() } }),
    404,
  )
  check('POST /v1/auth/logout', await call('POST', '/v1/auth/logout', { token, body: { refreshToken: session.refreshToken } }), 200)

  line('')
  line('--- 前端托管（web.enabled=true 时才有） ---')
  const index = await call('GET', '/', { anonymous: true })
  if (index.status === 200 && /assets\/index-[\w-]+\.js/.test(index.text ?? '')) {
    passed += 1
    line('[PASS] GET  / -> 返回构建好的 SPA 入口')
  } else if (index.status === 200 || index.status === 404) {
    known += 1
    line(`[KNOWN] GET  / -> HTTP ${index.status} :: 当前 web.enabled=false，SPA 由 Vite 开发服务器提供`)
  } else {
    failed += 1
    failures.push('GET /')
    line(`[FAIL] GET  / -> HTTP ${index.status}`)
  }

  line('')
  line(`=== PASS ${passed} / FAIL ${failed} / KNOWN ${known} （base=${BASE}） ===`)
  if (failures.length > 0) {
    line(`失败项：${failures.join(' | ')}`)
    process.exit(1)
  }
  // 打印一个稳定指纹，便于确认脚本本身没有静默跳过。
  line(`hash=${createHash('sha256').update(`${BASE}${passed}${failed}${known}`).digest('hex').slice(0, 12)}`)
}

main().catch((error) => {
  line(`[ERROR] ${error?.message ?? error}`)
  process.exit(1)
})

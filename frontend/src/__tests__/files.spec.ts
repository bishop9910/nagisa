import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { initiateUpload, uploadChunk, uploadSmallFile } from '@/api/files'

/**
 * 上传接口的请求体必须是 proto 的字段名。服务端的 codec 会丢掉不认识的字段，
 * AIP 校验器随后只会报“missing required field”，所以本地字段名（例如
 * contentBase64）一旦漏映射就会变成一句看不懂的 400。
 */
describe('上传请求体的字段名', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ id: 'node-1' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function lastBody(): Record<string, unknown> {
    const calls = fetchMock.mock.calls
    const init = calls[calls.length - 1]?.[1] as RequestInit
    return JSON.parse(String(init.body)) as Record<string, unknown>
  }

  it('内联上传把 base64 放进 content，而不是本地的 contentBase64', async () => {
    await uploadSmallFile({ name: 'a.pdf', contentBase64: 'aGVsbG8=', parentId: 'parent-1' })

    const body = lastBody()
    expect(body.content).toBe('aGVsbG8=')
    expect(body).not.toHaveProperty('contentBase64')
    expect(body.name).toBe('a.pdf')
    expect(body.parentId).toBe('parent-1')
  })

  it('分片上传用 content 提交分片内容', async () => {
    await uploadChunk('upload-1', 2, 'aGVsbG8=')

    expect(lastBody().content).toBe('aGVsbG8=')
  })

  it('创建上传会话用 proto 的 JSON 字段名', async () => {
    await initiateUpload({ name: 'a.pdf', size: 1024, chunkSize: 8388608, resumableUploadId: 'upload-1' })

    expect(lastBody()).toMatchObject({
      name: 'a.pdf',
      size: 1024,
      chunkSize: 8388608,
      resumableUploadId: 'upload-1',
    })
  })
})

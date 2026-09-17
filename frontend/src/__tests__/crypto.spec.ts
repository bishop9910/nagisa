/** 口令加密：WebCrypto 路径与无 WebCrypto 时的内置兜底路径。 */

import { constants, createPrivateKey, createPublicKey, generateKeyPairSync, privateDecrypt } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { encryptPassword, hasWebCrypto, passwordEncryptionMode } from '@/utils/crypto'
import { parseRsaPublicKey, rsaOaepEncrypt, sha256 } from '@/utils/rsa'

function newKeyPair(modulusLength = 3072): { publicPem: string; privatePem: string } {
  const { publicKey, privateKey } = generateKeyPairSync('rsa', {
    modulusLength,
    publicKeyEncoding: { type: 'spki', format: 'pem' },
    privateKeyEncoding: { type: 'pkcs8', format: 'pem' },
  })
  return { publicPem: publicKey, privatePem: privateKey }
}

/** 用标准库按 Go 的 rsa.DecryptOAEP(sha256) 解开密文，验证格式真的等价。 */
function decryptOaep(privatePem: string, ciphertext: Buffer): string {
  return privateDecrypt(
    { key: privatePem, padding: constants.RSA_PKCS1_OAEP_PADDING, oaepHash: 'sha256' },
    ciphertext,
  ).toString()
}

function spkiDer(publicPem: string): Uint8Array {
  return new Uint8Array(createPublicKey(publicPem).export({ type: 'spki', format: 'der' }))
}

/** 把全局 crypto 换成「只有 getRandomValues」的样子，即局域网 http 下的真实处境。 */
function dropSubtle(): void {
  const native = globalThis.crypto
  vi.stubGlobal('crypto', {
    getRandomValues: (bytes: Uint8Array<ArrayBuffer>) => native.getRandomValues(bytes),
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('内置 RSA-OAEP 兜底', () => {
  it('SHA-256 与标准实现一致', () => {
    const digest = sha256(new TextEncoder().encode('abc'))
    expect(Buffer.from(digest).toString('hex')).toBe(
      'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
    )
    const empty = sha256(new Uint8Array(0))
    expect(Buffer.from(empty).toString('hex')).toBe(
      'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
    )
  })

  it('能从 PKCS#8 公钥里解析出模数与指数', () => {
    const { publicPem } = newKeyPair(3072)
    const { modulus, exponent } = parseRsaPublicKey(spkiDer(publicPem))
    expect(modulus.toString(2)).toHaveLength(3072)
    expect(exponent).toBe(65537n)
  })

  it('产出的密文能被标准 OAEP(SHA-256) 私钥解开', () => {
    const { publicPem, privatePem } = newKeyPair(3072)
    const cipher = rsaOaepEncrypt(spkiDer(publicPem), new TextEncoder().encode('Admin@12345'))
    expect(cipher).toHaveLength(384)
    expect(decryptOaep(privatePem, Buffer.from(cipher))).toBe('Admin@12345')
  })

  it('两次加密的密文不同（seed 每次都随机）', () => {
    const { publicPem } = newKeyPair(2048)
    const der = spkiDer(publicPem)
    const message = new TextEncoder().encode('same-password')
    const first = Buffer.from(rsaOaepEncrypt(der, message)).toString('base64')
    const second = Buffer.from(rsaOaepEncrypt(der, message)).toString('base64')
    expect(first).not.toBe(second)
  })

  it('超过单块上限时报「口令过长」', () => {
    const { publicPem } = newKeyPair(2048)
    const der = spkiDer(publicPem)
    expect(() => rsaOaepEncrypt(der, new Uint8Array(190))).not.toThrow()
    expect(() => rsaOaepEncrypt(der, new Uint8Array(191))).toThrow('口令过长')
  })

  it('非 RSA 公钥给出明确错误', () => {
    const { publicKey } = generateKeyPairSync('ec', {
      namedCurve: 'P-256',
      publicKeyEncoding: { type: 'spki', format: 'pem' },
      privateKeyEncoding: { type: 'pkcs8', format: 'pem' },
    })
    expect(() => parseRsaPublicKey(spkiDer(publicKey))).toThrow('不是 RSA 公钥')
  })
})

describe('安全上下文', () => {
  it('有 crypto.subtle 时走 WebCrypto', () => {
    expect(hasWebCrypto()).toBe(true)
    expect(passwordEncryptionMode()).toBe('webcrypto')
  })

  it('没有 crypto.subtle 时（局域网 http）自动降级，密文格式不变', async () => {
    const { publicPem, privatePem } = newKeyPair(2048)
    dropSubtle()
    expect(hasWebCrypto()).toBe(false)
    expect(passwordEncryptionMode()).toBe('fallback')

    const cipher = await encryptPassword('Admin@12345', publicPem)
    expect(cipher).not.toContain('Admin@12345')
    expect(decryptOaep(privatePem, Buffer.from(cipher, 'base64'))).toBe('Admin@12345')
  })

  it('降级路径下已加密的口令仍能被 WebCrypto 解开', async () => {
    const { publicPem, privatePem } = newKeyPair(2048)
    dropSubtle()
    const cipher = await encryptPassword('Admin@12345', publicPem)
    vi.unstubAllGlobals()

    const pkcs8 = new Uint8Array(createPrivateKey(privatePem).export({ type: 'pkcs8', format: 'der' }))
    const key = await crypto.subtle.importKey(
      'pkcs8',
      pkcs8 as unknown as BufferSource,
      { name: 'RSA-OAEP', hash: 'SHA-256' },
      false,
      ['decrypt'],
    )
    const plain = await crypto.subtle.decrypt({ name: 'RSA-OAEP' }, key, Buffer.from(cipher, 'base64'))
    expect(new TextDecoder().decode(plain)).toBe('Admin@12345')
  })

  it('公钥为空时给出明确错误', async () => {
    await expect(encryptPassword('secret', '')).rejects.toThrow()
  })
})

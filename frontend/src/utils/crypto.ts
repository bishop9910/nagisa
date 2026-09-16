/**
 * 口令加密。
 *
 * 后端从不接受明文口令：所有口令字段都必须是
 * base64(RSA-OAEP(SHA-256, 明文)) ，公钥由 GET /v1/auth/config 下发
 * （PEM 编码的 PKCS#8，即 "BEGIN PUBLIC KEY"）。
 * 浏览器侧用 WebCrypto 完成同一件事，等价于 Go 的
 * rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, []byte(plain), nil)。
 */

/** 缓存已导入的公钥，避免每次登录都重新解析 PEM。 */
const keyCache = new Map<string, Promise<CryptoKey>>()

function base64ToBytes(base64: string): Uint8Array {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk))
  }
  return btoa(binary)
}

/** PEM → DER。 */
function pemToDer(pem: string): Uint8Array {
  const body = pem
    .replace(/-----BEGIN [^-]+-----/g, '')
    .replace(/-----END [^-]+-----/g, '')
    .replace(/\s+/g, '')
  if (!body) throw new Error('公钥内容为空')
  return base64ToBytes(body)
}

/** 导入 RSA 公钥（按 PEM 内容缓存）。 */
export function importPasswordKey(pem: string): Promise<CryptoKey> {
  const cached = keyCache.get(pem)
  if (cached) return cached
  const promise = (async () => {
    if (typeof crypto === 'undefined' || !crypto.subtle) {
      throw new Error('当前浏览器不支持 WebCrypto，无法加密口令')
    }
    const der = pemToDer(pem)
    return crypto.subtle.importKey(
      'spki',
      der as unknown as BufferSource,
      { name: 'RSA-OAEP', hash: 'SHA-256' },
      false,
      ['encrypt'],
    )
  })()
  keyCache.set(pem, promise)
  // 导入失败时不要留下坏缓存。
  promise.catch(() => keyCache.delete(pem))
  return promise
}

/** 用 PEM 公钥加密口令，返回 base64 密文。 */
export async function encryptPassword(plain: string, publicKeyPem: string): Promise<string> {
  const key = await importPasswordKey(publicKeyPem)
  const encoded = new TextEncoder().encode(plain)
  // RSA-OAEP(SHA-256) 单块上限 = 密钥字节数 - 2*32 - 2；3072 位密钥为 318 字节。
  const maxBytes = (key.algorithm as RsaKeyAlgorithm).modulusLength / 8 - 66
  if (encoded.length > maxBytes) {
    throw new Error(`口令过长（超过 ${maxBytes} 字节）`)
  }
  const cipher = await crypto.subtle.encrypt({ name: 'RSA-OAEP' }, key, encoded as unknown as BufferSource)
  return bytesToBase64(new Uint8Array(cipher))
}

/**
 * 无 WebCrypto 时的 RSA-OAEP(SHA-256) 兜底实现。
 *
 * `crypto.subtle` 只在**安全上下文**里存在：https、`http://localhost`、
 * `http://127.0.0.1` 有，`http://192.168.x.x` 没有。局域网里用别的设备按 IP
 * 访问时，浏览器会故意把 `crypto.subtle` 置成 undefined，登录就会卡在
 * 「无法加密口令」上——这跟浏览器新旧无关。
 *
 * 这里用 BigInt 补齐了同一套原语（SHA-256、MGF1、OAEP 填充、模幂），
 * 输出与 Go 的 `rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, msg, nil)`
 * 逐字节等价，所以后端不需要任何改动。
 *
 * 安全边界（务必清楚）：没有 TLS 就没有服务端身份认证，
 * `GET /v1/auth/config` 下发的公钥本身可能被中间人替换，被动窃听也被
 * 挡不住重放。这条路径只是让功能在纯 HTTP 的局域网里可用，
 * 不等同于 HTTPS，正式方案见 docs/deployment.md §3.3。
 */

const HASH_LENGTH = 32

/** 空字符串的 SHA-256，即 OAEP 里的 lHash（后端 label 传的是 nil）。 */
let emptyHash: Uint8Array | null = null

const SHA256_K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
])

function rotateRight(value: number, bits: number): number {
  return ((value >>> bits) | (value << (32 - bits))) >>> 0
}

/** SHA-256，单块压缩函数的最直白写法。 */
export function sha256(input: Uint8Array): Uint8Array {
  const h = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ])

  const length = input.length
  // 0x80 结尾 + 8 字节大端比特长度，补齐到 64 字节的整数倍。
  const total = Math.ceil((length + 9) / 64) * 64
  const padded = new Uint8Array(total)
  padded.set(input, 0)
  padded[length] = 0x80
  const view = new DataView(padded.buffer)
  const bits = length * 8
  view.setUint32(total - 8, Math.floor(bits / 0x100000000))
  view.setUint32(total - 4, bits >>> 0)

  const w = new Uint32Array(64)
  for (let offset = 0; offset < total; offset += 64) {
    for (let i = 0; i < 16; i += 1) w[i] = view.getUint32(offset + i * 4)
    for (let i = 16; i < 64; i += 1) {
      const x = w[i - 15] as number
      const y = w[i - 2] as number
      const s0 = rotateRight(x, 7) ^ rotateRight(x, 18) ^ (x >>> 3)
      const s1 = rotateRight(y, 17) ^ rotateRight(y, 19) ^ (y >>> 10)
      w[i] = ((w[i - 16] as number) + s0 + (w[i - 7] as number) + s1) >>> 0
    }

    let a = h[0] as number
    let b = h[1] as number
    let c = h[2] as number
    let d = h[3] as number
    let e = h[4] as number
    let f = h[5] as number
    let g = h[6] as number
    let hh = h[7] as number

    for (let i = 0; i < 64; i += 1) {
      const s1 = rotateRight(e, 6) ^ rotateRight(e, 11) ^ rotateRight(e, 25)
      const ch = (e & f) ^ (~e & g)
      const t1 = (hh + s1 + ch + (SHA256_K[i] as number) + (w[i] as number)) >>> 0
      const s0 = rotateRight(a, 2) ^ rotateRight(a, 13) ^ rotateRight(a, 22)
      const maj = (a & b) ^ (a & c) ^ (b & c)
      const t2 = (s0 + maj) >>> 0
      hh = g
      g = f
      f = e
      e = (d + t1) >>> 0
      d = c
      c = b
      b = a
      a = (t1 + t2) >>> 0
    }

    h[0] = ((h[0] as number) + a) >>> 0
    h[1] = ((h[1] as number) + b) >>> 0
    h[2] = ((h[2] as number) + c) >>> 0
    h[3] = ((h[3] as number) + d) >>> 0
    h[4] = ((h[4] as number) + e) >>> 0
    h[5] = ((h[5] as number) + f) >>> 0
    h[6] = ((h[6] as number) + g) >>> 0
    h[7] = ((h[7] as number) + hh) >>> 0
  }

  const digest = new Uint8Array(HASH_LENGTH)
  const out = new DataView(digest.buffer)
  for (let i = 0; i < 8; i += 1) out.setUint32(i * 4, h[i] as number)
  return digest
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((sum, part) => sum + part.length, 0)
  const out = new Uint8Array(total)
  let offset = 0
  for (const part of parts) {
    out.set(part, offset)
    offset += part.length
  }
  return out
}

function xorBytes(left: Uint8Array, right: Uint8Array): Uint8Array {
  const out = new Uint8Array(left.length)
  for (let i = 0; i < left.length; i += 1) out[i] = (left[i] as number) ^ (right[i] as number)
  return out
}

interface Tlv {
  tag: number
  /** 值域起点（跳过 tag 与长度字节）。 */
  start: number
  /** 值域终点（开区间）。 */
  end: number
}

/** 读一个 DER TLV，只支持长度 ≤ 4 字节的长形式。 */
function readTlv(bytes: Uint8Array, offset: number): Tlv {
  if (offset + 2 > bytes.length) throw new Error('公钥 DER 截断')
  const tag = bytes[offset] as number
  let length = bytes[offset + 1] as number
  let cursor = offset + 2
  if (length & 0x80) {
    const count = length & 0x7f
    if (count === 0 || count > 4 || cursor + count > bytes.length) throw new Error('公钥 DER 长度不合法')
    length = 0
    for (let i = 0; i < count; i += 1) length = length * 256 + (bytes[cursor + i] as number)
    cursor += count
  }
  if (cursor + length > bytes.length) throw new Error('公钥 DER 截断')
  return { tag, start: cursor, end: cursor + length }
}

function bytesToBigInt(bytes: Uint8Array): bigint {
  let value = 0n
  for (const byte of bytes) value = (value << 8n) | BigInt(byte)
  return value
}

function bigIntToBytes(value: bigint, length: number): Uint8Array {
  const out = new Uint8Array(length)
  let remaining = value
  for (let i = length - 1; i >= 0; i -= 1) {
    out[i] = Number(remaining & 0xffn)
    remaining >>= 8n
  }
  return out
}

/** rsaEncryption 的 OID 值（1.2.840.113549.1.1.1）。 */
const RSA_OID = new Uint8Array([0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x01])

/**
 * 解析 PKCS#8 SubjectPublicKeyInfo（`-----BEGIN PUBLIC KEY-----`）里的 RSA 公钥。
 * 这正是 internal/pkg/crypt 用 x509.MarshalPKIXPublicKey 产出的格式。
 */
export function parseRsaPublicKey(der: Uint8Array): { modulus: bigint; exponent: bigint } {
  const spki = readTlv(der, 0)
  if (spki.tag !== 0x30) throw new Error('公钥不是 DER SEQUENCE（需要 PKCS#8 的 PUBLIC KEY）')
  const algorithm = readTlv(der, spki.start)
  if (algorithm.tag !== 0x30) throw new Error('公钥算法标识缺失')
  const oid = readTlv(der, algorithm.start)
  const oidValue = der.subarray(oid.start, oid.end)
  const isRsa =
    oid.tag === 0x06 && oidValue.length === RSA_OID.length && RSA_OID.every((byte, i) => byte === oidValue[i])
  if (!isRsa) throw new Error('公钥不是 RSA 公钥，无法用内置实现加密')

  const bitString = readTlv(der, algorithm.end)
  if (bitString.tag !== 0x03) throw new Error('公钥 RSAPublicKey 缺失')
  // BIT STRING 的首字节是「未使用位数」，公钥固定为 0。
  if (der[bitString.start] !== 0) throw new Error('公钥 BIT STRING 不合法')
  const inner = readTlv(der, bitString.start + 1)
  if (inner.tag !== 0x30) throw new Error('公钥 RSAPublicKey 不合法')
  const modulusTlv = readTlv(der, inner.start)
  const exponentTlv = readTlv(der, modulusTlv.end)
  if (modulusTlv.tag !== 0x02 || exponentTlv.tag !== 0x02) throw new Error('公钥缺少模数或指数')

  const modulus = bytesToBigInt(der.subarray(modulusTlv.start, modulusTlv.end))
  const exponent = bytesToBigInt(der.subarray(exponentTlv.start, exponentTlv.end))
  if (modulus <= 0n || exponent < 3n) throw new Error('公钥参数不合法')
  return { modulus, exponent }
}

/** MGF1-SHA256：RFC 8017 附录 B.2.1。 */
function mgf1(seed: Uint8Array, length: number): Uint8Array {
  const out = new Uint8Array(length)
  const counter = new Uint8Array(4)
  for (let offset = 0, index = 0; offset < length; offset += HASH_LENGTH, index += 1) {
    counter[0] = (index >>> 24) & 0xff
    counter[1] = (index >>> 16) & 0xff
    counter[2] = (index >>> 8) & 0xff
    counter[3] = index & 0xff
    const block = sha256(concatBytes(seed, counter))
    out.set(block.subarray(0, Math.min(HASH_LENGTH, length - offset)), offset)
  }
  return out
}

function modularPow(base: bigint, exponent: bigint, modulus: bigint): bigint {
  let result = 1n
  let factor = base % modulus
  let power = exponent
  while (power > 0n) {
    if (power & 1n) result = (result * factor) % modulus
    factor = (factor * factor) % modulus
    power >>= 1n
  }
  return result
}

/**
 * `crypto.getRandomValues` 在任何上下文里都有（被收掉的只有 `subtle`），
 * 所以 OAEP 的 seed 依旧是密码学安全的随机数。
 */
function randomBytes(length: number): Uint8Array {
  if (typeof crypto === 'undefined' || typeof crypto.getRandomValues !== 'function') {
    throw new Error('当前环境没有可用的安全随机数源，无法加密口令')
  }
  const bytes = new Uint8Array(length)
  crypto.getRandomValues(bytes)
  return bytes
}

/**
 * RSA-OAEP(SHA-256) 加密，返回与模数等长的密文。
 * 等价于 Go 的 rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, message, nil)。
 */
export function rsaOaepEncrypt(
  der: Uint8Array,
  message: Uint8Array,
  random: (length: number) => Uint8Array = randomBytes,
): Uint8Array {
  const { modulus, exponent } = parseRsaPublicKey(der)
  const keyLength = Math.ceil(modulus.toString(2).length / 8)
  const maxBytes = keyLength - 2 * HASH_LENGTH - 2
  if (message.length > maxBytes) throw new Error(`口令过长（超过 ${maxBytes} 字节）`)

  emptyHash ??= sha256(new Uint8Array(0))
  const seed = random(HASH_LENGTH)
  if (seed.length !== HASH_LENGTH) throw new Error('随机数源返回的长度不正确')

  // DB = lHash || PS(全零) || 0x01 || M，长度 k - hLen - 1。
  const db = new Uint8Array(keyLength - HASH_LENGTH - 1)
  db.set(emptyHash, 0)
  db[db.length - message.length - 1] = 0x01
  db.set(message, db.length - message.length)

  const maskedDb = xorBytes(db, mgf1(seed, db.length))
  const maskedSeed = xorBytes(seed, mgf1(maskedDb, HASH_LENGTH))

  // EM = 0x00 || maskedSeed || maskedDB，整体按大端整数做模幂。
  const block = concatBytes(new Uint8Array(1), maskedSeed, maskedDb)
  return bigIntToBytes(modularPow(bytesToBigInt(block), exponent, modulus), keyLength)
}

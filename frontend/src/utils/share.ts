/**
 * 分享链接的最终形态。
 *
 * 服务端在 `web.public_base_url` 留空时下发的是相对地址（`/s/<token>`），
 * 浏览器按当前 origin 解析最稳——同一个部署从局域网 IP、localhost、反代域名
 * 访问都对。但「复制链接」需要一个能贴到别处的绝对地址，所以这里把相对地址
 * 补成绝对地址：优先用服务端配置的公网前缀，没配就用当前 origin。
 */

/** 把服务端给的分享地址解析成可以直接复制、打开或跳转的绝对地址。 */
export function resolveShareUrl(share: { url?: string; token?: string }, publicBaseUrl = ''): string {
  const origin = typeof window === 'undefined' ? '' : window.location.origin
  const base = (publicBaseUrl || origin).replace(/\/+$/, '')
  const url = share.url || `/s/${share.token ?? ''}`
  // 已经是绝对地址（服务端配了 public_base_url）就原样用；相对地址补 origin。
  return url.startsWith('/') ? `${base}${url}` : url
}

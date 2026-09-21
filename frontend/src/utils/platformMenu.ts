/** 判断自定义菜单 URL 是否指向当前本地 API 网关。 */
export function isLocalGatewayUrl(rawUrl: string | undefined | null, currentHostname?: string): boolean {
  const value = String(rawUrl ?? '').trim()
  if (!value || value.startsWith('md:')) return false

  // 相对地址始终由当前网关提供，属于本地入口。
  if (value.startsWith('/') && !value.startsWith('//')) return true

  try {
    const url = new URL(value, typeof window !== 'undefined' ? window.location.origin : 'http://localhost')
    const hostname = url.hostname.toLowerCase()
    const localHostname = (currentHostname || (typeof window !== 'undefined' ? window.location.hostname : '')).toLowerCase()
    return (
      hostname === 'localhost' ||
      hostname === '127.0.0.1' ||
      hostname === '0.0.0.0' ||
      hostname === '::1' ||
      (!!localHostname && hostname === localHostname)
    )
  } catch {
    return false
  }
}

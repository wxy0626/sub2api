import { describe, expect, it } from 'vitest'
import { isLocalGatewayUrl } from '../platformMenu'

describe('isLocalGatewayUrl', () => {
  it('识别当前网关的相对地址和本地回环地址', () => {
    expect(isLocalGatewayUrl('/custom/codex', 'sun2api.local')).toBe(true)
    expect(isLocalGatewayUrl('http://localhost:8317', 'sun2api.local')).toBe(true)
    expect(isLocalGatewayUrl('http://127.0.0.1:8317', 'sun2api.local')).toBe(true)
    expect(isLocalGatewayUrl('http://sun2api.local:8317', 'sun2api.local')).toBe(true)
  })

  it('不把外部页面和 Markdown 页面当成本地网关', () => {
    expect(isLocalGatewayUrl('https://example.com', 'sun2api.local')).toBe(false)
    expect(isLocalGatewayUrl('md:guide', 'sun2api.local')).toBe(false)
    expect(isLocalGatewayUrl('', 'sun2api.local')).toBe(false)
  })
})

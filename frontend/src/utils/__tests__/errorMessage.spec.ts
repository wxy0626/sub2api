import { describe, expect, it } from 'vitest'
import { normalizeDisplayErrorMessage } from '../errorMessage'

describe('normalizeDisplayErrorMessage', () => {
  it('将 Codex EOF 错误转换为中文', () => {
    expect(normalizeDisplayErrorMessage('Request failed: backend-api/codex/responses EOF'))
      .toContain('与 ChatGPT 上游服务的连接意外中断')
  })

  it('将未知英文内部错误显示为中文说明与技术详情', () => {
    const message = normalizeDisplayErrorMessage('Unexpected upstream gateway failure')
    expect(message).toContain('上游请求失败')
    expect(message).toContain('技术详情：Unexpected upstream gateway failure')
  })

  it('将上游额度不足错误转换为可操作的中文提示', () => {
    const balanceMessage = normalizeDisplayErrorMessage('API returned 403: {"code":"INSUFFICIENT_BALANCE","message":"Insufficient account balance"}')
    expect(balanceMessage).toContain('上游账号额度不足，请充值或更换账号后重试。')
    expect(balanceMessage).toContain('INSUFFICIENT_BALANCE')

    const quotaMessage = normalizeDisplayErrorMessage('API returned 403: insufficient_user_quota')
    expect(quotaMessage).toContain('上游账号额度不足，请充值或更换账号后重试。')
    expect(quotaMessage).toContain('insufficient_user_quota')
  })

  it('保留已中文化的服务端错误', () => {
    expect(normalizeDisplayErrorMessage('账号授权已过期，请重新授权。'))
      .toBe('账号授权已过期，请重新授权。')
  })

  it('保留管理员后端返回的完整技术详情', () => {
    const message = normalizeDisplayErrorMessage('API returned 401: access_token=secret-token Authorization: Bearer top-secret')
    expect(message).toContain('access_token=secret-token')
    expect(message).toContain('Authorization: Bearer top-secret')
  })
})

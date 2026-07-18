import { describe, expect, it } from 'vitest'
import { normalizeDisplayErrorMessage } from '../errorMessage'

describe('normalizeDisplayErrorMessage', () => {
  it('将 Codex EOF 错误转换为中文', () => {
    expect(normalizeDisplayErrorMessage('Request failed: backend-api/codex/responses EOF'))
      .toContain('与 ChatGPT 上游服务的连接意外中断')
  })

  it('将未知英文内部错误隐藏为中文概述', () => {
    expect(normalizeDisplayErrorMessage('Unexpected upstream gateway failure'))
      .toBe('请求失败，请检查服务端日志中的详细原因。')
  })

  it('保留已中文化的服务端错误', () => {
    expect(normalizeDisplayErrorMessage('账号授权已过期，请重新授权。'))
      .toBe('账号授权已过期，请重新授权。')
  })
})

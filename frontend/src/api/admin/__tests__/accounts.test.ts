import { beforeEach, describe, expect, it, vi } from 'vitest'

const { put } = vi.hoisted(() => ({ put: vi.fn() }))

vi.mock('../../client', () => ({
  apiClient: { put },
  buildApiUrl: (path: string) => `/api/v1${path}`
}))

import { accountsAPI, getFilterOptions, testAccount, updateTestMode } from '../accounts'

describe('admin accounts testAccount', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    put.mockReset()
  })

  it('通过 accountsAPI 聚合暴露列表筛选枚举接口', () => {
    expect(accountsAPI.getFilterOptions).toBe(getFilterOptions)
  })

  it('解析成功 SSE 终态而不是将流式响应当作 JSON', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      'data: {\"type\":\"test_start\"}\n\ndata: {\"type\":\"test_complete\",\"success\":true}\n\n',
      { status: 200, headers: { 'Content-Type': 'text/event-stream' } }
    )))

    await expect(testAccount(8)).resolves.toMatchObject({ success: true, message: '账号测试成功' })
    expect(fetch).toHaveBeenCalledWith('/api/v1/admin/accounts/8/test', expect.objectContaining({
      method: 'POST',
      credentials: 'include'
    }))
  })

  it('将立即测试选定的模型和探测模式发送给后端', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      'data: {\"type\":\"test_complete\",\"success\":true}\n\n',
      { status: 200, headers: { 'Content-Type': 'text/event-stream' } }
    )))

    await testAccount(18, { modelId: 'gpt-5.6-luna', mode: 'default' })

    expect(fetch).toHaveBeenCalledWith('/api/v1/admin/accounts/18/test', expect.objectContaining({
      body: JSON.stringify({ model_id: 'gpt-5.6-luna', mode: 'default' })
    }))
  })

  it('将 /responses 测试模式发送给后端', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      'data: {"type":"test_complete","success":true}\n\n',
      { status: 200, headers: { 'Content-Type': 'text/event-stream' } }
    )))

    await testAccount(19, { modelId: 'gpt-5.6-luna', mode: 'responses' })

    expect(fetch).toHaveBeenCalledWith('/api/v1/admin/accounts/19/test', expect.objectContaining({
      body: JSON.stringify({ model_id: 'gpt-5.6-luna', mode: 'responses' })
    }))
  })

  it('保存 OpenAI 账号模型测试模式时调用专用接口', async () => {
    put.mockResolvedValue({ data: { id: 19, extra: { account_test_mode: 'responses' } } })

    await expect(updateTestMode(19, 'responses')).resolves.toMatchObject({
      id: 19,
      extra: { account_test_mode: 'responses' }
    })
    expect(put).toHaveBeenCalledWith('/admin/accounts/19/test-mode', { mode: 'responses' })
  })

  it('保留 HTTP 200 SSE 内的真实测试错误', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      'data: {\"type\":\"error\",\"error\":\"上游连接超时\"}\n\n',
      { status: 200, headers: { 'Content-Type': 'text/event-stream' } }
    )))

    await expect(testAccount(9)).resolves.toMatchObject({ success: false, message: '上游连接超时' })
  })

  it('缺少完成事件时返回明确失败结果', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      'data: {\"type\":\"test_start\"}\n\n',
      { status: 200, headers: { 'Content-Type': 'text/event-stream' } }
    )))

    await expect(testAccount(10)).resolves.toMatchObject({ success: false, message: '账号测试未返回完成状态，请重试。' })
  })
})

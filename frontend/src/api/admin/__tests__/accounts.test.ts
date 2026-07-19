import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../../client', () => ({
  apiClient: {},
  buildApiUrl: (path: string) => `/api/v1${path}`
}))

import { testAccount } from '../accounts'

describe('admin accounts testAccount', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
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

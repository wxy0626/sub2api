import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('../../client', () => ({
  apiClient: { get },
}))

import { getTestStats, listTestLogs } from '../usage'

describe('管理员账号测试使用记录 API', () => {
  beforeEach(() => {
    get.mockReset().mockResolvedValue({ data: {} })
  })

  it('使用独立测试记录路径并只发送约定筛选字段', async () => {
    const params = {
      start_date: '2026-08-01',
      end_date: '2026-08-02',
      platform: 'deepseek',
      account_id: 61,
      model: 'deepseek-v4-flash',
      success: false,
      page: 1,
      page_size: 20,
      timezone: 'Asia/Shanghai',
    }

    await listTestLogs(params, { signal: new AbortController().signal })

    expect(get).toHaveBeenCalledWith('/admin/usage/test-logs', {
      params,
      signal: expect.any(AbortSignal),
    })
  })

  it('使用独立统计路径并不携带分页或账单字段', async () => {
    const params = {
      start_date: '2026-08-01',
      end_date: '2026-08-02',
      platform: 'grok',
      account_id: 7,
      model: 'grok-4',
      success: true,
      timezone: 'Asia/Shanghai',
    }

    await getTestStats(params)

    expect(get).toHaveBeenCalledWith('/admin/usage/test-stats', { params, signal: undefined })
    expect(params).not.toHaveProperty('page')
    expect(params).not.toHaveProperty('total_cost')
    expect(params).not.toHaveProperty('billing_type')
  })
})

import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

// 同步上游模型失败展示的接口与提示测试 mock。
const { showErrorMock, syncUpstreamModelsMock } = vi.hoisted(() => ({
  showErrorMock: vi.fn(),
  syncUpstreamModelsMock: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels: syncUpstreamModelsMock,
    syncUpstreamModelsPreview: vi.fn(),
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

describe('ModelWhitelistSelector', () => {
  it('uses the upstream response message without a second sync-error wrapper', async () => {
    showErrorMock.mockReset()
    syncUpstreamModelsMock.mockRejectedValue({
      message: '操作失败，请根据下方技术详情定位原因。 技术详情：The origin web server returned an invalid or incomplete response to Cloudflare.'
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountId: 42,
      },
      global: {
        stubs: {
          Icon: true,
          ModelIcon: true,
        },
      },
    })
    const syncButton = wrapper
      .findAll('button')
      .find((candidate) => candidate.text().includes('admin.accounts.syncUpstreamModels'))

    expect(syncButton).toBeDefined()
    await syncButton?.trigger('click')

    await vi.waitFor(() => {
      expect(showErrorMock).toHaveBeenCalledWith(
        '操作失败，请根据下方技术详情定位原因。 技术详情：The origin web server returned an invalid or incomplete response to Cloudflare.'
      )
    })
  })
})

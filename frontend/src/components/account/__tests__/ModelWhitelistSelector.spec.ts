import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const { syncPricingModelsMock, syncUpstreamModelsMock, showErrorMock, showSuccessMock } = vi.hoisted(() => ({
  syncPricingModelsMock: vi.fn(),
  syncUpstreamModelsMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showInfo: vi.fn(),
    showSuccess: showSuccessMock
  })
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels: syncUpstreamModelsMock,
    syncUpstreamModelsPreview: vi.fn()
  },
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('@/api/admin/channels', () => ({
  syncPricingModels: syncPricingModelsMock
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('ModelWhitelistSelector', () => {
  beforeEach(() => {
    syncPricingModelsMock.mockReset()
    syncUpstreamModelsMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
  })

  it('同步最新支持模型会从实时目录替换旧值，并过滤不允许的系列', async () => {
    syncPricingModelsMock.mockResolvedValue({
      models: ['gpt-5.6-luna', 'gpt-5.5', 'gpt-5.4', 'gpt-image-2', 'claude-sonnet-4-6']
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: ['gpt-5.2', 'gpt-5.4'],
        platform: 'openai'
      },
      global: {
        stubs: {
          Icon: true,
          ModelIcon: true
        }
      }
    })

    const latestSyncButton = wrapper.findAll('button')
      .find(button => button.text().includes('admin.accounts.fillRelatedModels'))
    expect(latestSyncButton).toBeDefined()
    await latestSyncButton!.trigger('click')
    await flushPromises()

    expect(syncPricingModelsMock).toHaveBeenCalledWith('openai')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['gpt-5.6-luna', 'gpt-5.5', 'gpt-image-2']
    ])
    expect(showSuccessMock).toHaveBeenCalled()
  })

  it('同步上游支持模型会替换白名单，并移除上游已不支持的模型', async () => {
    syncUpstreamModelsMock.mockResolvedValue({
      models: ['gpt-5.6-luna', 'gpt-image-2', 'gpt-5.4']
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: ['gpt-5.5', 'gpt-5.5-pro', 'gpt-5.6', 'gpt-5.6-luna', 'gpt-image-1', 'gpt-image-2'],
        platform: 'openai',
        accountId: 1
      },
      global: {
        stubs: {
          Icon: true,
          ModelIcon: true
        }
      }
    })

    const upstreamSyncButton = wrapper.findAll('button')
      .find(button => button.text().includes('admin.accounts.syncUpstreamModels'))
    expect(upstreamSyncButton).toBeDefined()
    await upstreamSyncButton!.trigger('click')
    await flushPromises()

    expect(syncUpstreamModelsMock).toHaveBeenCalledWith(1)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['gpt-5.6-luna', 'gpt-image-2']
    ])
  })

  it('OpenAI OAuth 账号不显示上游模型同步入口，也不会发起不受支持的请求', async () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountId: 1,
        accountType: 'oauth'
      },
      global: {
        stubs: {
          Icon: true,
          ModelIcon: true
        }
      }
    })

    const upstreamSyncButton = wrapper.findAll('button')
      .find(button => button.text().includes('admin.accounts.syncUpstreamModels'))
    expect(upstreamSyncButton).toBeUndefined()
    expect(syncUpstreamModelsMock).not.toHaveBeenCalled()
  })
})

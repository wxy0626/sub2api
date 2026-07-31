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
      models: ['gpt-5.6-luna', 'gpt-5.7-preview', 'gpt-5.5', 'gpt-5.4', 'gpt-image-1.5', 'gpt-image-2', 'claude-sonnet-4-6']
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
      ['gpt-5.6-luna', 'gpt-5.7-preview', 'gpt-image-2']
    ])
    expect(showSuccessMock).toHaveBeenCalled()
  })

  it('同步上游支持模型会替换白名单，并移除上游已不支持的模型', async () => {
    syncUpstreamModelsMock.mockResolvedValue({
      models: ['gpt-5.6-luna', 'gpt-5.5', 'gpt-image-1.5', 'gpt-image-2', 'gpt-5.4']
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

  it('DeepSeek 同步最新目录会保留上游模型并去重', async () => {
    syncPricingModelsMock.mockResolvedValue({
      models: ['deepseek-v4-flash', 'deepseek-chat', 'deepseek-v4-flash', '', 'gpt-5.5']
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: ['deepseek-old'],
        platform: 'deepseek'
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

    expect(syncPricingModelsMock).toHaveBeenCalledWith('deepseek')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['deepseek-v4-flash', 'deepseek-chat', 'gpt-5.5']
    ])
  })

  it('多平台同步最新目录会分别过滤后合并结果', async () => {
    syncPricingModelsMock
      .mockResolvedValueOnce({ models: ['gpt-5.6-luna', 'gpt-5.5', 'claude-sonnet-4-6'] })
      .mockResolvedValueOnce({ models: ['deepseek-v4-flash', 'deepseek-chat', 'deepseek-chat'] })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platforms: ['openai', 'deepseek']
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

    expect(syncPricingModelsMock).toHaveBeenNthCalledWith(1, 'openai')
    expect(syncPricingModelsMock).toHaveBeenNthCalledWith(2, 'deepseek')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['gpt-5.6-luna', 'deepseek-v4-flash', 'deepseek-chat']
    ])
  })

  it('DeepSeek 同步上游模型会保留非 GPT 模型并去重', async () => {
    syncUpstreamModelsMock.mockResolvedValue({
      models: ['deepseek-v4-flash', 'deepseek-chat', 'deepseek-chat', '']
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: ['deepseek-old'],
        platform: 'deepseek',
        accountId: 2
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

    expect(syncUpstreamModelsMock).toHaveBeenCalledWith(2)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['deepseek-v4-flash', 'deepseek-chat']
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

  it('Grok API Key 同步到动态模型后，动态模型仍出现在白名单选项中', async () => {
    syncUpstreamModelsMock.mockResolvedValue({ models: ['grok-live-only', 'grok-4.5'] })
    const wrapper = mount(ModelWhitelistSelector, {
      props: { modelValue: [], platform: 'grok', accountId: 60 },
      global: { stubs: { Icon: true, ModelIcon: true } }
    })

    const syncButton = wrapper.findAll('button')
      .find(button => button.text().includes('admin.accounts.syncUpstreamModels'))
    expect(syncButton).toBeDefined()
    await syncButton!.trigger('click')
    await flushPromises()
    await wrapper.find('div.cursor-pointer').trigger('click')

    expect(wrapper.findAll('[data-testid="model-option"]').map(option => option.text())).toEqual(
      expect.arrayContaining(['grok-live-only', 'grok-4.5'])
    )
  })
})

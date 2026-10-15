import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import {
  BUILTIN_PLATFORM_CATALOG,
  resetPlatformCatalog,
  setPlatformCatalog
} from '@/constants/platformCatalog'

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const {
  copyToClipboard,
  showError,
  showSuccess,
  showInfo,
  showWarning,
  syncPricingModels,
  syncUpstreamModels,
  syncUpstreamModelsPreview
} = vi.hoisted(() => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
  showWarning: vi.fn(),
  syncPricingModels: vi.fn(),
  syncUpstreamModels: vi.fn(),
  syncUpstreamModelsPreview: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo,
    showWarning
  })
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels,
    syncUpstreamModelsPreview
  },
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('@/api/admin/channels', () => ({
  syncPricingModels
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copied: ref(false),
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => key === 'common.copy' ? '复制' : key === 'admin.accounts.modelMappingConflict' ? `Model mapping conflict: ${params?.from} → ${params?.to}` : key === 'admin.accounts.syncUpstreamModelsError' ? `${key}: ${params?.message}` : key
    })
  }
})


function mountSelector(props: Record<string, unknown> = {}) {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai',
      ...props
    },
    global: {
      stubs: {
        ModelIcon: true
      }
    }
  })
}

describe('ModelWhitelistSelector', () => {
  beforeEach(() => {
    syncPricingModels.mockReset()
    syncUpstreamModels.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    copyToClipboard.mockReset()
  })

  afterEach(() => {
    resetPlatformCatalog()
  })

  it.each(['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'command_code', 'cline'])(
    'supports upstream sync for %s saved accounts and creation previews',
    (platform) => {
      const wrappers = [
        mountSelector({ platform, accountId: 46 }),
        mountSelector({ platform, syncCredentials: { platform, type: 'apikey', api_key: 'test-key' } })
      ]
      for (const wrapper of wrappers) {
        expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.syncUpstreamModels')).toBe(true)
        wrapper.unmount()
      }
    }
  )

  it.each(['typesafe', 'unregistered'])(
    'hides upstream sync for unsupported %s saved accounts and creation previews',
    (platform) => {
      const wrappers = [
        mountSelector({ platform, accountId: 46 }),
        mountSelector({ platform, syncCredentials: { platform, type: 'apikey', api_key: 'test-key' } })
      ]
      for (const wrapper of wrappers) {
        expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.syncUpstreamModels')).toBe(false)
        wrapper.unmount()
      }
      expect(syncUpstreamModels).not.toHaveBeenCalled()
      expect(syncUpstreamModelsPreview).not.toHaveBeenCalled()
    }
  )

  it('requires a supported request builder for newly registered platforms', async () => {
    const wrapper = mountSelector({ platform: 'acme_router', accountId: 46 })
    const platforms = [
      ...BUILTIN_PLATFORM_CATALOG.platforms,
      { id: 'acme_router', display_name: 'Acme Router', gateway: 'openai' as const, cn_provider: false }
    ]
    setPlatformCatalog({ ...BUILTIN_PLATFORM_CATALOG, platforms })
    await flushPromises()
    expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.syncUpstreamModels')).toBe(false)

    setPlatformCatalog({
      ...BUILTIN_PLATFORM_CATALOG,
      platforms: platforms.map(spec => spec.id === 'acme_router'
        ? { ...spec, multi_protocol: { default_mode: 'default', routing: 'by_inbound', modes: [] } }
        : spec)
    })
    await flushPromises()
    expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.syncUpstreamModels')).toBe(true)
    wrapper.unmount()
  })

  it('rejects a custom whitelist model that is already mapped to a different target', async () => {
    const wrapper = mountSelector({ modelMappings: [{ from: 'gpt-latest', to: 'deepseek-chat' }] })
    await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue(' gpt-latest ')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(showInfo).toHaveBeenCalledWith(expect.stringContaining('gpt-latest → deepseek-chat'))
  })

  it('keeps the existing duplicate identity warning before checking mappings', async () => {
    const wrapper = mountSelector({ modelValue: ['gpt-latest'], modelMappings: [{ from: 'gpt-latest', to: 'deepseek-chat' }] })
    await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue('gpt-latest')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')
    expect(showInfo).toHaveBeenCalledWith('admin.accounts.modelExists')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('allows matching identity mapping as a whitelist model', async () => {
    const wrapper = mountSelector({ modelMappings: [{ from: 'gpt-latest', to: 'gpt-latest' }] })
    await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue('gpt-latest')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-latest']]])
  })

  it('still allows custom models without a mapping prop', async () => {
    const wrapper = mountSelector()
    await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue('custom-model')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[['custom-model']]])
  })

  it('copies a model ID without selecting the model', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = wrapper
      .findAll('[data-testid="model-option"]')
      .find(option => option.text().includes('gpt-5.6-sol'))
    expect(row).toBeDefined()

    const copyButton = row!.get('[data-testid="copy-model-id"]')
    expect(copyButton.attributes('aria-label')).toBe('复制 gpt-5.6-sol')
    await copyButton.trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.6-sol')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('同步最新支持模型会从实时目录替换旧值，并只做非空去重', async () => {
    syncPricingModels.mockResolvedValue({
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

    expect(syncPricingModels).toHaveBeenCalledWith('openai')
    // 前端不再按 GPT 系列过滤：OpenAI 兼容端点可能返回 Gemini、Claude 等合法模型。
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['gpt-5.6-luna', 'gpt-5.7-preview', 'gpt-5.5', 'gpt-5.4', 'gpt-image-1.5', 'gpt-image-2', 'claude-sonnet-4-6']
    ])
    expect(showSuccess).toHaveBeenCalled()
  })

  it('同步上游支持模型会替换白名单，并移除上游已不支持的模型', async () => {
    syncUpstreamModels.mockResolvedValue({
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

    expect(syncUpstreamModels).toHaveBeenCalledWith(1)
    // 上游结果整体替换旧白名单，gpt-5.5-pro / gpt-5.6 / gpt-image-1 等不再支持的模型被移除。
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['gpt-5.6-luna', 'gpt-5.5', 'gpt-image-1.5', 'gpt-image-2', 'gpt-5.4']
    ])
  })

  it('DeepSeek 同步最新目录会保留上游模型并去重', async () => {
    syncPricingModels.mockResolvedValue({
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

    expect(syncPricingModels).toHaveBeenCalledWith('deepseek')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['deepseek-v4-flash', 'deepseek-chat', 'gpt-5.5']
    ])
  })

  it('多平台同步最新目录会分别清理后合并并去重', async () => {
    syncPricingModels
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

    expect(syncPricingModels).toHaveBeenNthCalledWith(1, 'openai')
    expect(syncPricingModels).toHaveBeenNthCalledWith(2, 'deepseek')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      ['gpt-5.6-luna', 'gpt-5.5', 'claude-sonnet-4-6', 'deepseek-v4-flash', 'deepseek-chat']
    ])
  })

  it('DeepSeek 同步上游模型会保留非 GPT 模型并去重', async () => {
    syncUpstreamModels.mockResolvedValue({
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

    expect(syncUpstreamModels).toHaveBeenCalledWith(2)
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
    expect(syncUpstreamModels).not.toHaveBeenCalled()
  })

  it('Grok API Key 同步到动态模型后，动态模型仍出现在白名单选项中', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['grok-live-only', 'grok-4.5'] })
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

  it('点击已选模型名复制模型名且不下拉展开列表', async () => {
    copyToClipboard.mockResolvedValue(true)
    const wrapper = mount(ModelWhitelistSelector, {
      props: { modelValue: ['gpt-5.2'], platform: 'openai' },
      global: { stubs: { Icon: true, ModelIcon: true } }
    })

    const modelNameSpan = wrapper.findAll('span.cursor-pointer')
      .find(span => span.text() === 'gpt-5.2')
    expect(modelNameSpan).toBeDefined()
    await modelNameSpan!.trigger('click')

    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.2')
    expect(wrapper.find('[data-testid="model-option"]').exists()).toBe(false)
  })

  it('点击 X 按钮从白名单移除该模型', async () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: { modelValue: ['gpt-5.2', 'gpt-5.4'], platform: 'openai' },
      global: { stubs: { Icon: true, ModelIcon: true } }
    })

    const removeButtons = wrapper.findAll('span.inline-flex > button')
    expect(removeButtons.length).toBe(2)
    await removeButtons[0].trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['gpt-5.4']])
  })

  it('点击下拉区域展开模型列表', async () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: { modelValue: ['gpt-5.2'], platform: 'openai' },
      global: { stubs: { Icon: true, ModelIcon: true } }
    })

    expect(wrapper.find('[data-testid="model-option"]').exists()).toBe(false)
    await wrapper.find('div.cursor-pointer').trigger('click')
    expect(wrapper.find('[data-testid="model-option"]').exists()).toBe(true)
  })

  it('warns when model IDs sync but capability metadata is incomplete', async () => {
    syncUpstreamModels.mockResolvedValue({
      models: ['x-preview-f-free'],
      warnings: [
        {
          code: 'upstream_model_metadata_incomplete',
          message: 'Model IDs were synced, but capability metadata could not be updated.'
        }
      ]
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountId: 46
      },
      global: {
        stubs: {
          ModelIcon: true
        }
      }
    })

    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')
    expect(syncButton).toBeDefined()
    await syncButton!.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')).toEqual([[['x-preview-f-free']]])
    expect(showWarning).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsMetadataIncomplete')
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('shows the backend message when upstream sync is rejected', async () => {
    syncUpstreamModels.mockRejectedValue({
      status: 400,
      code: 'BAD_REQUEST',
      message: 'Vertex API key accounts do not support upstream model listing'
    })
    const wrapper = mountSelector({ platform: 'gemini', accountId: 174 })

    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')
    expect(syncButton).toBeDefined()
    await syncButton!.trigger('click')
    await flushPromises()

    // 本地约定：中文说明 + 后端返回的原始技术详情，技术字段保留英文便于排查。
    expect(showError).toHaveBeenCalledWith(
      'admin.accounts.syncUpstreamModelsError: 上游请求失败，请根据下方技术详情检查账号、模型、代理或上游服务。\n技术详情：Vertex API key accounts do not support upstream model listing'
    )
  })

  it('shows success and a partial warning when some capabilities were saved', async () => {
    syncUpstreamModels.mockResolvedValue({
      models: ['gpt-6-astra', 'gpt-image-2'],
      warnings: [
        {
          code: 'upstream_model_metadata_partial',
          message: 'Some model capabilities were saved; remaining models are still incomplete.'
        }
      ]
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountId: 46
      },
      global: {
        stubs: {
          ModelIcon: true
        }
      }
    })

    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')
    expect(syncButton).toBeDefined()
    await syncButton!.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-6-astra', 'gpt-image-2']]])
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsSuccess')
    expect(showWarning).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsMetadataPartial')
  })

  it('reports a successful preview so account creation can persist metadata', async () => {
    syncUpstreamModelsPreview.mockResolvedValue({
      models: ['x-preview-f-free'],
      metadata: {
        'x-preview-f-free': {
          id: 'x-preview-f-free',
          reasoning: true,
          supported_reasoning_levels: ['low', 'high', 'max'],
        },
      },
    })
    const wrapper = mountSelector({
      syncCredentials: {
        platform: 'openai',
        type: 'apikey',
        base_url: 'https://opencode.ai/zen/v1',
        api_key: 'test-key',
      },
    })
    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')

    expect(syncButton).toBeDefined()
    await syncButton?.trigger('click')
    await flushPromises()

    expect(syncUpstreamModelsPreview).toHaveBeenCalledOnce()
    expect(wrapper.emitted('upstream-synced')).toEqual([[]])
    expect(wrapper.emitted('update:modelValue')).toEqual([[['x-preview-f-free']]])
  })

  it('shows the upstream sync button for OpenCode Go create-account credentials', () => {
    const wrapper = mountSelector({
      platform: 'opencode_go',
      syncCredentials: {
        platform: 'opencode_go',
        type: 'apikey',
        base_url: 'https://opencode.ai/zen/go/v1',
        api_key: 'sk-test',
      },
    })
    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')

    expect(syncButton).toBeDefined()
    expect(syncButton?.exists()).toBe(true)
  })
})

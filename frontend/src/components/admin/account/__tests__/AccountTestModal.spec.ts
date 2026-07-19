import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'

const { getAvailableModels, updateTestMode, copyToClipboard } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  updateTestMode: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { getAvailableModels, updateTestMode } }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const createStreamResponse = (lines: string[]) => {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0
  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => (
          index < chunks.length
            ? { done: false, value: chunks[index++] }
            : { done: true, value: undefined }
        ))
      })
    }
  } as Response
}

describe('AccountTestModal', () => {
  beforeEach(() => {
    getAvailableModels.mockReset()
    getAvailableModels.mockResolvedValue([{ id: 'gpt-5.6-luna', display_name: 'GPT-5.6 Luna' }])
    updateTestMode.mockReset()
    updateTestMode.mockImplementation(async (_id: number, mode: string) => ({ id: 43, extra: { account_test_mode: mode } }))
    copyToClipboard.mockReset()
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      value: { getItem: vi.fn(() => 'test-token') }
    })
    global.fetch = vi.fn().mockResolvedValue(createStreamResponse([
      'data: {"type":"test_complete","success":true}\n'
    ])) as any
  })

  it('开始和完成模型测试时同步 testing-changed 状态', async () => {
    const wrapper = mount(AccountTestModal, {
      props: {
        show: false,
        account: { id: 42, name: 'OpenAI', platform: 'openai', type: 'oauth', status: 'active' } as any
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Select: true,
          TextArea: true,
          Icon: true
        }
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('testing-changed')).toEqual([[true], [false]])
  })

  it('选择 /responses 测试时将模式写入弹窗请求体', async () => {
    const wrapper = mount(AccountTestModal, {
      props: {
        show: false,
        account: { id: 43, name: 'OpenAI API Key', platform: 'openai', type: 'apikey', status: 'active' } as any
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Select: true,
          TextArea: true,
          Icon: true
        }
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    ;(wrapper.vm as any).testMode = 'responses'
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledWith('/api/v1/admin/accounts/43/test', expect.objectContaining({
      body: JSON.stringify({ model_id: 'gpt-5.6-luna', prompt: '', mode: 'responses' })
    }))
  })

  it('打开时读取保存的模式，切换后立即保存并回传完整账号', async () => {
    const account = {
      id: 44,
      name: 'OpenAI API Key',
      platform: 'openai',
      type: 'apikey',
      status: 'active',
      extra: { account_test_mode: 'responses' }
    }
    updateTestMode.mockResolvedValue({ ...account, extra: { account_test_mode: 'compact' } })
    const wrapper = mount(AccountTestModal, {
      props: { show: false, account },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }, Select: true, TextArea: true, Icon: true } }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    expect((wrapper.vm as any).testMode).toBe('responses')

    ;(wrapper.vm as any).handleTestModeChange('compact')
    await flushPromises()

    expect(updateTestMode).toHaveBeenCalledWith(44, 'compact')
    expect(wrapper.emitted('account-updated')?.[0]?.[0]).toMatchObject({
      id: 44,
      extra: { account_test_mode: 'compact' }
    })
  })

  it('保存最终选择失败时回滚到上一次已保存的模式并显示具体错误', async () => {
    updateTestMode.mockRejectedValue(new Error('HTTP 403: forbidden'))
    const wrapper = mount(AccountTestModal, {
      props: {
        show: false,
        account: { id: 45, name: 'OpenAI', platform: 'openai', type: 'apikey', status: 'active', extra: { account_test_mode: 'responses' } }
      },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }, Select: true, TextArea: true, Icon: true } }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    ;(wrapper.vm as any).handleTestModeChange('compact')
    await flushPromises()

    expect((wrapper.vm as any).testMode).toBe('responses')
    expect((wrapper.vm as any).errorMessage).toContain('没有执行此操作的权限')
  })
})

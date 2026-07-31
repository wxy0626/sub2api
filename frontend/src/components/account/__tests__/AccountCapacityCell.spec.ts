import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { Account } from '@/types'
import AccountCapacityCell from '../AccountCapacityCell.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'admin.accounts.columns.capacity': '并发负载',
      'admin.accounts.concurrencyLimit': '并发上限',
      'admin.accounts.loadFactor': '负载因子',
      'admin.accounts.loadFactorFollowConcurrency': '默认跟随并发容量',
      'admin.accounts.concurrencyCapacityInvalid': '并发容量必须是正整数，负载因子必须是 1 到 10000 的整数。',
      'common.cancel': '取消',
      'common.save': '保存'
    }[key] ?? key)
  })
}))

// 构造只覆盖容量编辑所需字段的账号，避免测试被无关账号属性干扰。
const createAccount = (overrides: Partial<Account> = {}): Account => ({
  id: 1,
  name: 'test-account',
  platform: 'openai',
  type: 'apikey',
  proxy_id: null,
  concurrency: 5,
  load_factor: null,
  current_concurrency: 1,
  ...overrides
} as Account)

// 每个用例独立维护挂载实例，避免 Teleport 浮层残留到下一个用例。
const mountedWrappers: Array<{ unmount: () => void }> = []

// 创建可编辑容量单元格，并保留实例供用例结束时清理。
const mountCell = (account: Account = createAccount(), saving = false) => {
  const wrapper = mount(AccountCapacityCell, {
    props: { account, editable: true, saving },
    global: {
      stubs: {
        CapacityBadge: { props: ['current', 'max'], template: '<span><slot />{{ current }}/{{ max }}</span>' },
        QuotaBadge: true
      }
    }
  })
  mountedWrappers.push(wrapper)
  return wrapper
}

// Teleport 内容在 document.body 中，测试通过页面层节点验证它没有撑高单元格。
const getBodyElement = <T extends HTMLElement>(testId: string): T | null =>
  document.body.querySelector<T>('[data-testid="' + testId + '"]')

const openEditor = async (wrapper: ReturnType<typeof mountCell>) => {
  await wrapper.get('[data-testid="account-capacity-display"]').trigger('click')
  await nextTick()
  const editor = getBodyElement<HTMLElement>('account-capacity-editor')
  expect(editor).not.toBeNull()
  return editor!
}

afterEach(() => {
  mountedWrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  document.body.querySelectorAll('[data-testid="account-capacity-editor"]').forEach((node) => node.remove())
})

describe('AccountCapacityCell', () => {
  it('点击容量徽标后通过 Teleport 显示独立浮层，不改变单元格布局', async () => {
    const wrapper = mountCell(createAccount({ current_concurrency: 0 }))

    expect(wrapper.get('[data-testid="account-capacity-display"]').text()).toContain('0/5')
    expect(wrapper.find('[data-testid="account-capacity-editor"]').exists()).toBe(false)

    const editor = await openEditor(wrapper)

    expect(editor.textContent).toContain('并发上限')
    expect(editor.textContent).toContain('负载因子')
    expect(getBodyElement<HTMLInputElement>('concurrency-input')?.value).toBe('5')
    expect(getBodyElement<HTMLButtonElement>('account-capacity-save')).not.toBeNull()
    expect(wrapper.find('[data-testid="account-capacity-editor"]').exists()).toBe(false)
    expect(editor.className).toContain('fixed')
  })

  it('点击取消会关闭浮层并丢弃草稿', async () => {
    const wrapper = mountCell(createAccount({ current_concurrency: 0 }))

    await openEditor(wrapper)
    const concurrencyInput = getBodyElement<HTMLInputElement>('concurrency-input')!
    concurrencyInput.value = '8'
    concurrencyInput.dispatchEvent(new Event('input', { bubbles: true }))
    getBodyElement<HTMLButtonElement>('account-capacity-cancel')!.click()
    await nextTick()

    expect(getBodyElement('account-capacity-editor')).toBeNull()
    expect(wrapper.get('[data-testid="account-capacity-display"]').text()).toContain('0/5')
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('默认负载因子跟随并发容量，保存时清除显式覆盖', async () => {
    const wrapper = mountCell()

    await openEditor(wrapper)
    const concurrencyInput = getBodyElement<HTMLInputElement>('concurrency-input')!
    concurrencyInput.value = '8'
    concurrencyInput.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(getBodyElement<HTMLInputElement>('load-factor-input')?.value).toBe('8')
    getBodyElement<HTMLButtonElement>('account-capacity-save')!.click()
    await nextTick()

    expect(wrapper.emitted('save')).toEqual([[{ concurrency: 8, load_factor: 0 }]])
  })

  it('显式填写负载因子时独立保存', async () => {
    const wrapper = mountCell(createAccount({ load_factor: 20 }))

    await openEditor(wrapper)
    const concurrencyInput = getBodyElement<HTMLInputElement>('concurrency-input')!
    concurrencyInput.value = '7'
    concurrencyInput.dispatchEvent(new Event('input', { bubbles: true }))
    getBodyElement<HTMLButtonElement>('account-capacity-save')!.click()
    await nextTick()

    expect(wrapper.emitted('save')).toEqual([[{ concurrency: 7, load_factor: 20 }]])
  })

  it('清空与并发容量相等的显式负载因子时仍会恢复跟随模式', async () => {
    const wrapper = mountCell(createAccount({ load_factor: 5 }))

    await openEditor(wrapper)
    const loadFactorInput = getBodyElement<HTMLInputElement>('load-factor-input')!
    loadFactorInput.value = ''
    loadFactorInput.dispatchEvent(new Event('input', { bubbles: true }))
    getBodyElement<HTMLButtonElement>('account-capacity-save')!.click()
    await nextTick()

    expect(wrapper.emitted('save')).toEqual([[{ concurrency: 5, load_factor: 0 }]])
  })

  it('非法输入显示中文原因且不提交', async () => {
    const wrapper = mountCell()

    await openEditor(wrapper)
    const loadFactorInput = getBodyElement<HTMLInputElement>('load-factor-input')!
    loadFactorInput.value = '10001'
    loadFactorInput.dispatchEvent(new Event('input', { bubbles: true }))
    getBodyElement<HTMLButtonElement>('account-capacity-save')!.click()
    await nextTick()

    expect(wrapper.emitted('save')).toBeUndefined()
    expect(getBodyElement('account-capacity-error')?.textContent).toContain('负载因子必须是 1 到 10000 的整数')
    expect(getBodyElement('account-capacity-editor')).not.toBeNull()
  })

  it('点击外部区域或按 Escape 会关闭浮层并恢复草稿', async () => {
    const wrapper = mountCell(createAccount({ current_concurrency: 0 }))

    await openEditor(wrapper)
    const concurrencyInput = getBodyElement<HTMLInputElement>('concurrency-input')!
    concurrencyInput.value = '8'
    concurrencyInput.dispatchEvent(new Event('input', { bubbles: true }))
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(getBodyElement('account-capacity-editor')).toBeNull()

    await openEditor(wrapper)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()

    expect(getBodyElement('account-capacity-editor')).toBeNull()
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('保存期间禁用浮层中的输入和操作按钮', async () => {
    const wrapper = mountCell()

    await openEditor(wrapper)
    await wrapper.setProps({ saving: true })
    await nextTick()
    expect(getBodyElement<HTMLInputElement>('concurrency-input')?.disabled).toBe(true)
    expect(getBodyElement<HTMLInputElement>('load-factor-input')?.disabled).toBe(true)
    expect(getBodyElement<HTMLButtonElement>('account-capacity-cancel')?.disabled).toBe(true)
    expect(getBodyElement<HTMLButtonElement>('account-capacity-save')?.disabled).toBe(true)
  })
})

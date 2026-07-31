import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Account } from '@/types'
import AccountCapacityCell from '../AccountCapacityCell.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'admin.accounts.concurrencyLimit': '并发上限',
      'admin.accounts.loadFactor': '负载因子'
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

describe('AccountCapacityCell', () => {
  it('默认只读显示当前占用和容量，点击后才显示编辑控件', async () => {
    const wrapper = mount(AccountCapacityCell, {
      props: {
        account: createAccount({ current_concurrency: 0 }),
        editable: true
      },
      global: {
        stubs: {
          CapacityBadge: { props: ['current', 'max'], template: '<span><slot />{{ current }}/{{ max }}</span>' },
          QuotaBadge: true
        }
      }
    })

    expect(wrapper.get('[data-testid="account-capacity-display"]').text()).toContain('0/5')
    expect(wrapper.find('[data-testid="account-capacity-editor"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="concurrency-input"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="account-capacity-save"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="account-capacity-cancel"]').exists()).toBe(false)

    await wrapper.get('[data-testid="account-capacity-display"]').trigger('click')

    expect(wrapper.find('[data-testid="account-capacity-editor"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="account-capacity-editor"]').text()).toContain('并发上限')
    expect(wrapper.get('[data-testid="account-capacity-editor"]').text()).toContain('负载因子')
    expect(wrapper.find('[data-testid="concurrency-input"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="account-capacity-save"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="account-capacity-cancel"]').exists()).toBe(true)
  })

  it('取消编辑会丢弃草稿并恢复只读容量显示', async () => {
    const wrapper = mount(AccountCapacityCell, {
      props: {
        account: createAccount({ current_concurrency: 0 }),
        editable: true
      },
      global: {
        stubs: {
          CapacityBadge: { props: ['current', 'max'], template: '<span><slot />{{ current }}/{{ max }}</span>' },
          QuotaBadge: true
        }
      }
    })

    await wrapper.get('[data-testid="account-capacity-display"]').trigger('click')
    await wrapper.get('[data-testid="concurrency-input"]').setValue('8')
    await wrapper.get('[data-testid="account-capacity-cancel"]').trigger('click')

    expect(wrapper.get('[data-testid="account-capacity-display"]').text()).toContain('0/5')
    expect(wrapper.find('[data-testid="account-capacity-editor"]').exists()).toBe(false)
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('defaults load factor to concurrency and clears explicit override when saving', async () => {
    const wrapper = mount(AccountCapacityCell, {
      props: {
        account: createAccount(),
        editable: true
      },
      global: {
        stubs: {
          CapacityBadge: { props: ['current', 'max'], template: '<span><slot />{{ current }}/{{ max }}</span>' },
          QuotaBadge: true
        }
      }
    })

    await wrapper.get('[data-testid="account-capacity-display"]').trigger('click')
    expect((wrapper.get('[data-testid="load-factor-input"]').element as HTMLInputElement).value).toBe('5')

    await wrapper.get('[data-testid="concurrency-input"]').setValue('8')
    await wrapper.get('[data-testid="account-capacity-save"]').trigger('click')

    expect(wrapper.emitted('save')).toEqual([[{ concurrency: 8, load_factor: 0 }]])
  })

  it('keeps an explicitly configured load factor when concurrency changes', async () => {
    const wrapper = mount(AccountCapacityCell, {
      props: {
        account: createAccount({ concurrency: 5, load_factor: 20 }),
        editable: true
      },
      global: {
        stubs: {
          CapacityBadge: { props: ['current', 'max'], template: '<span><slot />{{ current }}/{{ max }}</span>' },
          QuotaBadge: true
        }
      }
    })

    await wrapper.get('[data-testid="account-capacity-display"]').trigger('click')
    expect((wrapper.get('[data-testid="load-factor-input"]').element as HTMLInputElement).value).toBe('20')

    await wrapper.get('[data-testid="concurrency-input"]').setValue('7')
    await wrapper.get('[data-testid="account-capacity-save"]').trigger('click')

    expect(wrapper.emitted('save')).toEqual([[{ concurrency: 7, load_factor: 20 }]])
  })

  it('shows a validation reason and does not save an invalid load factor', async () => {
    const wrapper = mount(AccountCapacityCell, {
      props: {
        account: createAccount(),
        editable: true
      },
      global: {
        stubs: {
          CapacityBadge: { props: ['current', 'max'], template: '<span><slot />{{ current }}/{{ max }}</span>' },
          QuotaBadge: true
        }
      }
    })

    await wrapper.get('[data-testid="account-capacity-display"]').trigger('click')
    await wrapper.get('[data-testid="load-factor-input"]').setValue('10001')
    await wrapper.get('[data-testid="account-capacity-save"]').trigger('click')

    expect(wrapper.emitted('save')).toBeUndefined()
    expect(wrapper.get('[data-testid="account-capacity-error"]').text()).toContain(
      'admin.accounts.concurrencyCapacityInvalid'
    )
  })
})

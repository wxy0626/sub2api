import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountBulkActionsBar from '../AccountBulkActionsBar.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('AccountBulkActionsBar', () => {
  it('未勾选账号时显示禁用的批量测试按钮和中文提示', () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: { selectedIds: [] }
    })

    const testButton = wrapper.get('button:disabled')
    expect(testButton.text()).toBe('admin.accounts.bulkActions.test')
    expect(testButton.element.parentElement?.getAttribute('title')).toBe('admin.accounts.bulkActions.testSelectionRequired')
  })

  it('勾选账号后启用批量测试并派发测试事件', async () => {
    const wrapper = mount(AccountBulkActionsBar, {
      props: { selectedIds: [7] }
    })

    const testButton = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.bulkActions.test')
    expect(testButton).toBeDefined()
    expect(testButton!.attributes('disabled')).toBeUndefined()
    await testButton!.trigger('click')
    expect(wrapper.emitted('test')).toHaveLength(1)
  })
})

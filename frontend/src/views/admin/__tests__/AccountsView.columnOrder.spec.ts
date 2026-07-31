import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getFilterOptions,
  getAllProxies,
  getAllGroups
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getFilterOptions: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      getFilterOptions,
      getById: vi.fn(),
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      getAvailableModels: vi.fn(),
      testAccount: vi.fn(),
      update: vi.fn(),
      probeUpstreamBillingBatch: vi.fn(),
      toggleSchedulable: vi.fn(),
      setSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token',
    isSimpleMode: false
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = {
  props: ['columns', 'data', 'draggableColumns', 'draggableColumnKeys'],
  emits: ['column-reorder', 'sort'],
  template: `
    <div data-test="data-table">
      <div data-test="columns">{{ columns.map((column) => column.key).join(',') }}</div>
      <slot
        v-if="columns.some((column) => column.key === 'actions')"
        name="header-actions"
        :column="columns.find((column) => column.key === 'actions')"
      />
    </div>
  `
}

// 使用最小化挂载环境验证账号页顺序状态，不让无关弹窗和单元格实现干扰断言。
const mountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      },
      DataTable: DataTableStub,
      HelpTooltip: true,
      Pagination: true,
      ConfirmDialog: true,
      AccountTableActions: { template: '<div><slot name="after" /></div>' },
      AccountTableFilters: { template: '<div></div>' },
      AccountBulkActionsBar: true,
      AccountActionMenu: true,
      ImportDataModal: true,
      ReAuthAccountModal: true,
      AccountStatsModal: true,
      ScheduledTestsPanel: true,
      SyncFromCrsModal: true,
      TempUnschedStatusModal: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      CreateAccountModal: true,
      EditAccountModal: true,
      BulkEditAccountModal: true,
      PlatformTypeBadge: true,
      AccountCapacityCell: true,
      AccountStatusIndicator: true,
      AccountTodayStatsCell: true,
      AccountGroupsCell: true,
      AccountUsageCell: true,
      Icon: true
    }
  }
})

describe('admin AccountsView column order', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getUpstreamBillingProbeSettings.mockReset()
    getFilterOptions.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()

    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: true, interval_minutes: 30 })
    getFilterOptions.mockResolvedValue({ platforms: [], types: [] })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('uses one order source for header reordering and column display reordering', async () => {
    // 代理默认隐藏；本测试显式打开它后再验证列宽配置。
    localStorage.setItem('account-hidden-columns', JSON.stringify([]))
    localStorage.setItem('account-column-order', JSON.stringify(['status', 'capacity', 'platform']))
    const wrapper = mountView()
    await flushPromises()

    const dataTable = wrapper.getComponent(DataTableStub)
    // 从表格属性读取当前可见列顺序，验证表头和列显示菜单使用同一结果。
    const getColumns = () => dataTable.props('columns') as Array<{ key: string; class?: string }>
    const getColumnKeys = () => getColumns().map(column => column.key)
    expect(getColumns().find(column => column.key === 'proxy')?.class).toBe('w-36 max-w-36')
    expect(getColumnKeys().indexOf('status')).toBeLessThan(getColumnKeys().indexOf('capacity'))

    dataTable.vm.$emit('column-reorder', {
      sourceKey: 'capacity',
      targetKey: 'status',
      position: 'after'
    })
    await wrapper.vm.$nextTick()

    const tableOrderAfterHeaderDrag = getColumnKeys()
    expect(tableOrderAfterHeaderDrag.indexOf('capacity')).toBeGreaterThan(tableOrderAfterHeaderDrag.indexOf('status'))
    const menuOrderAfterHeaderDrag = (wrapper.vm as any).columnDisplayItems.map((column: { key: string }) => column.key)
    expect(menuOrderAfterHeaderDrag.indexOf('capacity')).toBeGreaterThan(menuOrderAfterHeaderDrag.indexOf('status'))

    const reorderedMenu = [...(wrapper.vm as any).columnDisplayItems]
    const statusIndex = reorderedMenu.findIndex((column: { key: string }) => column.key === 'status')
    const capacityIndex = reorderedMenu.findIndex((column: { key: string }) => column.key === 'capacity')
    const [capacity] = reorderedMenu.splice(capacityIndex, 1)
    reorderedMenu.splice(statusIndex, 0, capacity)
    ;(wrapper.vm as any).columnDisplayItems = reorderedMenu
    await wrapper.vm.$nextTick()

    const tableOrderAfterMenuDrag = getColumnKeys()
    expect(tableOrderAfterMenuDrag.indexOf('capacity')).toBeLessThan(tableOrderAfterMenuDrag.indexOf('status'))
    expect(JSON.parse(localStorage.getItem('account-column-order') || '[]').slice(0, 2)).toEqual(['capacity', 'status'])
  })

  it('renders the column settings icon beside the actions header and removes it from more tools', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="column-settings-button"]').attributes('aria-label')).toBe('admin.accounts.viewColumns')

    const vm = wrapper.vm as any
    vm.showColumnSettingsDropdown = true
    await wrapper.vm.$nextTick()
    expect(document.body.querySelector('[data-test="column-settings-dropdown"]')).not.toBeNull()

    vm.showColumnSettingsDropdown = false
    vm.showAccountToolsDropdown = true
    await wrapper.vm.$nextTick()
    expect(document.body.textContent).not.toContain('admin.accounts.viewColumns')
  })
})

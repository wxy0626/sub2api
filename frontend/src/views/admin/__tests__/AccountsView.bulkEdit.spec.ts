import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getAccountById,
  getAllProxies,
  getAllGroups,
  getFilterOptions,
  getAvailableModels,
  probeUpstreamBillingBatch,
  testAccount,
  updateAccount,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAccountById: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  getFilterOptions: vi.fn(),
  getAvailableModels: vi.fn(),
  probeUpstreamBillingBatch: vi.fn(),
  testAccount: vi.fn(),
  updateAccount: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      getFilterOptions,
      getById: getAccountById,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      getAvailableModels,
      testAccount,
      update: updateAccount,
      probeUpstreamBillingBatch,
      toggleSchedulable: vi.fn()
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
    showError,
    showSuccess,
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
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
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <span v-for="column in columns" :key="column.key" data-test="column-key">{{ column.key }}</span>
      <div data-test="header-groups"><slot name="header-groups" /></div>
      <div data-test="header-platform"><slot name="header-platform" /></div>
      <div data-test="header-type"><slot name="header-type" /></div>
      <div data-test="header-proxy"><slot name="header-proxy" /></div>
      <div v-for="row in data" :key="row.id">
        <div data-test="select-row"><slot name="cell-select" :row="row" /></div>
        <slot name="cell-created_at" :value="row.created_at" :row="row" />
        <slot name="cell-proxy" :row="row" />
        <div v-if="columns.some(column => column.key === 'priority')" data-test="priority-cell">
          <slot name="cell-priority" :value="row.priority" :row="row" />
        </div>
      </div>
    </div>
  `
}

const AccountBulkActionsBarStub = {
  props: ['selectedIds'],
  emits: ['edit-filtered', 'probe-upstream-billing', 'test'],
  template: `
    <div>
      <button data-test="edit-filtered" @click="$emit('edit-filtered')">edit filtered</button>
      <button data-test="probe-upstream-billing" @click="$emit('probe-upstream-billing')">probe</button>
      <button data-test="bulk-test" @click="$emit('test')">test</button>
    </div>
  `
}

const PaginationStub = {
  emits: ['update:page'],
  template: '<button data-test="next-page" @click="$emit(\'update:page\', 2)">next</button>'
}

const BulkEditAccountModalStub = {
  props: ['show', 'target'],
  template: '<div data-test="bulk-edit-modal" :data-show="String(show)" :data-target-mode="target?.mode ?? \'\'"></div>'
}

const HeaderGroupSelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: '<button data-test="header-group-select" @click="$emit(\'update:modelValue\', \'2\')"></button>'
}

describe('admin AccountsView bulk edit scope', () => {
  beforeEach(() => {
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getUpstreamBillingProbeSettings.mockReset()
    getAccountById.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    getFilterOptions.mockReset()
    getAvailableModels.mockReset()
    probeUpstreamBillingBatch.mockReset()
    testAccount.mockReset()
    updateAccount.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    listAccounts.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })
    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: true, interval_minutes: 30 })
    getAccountById.mockResolvedValue(null)
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    getFilterOptions.mockResolvedValue({ platforms: [], types: [] })
    getAvailableModels.mockResolvedValue([])
    probeUpstreamBillingBatch.mockResolvedValue([])
    testAccount.mockResolvedValue({ success: true, message: '账号测试成功' })
    updateAccount.mockResolvedValue({})
  })

  it('opens bulk edit in filtered-results mode from the bulk actions dropdown', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await wrapper.get('[data-test="edit-filtered"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-show')).toBe('true')
    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-target-mode')).toBe('filtered')
  })

  it('renders the created_at column by default', async () => {
    listAccounts.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'test-account',
          platform: 'anthropic',
          type: 'oauth',
          status: 'active',
          schedulable: true,
          created_at: '2026-03-07T10:00:00Z',
          updated_at: '2026-03-07T10:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()

    const columnKeys = wrapper.findAll('[data-test="column-key"]').map(node => node.text())
    expect(columnKeys).toContain('created_at')
    const columns = wrapper.getComponent(DataTableStub).props('columns') as Array<{ key: string; label: string; sortable: boolean }>
    expect(columns.find(column => column.key === 'created_at')).toMatchObject({
      label: 'admin.accounts.columns.createdAt',
      sortable: true
    })
  })

  it('passes the loaded global probe state to every upstream billing cell', async () => {
    listAccounts.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'upstream',
          platform: 'openai',
          type: 'apikey',
          status: 'active',
          schedulable: true,
          created_at: '2026-07-13T00:00:00Z',
          updated_at: '2026-07-13T00:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: false, interval_minutes: 30 })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="table" /></div>' },
          DataTable: {
            props: ['data'],
            template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-upstream_billing_rate" :row="row" /></div></div>'
          },
          UpstreamBillingRateCell: {
            props: ['globalProbeEnabled'],
            template: '<span data-test="upstream-billing-cell" :data-global-enabled="String(globalProbeEnabled)"></span>'
          },
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountBulkActionsBar: true,
          AccountActionMenu: true,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
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

    await flushPromises()

    expect(getUpstreamBillingProbeSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="upstream-billing-cell"]').attributes('data-global-enabled')).toBe('false')
  })

  it('loads available groups into the table header filter and reloads by the selected group', async () => {
    vi.useFakeTimers()
    getAllGroups.mockResolvedValue([{ id: 2, name: '生产分组' }])
    getAllProxies.mockResolvedValue([{ id: 7, name: '日本 11', country_code: 'JP', status: 'active' }])
    getFilterOptions.mockResolvedValue({ platforms: ['grok', 'openai'], types: ['apikey', 'oauth'] })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
          DataTable: DataTableStub,
          Select: HeaderGroupSelectStub,
          SearchInput: {
            props: ['modelValue'],
            template: '<input data-test="account-search-input" :value="modelValue" />'
          },
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    const listControls = wrapper.get('[data-test="account-list-controls"]')
    expect(listControls.find('[data-test="account-search-input"]').exists()).toBe(true)
    const toolbarSelects = listControls.findAllComponents(HeaderGroupSelectStub)
    expect(toolbarSelects).toHaveLength(0)

    const groupSelect = wrapper.get('[data-test="header-groups"]').getComponent(HeaderGroupSelectStub)
    expect(groupSelect.props('modelValue')).toBe('')
    expect(groupSelect.props('options')).toEqual([
      { value: '', label: 'admin.accounts.allGroups' },
      { value: 'ungrouped', label: 'admin.accounts.ungroupedGroup' },
      { value: '2', label: '生产分组' }
    ])
    expect(wrapper.get('[data-test="header-platform"]').getComponent(HeaderGroupSelectStub).props('options')).toEqual([
      { value: '', label: 'admin.accounts.allPlatforms' },
      { value: 'grok', label: 'grok' },
      { value: 'openai', label: 'openai' }
    ])
    expect(wrapper.get('[data-test="header-type"]').getComponent(HeaderGroupSelectStub).props('options')).toEqual([
      { value: '', label: 'admin.accounts.allTypes' },
      { value: 'apikey', label: 'apikey' },
      { value: 'oauth', label: 'oauth' }
    ])
    expect(wrapper.get('[data-test="header-proxy"]').getComponent(HeaderGroupSelectStub).props('options')).toEqual([
      { value: '', label: 'admin.accounts.allProxies' },
      { value: 7, label: '日本 11 (JP)' }
    ])

    const columnKeys = wrapper.findAll('[data-test="column-key"]').map(node => node.text())
    expect(columnKeys).toContain('platform')
    expect(columnKeys).toContain('type')
    expect(columnKeys).not.toContain('platform_type')

    await wrapper.get('[data-test="header-groups"]').get('[data-test="header-group-select"]').trigger('click')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      expect.any(Number),
      expect.objectContaining({ group: '2' }),
      expect.any(Object)
    )

    ;(wrapper.vm as any).handleHeaderPlatformFilterChange('openai')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      expect.any(Number),
      expect.objectContaining({ platform: 'openai' }),
      expect.any(Object)
    )

    ;(wrapper.vm as any).handleHeaderTypeFilterChange('oauth')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      expect.any(Number),
      expect.objectContaining({ platform: 'openai', type: 'oauth' }),
      expect.any(Object)
    )

    ;(wrapper.vm as any).handleHeaderProxyFilterChange(7)
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      expect.any(Number),
      expect.objectContaining({ proxy_id: 7 }),
      expect.any(Object)
    )

    vi.useRealTimers()
  })

  it('uses the account update API to bind and clear a row proxy', async () => {
    const account = {
      id: 9,
      name: 'plus',
      platform: 'openai',
      type: 'oauth',
      status: 'active',
      schedulable: true,
      proxy_id: null,
      created_at: '2026-03-07T10:00:00Z',
      updated_at: '2026-03-07T10:00:00Z'
    }
    listAccounts.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })
    getAllProxies.mockResolvedValue([
      { id: 7, name: '日本 11', country_code: 'JP', status: 'active' },
      { id: 8, name: '失效代理', status: 'inactive' }
    ])
    updateAccount
      .mockResolvedValueOnce({ ...account, proxy_id: 7, proxy: { id: 7, name: '日本 11', country_code: 'JP', status: 'active' } })
      .mockResolvedValueOnce({ ...account, proxy_id: null, proxy: undefined })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
          DataTable: DataTableStub,
          Select: HeaderGroupSelectStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    expect((wrapper.vm as any).accountProxyOptions(account)).toEqual([
      { value: null, label: '-' },
      { value: 7, label: '日本 11 (JP)' }
    ])

    await (wrapper.vm as any).handleAccountProxyChange(account, 7)
    expect(updateAccount).toHaveBeenLastCalledWith(9, { proxy_id: 7 })

    await (wrapper.vm as any).handleAccountProxyChange({ ...account, proxy_id: 7 }, null)
    expect(updateAccount).toHaveBeenLastCalledWith(9, { proxy_id: 0 })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.proxyUpdated')
  })

  it('allows manually entering and saving a row priority level', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify([]))
    const account = {
      id: 10,
      name: 'priority-account',
      platform: 'openai',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      priority: 1,
      created_at: '2026-03-07T10:00:00Z',
      updated_at: '2026-03-07T10:00:00Z'
    }
    const updatedAccount = { ...account, priority: 7 }
    listAccounts.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })
    updateAccount.mockResolvedValue(updatedAccount)

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    const priorityInput = wrapper.get('[data-test="priority-input"]')
    await priorityInput.setValue('7')
    await priorityInput.trigger('change')
    await flushPromises()

    expect(updateAccount).toHaveBeenCalledWith(10, { priority: 7 })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.priorityUpdated')
    expect((priorityInput.element as HTMLInputElement).value).toBe('7')
  })

  it('rejects an invalid priority level and keeps the original value', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify([]))
    const account = {
      id: 11,
      name: 'invalid-priority-account',
      platform: 'openai',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      priority: 2,
      created_at: '2026-03-07T10:00:00Z',
      updated_at: '2026-03-07T10:00:00Z'
    }
    listAccounts.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    const priorityInput = wrapper.get('[data-test="priority-input"]')
    await priorityInput.setValue('-1')
    await priorityInput.trigger('change')

    expect(updateAccount).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.accounts.priorityInvalid')
    expect((priorityInput.element as HTMLInputElement).value).toBe('2')

    updateAccount.mockRejectedValue({ status: 400, message: 'priority must be >= 0' })
    await priorityInput.setValue('3')
    await priorityInput.trigger('change')
    await flushPromises()

    expect(showError).toHaveBeenLastCalledWith(
      '操作失败，请根据下方技术详情定位原因。\n技术详情：priority must be >= 0'
    )
    expect((priorityInput.element as HTMLInputElement).value).toBe('2')
  })

  it('submits selected account IDs from every page for backend eligibility checks', async () => {
    const account = (id: number) => ({
      id,
      name: `account-${id}`,
      platform: 'openai',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      created_at: '2026-07-13T00:00:00Z',
      updated_at: '2026-07-13T00:00:00Z'
    })
    listAccounts
      .mockResolvedValueOnce({ items: [account(7)], total: 2, page: 1, page_size: 1, pages: 2 })
      .mockResolvedValueOnce({ items: [account(11)], total: 2, page: 2, page_size: 1, pages: 2 })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="table" /><slot name="pagination" /></div>' },
          DataTable: DataTableStub,
          Pagination: PaginationStub,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.get('[data-test="next-page"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.get('[data-test="probe-upstream-billing"]').trigger('click')
    await flushPromises()

    expect(probeUpstreamBillingBatch).toHaveBeenCalledWith([7, 11])
  })

  it('reloads the server-sorted list after a batch probe changes a snapshot', async () => {
    localStorage.setItem('account-table-sort', JSON.stringify({ key: 'upstream_billing_rate', order: 'asc' }))
    const account = (id: number) => ({
      id,
      name: `account-${id}`,
      platform: 'openai',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      created_at: '2026-07-13T00:00:00Z',
      updated_at: '2026-07-13T00:00:00Z'
    })
    listAccounts
      .mockResolvedValueOnce({ items: [account(7)], total: 1, page: 1, page_size: 20, pages: 1 })
      .mockResolvedValueOnce({ items: [account(7)], total: 1, page: 1, page_size: 20, pages: 1 })
    probeUpstreamBillingBatch.mockResolvedValue([
      {
        account_id: 7,
        snapshot: {
          status: 'ok',
          data: { effective_rate_multiplier: 0.5 },
          last_attempt_at: '2026-07-13T00:00:00Z',
          next_probe_at: '2026-07-13T00:30:00Z'
        }
      }
    ])

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="table" /></div>' },
          DataTable: DataTableStub,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountActionMenu: true,
          Pagination: true,
          ConfirmDialog: true,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.get('[data-test="probe-upstream-billing"]').trigger('click')
    await flushPromises()

    expect(probeUpstreamBillingBatch).toHaveBeenCalledWith([7])
    expect(listAccounts).toHaveBeenCalledTimes(2)
  })

  it('批量测试中每个账号结束后立即停止自身的刷新状态', async () => {
    const account = (id: number) => ({
      id,
      name: `account-${id}`,
      platform: 'openai',
      type: 'oauth',
      status: 'active',
      schedulable: true,
      extra: id === 7 ? { account_test_mode: 'responses' } : undefined,
      created_at: '2026-07-19T00:00:00Z',
      updated_at: '2026-07-19T00:00:00Z'
    })
    listAccounts.mockResolvedValue({ items: [account(7), account(11)], total: 2, page: 1, page_size: 20, pages: 1 })
    getAvailableModels.mockResolvedValue([{ id: 'gpt-5.6-luna', display_name: 'GPT-5.6 Luna' }])

    let resolveFirst!: (value: { success: boolean; message: string }) => void
    let resolveSecond!: (value: { success: boolean; message: string }) => void
    testAccount
      .mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { resolveSecond = resolve }))

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await wrapper.get('[data-test="select-row"] input').trigger('change')
    await wrapper.findAll('[data-test="select-row"] input')[1].trigger('change')
    await wrapper.get('[data-test="bulk-test"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(testAccount).toHaveBeenCalledWith(7, { modelId: 'gpt-5.6-luna', mode: 'responses' })

    const testingIds = (wrapper.vm as any).testingAccountIds as Set<number>
    expect(testingIds).toEqual(new Set([7, 11]))

    resolveFirst({ success: true, message: '账号测试成功' })
    await flushPromises()
    expect(testingIds.has(7)).toBe(false)
    expect(testingIds.has(11)).toBe(true)

    resolveSecond({ success: true, message: '账号测试成功' })
    await flushPromises()
    expect(testingIds.size).toBe(0)
  })

  it('菜单模型检测完成后只更新当前账号行，不重载列表或跳回首个账号', async () => {
    const account = {
      id: 99,
      name: 'bottom-account',
      platform: 'openai',
      type: 'oauth',
      status: 'active',
      schedulable: true,
      created_at: '2026-07-19T00:00:00Z',
      updated_at: '2026-07-19T00:00:00Z'
    }
    listAccounts.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })
    getAccountById.mockResolvedValue({ ...account, status: 'error', error_message: 'API returned 403: INSUFFICIENT_BALANCE' })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: {
            emits: ['test'],
            template: '<button data-test="model-test" @click="$emit(\'test\', $attrs.account)">模型测试</button>'
          },
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: { template: '<div data-test="account-test-modal" />' },
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    listAccounts.mockClear()

    ;(wrapper.vm as any).menu.acc = account
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-test="model-test"]').trigger('click')
    await flushPromises()

    expect(testAccount).not.toHaveBeenCalled()
    expect(getAccountById).not.toHaveBeenCalled()
    expect(listAccounts).not.toHaveBeenCalled()
    expect((wrapper.vm as any).accounts[0]).toMatchObject({ id: 99, status: 'active' })
    expect(wrapper.find('[data-test="account-test-modal"]').exists()).toBe(true)
  })

  it('OpenAI 的 Luna 测试失败时不回退为默认模型测试', async () => {
    const account = {
      id: 18,
      name: 'fallback-account',
      platform: 'openai',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      extra: { account_test_mode: 'responses' },
      created_at: '2026-07-19T00:00:00Z',
      updated_at: '2026-07-19T00:00:00Z'
    }
    listAccounts.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })
    getAccountById.mockResolvedValue(account)
    getAvailableModels.mockResolvedValue([{ id: 'gpt-5.6-luna', display_name: 'GPT-5.6 Luna' }])
    testAccount.mockResolvedValueOnce({ success: false, message: 'Luna 模型不可用' })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await (wrapper.vm as any).handleQuickTest(account)

    expect(getAvailableModels).toHaveBeenCalledTimes(1)
    expect(getAvailableModels).toHaveBeenCalledWith(18)
    expect(testAccount).toHaveBeenCalledTimes(1)
    expect(testAccount).toHaveBeenCalledWith(18, { modelId: 'gpt-5.6-luna', mode: 'responses' })
  })

  it('DeepSeek 快速测试为 V4 Flash 发送 Responses 模式', async () => {
    const account = {
      id: 24,
      name: 'deepseek-account',
      platform: 'deepseek',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      created_at: '2026-07-19T00:00:00Z',
      updated_at: '2026-07-19T00:00:00Z'
    }
    listAccounts.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })
    getAccountById.mockResolvedValue(account)
    getAvailableModels.mockResolvedValue([{ id: 'deepseek-v4-flash', display_name: 'deepseek-v4-flash' }])
    testAccount.mockResolvedValueOnce({ success: true, message: '账号测试成功' })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await (wrapper.vm as any).handleQuickTest(account)

    expect(testAccount).toHaveBeenCalledWith(24, { modelId: 'deepseek-v4-flash', mode: 'responses' })
  })

  it('普通 DeepSeek 快速测试发送 default Chat Completions 模式', async () => {
    const account = {
      id: 25,
      name: 'deepseek-chat-account',
      platform: 'deepseek',
      type: 'apikey',
      status: 'active',
      schedulable: true,
      created_at: '2026-07-19T00:00:00Z',
      updated_at: '2026-07-19T00:00:00Z'
    }
    listAccounts.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })
    getAccountById.mockResolvedValue(account)
    getAvailableModels.mockResolvedValue([{ id: 'deepseek-chat', display_name: 'deepseek-chat' }])
    testAccount.mockResolvedValueOnce({ success: true, message: '账号测试成功' })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await (wrapper.vm as any).handleQuickTest(account)

    expect(testAccount).toHaveBeenCalledWith(25, { modelId: 'deepseek-chat', mode: 'default' })
  })

  it('状态栏模型检测只使用弹窗预填的单一模型，不因模型列表为空改用其他模型', async () => {
    const account = {
      id: 23,
      name: 'empty-models-account',
      platform: 'openai',
      type: 'oauth',
      status: 'active',
      schedulable: true,
      created_at: '2026-07-19T00:00:00Z',
      updated_at: '2026-07-19T00:00:00Z'
    }
    listAccounts.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 20, pages: 1 })
    getAvailableModels.mockResolvedValue([])
    getAccountById.mockResolvedValue(account)

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: true,
          AccountTableFilters: true,
          AccountBulkActionsBar: AccountBulkActionsBarStub,
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
          BulkEditAccountModal: BulkEditAccountModalStub,
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

    await flushPromises()
    await (wrapper.vm as any).handleQuickTest(account)

    expect(getAvailableModels).toHaveBeenCalledWith(23)
    expect(testAccount).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('账号未返回可用于测试的模型'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('available_models is empty'))
  })
})

import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountTestUsageTable from '../AccountTestUsageTable.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'admin.usage.accountTest.unknownAccount': 'Unknown account',
      'admin.usage.accountTest.empty': 'Empty',
      'admin.usage.accountTest.success': 'Success',
      'admin.usage.accountTest.failed': 'Failed',
      'admin.usage.accountTest.inputShort': 'Input',
      'admin.usage.accountTest.outputShort': 'Output',
      'admin.usage.accountTest.cacheShort': 'Cache',
      'admin.usage.accountTest.totalShort': 'Total',
    }[key] ?? key),
  }),
}))

describe('AccountTestUsageTable', () => {
  it('renders platform, account, token totals, status, error and time', () => {
    const wrapper = mount(AccountTestUsageTable, {
      props: {
        loading: false,
        records: [{
          id: 7, account_id: 61, account_name: 'DeepSeek', platform: 'deepseek',
          model: 'deepseek-v4-flash', test_mode: 'responses', endpoint: '/v1/responses',
          input_tokens: 84, output_tokens: 16, cache_creation_tokens: 3, cache_read_tokens: 2,
          duration_ms: 1200, success: false, status_code: 502, error_message: 'upstream failed',
           created_at: '2026-08-01T08:47:36.000Z',
         }],
      },
      global: { stubs: { Icon: true } },
    })

    expect(wrapper.text()).toContain('deepseek')
    expect(wrapper.text()).toContain('DeepSeek')
    expect(wrapper.text()).toContain('#61')
    expect(wrapper.text()).toContain('deepseek-v4-flash')
    expect(wrapper.text()).toContain('105')
    expect(wrapper.text()).toContain('Failed')
    expect(wrapper.text()).toContain('HTTP 502')
    expect(wrapper.text()).toContain('upstream failed')
  })

  it('renders Grok and DeepSeek platform/model values independently', () => {
    const wrapper = mount(AccountTestUsageTable, {
      props: {
        loading: false,
        records: [
          {
            id: 1, account_id: 10, account_name: 'Grok account', platform: 'grok',
            model: 'grok-4', test_mode: 'default', endpoint: '/v1/chat/completions',
            input_tokens: 1, output_tokens: 2, cache_creation_tokens: 0, cache_read_tokens: 0,
            duration_ms: 50, success: true, status_code: 200, created_at: '2026-08-01T08:00:00.000Z',
          },
          {
            id: 2, account_id: 11, account_name: 'DeepSeek account', platform: 'deepseek',
            model: 'deepseek-chat', test_mode: 'default', endpoint: '/v1/chat/completions',
            input_tokens: 3, output_tokens: 4, cache_creation_tokens: 0, cache_read_tokens: 0,
            duration_ms: 80, success: true, status_code: 200, created_at: '2026-08-01T08:01:00.000Z',
          },
        ],
      },
      global: { stubs: { Icon: true } },
    })

    expect(wrapper.text()).toContain('grok')
    expect(wrapper.text()).toContain('grok-4')
    expect(wrapper.text()).toContain('deepseek')
    expect(wrapper.text()).toContain('deepseek-chat')
  })

  it('renders loading and empty states', () => {
    expect(mount(AccountTestUsageTable, { props: { loading: true, records: [] }, global: { stubs: { Icon: true } } }).text()).not.toContain('Empty')
    expect(mount(AccountTestUsageTable, { props: { loading: false, records: [] }, global: { stubs: { Icon: true } } }).text()).toContain('Empty')
  })
})

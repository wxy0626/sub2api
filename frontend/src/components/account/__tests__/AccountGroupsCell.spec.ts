import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountGroupsCell from '../AccountGroupsCell.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

// 用于测试多选保存的分组数据。
const groups = [
  { id: 1, name: 'OpenAI A', platform: 'openai', rate_multiplier: 1 },
  { id: 2, name: 'OpenAI B', platform: 'openai', rate_multiplier: 1 }
] as any[]

// 挂载单元格并用可控的分组选择器替代视觉组件。
const mountCell = (assignedGroups = groups.slice(0, 1)) => mount(AccountGroupsCell, {
  props: {
    groups: assignedGroups,
    availableGroups: groups,
    platform: 'openai'
  },
  global: {
    stubs: {
      GroupBadge: { template: '<span>{{ name }}</span>', props: ['name'] },
      GroupSelector: {
        props: ['modelValue'],
        emits: ['update:modelValue'],
        template: '<button data-testid="select-second-group" @click="$emit(\'update:modelValue\', [1, 2])">select</button>'
      },
      Teleport: true
    }
  }
})

describe('AccountGroupsCell', () => {
  it('clicking the cell lets the admin select multiple groups and save once', async () => {
    const wrapper = mountCell()

    await wrapper.get('[data-testid="account-groups-trigger"]').trigger('click')
    await wrapper.get('[data-testid="select-second-group"]').trigger('click')
    await wrapper.get('[data-testid="account-groups-save"]').trigger('click')

    expect(wrapper.emitted('save')).toEqual([[[1, 2]]])
  })

  it('shows an editable placeholder for ungrouped accounts', () => {
    const wrapper = mountCell([])

    expect(wrapper.get('[data-testid="account-groups-trigger"]').text()).toBe('-')
  })

  it('preserves group IDs when the lightweight row omits group objects', async () => {
    const wrapper = mountCell(undefined as any)
    await wrapper.setProps({ groups: undefined, groupIDs: [1] })

    await wrapper.get('[data-testid="account-groups-trigger"]').trigger('click')
    await wrapper.get('[data-testid="select-second-group"]').trigger('click')
    await wrapper.get('[data-testid="account-groups-save"]').trigger('click')

    expect(wrapper.emitted('save')).toEqual([[[1, 2]]])
  })
})

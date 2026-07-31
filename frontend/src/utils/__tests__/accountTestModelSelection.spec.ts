import { describe, expect, it } from 'vitest'
import { resolveAccountTestModeForModel, resolveAccountTestModelSelection } from '@/utils/accountTestModelSelection'
import type { ClaudeModel } from '@/types'

// createModel 构造账号模型接口返回的最小完整模型数据，便于验证纯预填规则。
const createModel = (id: string): ClaudeModel => ({
  id,
  type: 'model',
  display_name: id,
  created_at: '2026-07-19T00:00:00Z'
})

describe('accountTestModelSelection', () => {
  it('OpenAI 未返回 Luna 时按弹窗白名单后的首项预填，并固定 default 模式', () => {
    const selection = resolveAccountTestModelSelection('openai', [
      createModel('unsupported-model'),
      createModel('gpt-5.6-terra'),
      createModel('gpt-image-2')
    ])

    expect(selection.models.map((model) => model.id)).toEqual(['gpt-5.6-terra', 'gpt-image-2'])
    expect(selection).toMatchObject({ modelId: 'gpt-5.6-terra', mode: 'default' })
  })

  it('Gemini 使用弹窗原有的优先顺序预填第一项', () => {
    const selection = resolveAccountTestModelSelection('gemini', [
      createModel('gemini-2.5-pro'),
      createModel('gemini-3.5-flash'),
      createModel('gemini-custom')
    ])

    expect(selection.models.map((model) => model.id)).toEqual([
      'gemini-3.5-flash',
      'gemini-2.5-pro',
      'gemini-custom'
    ])
    expect(selection.modelId).toBe('gemini-3.5-flash')
  })

  it('其他平台保持弹窗原有的 Sonnet 优先预填', () => {
    const selection = resolveAccountTestModelSelection('anthropic', [
      createModel('claude-opus-4'),
      createModel('claude-sonnet-4')
    ])

    expect(selection.modelId).toBe('claude-sonnet-4')
    expect(selection.mode).toBeUndefined()
  })

  it('DeepSeek 优先使用最新模型，并保留其他上游模型', () => {
    const selection = resolveAccountTestModelSelection('deepseek', [
      createModel('deepseek-reasoner'),
      createModel('deepseek-v4-flash'),
      createModel('deepseek-v4-pro'),
      createModel('deepseek-chat')
    ])

    expect(selection.models.map((model) => model.id)).toEqual([
      'deepseek-v4-flash',
      'deepseek-v4-pro',
      'deepseek-reasoner',
      'deepseek-chat'
    ])
    expect(selection.modelId).toBe('deepseek-v4-flash')
  })

  it('DeepSeek V4 Flash 使用 Responses，其余模型使用 Chat Completions', () => {
    expect(resolveAccountTestModeForModel('deepseek', 'DeepSeek-V4-Flash')).toBe('responses')
    expect(resolveAccountTestModeForModel('deepseek', 'deepseek-chat')).toBe('default')
    expect(resolveAccountTestModeForModel('openai', 'deepseek-v4-flash')).toBe('default')
  })
})

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

// createUpstreamModel 构造后端标记为上游实时目录的模型（owned_by='upstream'）。
const createUpstreamModel = (id: string): ClaudeModel => ({
  ...createModel(id),
  owned_by: 'upstream'
})

describe('accountTestModelSelection', () => {
  it('OpenAI 兼容端点返回的上游实时目录全部保留，不再限制为 GPT 模型', () => {
    const selection = resolveAccountTestModelSelection('openai', [
      createUpstreamModel('gpt-4o'),
      createUpstreamModel('gemini-3-pro-preview'),
      createUpstreamModel('deepseek-chat'),
      createUpstreamModel('gpt-5.6-luna')
    ])

    expect(selection.models.map((model) => model.id)).toEqual([
      'gpt-4o',
      'gemini-3-pro-preview',
      'deepseek-chat',
      'gpt-5.6-luna'
    ])
    expect(selection).toMatchObject({ modelId: 'gpt-5.6-luna', mode: 'default' })
  })

  it('OpenAI 上游目录不含 GPT 模型时同样全部保留并预填首项', () => {
    const selection = resolveAccountTestModelSelection('openai', [
      createUpstreamModel('glm-4.6'),
      createUpstreamModel('qwen3-max')
    ])

    expect(selection.models.map((model) => model.id)).toEqual(['glm-4.6', 'qwen3-max'])
    expect(selection).toMatchObject({ modelId: 'glm-4.6', mode: 'default' })
  })

  it('OpenAI 内置默认模型集未标记上游时仍受原白名单限制', () => {
    const selection = resolveAccountTestModelSelection('openai', [
      createModel('gpt-5.5'),
      createModel('gpt-5.4'),
      createModel('gpt-5.6-terra'),
      createModel('gpt-image-2')
    ])

    expect(selection.models.map((model) => model.id)).toEqual(['gpt-5.6-terra', 'gpt-image-2'])
    expect(selection).toMatchObject({ modelId: 'gpt-5.6-terra', mode: 'default' })
  })

  it('OpenAI 未返回 Luna 时按弹窗白名单后的首项预填，并固定 default 模式', () => {
    const selection = resolveAccountTestModelSelection('openai', [
      createModel('unsupported-model'),
      createModel('gpt-5.6-terra'),
      createModel('gpt-image-2')
    ])

    expect(selection.models.map((model) => model.id)).toEqual(['gpt-5.6-terra', 'gpt-image-2'])
    expect(selection).toMatchObject({ modelId: 'gpt-5.6-terra', mode: 'default' })
  })

  it('OpenAI 上游只返回非 OpenAI 模型时回退使用上游模型，避免 v1 兼容代理测试被白名单滤空', () => {
    const selection = resolveAccountTestModelSelection('openai', [
      createModel('gemini-2.5-pro'),
      createModel('gemini:130')
    ])

    expect(selection.models.map((model) => model.id)).toEqual(['gemini-2.5-pro', 'gemini:130'])
    expect(selection).toMatchObject({ modelId: 'gemini-2.5-pro', mode: 'default' })
  })

  it('OpenAI 同时返回固定白名单模型与非 OpenAI 模型时二者都保留，仍优先 Luna', () => {
    const selection = resolveAccountTestModelSelection('openai', [
      createModel('unsupported-model'),
      createModel('gpt-5.6-luna'),
      createModel('gemini-2.5-pro')
    ])

    expect(selection.models.map((model) => model.id)).toEqual(['gpt-5.6-luna', 'gemini-2.5-pro'])
    expect(selection).toMatchObject({ modelId: 'gpt-5.6-luna', mode: 'default' })
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

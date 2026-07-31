import { describe, expect, it, vi } from 'vitest'

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

import {
  buildModelMappingObject,
  getDefaultModelWhitelist,
  getModelsByPlatform,
  isAllowedSyncedModel,
  restrictSyncedModels,
  splitModelMappingObject
} from '../useModelWhitelist'

describe('useModelWhitelist', () => {
  it('openai 模型列表仅保留 GPT-5.5、GPT-5.6 和 GPT Image 系列', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.5')
    expect(models).toContain('gpt-5.6')
    expect(models).toContain('gpt-image-2')
    expect(models).not.toContain('gpt-5.4')
    expect(models).not.toContain('gpt-5.3-codex-spark')
    expect(models).not.toContain('gpt-5.2')
    expect(models).not.toContain('gpt-4o-audio-preview')
  })

  it('openai 模型列表不再暴露已下线的 ChatGPT 登录 Codex 模型', () => {
    const models = getModelsByPlatform('openai')

    expect(models).not.toContain('gpt-5')
    expect(models).not.toContain('gpt-5.1')
    expect(models).not.toContain('gpt-5.1-codex')
    expect(models).not.toContain('gpt-5.1-codex-max')
    expect(models).not.toContain('gpt-5.1-codex-mini')
    expect(models).not.toContain('gpt-5.2-codex')
  })

  it('antigravity 模型列表包含图片模型兼容项', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
  })

  it('Claude 模型列表包含新发布的 Claude 模型', () => {
    expect(getModelsByPlatform('claude')).toContain('claude-fable-5')
    expect(getModelsByPlatform('antigravity')).toContain('claude-fable-5')
    expect(getModelsByPlatform('claude')).toContain('claude-opus-4-8')
    expect(getModelsByPlatform('antigravity')).toContain('claude-opus-4-8')
  })

  it('xAI 模型列表包含 Grok 4.5 官方模型和别名', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-4.5')
    expect(models).toContain('grok-4.5-latest')
    expect(models).toContain('grok-build-latest')
  })

  it('combined 模式支持 Grok 4.5 官方别名映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['grok-4.5'],
      [
        { from: 'grok-latest', to: 'grok-4.5' },
        { from: 'grok-4.5-latest', to: 'grok-4.5' },
        { from: 'grok-build-latest', to: 'grok-4.5' }
      ]
    )

    expect(mapping).toEqual({
      'grok-4.5': 'grok-4.5',
      'grok-latest': 'grok-4.5',
      'grok-4.5-latest': 'grok-4.5',
      'grok-build-latest': 'grok-4.5'
    })
  })

  it('grok 模型列表包含 Composer 默认项和兼容别名', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-composer-2.5-fast')
    expect(models).toContain('grok-composer')
    expect(models).toContain('composer-2.5')
  })

  it('gemini 模型列表包含原生生图模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.0-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
  })

  it('antigravity 模型列表会把新的 Gemini 图片模型排在前面', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash-lite'))
  })

  it('antigravity 模型列表包含 Gemini 3.1 Pro 通用别名', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-3.1-pro')
  })

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    })
  })

  it('whitelist 模式会保留 GPT-5.6 官方快照的精确映射', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.6-luna'], [])

    expect(mapping).toEqual({
      'gpt-5.6-luna': 'gpt-5.6-luna'
    })
  })

  it('whitelist keeps GPT Image exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-image-2'], [])

    expect(mapping).toEqual({
      'gpt-image-2': 'gpt-image-2'
    })
  })

  it('同步模型清理只保留 GPT-5.6 及以上和 GPT Image 2', () => {
    expect(isAllowedSyncedModel(' GPT-5.6-LUNA ')).toBe(true)
    expect(isAllowedSyncedModel('gpt-5.7-preview')).toBe(true)
    expect(isAllowedSyncedModel('gpt-6')).toBe(true)
    expect(isAllowedSyncedModel('gpt-image-2')).toBe(true)
    expect(isAllowedSyncedModel('gpt-image-2-2026-04-21')).toBe(true)
    expect(isAllowedSyncedModel('gpt-5.5')).toBe(false)
    expect(isAllowedSyncedModel('gpt-5.4')).toBe(false)
    expect(isAllowedSyncedModel('gpt-5.50')).toBe(false)
    expect(isAllowedSyncedModel('gpt-5.6x')).toBe(false)
    expect(isAllowedSyncedModel('gpt-image-1.5')).toBe(false)
    expect(isAllowedSyncedModel('claude-sonnet-4-6')).toBe(false)
    expect(restrictSyncedModels(['gpt-5.4', 'gpt-5.5', 'gpt-5.6-luna', 'gpt-5.7-preview', 'gpt-5.5', 'gpt-image-1.5', 'gpt-image-2', 'o3']))
      .toEqual(['gpt-5.6-luna', 'gpt-5.7-preview', 'gpt-image-2'])
  })

  it('DeepSeek 同步模型保留非空上游模型并去重，不套用 GPT 白名单', () => {
    expect(getModelsByPlatform('deepseek')).toEqual([
      'deepseek-v4-flash',
      'deepseek-v4-pro'
    ])
    expect(isAllowedSyncedModel('deepseek-v4-flash', 'deepseek')).toBe(true)
    expect(isAllowedSyncedModel('deepseek-custom-model', 'deepseek')).toBe(true)
    expect(isAllowedSyncedModel('   ', 'deepseek')).toBe(false)
    expect(restrictSyncedModels([
      'deepseek-v4-flash',
      '',
      ' deepseek-chat ',
      'deepseek-v4-flash',
      'deepseek-v4-pro',
      'deepseek-custom-model',
      'gpt-5.5'
    ], 'deepseek'))
      .toEqual(['deepseek-v4-flash', 'deepseek-chat', 'deepseek-v4-pro', 'deepseek-custom-model', 'gpt-5.5'])
  })

  it('非 OpenAI 平台同步模型保留非空模型并去重', () => {
    expect(restrictSyncedModels(['claude-sonnet-4-6', 'claude-sonnet-4-6', '  ', 'custom-model'], 'anthropic'))
      .toEqual(['claude-sonnet-4-6', 'custom-model'])
  })

  it('OpenAI 编辑空白名单时默认开放 GPT-5.6 和 GPT Image 2', () => {
    expect(getDefaultModelWhitelist('openai')).toEqual(['gpt-5.6', 'gpt-image-2'])
    expect(getDefaultModelWhitelist('OpenAI')).toEqual(['gpt-5.6', 'gpt-image-2'])
    expect(getDefaultModelWhitelist('anthropic')).toEqual([])
  })

  it('combined 模式会同时保留白名单身份映射和模型映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['gpt-5.4', 'claude-*'],
      [
        { from: 'gpt-latest', to: 'gpt-5.4' },
        { from: 'gpt-5.4', to: 'gpt-5.4-mini' }
      ]
    )

    expect(mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-mini',
      'gpt-latest': 'gpt-5.4'
    })
  })

  it('splitModelMappingObject 会把身份映射还原成白名单，其余保留为映射', () => {
    const parsed = splitModelMappingObject({
      'gpt-5.4': 'gpt-5.4',
      'gpt-latest': 'gpt-5.4',
      ' ': 'gpt-empty',
      broken: 123
    })

    expect(parsed).toEqual({
      allowedModels: ['gpt-5.4'],
      modelMappings: [{ from: 'gpt-latest', to: 'gpt-5.4' }]
    })
  })
})

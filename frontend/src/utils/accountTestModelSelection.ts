import type { AccountTestMode } from '@/api/admin/accounts'
import type { AccountPlatform, ClaudeModel } from '@/types'

// 管理员账号模型测试中允许展示并预填的 OpenAI 模型 ID（白名单限制保持不变）。
const openAITestModelIDs = new Set([
  'gpt-5.6-sol',
  'gpt-5.6-terra',
  'gpt-5.6-luna',
  'gpt-image-2'
])

// 第三方 v1 兼容代理常暴露非 OpenAI 模型，OpenAI 平台测试也需允许这些模型通过。
// 仅放开已知前缀（如 gemini），避免任意垃圾模型进入测试选择。
const openAINonOpenAITestModelPatterns = ['gemini-*', 'gemini:*']

// matchesOpenAITestModel 判断 OpenAI 平台下某模型是否可用于测试：
// 命中白名单固定 ID，或命中非 OpenAI 第三方模型前缀（仅尾部 * 通配）。
function matchesOpenAITestModel(modelID: string): boolean {
  if (openAITestModelIDs.has(modelID)) return true
  return openAINonOpenAITestModelPatterns.some((pattern) => {
    if (!pattern.endsWith('*')) return modelID === pattern
    return modelID.startsWith(pattern.slice(0, -1))
  })
}

// 账号连接测试的统一首选模型 ID：可用列表含 Luna 时优先使用。
const defaultTestModelID = 'gpt-5.6-luna'

// Gemini 与 Antigravity 账号在模型测试选择器中的既有展示优先级。
const prioritizedGeminiModels = [
  'gemini-3.1-flash-image',
  'gemini-2.5-flash-image',
  'gemini-3.5-flash',
  'gemini-2.5-flash',
  'gemini-2.5-pro',
  'gemini-3-flash-preview',
  'gemini-3-pro-preview',
  'gemini-2.0-flash'
]

// DeepSeek 账号测试只对当前最新模型设置默认优先级，其他模型保持上游返回顺序。
const prioritizedDeepSeekModels = [
  'deepseek-v4-flash',
  'deepseek-v4-pro'
]

// 统一模型测试预填结果，供弹窗展示和状态栏单次检测共同使用。
export interface AccountTestModelSelection {
  models: ClaudeModel[]
  modelId: string
  mode?: 'default'
}

// resolveAccountTestModeForModel 统一决定账号测试模型对应的协议，避免弹窗与快速测试分叉。
export function resolveAccountTestModeForModel(platform: AccountPlatform, modelID: string): AccountTestMode {
  if (platform === 'deepseek' && modelID.trim().toLowerCase() === 'deepseek-v4-flash') {
    return 'responses'
  }
  return 'default'
}

// sortAccountTestModels 按弹窗原有的 Gemini 模型优先级排序，未命中优先级的模型保持相对顺序。
function sortAccountTestModels(models: ClaudeModel[]): ClaudeModel[] {
  const priorityMap = new Map(prioritizedGeminiModels.map((id, index) => [id, index]))

  return [...models].sort((firstModel, secondModel) => {
    const firstPriority = priorityMap.get(firstModel.id) ?? Number.MAX_SAFE_INTEGER
    const secondPriority = priorityMap.get(secondModel.id) ?? Number.MAX_SAFE_INTEGER
    if (firstPriority !== secondPriority) return firstPriority - secondPriority
    return 0
  })
}

// sortDeepSeekTestModels 按 DeepSeek 官方模型优先级排序，避免测试默认落到已废弃模型。
function sortDeepSeekTestModels(models: ClaudeModel[]): ClaudeModel[] {
  const priorityMap = new Map(prioritizedDeepSeekModels.map((id, index) => [id, index]))

  return [...models].sort((firstModel, secondModel) => {
    const firstPriority = priorityMap.get(firstModel.id) ?? Number.MAX_SAFE_INTEGER
    const secondPriority = priorityMap.get(secondModel.id) ?? Number.MAX_SAFE_INTEGER
    if (firstPriority !== secondPriority) return firstPriority - secondPriority
    return 0
  })
}

// resolveAccountTestModelSelection 复用模型测试弹窗的过滤、排序及单一预填模型选择规则。
export function resolveAccountTestModelSelection(
  platform: AccountPlatform,
  models: ClaudeModel[]
): AccountTestModelSelection {
  // OpenAI 测试模型白名单只影响管理员模型测试，不改变账号模型映射或网关模型列表。
  // 固定 OpenAI ID 保持原白名单限制不变；同时放行上游返回的非 OpenAI 模型（如 gemini-*）。
  // 若二者均无命中（上游只返回其它未放开前缀的模型），回退到上游返回的全部模型，保证仍可测试。
  let filteredModels = platform === 'openai'
    ? models.filter((model) => matchesOpenAITestModel(model.id))
    : models
  if (platform === 'openai' && filteredModels.length === 0) {
    filteredModels = models
  }
  // Gemini 与 Antigravity 保留弹窗已有的优先展示顺序，其他平台保持后端返回顺序。
  const selectedModels = platform === 'gemini' || platform === 'antigravity'
    ? sortAccountTestModels(filteredModels)
    : platform === 'deepseek'
      ? sortDeepSeekTestModels(filteredModels)
    : [...filteredModels]
  // 当前弹窗的预填规则：Luna 优先；Gemini 否则第一项；其他平台优先 Sonnet，再取第一项。
  const lunaModel = selectedModels.find((model) => model.id === defaultTestModelID)
  const sonnetModel = selectedModels.find((model) => model.id.includes('sonnet'))
  const deepSeekModel = platform === 'deepseek'
    ? selectedModels.find((model) => prioritizedDeepSeekModels.includes(model.id))
    : undefined
  const defaultModel = lunaModel
    ?? deepSeekModel
    ?? (platform === 'gemini' ? selectedModels[0] : sonnetModel ?? selectedModels[0])

  return {
    models: selectedModels,
    modelId: defaultModel?.id ?? '',
    ...(platform === 'openai' ? { mode: 'default' as const } : {})
  }
}

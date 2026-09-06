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

// upstreamCatalogOwner 对应后端 openai.UpstreamCatalogOwner：
// 该标记说明模型来自上游 /v1/models 实时目录，只要端点兼容 OpenAI 协议就应全部放行，
// 不再限制为内置 GPT 模型集；未带该标记的内置默认模型集仍沿用原白名单限制。
const upstreamCatalogOwner = 'upstream'

// matchesOpenAITestModel 判断 OpenAI 平台下某模型是否可用于测试：
// 上游实时目录模型全部放行；账号 model_mapping（模型白名单/映射）中的源模型一律放行——
// 管理员在白名单里配置的模型必须出现在测试下拉中；否则命中白名单固定 ID，
// 或命中非 OpenAI 第三方模型前缀（仅尾部 * 通配）。
function matchesOpenAITestModel(model: ClaudeModel, allowedIDs: Set<string>): boolean {
  if (model.owned_by === upstreamCatalogOwner) return true
  if (allowedIDs.has(model.id)) return true
  const modelID = model.id
  if (openAITestModelIDs.has(modelID)) return true
  return openAINonOpenAITestModelPatterns.some((pattern) => {
    if (!pattern.endsWith('*')) return modelID === pattern
    return modelID.startsWith(pattern.slice(0, -1))
  })
}

// collectAccountMappingModelIDs 提取账号 model_mapping 的源模型 ID 列表。
// 账号的"模型白名单"与"模型映射"统一存放在 model_mapping 中：白名单条目 from === to，
// 映射条目 from 是请求侧模型。这些源模型都是管理员明确允许该账号使用的模型。
export function collectAccountMappingModelIDs(
  modelMapping?: Record<string, unknown> | null
): string[] {
  if (!modelMapping || typeof modelMapping !== 'object') return []
  return Object.keys(modelMapping)
    .map(modelID => modelID.trim())
    .filter(modelID => modelID.length > 0)
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
// allowedModelIDs 是账号 model_mapping 的源模型 ID（collectAccountMappingModelIDs 的结果），
// 这些模型在 OpenAI 平台过滤中一律放行，保证"白名单里配置的模型都能在测试下拉中选择"。
export function resolveAccountTestModelSelection(
  platform: AccountPlatform,
  models: ClaudeModel[],
  allowedModelIDs: string[] = []
): AccountTestModelSelection {
  // OpenAI 测试模型白名单只影响管理员模型测试，不改变账号模型映射或网关模型列表。
  // 后端标记为上游实时目录（owned_by='upstream'）的模型全部放行：只要端点兼容 OpenAI 协议，
  // 上游返回什么模型就能测什么，不再限制为 GPT 模型。
  // 账号 model_mapping 中的源模型（模型白名单/映射）同样全部放行。
  // 内置默认模型集仍保持原白名单限制，并继续放行 gemini-* 等已知非 OpenAI 前缀。
  // 若全部被滤空（上游只返回其它未放开前缀的模型），回退到上游返回的全部模型，保证仍可测试。
  const allowedIDSet = new Set(allowedModelIDs)
  let filteredModels = platform === 'openai'
    ? models.filter((model) => matchesOpenAITestModel(model, allowedIDSet))
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

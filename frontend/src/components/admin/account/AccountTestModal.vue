<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.testAccountConnection')"
    width="normal"
    @close="handleClose"
  >
    <div class="space-y-4">
      <!-- Account Info Card -->
      <div
        v-if="account"
        class="flex items-center justify-between rounded-xl border border-gray-200 bg-gradient-to-r from-gray-50 to-gray-100 p-3 dark:border-dark-500 dark:from-dark-700 dark:to-dark-600"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-primary-500 to-primary-600"
          >
            <Icon name="play" size="md" class="text-white" :stroke-width="2" />
          </div>
          <div>
            <div class="font-semibold text-gray-900 dark:text-gray-100">{{ account.name }}</div>
            <div class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
              <span
                class="rounded bg-gray-200 px-1.5 py-0.5 text-[10px] font-medium uppercase dark:bg-dark-500"
              >
                {{ account.type }}
              </span>
              <span>{{ t('admin.accounts.account') }}</span>
            </div>
          </div>
        </div>
        <span
          :class="[
            'rounded-full px-2.5 py-1 text-xs font-semibold',
            account.status === 'active'
              ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
              : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
          ]"
        >
          {{ account.status }}
        </span>
      </div>

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.selectTestModel') }}
        </label>
        <Select
          v-model="selectedModelId"
          :options="availableModels"
          :disabled="loadingModels || status === 'connecting'"
          value-key="id"
          label-key="display_name"
          :placeholder="loadingModels ? t('common.loading') + '...' : t('admin.accounts.selectTestModel')"
        />
      </div>

      <div v-if="supportsTestMode" class="space-y-1.5">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.openai.testMode') }}
        </label>
        <Select
          v-model="testMode"
          :options="testModeOptions"
          :disabled="status === 'connecting'"
          @update:model-value="handleTestModeChange"
        />
      </div>

      <div v-if="supportsImageTest" class="space-y-1.5">
        <TextArea
          v-model="testPrompt"
          :label="t('admin.accounts.imagePromptLabel')"
          :placeholder="t('admin.accounts.imagePromptPlaceholder')"
          :hint="t('admin.accounts.imageTestHint')"
          :disabled="status === 'connecting'"
          rows="3"
        />
      </div>

      <!-- Terminal Output -->
      <div class="group relative">
        <div
          ref="terminalRef"
          class="max-h-[240px] min-h-[120px] overflow-y-auto rounded-xl border border-gray-700 bg-gray-900 p-4 font-mono text-sm dark:border-gray-800 dark:bg-black"
        >
          <!-- Status Line -->
          <div v-if="status === 'idle'" class="flex items-center gap-2 text-gray-500">
            <Icon name="play" size="sm" :stroke-width="2" />
            <span>{{ t('admin.accounts.readyToTest') }}</span>
          </div>
          <div v-else-if="status === 'connecting'" class="flex items-center gap-2 text-yellow-400">
            <Icon name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
            <span>{{ t('admin.accounts.connectingToApi') }}</span>
          </div>

          <!-- Output Lines -->
          <div v-for="(line, index) in outputLines" :key="index" :class="line.class">
            {{ line.text }}
          </div>

          <!-- Streaming Content -->
          <div v-if="streamingContent" class="text-green-400">
            {{ streamingContent }}<span class="animate-pulse">_</span>
          </div>

          <!-- Result Status -->
          <div
            v-if="status === 'success'"
            class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-green-400"
          >
            <Icon name="check" size="sm" :stroke-width="2" />
            <span>{{ t('admin.accounts.testCompleted') }}</span>
          </div>
          <div
            v-else-if="status === 'error'"
            class="mt-3 flex items-center gap-2 border-t border-gray-700 pt-3 text-red-400"
          >
            <Icon name="x" size="sm" :stroke-width="2" />
            <span>{{ errorMessage }}</span>
          </div>
        </div>

        <!-- Copy Button -->
        <button
          v-if="outputLines.length > 0"
          @click="copyOutput"
          class="absolute right-2 top-2 rounded-lg bg-gray-800/80 p-1.5 text-gray-400 opacity-0 transition-all hover:bg-gray-700 hover:text-white group-hover:opacity-100"
          :title="t('admin.accounts.copyOutput')"
        >
          <Icon name="link" size="sm" :stroke-width="2" />
        </button>
      </div>

      <div v-if="generatedImages.length > 0" class="space-y-2">
        <div class="text-xs font-medium text-gray-600 dark:text-gray-300">
          {{ t('admin.accounts.imagePreview') }}
        </div>
        <div class="flex flex-wrap justify-center gap-3">
          <div
            v-for="(image, index) in generatedImages"
            :key="`${image.url}-${index}`"
            class="group/img relative cursor-pointer overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm transition hover:border-primary-300 hover:shadow-md dark:border-dark-500 dark:bg-dark-700"
            @click="previewImageUrl = image.url"
          >
            <img :src="image.url" :alt="`test-image-${index + 1}`" class="max-h-[360px] w-full object-contain" />
            <div class="absolute inset-0 flex items-center justify-center bg-black/0 transition-colors group-hover/img:bg-black/20">
              <Icon name="eye" size="lg" class="text-white opacity-0 drop-shadow-lg transition-opacity group-hover/img:opacity-100" :stroke-width="2" />
            </div>
            <div class="border-t border-gray-100 px-3 py-1.5 text-xs text-gray-500 dark:border-dark-500 dark:text-gray-300">
              {{ image.mimeType || 'image/*' }}
            </div>
          </div>
        </div>
      </div>

      <!-- Image Lightbox -->
      <Teleport to="body">
        <Transition name="fade">
          <div
            v-if="previewImageUrl"
            class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
            @click.self="previewImageUrl = ''"
          >
            <button
              class="absolute right-4 top-4 rounded-full bg-black/50 p-2 text-white transition-colors hover:bg-black/70"
              @click="previewImageUrl = ''"
            >
              <Icon name="x" size="lg" :stroke-width="2" />
            </button>
            <img
              :src="previewImageUrl"
              alt="preview"
              class="max-h-[90vh] max-w-[90vw] rounded-lg object-contain shadow-2xl"
            />
          </div>
        </Transition>
      </Teleport>

      <!-- Test Info -->
      <div class="flex items-center justify-between px-1 text-xs text-gray-500 dark:text-gray-400">
        <div class="flex items-center gap-3">
          <span class="flex items-center gap-1">
            <Icon name="grid" size="sm" :stroke-width="2" />
            {{ t('admin.accounts.testModel') }}
          </span>
        </div>
        <span class="flex items-center gap-1">
          <Icon name="chat" size="sm" :stroke-width="2" />
          {{
            supportsImageTest
              ? t('admin.accounts.imageTestMode')
              : t('admin.accounts.testPrompt')
          }}
        </span>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.close') }}
        </button>
        <button
          @click="startTest"
          :disabled="status === 'connecting' || !selectedModelId"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all',
            status === 'connecting' || !selectedModelId
              ? 'cursor-not-allowed bg-primary-400 text-white'
              : status === 'success'
                ? 'bg-green-500 text-white hover:bg-green-600'
                : status === 'error'
                  ? 'bg-orange-500 text-white hover:bg-orange-600'
                  : 'bg-primary-500 text-white hover:bg-primary-600'
          ]"
        >
          <Icon
            v-if="status === 'connecting'"
            name="refresh"
            size="sm"
            class="animate-spin"
            :stroke-width="2"
          />
          <Icon v-else-if="status === 'idle'" name="play" size="sm" :stroke-width="2" />
          <Icon v-else name="refresh" size="sm" :stroke-width="2" />
          <span>
            {{
              status === 'connecting'
                ? t('admin.accounts.testing')
                : status === 'idle'
                  ? t('admin.accounts.startTest')
                  : t('admin.accounts.retry')
            }}
          </span>
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import { Icon } from '@/components/icons'
import { useClipboard } from '@/composables/useClipboard'
import { buildApiUrl } from '@/api/client'
import { ADMIN_UI_REQUEST_HEADER } from '@/api/adminUIRequest'
import { adminAPI } from '@/api/admin'
import { normalizeDisplayErrorMessage } from '@/utils/errorMessage'
import { resolveAccountTestModeForModel, resolveAccountTestModelSelection } from '@/utils/accountTestModelSelection'
import type { AccountTestMode } from '@/api/admin/accounts'
import type { Account, ClaudeModel } from '@/types'

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

interface OutputLine {
  text: string
  class: string
}

interface PreviewImage {
  url: string
  mimeType?: string
}

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'testing-changed', testing: boolean): void
  (e: 'account-updated', account: Account): void
}>()

const terminalRef = ref<HTMLElement | null>(null)
const status = ref<'idle' | 'connecting' | 'success' | 'error'>('idle')
const outputLines = ref<OutputLine[]>([])
const streamingContent = ref('')
const errorMessage = ref('')
const availableModels = ref<ClaudeModel[]>([])
const selectedModelId = ref('')
const testPrompt = ref('')
const loadingModels = ref(false)
let abortController: AbortController | null = null
const generatedImages = ref<PreviewImage[]>([])
const previewImageUrl = ref('')
// testMode 为当前账号持久化的模型测试模式，OpenAI 和 DeepSeek 共用该字段。
const testMode = ref<AccountTestMode>('default')
// 已确认写入账号配置的模式，保存失败时用于回滚界面选择。
let persistedTestMode: AccountTestMode = 'default'
// 保存序号保证快速连续切换时，最后一次选择最终写入账号配置。
let testModeRevision = 0
let savedTestModeRevision = 0
let testModeSaveTask: Promise<void> | null = null
const isOpenAIAccount = computed(() => props.account?.platform === 'openai')
const isDeepSeekAccount = computed(() => props.account?.platform === 'deepseek')
const supportsTestMode = computed(() => isOpenAIAccount.value || isDeepSeekAccount.value)
// DeepSeek 仅支持 Chat Completions 和 Responses；OpenAI 保留既有四种探测模式。
const testModeOptions = computed(() => {
  const options = [
    { value: 'default', label: t('admin.accounts.openai.testModeDefault') },
    { value: 'responses', label: t('admin.accounts.openai.testModeResponses') }
  ]
  if (isDeepSeekAccount.value) return options
  return [
    ...options,
    { value: 'compact', label: t('admin.accounts.openai.testModeCompact') },
    { value: 'workspace', label: t('admin.accounts.openai.testModeWorkspace') }
  ]
})

// resolveAccountTestMode 仅接受后端允许保存的值，旧账号或异常值均按默认模式处理。
const resolveAccountTestMode = (account: Account | null): AccountTestMode => {
  const mode = account?.extra?.account_test_mode
  if (account?.platform === 'deepseek') {
    return mode === 'responses' || mode === 'default' ? mode : 'default'
  }
  return mode === 'responses' || mode === 'compact' || mode === 'workspace' || mode === 'default'
    ? mode
    : 'default'
}

// handleTestModeChange 将用户选择立即加入保存队列，避免关闭并重开弹窗后回到默认值。
const handleTestModeChange = (value: string | number | boolean | null) => {
  const allowedModes = isDeepSeekAccount.value
    ? (['default', 'responses'] as AccountTestMode[])
    : (['default', 'responses', 'compact', 'workspace'] as AccountTestMode[])
  if (typeof value !== 'string') return
  // nextMode 是经过平台模式白名单校验后的强类型值。
  const nextMode = value as AccountTestMode
  if (!allowedModes.includes(nextMode)) return
  testMode.value = nextMode
  if (!props.show || !supportsTestMode.value || !props.account) return

  testModeRevision += 1
  if (!testModeSaveTask) {
    const accountID = props.account.id
    testModeSaveTask = flushTestModeSave(accountID).finally(() => {
      testModeSaveTask = null
    })
  }
}

// flushTestModeSave 串行写入模式；连续切换时总会在前一次结束后写入最后一次选择。
const flushTestModeSave = async (accountID: number): Promise<void> => {
  while (savedTestModeRevision < testModeRevision) {
    const revision = testModeRevision
    const modeToSave = testMode.value
    try {
      const updatedAccount = await adminAPI.accounts.updateTestMode(accountID, modeToSave)
      persistedTestMode = modeToSave
      savedTestModeRevision = revision
      if (revision === testModeRevision) emit('account-updated', updatedAccount)
    } catch (error: unknown) {
      // 旧选择失败而用户已选了新值时，继续写入最新选择；最终选择失败才回滚。
      if (revision !== testModeRevision) continue
      testMode.value = persistedTestMode
      savedTestModeRevision = revision
      const technicalDetails = error instanceof Error ? error.message : String(error)
      errorMessage.value = normalizeDisplayErrorMessage(technicalDetails, '保存模型测试模式失败，请检查账号权限和网络后重试。')
      status.value = 'error'
      addLine(`错误：${errorMessage.value}`, 'text-red-400')
    }
  }
}
const supportsGeminiImageTest = computed(() => {
  const modelID = selectedModelId.value.toLowerCase()
  if (!modelID.startsWith('gemini-') || !modelID.includes('-image')) return false

  return props.account?.platform === 'gemini' || (props.account?.platform === 'antigravity' && props.account?.type === 'apikey')
})

const supportsOpenAIImageTest = computed(() => {
  const modelID = selectedModelId.value.toLowerCase()
  if (!modelID.startsWith('gpt-image-')) return false
  return props.account?.platform === 'openai'
})

const supportsImageTest = computed(() => supportsGeminiImageTest.value || supportsOpenAIImageTest.value)

// Load available models when modal opens
watch(
  () => props.show,
  async (newVal) => {
    if (newVal && props.account) {
      testPrompt.value = ''
      // 仅在账号缺少保存值时使用 default；不会因重新打开弹窗覆盖用户设置。
      persistedTestMode = resolveAccountTestMode(props.account)
      testMode.value = persistedTestMode
      testModeRevision = 0
      savedTestModeRevision = 0
      resetState()
      await loadAvailableModels()
    } else {
      abortStream()
    }
  }
)

watch(selectedModelId, () => {
  // DeepSeek 的 Responses 仅适用于 V4 Flash，切换到普通模型时自动回到 Chat。
  if (isDeepSeekAccount.value && testMode.value === 'responses' && selectedModelId.value.trim().toLowerCase() !== 'deepseek-v4-flash') {
    testMode.value = 'default'
  }
  if (supportsImageTest.value && !testPrompt.value.trim()) {
    testPrompt.value = t('admin.accounts.imagePromptDefault')
  }
})

const loadAvailableModels = async () => {
  if (!props.account) return

  loadingModels.value = true
  selectedModelId.value = '' // Reset selection before loading

  try {
    const models = await adminAPI.accounts.getAvailableModels(props.account.id)
    // 弹窗与状态栏快捷检测必须共享同一预填模型，避免同账号检测到不同模型。
    const selection = resolveAccountTestModelSelection(props.account.platform, models)
    availableModels.value = selection.models
    selectedModelId.value = selection.modelId
    // 未保存模式的 DeepSeek 账号让 V4 Flash 直接使用 Responses，其余模型使用 Chat。
    if (isDeepSeekAccount.value && !props.account.extra?.account_test_mode) {
      const defaultMode = resolveAccountTestModeForModel(props.account.platform, selection.modelId)
      testMode.value = defaultMode
      persistedTestMode = defaultMode
    }
    if (isDeepSeekAccount.value && testMode.value === 'responses' && selection.modelId.trim().toLowerCase() !== 'deepseek-v4-flash') {
      testMode.value = 'default'
    }
  } catch (error) {
    console.error('Failed to load available models:', error)
    // Fallback to empty list
    availableModels.value = []
    selectedModelId.value = ''
  } finally {
    loadingModels.value = false
  }
}

const resetState = () => {
  status.value = 'idle'
  outputLines.value = []
  streamingContent.value = ''
  errorMessage.value = ''
  generatedImages.value = []
  previewImageUrl.value = ''
}

const handleClose = () => {
  abortStream()
  emit('testing-changed', false)
  emit('close')
}

const abortStream = () => {
  if (abortController) {
    abortController.abort()
    abortController = null
  }
}

const addLine = (text: string, className: string = 'text-gray-300') => {
  outputLines.value.push({ text, class: className })
  scrollToBottom()
}

const scrollToBottom = async () => {
  await nextTick()
  if (terminalRef.value) {
    terminalRef.value.scrollTop = terminalRef.value.scrollHeight
  }
}

const startTest = async () => {
  if (!props.account || !selectedModelId.value) return

  resetState()
  status.value = 'connecting'
  emit('testing-changed', true)
  addLine(t('admin.accounts.startingTestForAccount', { name: props.account.name }), 'text-blue-400')
  addLine(t('admin.accounts.testAccountTypeLabel', { type: props.account.type }), 'text-gray-400')
  addLine('', 'text-gray-300')

  abortStream()

  abortController = new AbortController()

  try {
    const requestBody: {
      model_id: string
      prompt: string
      mode?: 'default' | 'responses' | 'compact' | 'workspace'
    } = {
      model_id: selectedModelId.value,
      prompt: supportsImageTest.value ? testPrompt.value.trim() : ''
    }
    if (supportsTestMode.value) {
      requestBody.mode = testMode.value
      if (testMode.value === 'workspace') {
        addLine(t('admin.accounts.workspaceProbeStarted'), 'text-cyan-300')
      }
    }

    // Use the configured API base; EventSource does not support POST.
    const url = buildApiUrl(`/admin/accounts/${props.account.id}/test`)

    // Use fetch with streaming for SSE since EventSource doesn't support POST
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
        'Content-Type': 'application/json',
        [ADMIN_UI_REQUEST_HEADER]: '1'
      },
      body: JSON.stringify(requestBody),
      signal: abortController.signal
    })

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error('No response body')
    }

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const jsonStr = line.slice(6).trim()
          if (jsonStr) {
            try {
              const event = JSON.parse(jsonStr)
              handleEvent(event)
            } catch (e) {
              console.error('Failed to parse SSE event:', e)
            }
          }
        }
      }
    }
  } catch (error: unknown) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      status.value = 'idle'
      return
    }
    status.value = 'error'
    const msg = normalizeAccountTestErrorMessage(error instanceof Error ? error.message : '')
    errorMessage.value = msg
    addLine(`错误：${msg}`, 'text-red-400')
  } finally {
    emit('testing-changed', false)
  }
}

const handleEvent = (event: {
  type: string
  text?: string
  model?: string
  success?: boolean
  error?: string
  code?: string
  image_url?: string
  mime_type?: string
}) => {
  switch (event.type) {
    case 'test_start':
      addLine(t('admin.accounts.connectedToApi'), 'text-green-400')
      if (event.model) {
        addLine(t('admin.accounts.usingModel', { model: event.model }), 'text-cyan-400')
      }
      addLine(
        supportsImageTest.value
            ? t('admin.accounts.sendingImageRequest')
            : t('admin.accounts.sendingTestMessage'),
        'text-gray-400'
      )
      addLine('', 'text-gray-300')
      addLine(t('admin.accounts.response'), 'text-yellow-400')
      break

    case 'content':
      if (event.text) {
        streamingContent.value += event.text
        scrollToBottom()
      }
      break

    case 'image':
      if (event.image_url) {
        generatedImages.value.push({
          url: event.image_url,
          mimeType: event.mime_type
        })
        addLine(t('admin.accounts.imageReceived', { count: generatedImages.value.length }), 'text-purple-300')
      }
      break

    case 'status':
      if (event.text) {
        addLine(normalizeAccountTestStatusMessage(event.text), 'text-cyan-300')
      }
      break

    case 'workspace_deactivated':
      status.value = 'error'
      errorMessage.value = normalizeAccountTestErrorMessage(event.error || event.code || 'deactivated_workspace')
      addLine(errorMessage.value, 'text-red-400')
      break

    case 'test_complete':
      // Move streaming content to output lines
      if (streamingContent.value) {
        addLine(streamingContent.value, 'text-green-300')
        streamingContent.value = ''
      }
      if (event.success) {
        status.value = 'success'
      } else {
        status.value = 'error'
        errorMessage.value = normalizeAccountTestErrorMessage(event.error)
      }
      break

    case 'error':
      status.value = 'error'
      errorMessage.value = normalizeAccountTestErrorMessage(event.error)
      if (streamingContent.value) {
        addLine(streamingContent.value, 'text-green-300')
        streamingContent.value = ''
      }
      break
  }
}

// normalizeAccountTestErrorMessage 统一显示中文说明与管理员后端提供的技术详情。
const normalizeAccountTestErrorMessage = (rawMessage?: string): string => {
  return normalizeDisplayErrorMessage(rawMessage, t('admin.accounts.testFailed'))
}

// normalizeAccountTestStatusMessage 将旧服务的英文状态信息转换为当前界面语言。
const normalizeAccountTestStatusMessage = (rawMessage: string): string => {
  if (rawMessage.toLowerCase().includes('upstream connection closed unexpectedly')) {
    return t('admin.accounts.upstreamRetrying')
  }
  return rawMessage
}

const copyOutput = () => {
  const text = outputLines.value.map((l) => l.text).join('\n')
  copyToClipboard(text, t('admin.accounts.outputCopied'))
}
</script>

<style>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>

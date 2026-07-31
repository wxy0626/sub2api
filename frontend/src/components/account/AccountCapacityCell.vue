<template>
  <div class="flex flex-col gap-0.5">
    <div
      v-if="isEditing"
      class="flex min-w-[136px] flex-col gap-1.5 rounded-md border border-gray-200 bg-white/70 p-1.5 dark:border-dark-600 dark:bg-dark-800/70"
      data-testid="account-capacity-editor"
      @click.stop
    >
      <label class="flex items-center justify-between gap-2 text-[10px] font-medium text-gray-600 dark:text-gray-300">
        <span>{{ t('admin.accounts.concurrencyLimit') }}</span>
        <input
          :value="concurrencyDraft"
          data-testid="concurrency-input"
          type="number"
          min="1"
          step="1"
          class="w-16 rounded border border-gray-300 bg-white px-1.5 py-0.5 text-right font-mono text-xs text-gray-700 outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-100"
          :disabled="saving"
          :aria-label="t('admin.accounts.concurrencyLimit')"
          @input="handleConcurrencyInput($event)"
        />
      </label>
      <label class="flex items-center justify-between gap-2 text-[10px] font-medium text-gray-600 dark:text-gray-300">
        <span>{{ t('admin.accounts.loadFactor') }}</span>
        <input
          :value="loadFactorDraft"
          data-testid="load-factor-input"
          type="number"
          min="1"
          max="10000"
          step="1"
          class="w-16 rounded border border-gray-300 bg-white px-1.5 py-0.5 text-right font-mono text-xs text-gray-700 outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-100"
          :disabled="saving"
          :aria-label="t('admin.accounts.loadFactor')"
          @input="handleLoadFactorInput($event)"
        />
      </label>
      <span v-if="loadFactorFollowsConcurrency" class="text-[10px] text-gray-400 dark:text-gray-500">
        {{ t('admin.accounts.loadFactorFollowConcurrency') }}
      </span>
      <span v-if="inputError" data-testid="account-capacity-error" class="text-[10px] leading-4 text-red-600 dark:text-red-300">
        {{ inputError }}
      </span>
      <div class="flex justify-end gap-1">
        <button
          type="button"
          data-testid="account-capacity-cancel"
          class="rounded px-2 py-0.5 text-[10px] font-medium text-gray-600 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-300 dark:hover:bg-dark-700"
          :disabled="saving"
          @click="cancelCapacityEditing"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          data-testid="account-capacity-save"
          class="rounded bg-primary-500 px-2 py-0.5 text-[10px] font-medium text-white hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="saving"
          @click="saveCapacity"
        >
          {{ t('common.save') }}
        </button>
      </div>
    </div>

    <!-- 并发槽位 -->
    <component
      v-if="!isEditing"
      :is="editable ? 'button' : 'div'"
      :type="editable ? 'button' : undefined"
      data-testid="account-capacity-display"
      class="inline-flex self-start"
      :class="editable ? 'cursor-pointer rounded-md hover:ring-2 hover:ring-primary-500/40 focus:outline-none focus:ring-2 focus:ring-primary-500' : ''"
      :disabled="editable ? saving : undefined"
      @click="editable ? startCapacityEditing() : undefined"
    >
      <CapacityBadge :color-class="concurrencyClass" :current="currentConcurrency" :max="account.concurrency">
        <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />
        </svg>
      </CapacityBadge>
    </component>

    <!-- 5h窗口费用限制 -->
    <CapacityBadge v-if="showWindowCost" :color-class="windowCostClass" :tooltip="windowCostTooltip" :current="'$' + formatCost(currentWindowCost)" :max="'$' + formatCost(account.window_cost_limit)">
      <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v12m-3-2.818l.879.659c1.171.879 3.07.879 4.242 0 1.172-.879 1.172-2.303 0-3.182C13.536 12.219 12.768 12 12 12c-.725 0-1.45-.22-2.003-.659-1.106-.879-1.106-2.303 0-3.182s2.9-.879 4.006 0l.415.33M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
    </CapacityBadge>

    <!-- 会话数量限制 -->
    <CapacityBadge v-if="showSessionLimit" :color-class="sessionLimitClass" :tooltip="sessionLimitTooltip" :current="activeSessions" :max="account.max_sessions!">
      <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
      </svg>
    </CapacityBadge>

    <!-- RPM 限制 -->
    <CapacityBadge v-if="showRpmLimit" :color-class="rpmClass" :tooltip="rpmTooltip" :current="currentRPM" :max="account.base_rpm!" :suffix="rpmStrategyTag">
      <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
      </svg>
    </CapacityBadge>

    <!-- API Key 账号配额限制 -->
    <QuotaBadge v-if="showDailyQuota" :used="account.quota_daily_used ?? 0" :limit="account.quota_daily_limit!" label="D" />
    <QuotaBadge v-if="showWeeklyQuota" :used="account.quota_weekly_used ?? 0" :limit="account.quota_weekly_limit!" label="W" />
    <QuotaBadge v-if="showTotalQuota" :used="account.quota_used ?? 0" :limit="account.quota_limit!" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, UpdateAccountRequest } from '@/types'
import CapacityBadge from '@/components/account/CapacityBadge.vue'
import QuotaBadge from '@/components/account/QuotaBadge.vue'

// 列表中的并发容量编辑事件只携带账号更新接口已支持的两个字段。
type AccountCapacityUpdate = Pick<UpdateAccountRequest, 'concurrency' | 'load_factor'>

const props = withDefaults(defineProps<{
  account: Account
  editable?: boolean
  saving?: boolean
}>(), {
  editable: false,
  saving: false
})

const emit = defineEmits<{
  save: [updates: AccountCapacityUpdate]
}>()

const { t } = useI18n()

// 记录容量单元格是否处于编辑状态，默认只展示当前占用和容量。
const isEditing = ref(false)
// 并发容量输入草稿，使用字符串保留用户清空输入时的中间状态。
const concurrencyDraft = ref('')
// 负载因子输入草稿；未显式设置时显示并发容量并跟随其变化。
const loadFactorDraft = ref('')
// 标记负载因子是否已被管理员显式修改，false 时保存为后端约定的跟随状态。
const loadFactorFollowsConcurrency = ref(true)
// 输入校验失败时展示可执行的中文原因，避免管理员只能看到输入被恢复。
const inputError = ref<string | null>(null)

// 根据服务端账号值同步列表单元格草稿，避免自动刷新覆盖未变化的本地输入。
const resetCapacityDraft = () => {
  const concurrency = props.account.concurrency > 0 ? props.account.concurrency : 1
  const explicitLoadFactor = props.account.load_factor != null && props.account.load_factor > 0
  concurrencyDraft.value = String(concurrency)
  loadFactorFollowsConcurrency.value = !explicitLoadFactor
  loadFactorDraft.value = String(explicitLoadFactor ? props.account.load_factor : concurrency)
}

watch(
  () => [props.account.id, props.account.concurrency, props.account.load_factor] as const,
  () => {
    resetCapacityDraft()
    isEditing.value = false
  },
  { immediate: true }
)

// 点击只读容量徽章后加载当前值，并进入可编辑状态。
const startCapacityEditing = () => {
  if (!props.editable || props.saving) return
  resetCapacityDraft()
  inputError.value = null
  isEditing.value = true
}

// 取消编辑时丢弃本地草稿，恢复服务端账号值和只读显示。
const cancelCapacityEditing = () => {
  resetCapacityDraft()
  inputError.value = null
  isEditing.value = false
}

// 从数字输入事件读取字符串，保留空值等用户编辑中的中间状态。
const readCapacityInputValue = (event: Event): string => {
  const target = event.currentTarget
  return target instanceof HTMLInputElement ? target.value : ''
}

// 负载因子未显式修改时，随着并发容量输入同步显示默认值。
const handleConcurrencyInput = (event: Event) => {
  concurrencyDraft.value = readCapacityInputValue(event)
  inputError.value = null
  if (loadFactorFollowsConcurrency.value) {
    loadFactorDraft.value = concurrencyDraft.value
  }
}

// 清空负载因子表示恢复“跟随并发容量”，填写数值则切换为显式负载因子。
const handleLoadFactorInput = (event: Event) => {
  loadFactorDraft.value = readCapacityInputValue(event)
  inputError.value = null
  loadFactorFollowsConcurrency.value = loadFactorDraft.value.trim() === ''
}

// 将输入转换为后端可接受的正整数，非法输入会在提交时恢复为服务端值。
const parsePositiveInteger = (value: string, max?: number): number | null => {
  const parsed = Number(value)
  if (!Number.isSafeInteger(parsed) || parsed < 1 || (max != null && parsed > max)) return null
  return parsed
}

// 保存并发容量和负载因子；负载因子跟随时发送 0，让后端清除显式值。
const saveCapacity = () => {
  if (!props.editable || !isEditing.value || props.saving) return

  const concurrency = parsePositiveInteger(concurrencyDraft.value)
  const loadFactor = loadFactorFollowsConcurrency.value
    ? concurrency
    : parsePositiveInteger(loadFactorDraft.value, 10000)
  if (concurrency == null || loadFactor == null) {
    inputError.value = t('admin.accounts.concurrencyCapacityInvalid')
    resetCapacityDraft()
    return
  }

  const currentLoadFactor = props.account.load_factor ?? props.account.concurrency
  if (concurrency === props.account.concurrency && loadFactor === currentLoadFactor) return

  isEditing.value = false
  emit('save', {
    concurrency,
    load_factor: loadFactorFollowsConcurrency.value ? 0 : loadFactor
  })
}

// ====== 并发 ======
const currentConcurrency = computed(() => props.account.current_concurrency || 0)

const concurrencyClass = computed(() => {
  const current = currentConcurrency.value
  const max = props.account.concurrency
  if (current >= max) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  if (current > 0) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
})

// ====== 窗口费用 ======
const isAnthropicOAuthOrSetupToken = computed(() =>
  props.account.platform === 'anthropic' &&
  (props.account.type === 'oauth' || props.account.type === 'setup-token')
)

const showWindowCost = computed(() =>
  isAnthropicOAuthOrSetupToken.value &&
  props.account.window_cost_limit != null &&
  props.account.window_cost_limit > 0
)

const currentWindowCost = computed(() => props.account.current_window_cost ?? 0)

const windowCostClass = computed(() => {
  if (!showWindowCost.value) return ''
  const current = currentWindowCost.value
  const limit = props.account.window_cost_limit || 0
  const reserve = props.account.window_cost_sticky_reserve || 10
  if (current >= limit + reserve) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  if (current >= limit) return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
  if (current >= limit * 0.8) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
})

const windowCostTooltip = computed(() => {
  if (!showWindowCost.value) return ''
  const current = currentWindowCost.value
  const limit = props.account.window_cost_limit || 0
  const reserve = props.account.window_cost_sticky_reserve || 10
  if (current >= limit + reserve) return t('admin.accounts.capacity.windowCost.blocked')
  if (current >= limit) return t('admin.accounts.capacity.windowCost.stickyOnly')
  return t('admin.accounts.capacity.windowCost.normal')
})

// ====== 会话限制 ======
const showSessionLimit = computed(() =>
  isAnthropicOAuthOrSetupToken.value &&
  props.account.max_sessions != null &&
  props.account.max_sessions > 0
)

const activeSessions = computed(() => props.account.active_sessions ?? 0)

const sessionLimitClass = computed(() => {
  if (!showSessionLimit.value) return ''
  const current = activeSessions.value
  const max = props.account.max_sessions || 0
  if (current >= max) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  if (current >= max * 0.8) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
})

const sessionLimitTooltip = computed(() => {
  if (!showSessionLimit.value) return ''
  const current = activeSessions.value
  const max = props.account.max_sessions || 0
  const idle = props.account.session_idle_timeout_minutes || 5
  if (current >= max) return t('admin.accounts.capacity.sessions.full', { idle })
  return t('admin.accounts.capacity.sessions.normal', { idle })
})

// ====== RPM ======
const showRpmLimit = computed(() =>
  isAnthropicOAuthOrSetupToken.value &&
  props.account.base_rpm != null &&
  props.account.base_rpm > 0
)

const currentRPM = computed(() => props.account.current_rpm ?? 0)
const rpmStrategy = computed(() => props.account.rpm_strategy || 'tiered')
const rpmStrategyTag = computed(() => rpmStrategy.value === 'sticky_exempt' ? '[S]' : '[T]')

const rpmBuffer = computed(() => {
  const base = props.account.base_rpm || 0
  return props.account.rpm_sticky_buffer ?? (base > 0 ? Math.max(1, Math.floor(base / 5)) : 0)
})

const rpmClass = computed(() => {
  if (!showRpmLimit.value) return ''
  const current = currentRPM.value
  const base = props.account.base_rpm ?? 0
  const buffer = rpmBuffer.value
  if (rpmStrategy.value === 'tiered') {
    if (current >= base + buffer) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
    if (current >= base) return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
  } else {
    if (current >= base) return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
  }
  if (current >= base * 0.8) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
})

const rpmTooltip = computed(() => {
  if (!showRpmLimit.value) return ''
  const current = currentRPM.value
  const base = props.account.base_rpm ?? 0
  const buffer = rpmBuffer.value
  if (rpmStrategy.value === 'tiered') {
    if (current >= base + buffer) return t('admin.accounts.capacity.rpm.tieredBlocked', { buffer })
    if (current >= base) return t('admin.accounts.capacity.rpm.tieredStickyOnly', { buffer })
    if (current >= base * 0.8) return t('admin.accounts.capacity.rpm.tieredWarning')
    return t('admin.accounts.capacity.rpm.tieredNormal')
  } else {
    if (current >= base) return t('admin.accounts.capacity.rpm.stickyExemptOver')
    if (current >= base * 0.8) return t('admin.accounts.capacity.rpm.stickyExemptWarning')
    return t('admin.accounts.capacity.rpm.stickyExemptNormal')
  }
})

// 格式化费用显示
const formatCost = (value: number | null | undefined) => {
  if (value === null || value === undefined) return '0'
  return value.toFixed(2)
}

// ====== 配额 ======
const isQuotaEligible = computed(() => props.account.type === 'apikey' || props.account.type === 'bedrock')

const showDailyQuota = computed(() =>
  isQuotaEligible.value && props.account.quota_daily_limit != null && props.account.quota_daily_limit > 0
)
const showWeeklyQuota = computed(() =>
  isQuotaEligible.value && props.account.quota_weekly_limit != null && props.account.quota_weekly_limit > 0
)
const showTotalQuota = computed(() =>
  isQuotaEligible.value && props.account.quota_limit != null && props.account.quota_limit > 0
)
</script>

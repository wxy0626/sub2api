<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center gap-2 text-sm">
      <span class="inline-flex items-center gap-1.5 rounded-full bg-amber-100 px-3 py-1 font-medium text-amber-800 dark:bg-amber-900/30 dark:text-amber-300">
        <Icon name="server" size="sm" />
        {{ t('admin.usage.accountTest.badge') }}
      </span>
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.accountTest.noBilling') }}</span>
    </div>

    <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <div class="card flex items-center gap-3 p-4">
        <div class="rounded-lg bg-blue-100 p-2 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400"><Icon name="document" size="md" /></div>
        <div>
          <p class="text-xs font-medium text-gray-500">{{ t('admin.usage.accountTest.totalRequests') }}</p>
          <p class="text-xl font-bold">{{ formatNumber(stats?.total_requests) }}</p>
        </div>
      </div>
      <div class="card flex items-center gap-3 p-4">
        <div class="rounded-lg bg-green-100 p-2 text-green-600 dark:bg-green-900/30 dark:text-green-400"><Icon name="checkCircle" size="md" /></div>
        <div>
          <p class="text-xs font-medium text-gray-500">{{ t('admin.usage.accountTest.successfulRequests') }}</p>
          <p class="text-xl font-bold text-green-600">{{ formatNumber(stats?.successful_requests) }}</p>
          <p class="text-xs text-gray-400">{{ t('admin.usage.accountTest.failedRequests') }}: {{ formatNumber(stats?.failed_requests) }}</p>
        </div>
      </div>
      <div class="card flex items-center gap-3 p-4">
        <div class="rounded-lg bg-amber-100 p-2 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400"><Icon name="chart" size="md" /></div>
        <div>
          <p class="text-xs font-medium text-gray-500">{{ t('admin.usage.accountTest.totalTokens') }}</p>
          <p class="text-xl font-bold">{{ formatTokens(stats?.total_tokens) }}</p>
          <p class="text-xs text-gray-400">{{ t('admin.usage.accountTest.inputOutput') }} {{ formatTokens(stats?.input_tokens) }} / {{ formatTokens(stats?.output_tokens) }}</p>
        </div>
      </div>
      <div class="card flex items-center gap-3 p-4">
        <div class="rounded-lg bg-purple-100 p-2 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400"><Icon name="clock" size="md" /></div>
        <div>
          <p class="text-xs font-medium text-gray-500">{{ t('admin.usage.accountTest.averageDuration') }}</p>
          <p class="text-xl font-bold">{{ formatDuration(stats?.avg_duration_ms) }}</p>
          <p class="text-xs text-gray-400">{{ t('admin.usage.accountTest.cacheTokens') }}: {{ formatTokens(stats?.cache_tokens) }}</p>
        </div>
      </div>
    </div>

    <div v-if="stats?.by_platform?.length" class="flex flex-wrap gap-2">
      <span v-for="item in stats.by_platform" :key="item.platform" class="inline-flex items-center gap-1 rounded border border-gray-200 px-2.5 py-1 text-xs text-gray-600 dark:border-dark-600 dark:text-gray-300">
        <span class="font-medium">{{ item.platform || '-' }}</span>
        <span>{{ item.total_requests }} {{ t('admin.usage.accountTest.requests') }}</span>
        <span class="text-gray-400">/</span>
        <span>{{ formatTokens(item.total_tokens) }} {{ t('admin.usage.accountTest.tokens') }}</span>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { AccountTestUsageStatsResponse } from '@/api/admin/usage'

defineProps<{
  stats: AccountTestUsageStatsResponse | null
}>()

const { t } = useI18n()

// 格式化全局测试统计，避免把未返回的字段显示成 NaN。
const formatNumber = (value: number | undefined): string => (value ?? 0).toLocaleString()
const formatTokens = (value: number | undefined): string => {
  const number = value ?? 0
  if (number >= 1e9) return (number / 1e9).toFixed(2) + 'B'
  if (number >= 1e6) return (number / 1e6).toFixed(2) + 'M'
  if (number >= 1e3) return (number / 1e3).toFixed(2) + 'K'
  return number.toLocaleString()
}
const formatDuration = (value: number | undefined): string => {
  const ms = value ?? 0
  return ms < 1000 ? Math.round(ms) + 'ms' : (ms / 1000).toFixed(2) + 's'
}
</script>

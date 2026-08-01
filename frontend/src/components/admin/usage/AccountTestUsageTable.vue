<template>
  <div class="overflow-auto">
    <table class="w-full min-w-[1080px] divide-y divide-gray-200 dark:divide-dark-700">
      <thead class="bg-gray-50 dark:bg-dark-800">
        <tr>
          <th v-for="column in columns" :key="column.key" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ column.label }}</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
        <template v-if="loading">
          <tr v-for="index in 5" :key="'loading-' + index">
            <td :colspan="columns.length" class="px-4 py-5"><div class="h-4 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div></td>
          </tr>
        </template>
        <tr v-else-if="records.length === 0">
          <td :colspan="columns.length" class="px-4 py-12 text-center text-gray-500 dark:text-gray-400">{{ t('admin.usage.accountTest.empty') }}</td>
        </tr>
        <template v-else>
          <tr v-for="record in records" :key="record.id" class="align-top hover:bg-gray-50 dark:hover:bg-dark-800">
          <td class="whitespace-nowrap px-4 py-4 text-sm text-gray-600 dark:text-gray-300">{{ formatDateTime(record.created_at) }}</td>
          <td class="px-4 py-4 text-sm font-medium text-gray-900 dark:text-white">{{ record.platform || '-' }}</td>
          <td class="px-4 py-4 text-sm text-gray-700 dark:text-gray-300">
            <div>{{ record.account_name || t('admin.usage.accountTest.unknownAccount') }}</div>
            <div class="mt-1 text-xs text-gray-400">#{{ record.account_id }}</div>
          </td>
          <td class="max-w-[220px] break-all px-4 py-4 text-sm text-gray-900 dark:text-white">{{ record.model || '-' }}</td>
          <td class="max-w-[260px] break-all px-4 py-4 text-xs text-gray-600 dark:text-gray-300">{{ record.endpoint || '-' }}</td>
          <td class="whitespace-nowrap px-4 py-4 text-sm tabular-nums text-gray-700 dark:text-gray-300">
            <div>{{ t('admin.usage.accountTest.inputShort') }} {{ formatNumber(record.input_tokens) }}</div>
            <div>{{ t('admin.usage.accountTest.outputShort') }} {{ formatNumber(record.output_tokens) }}</div>
            <div class="text-xs text-gray-400">{{ t('admin.usage.accountTest.cacheShort') }} {{ formatNumber(record.cache_creation_tokens) }} / {{ formatNumber(record.cache_read_tokens) }}</div>
            <div class="text-xs font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.usage.accountTest.totalShort') }} {{ formatNumber(totalTokens(record)) }}</div>
          </td>
          <td class="whitespace-nowrap px-4 py-4 text-sm text-gray-700 dark:text-gray-300">{{ record.test_mode || '-' }}</td>
          <td class="whitespace-nowrap px-4 py-4 text-sm">
            <span :class="record.success ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">{{ record.success ? t('admin.usage.accountTest.success') : t('admin.usage.accountTest.failed') }}</span>
          </td>
          <td class="whitespace-nowrap px-4 py-4 text-sm tabular-nums text-gray-700 dark:text-gray-300">HTTP {{ record.status_code || '-' }}</td>
          <td class="whitespace-nowrap px-4 py-4 text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ formatDuration(record.duration_ms) }}</td>
          <td class="max-w-[300px] break-words px-4 py-4 text-xs text-red-600 dark:text-red-400">{{ record.error_message || '-' }}</td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { AccountTestUsageRecord } from '@/api/admin/usage'

defineProps<{
  records: AccountTestUsageRecord[]
  loading: boolean
}>()

const { t } = useI18n()
const columns = [
  { key: 'created_at', label: t('admin.usage.accountTest.columns.time') },
  { key: 'platform', label: t('admin.usage.accountTest.columns.platform') },
  { key: 'account', label: t('admin.usage.accountTest.columns.account') },
  { key: 'model', label: t('admin.usage.accountTest.columns.model') },
  { key: 'endpoint', label: t('admin.usage.accountTest.columns.endpoint') },
  { key: 'tokens', label: t('admin.usage.accountTest.columns.tokens') },
  { key: 'test_mode', label: t('admin.usage.accountTest.columns.testMode') },
  { key: 'success', label: t('admin.usage.accountTest.columns.result') },
  { key: 'status_code', label: t('admin.usage.accountTest.columns.status') },
  { key: 'duration_ms', label: t('admin.usage.accountTest.columns.duration') },
  { key: 'error_message', label: t('admin.usage.accountTest.columns.error') },
] as const

const formatNumber = (value: number | null | undefined): string => (value ?? 0).toLocaleString()
const totalTokens = (record: AccountTestUsageRecord): number => record.tokens ?? (record.input_tokens || 0) + (record.output_tokens || 0) + (record.cache_creation_tokens || 0) + (record.cache_read_tokens || 0)
const formatDuration = (value: number | null | undefined): string => {
  const ms = value ?? 0
  return ms < 1000 ? Math.round(ms) + 'ms' : (ms / 1000).toFixed(2) + 's'
}
const formatDateTime = (value: string): string => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>

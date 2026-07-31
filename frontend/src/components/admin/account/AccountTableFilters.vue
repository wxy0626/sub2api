<template>
  <div class="flex flex-wrap items-center gap-3">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.accounts.searchAccounts')"
      class="w-full sm:w-64"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />
    <Select :model-value="filters.platform" class="w-40" :options="platformOptions" @update:model-value="updatePlatform" @change="$emit('change')" />
    <Select :model-value="filters.type" class="w-40" :options="typeOptions" @update:model-value="updateType" @change="$emit('change')" />
    <Select :model-value="filters.status" class="w-40" :options="statusOptions" @update:model-value="updateStatus" @change="$emit('change')" />
    <Select :model-value="filters.privacy_mode" class="w-40" :options="privacyOptions" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
    <Select :model-value="filters.group" class="w-40" :options="groupOptions" @update:model-value="updateGroup" @change="$emit('change')" />
    <Select :model-value="filters.proxy_id" class="w-40" :options="proxyOptions" @update:model-value="updateProxy" @change="$emit('change')" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import type { AdminGroup } from '@/types'

type AccountFilterValue = string | number | boolean | null | undefined

const props = defineProps<{
  searchQuery: string
  filters: Record<string, AccountFilterValue>
  groups?: AdminGroup[]
  proxyOptions: SelectOption[]
}>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change'])
const { t } = useI18n()

const updatePlatform = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, platform: value }) }
const updateType = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, type: value }) }
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value }) }
const updatePrivacyMode = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, privacy_mode: value }) }
const updateGroup = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, group: value }) }
// 工具栏代理下拉复用页面 params.proxy_id，保证与表头代理筛选同步。
const updateProxy = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, proxy_id: value }) }

const platformOptions = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') }, { value: 'anthropic', label: 'Anthropic' }, { value: 'openai', label: 'OpenAI' }, { value: 'gemini', label: 'Gemini' }, { value: 'antigravity', label: 'Antigravity' }, { value: 'grok', label: 'Grok' }, { value: 'deepseek', label: 'DeepSeek' }])
const typeOptions = computed(() => [{ value: '', label: t('admin.accounts.allTypes') }, { value: 'oauth', label: t('admin.accounts.oauthType') }, { value: 'setup-token', label: t('admin.accounts.setupToken') }, { value: 'apikey', label: t('admin.accounts.apiKey') }, { value: 'bedrock', label: 'AWS Bedrock' }])
const statusOptions = computed(() => [{ value: '', label: t('admin.accounts.allStatus') }, { value: 'active', label: t('admin.accounts.status.active') }, { value: 'inactive', label: t('admin.accounts.status.inactive') }, { value: 'error', label: t('admin.accounts.status.error') }, { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') }, { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') }, { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') }])
const privacyOptions = computed(() => [
  { value: '', label: t('admin.accounts.allPrivacyModes') },
  { value: '__unset__', label: t('admin.accounts.privacyUnset') },
  { value: 'training_off', label: 'Privacy' },
  { value: 'training_set_cf_blocked', label: 'CF' },
  { value: 'training_set_failed', label: 'Fail' }
])
const groupOptions = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...(props.groups || []).map(group => ({ value: String(group.id), label: group.name }))
])
</script>

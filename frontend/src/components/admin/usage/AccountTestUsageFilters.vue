<template>
  <div class="flex flex-wrap items-end gap-4 border-b border-gray-100 p-4 dark:border-dark-700/50 sm:p-6">
    <div class="w-full sm:w-auto sm:min-w-[180px]">
      <label class="input-label">{{ t('admin.usage.accountTest.platform') }}</label>
      <input v-model="local.platform" class="input" :placeholder="t('admin.usage.accountTest.platformPlaceholder')" @change="emitChange" @keyup.enter="emitChange" />
    </div>
    <div class="w-full sm:w-auto sm:min-w-[220px]">
      <label class="input-label">{{ t('admin.usage.accountTest.accountId') }}</label>
      <input v-model.number="local.account_id" type="number" min="1" class="input" :placeholder="t('admin.usage.accountTest.accountIdPlaceholder')" @change="emitChange" @keyup.enter="emitChange" />
    </div>
    <div class="w-full sm:w-auto sm:min-w-[220px]">
      <label class="input-label">{{ t('usage.model') }}</label>
      <Select v-model="local.model" :options="modelSelectOptions" searchable @change="emitChange" />
    </div>
    <div class="w-full sm:w-auto sm:min-w-[180px]">
      <label class="input-label">{{ t('admin.usage.accountTest.result') }}</label>
      <Select v-model="local.success" :options="successOptions" @change="emitChange" />
    </div>
    <div class="ml-auto flex w-full items-center justify-end gap-3 sm:w-auto">
      <button type="button" class="btn btn-secondary" @click="$emit('refresh')">{{ t('common.refresh') }}</button>
      <button type="button" class="btn btn-secondary" @click="reset">{{ t('common.reset') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import type { AccountTestUsageQueryParams } from '@/api/admin/usage'

const props = defineProps<{
  modelValue: AccountTestUsageQueryParams
  // modelOptions 是当前筛选范围内实际测试过的模型名称。
  modelOptions: string[]
}>()
const emit = defineEmits<{
  'update:modelValue': [value: AccountTestUsageQueryParams]
  change: []
  refresh: []
  reset: []
}>()
const { t } = useI18n()

const local = reactive<AccountTestUsageQueryParams>({ ...props.modelValue })
watch(() => props.modelValue, (value) => Object.assign(local, value), { deep: true })

const successOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.accountTest.allResults') },
  { value: true, label: t('admin.usage.accountTest.success') },
  { value: false, label: t('admin.usage.accountTest.failed') },
])
const modelSelectOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allModels') },
  ...props.modelOptions.map((model) => ({ value: model, label: model })),
])

const emitChange = () => {
  emit('update:modelValue', { ...local })
  emit('change')
}
const reset = () => {
  local.platform = undefined
  local.account_id = undefined
  local.model = undefined
  local.success = undefined
  emit('update:modelValue', { ...local })
  emit('reset')
}
</script>

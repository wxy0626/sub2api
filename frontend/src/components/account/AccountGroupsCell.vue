<template>
  <div class="relative max-w-56">
    <!-- 分组容器：固定最大宽度，最多显示2行 -->
    <button
      ref="triggerRef"
      type="button"
      data-testid="account-groups-trigger"
      class="flex max-h-14 w-full cursor-pointer flex-wrap gap-1 overflow-hidden text-left hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-60 dark:hover:bg-dark-700"
      :disabled="saving"
      @click.stop="openPopover"
    >
      <GroupBadge
        v-for="group in displayGroups"
        :key="group.id"
        :name="group.name"
        :platform="group.platform"
        :subscription-type="group.subscription_type"
        :rate-multiplier="group.rate_multiplier"
        :show-rate="false"
        class="max-w-24"
      />
      <!-- 更多数量徽章 -->
      <span
        v-if="hiddenCount > 0"
        ref="moreButtonRef"
        class="inline-flex cursor-pointer items-center gap-0.5 whitespace-nowrap rounded-md bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
      >
        <span>+{{ hiddenCount }}</span>
      </span>
      <span v-if="displayGroups.length === 0" class="text-sm text-gray-400 dark:text-dark-500">-</span>
    </button>

    <!-- Popover 显示完整列表 -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition duration-150 ease-out"
        enter-from-class="opacity-0 scale-95"
        enter-to-class="opacity-100 scale-100"
        leave-active-class="transition duration-100 ease-in"
        leave-from-class="opacity-100 scale-100"
        leave-to-class="opacity-0 scale-95"
      >
        <div
          v-if="showPopover"
          ref="popoverRef"
          class="fixed z-50 w-[min(42rem,calc(100vw-2rem))] rounded-lg border border-gray-200 bg-white p-3 shadow-lg dark:border-dark-600 dark:bg-dark-800"
          :style="popoverStyle"
          @click.stop
        >
          <div class="mb-2 flex items-center justify-between">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.groupCountTotal', { count: selectedGroupIDs.length }) }}
            </span>
            <button
              type="button"
              aria-label="Close group selector"
              class="rounded p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-300"
              @click="showPopover = false"
            >
              <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <GroupSelector
            v-model="selectedGroupIDs"
            :groups="availableGroups"
            :platform="platform"
            :mixed-scheduling="mixedScheduling"
            :searchable="false"
          />
          <div class="mt-3 flex justify-end gap-2">
            <button
              type="button"
              class="rounded-md px-2.5 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              @click="showPopover = false"
            >
              {{ t('common.cancel') }}
            </button>
            <button
              type="button"
              data-testid="account-groups-save"
              class="rounded-md bg-primary-500 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="saving || !hasGroupChanges"
              @click="saveGroups"
            >
              {{ t('common.save') }}
            </button>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 点击外部关闭 popover -->
    <div
      v-if="showPopover"
      class="fixed inset-0 z-40"
      @click="showPopover = false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import type { AdminGroup, Group, GroupPlatform } from '@/types'

interface Props {
  groups: Group[] | null | undefined
  groupIDs?: number[] | null
  availableGroups: AdminGroup[]
  platform: GroupPlatform
  mixedScheduling?: boolean
  saving?: boolean
  maxDisplay?: number
}

const props = withDefaults(defineProps<Props>(), {
  mixedScheduling: false,
  saving: false,
  maxDisplay: 4
})
const emit = defineEmits<{
  save: [groupIDs: number[]]
}>()

const { t } = useI18n()

const moreButtonRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLElement | null>(null)
const popoverRef = ref<HTMLElement | null>(null)
const showPopover = ref(false)
// 编辑中的分组 ID，取消编辑时不影响列表原始数据。
const selectedGroupIDs = ref<number[]>([])
// 账号当前绑定的分组 ID，优先使用已展开的分组对象，缺失时回退到轻量列表字段。
const assignedGroupIDs = computed(() => {
  if (props.groups && props.groups.length > 0) return props.groups.map((group) => group.id)
  return props.groupIDs ?? props.groups?.map((group) => group.id) ?? []
})

// 显示的分组（最多显示 maxDisplay 个）
const displayGroups = computed(() => {
  if (!props.groups) return []
  if (props.groups.length <= props.maxDisplay) {
    return props.groups
  }
  // 留一个位置给 +N 按钮
  return props.groups.slice(0, props.maxDisplay - 1)
})

// 隐藏的数量
const hiddenCount = computed(() => {
  if (!props.groups) return 0
  if (props.groups.length <= props.maxDisplay) return 0
  return props.groups.length - (props.maxDisplay - 1)
})

// 判断本次选择是否需要保存，避免无改动请求。
const hasGroupChanges = computed(() => {
  const currentGroupIDs = [...assignedGroupIDs.value].sort((a, b) => a - b)
  const nextGroupIDs = [...selectedGroupIDs.value].sort((a, b) => a - b)
  return currentGroupIDs.length !== nextGroupIDs.length || currentGroupIDs.some((id, index) => id !== nextGroupIDs[index])
})

// 打开编辑浮层时从当前行重新复制分组，防止遗留未保存的选择。
const openPopover = () => {
  if (props.saving) return
  selectedGroupIDs.value = [...assignedGroupIDs.value]
  showPopover.value = true
}

// 将多选后的分组一次性提交给父级保存。
const saveGroups = () => {
  if (props.saving || !hasGroupChanges.value) return
  emit('save', [...selectedGroupIDs.value])
  showPopover.value = false
}

// Popover 位置样式：按加宽后的浮层和当前视口计算左右边界。
const popoverStyle = computed(() => {
  const anchor = moreButtonRef.value || triggerRef.value
  if (!anchor) return {}
  const rect = anchor.getBoundingClientRect()
  const viewportHeight = window.innerHeight
  const viewportWidth = window.innerWidth
  const popoverMaxWidth = 672
  const viewportPadding = 8
  const popoverWidth = Math.min(popoverMaxWidth, Math.max(0, viewportWidth - viewportPadding * 2))
  const popoverHeight = 320

  let top = rect.bottom + 8
  let left = rect.left

  // 如果下方空间不足，显示在上方。
  if (top + popoverHeight > viewportHeight) {
    top = Math.max(viewportPadding, rect.top - popoverHeight)
  }

  // 如果右侧空间不足，向左偏移并保留视口边距。
  if (left + popoverWidth > viewportWidth - viewportPadding) {
    left = Math.max(viewportPadding, viewportWidth - popoverWidth - viewportPadding)
  }

  return {
    top: `${top}px`,
    left: `${left}px`
  }
})

// 关闭 popover 的键盘事件
const handleKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') {
    showPopover.value = false
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>

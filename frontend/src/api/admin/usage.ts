/**
 * Admin Usage API endpoints
 * Handles admin-level usage logs and statistics retrieval
 */

import { apiClient } from '../client'
import type { AdminUsageLog, UsageQueryParams, PaginatedResponse, UsageRequestType } from '@/types'
import type { EndpointStat } from '@/types'

// ==================== Types ====================

export interface AdminUsageStatsResponse {
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_tokens: number
  total_cache_creation_tokens: number
  total_cache_read_tokens: number
  total_tokens: number
  total_cost: number
  total_actual_cost: number
  total_account_cost: number
  average_duration_ms: number
  endpoints?: EndpointStat[]
  upstream_endpoints?: EndpointStat[]
  endpoint_paths?: EndpointStat[]
}

export interface SimpleUser {
  id: number
  email: string
  deleted: boolean
}

export interface SimpleApiKey {
  id: number
  name: string
  user_id: number
}

export interface UsageCleanupFilters {
  start_time: string
  end_time: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string | null
  request_type?: UsageRequestType | null
  stream?: boolean | null
  billing_type?: number | null
}

export interface UsageCleanupTask {
  id: number
  status: string
  filters: UsageCleanupFilters
  created_by: number
  deleted_rows: number
  error_message?: string | null
  canceled_by?: number | null
  canceled_at?: string | null
  started_at?: string | null
  finished_at?: string | null
  created_at: string
  updated_at: string
}

export interface CreateUsageCleanupTaskRequest {
  start_date: string
  end_date: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string | null
  request_type?: UsageRequestType | null
  stream?: boolean | null
  billing_type?: number | null
  timezone?: string
}

export interface AdminUsageQueryParams extends UsageQueryParams {
  user_id?: number
  exact_total?: boolean
  billing_mode?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
  // 错误请求 tab 专属筛选(仅传给错误列表接口;共用同一 filters 对象)
  error_phase?: string | null
  error_category?: string | null
  status_code?: number | null
  // 账号测试 Tab 的独立筛选字段，不会传入正式账单接口。
  platform?: string
  success?: boolean
}

// 管理员账号测试明细独立于正式 usage_logs，不包含任何计费字段。
export interface AccountTestUsageRecord {
  id: number
  account_id: number
  account_name?: string | null
  account?: { id: number; name: string } | null
  platform: string
  model: string
  test_mode: string
  endpoint: string
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  tokens?: number | null
  duration_ms: number
  success: boolean
  status_code: number
  error_message?: string | null
  created_at: string
}

// 全局账号测试统计只描述请求、Token 和耗时，不产生费用。
export interface AccountTestUsageStatsResponse {
  total_requests: number
  successful_requests: number
  failed_requests: number
  input_tokens: number
  output_tokens: number
  cache_tokens: number
  total_tokens: number
  avg_duration_ms: number
  by_platform: Array<{
    platform: string
    total_requests: number
    successful_requests: number
    failed_requests: number
    input_tokens: number
    output_tokens: number
    cache_tokens: number
    total_tokens: number
    avg_duration_ms: number
  }>
  by_model: Array<{
    model: string
    total_requests: number
  }>
}

export interface AccountTestUsageQueryParams {
  start_date?: string
  end_date?: string
  platform?: string
  account_id?: number
  model?: string
  success?: boolean
  page?: number
  page_size?: number
  timezone?: string
}

// ==================== API Functions ====================

/**
 * List all usage logs with optional filters (admin only)
 * @param params - Query parameters for filtering and pagination
 * @returns Paginated list of usage logs
 */
export async function list(
  params: AdminUsageQueryParams,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<AdminUsageLog>> {
  const { data } = await apiClient.get<PaginatedResponse<AdminUsageLog>>('/admin/usage', {
    params,
    signal: options?.signal
  })
  return data
}

/**
 * Get usage statistics with optional filters (admin only)
 * @param params - Query parameters for filtering
 * @returns Usage statistics
 */
export async function getStats(params: {
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string
  request_type?: UsageRequestType
  stream?: boolean
  period?: string
  start_date?: string
  end_date?: string
  timezone?: string
  nocache?: number
}): Promise<AdminUsageStatsResponse> {
  const { data } = await apiClient.get<AdminUsageStatsResponse>('/admin/usage/stats', {
    params
  })
  return data
}

/**
 * 查询管理员账号测试记录；该接口不读取正式用户计费记录。
 */
export async function listTestLogs(
  params: AccountTestUsageQueryParams,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<AccountTestUsageRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<AccountTestUsageRecord>>('/admin/usage/test-logs', {
    params,
    signal: options?.signal
  })
  return data
}

/**
 * 查询管理员账号测试汇总；统计不包含费用，也不会改变正式用量统计。
 */
export async function getTestStats(
  params: Omit<AccountTestUsageQueryParams, 'page' | 'page_size'>,
  options?: { signal?: AbortSignal }
): Promise<AccountTestUsageStatsResponse> {
  const { data } = await apiClient.get<AccountTestUsageStatsResponse>('/admin/usage/test-stats', {
    params,
    signal: options?.signal
  })
  return data
}

// 保留旧命名别名，兼容已有前端分支或外部调用方；别名仍指向新接口。
export const listAccountTests = listTestLogs
export const getAccountTestStats = getTestStats

/**
 * Search users by email keyword (admin only)
 * @param keyword - Email keyword to search
 * @returns List of matching users (max 30)
 */
export async function searchUsers(keyword: string): Promise<SimpleUser[]> {
  const { data } = await apiClient.get<SimpleUser[]>('/admin/usage/search-users', {
    params: { q: keyword }
  })
  return data
}

/**
 * Search API keys by user ID and/or keyword (admin only)
 * @param userId - Optional user ID to filter by
 * @param keyword - Optional keyword to search in key name
 * @returns List of matching API keys (max 30)
 */
export async function searchApiKeys(userId?: number, keyword?: string): Promise<SimpleApiKey[]> {
  const params: Record<string, unknown> = {}
  if (userId !== undefined) {
    params.user_id = userId
  }
  if (keyword) {
    params.q = keyword
  }
  const { data } = await apiClient.get<SimpleApiKey[]>('/admin/usage/search-api-keys', {
    params
  })
  return data
}

/**
 * List usage cleanup tasks (admin only)
 * @param params - Query parameters for pagination
 * @returns Paginated list of cleanup tasks
 */
export async function listCleanupTasks(
  params: { page?: number; page_size?: number },
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<UsageCleanupTask>> {
  const { data } = await apiClient.get<PaginatedResponse<UsageCleanupTask>>('/admin/usage/cleanup-tasks', {
    params,
    signal: options?.signal
  })
  return data
}

/**
 * Create a usage cleanup task (admin only)
 * @param payload - Cleanup task parameters
 * @returns Created cleanup task
 */
export async function createCleanupTask(payload: CreateUsageCleanupTaskRequest): Promise<UsageCleanupTask> {
  const { data } = await apiClient.post<UsageCleanupTask>('/admin/usage/cleanup-tasks', payload)
  return data
}

/**
 * Cancel a usage cleanup task (admin only)
 * @param taskId - Task ID to cancel
 */
export async function cancelCleanupTask(taskId: number): Promise<{ id: number; status: string }> {
  const { data } = await apiClient.post<{ id: number; status: string }>(
    `/admin/usage/cleanup-tasks/${taskId}/cancel`
  )
  return data
}

export const adminUsageAPI = {
  list,
  getStats,
  listTestLogs,
  getTestStats,
  listAccountTests,
  getAccountTestStats,
  searchUsers,
  searchApiKeys,
  listCleanupTasks,
  createCleanupTask,
  cancelCleanupTask
}

export default adminUsageAPI

/**
 * System API endpoints for admin operations
 */

import { apiClient } from '../client'

export interface ReleaseInfo {
  name: string
  body: string
  published_at: string
  html_url: string
}

export interface VersionInfo {
  current_version: string
  latest_version: string
  has_update: boolean
  release_info?: ReleaseInfo
  cached: boolean
  warning?: string
  build_type: string // "source" for manual builds, "release" for CI builds
}

/**
 * Get current version
 */
export async function getVersion(): Promise<{ version: string }> {
  const { data } = await apiClient.get<{ version: string }>('/admin/system/version')
  return data
}

/**
 * Check for updates
 * @param force - Force refresh from GitHub API
 */
export async function checkUpdates(force = false): Promise<VersionInfo> {
  const { data } = await apiClient.get<VersionInfo>('/admin/system/check-updates', {
    params: force ? { force: 'true' } : undefined
  })
  return data
}

export interface UpdateResult {
	message: string
	need_restart: boolean
	restart_scheduled?: boolean
}

export interface PersonalDeploymentVersion {
	tag: string
	commit: string
	digest: string
	reference: string
}

/**
 * 获取已由用户 Git tag、OCI digest 与 revision 验证的个人可部署版本。
 */
export async function getPersonalDeploymentVersions(): Promise<{ versions: PersonalDeploymentVersion[] }> {
	const { data } = await apiClient.get<{ versions: PersonalDeploymentVersion[] }>(
		'/admin/system/deployment-versions'
	)
	return data
}

/**
 * 在线更新和本地镜像回退均可能触发服务重建；使用后端允许的最长等待时间，避免全局 30 秒超时中断请求。
 */
const UPDATE_REQUEST_TIMEOUT_MS = 15 * 60 * 1000

/**
 * Perform system update
 * Downloads and applies the latest version
 */
export async function performUpdate(): Promise<UpdateResult> {
  const { data } = await apiClient.post<UpdateResult>('/admin/system/update', undefined, {
    timeout: UPDATE_REQUEST_TIMEOUT_MS
  })
  return data
}

/**
 * 部署后端自行确定的最新个人 Git tag 对应镜像。
 */
export async function deployLatestPersonalVersion(): Promise<UpdateResult> {
	const { data } = await apiClient.post<UpdateResult>(
		'/admin/system/deployment/update',
		undefined,
		{ timeout: UPDATE_REQUEST_TIMEOUT_MS }
	)
	return data
}

/**
 * 回退到后端白名单确认的个人 Git tag 和 OCI digest 镜像。
 */
export async function rollback(tag: string, digest: string): Promise<UpdateResult> {
	const { data } = await apiClient.post<UpdateResult>(
		'/admin/system/rollback',
		{ tag, digest },
		{ timeout: UPDATE_REQUEST_TIMEOUT_MS }
	)
  return data
}

/**
 * Restart the service
 */
export async function restartService(): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/admin/system/restart')
  return data
}

export const systemAPI = {
	getVersion,
	checkUpdates,
	performUpdate,
	getPersonalDeploymentVersions,
	deployLatestPersonalVersion,
  rollback,
  restartService
}

export default systemAPI

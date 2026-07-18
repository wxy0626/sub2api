/**
 * normalizeDisplayErrorMessage 将原始错误统一转换为可直接展示给用户的中文提示。
 * 原始上游错误仍保留在浏览器控制台和服务端日志中，避免把英文内部信息暴露到界面。
 */
export function normalizeDisplayErrorMessage(rawMessage: unknown, fallback = '操作失败，请稍后重试。'): string {
  const message = typeof rawMessage === 'string' ? rawMessage.trim() : ''
  if (!message) return fallback

  const normalizedMessage = message.toLowerCase()
  if (
    normalizedMessage.includes('deactivated_workspace') ||
    normalizedMessage.includes('workspace deactivated') ||
    normalizedMessage.includes('workspace has been deactivated')
  ) {
    return 'ChatGPT 工作区已停用（402）：该工作区已被停用。'
  }
  if (normalizedMessage.includes('backend-api/codex/responses') && normalizedMessage.includes('eof')) {
    return '请求失败：与 ChatGPT 上游服务的连接意外中断（EOF），请检查代理和网络后重试。'
  }
  if (normalizedMessage.includes('eof')) {
    return '请求失败：上游服务连接意外中断，请检查网络或代理后重试。'
  }
  if (
    normalizedMessage.includes('network error') ||
    normalizedMessage.includes('failed to fetch') ||
    normalizedMessage.includes('err_network') ||
    normalizedMessage.includes('econnrefused') ||
    normalizedMessage.includes('connection refused') ||
    normalizedMessage.includes('connection reset')
  ) {
    return '网络连接失败，请检查网络、代理和服务状态后重试。'
  }
  if (normalizedMessage.includes('timeout') || normalizedMessage.includes('timed out')) {
    return '请求超时，请稍后重试。'
  }
  if (
    normalizedMessage.includes('authentication failed') ||
    normalizedMessage.includes('unauthorized') ||
    normalizedMessage.includes('invalid credentials') ||
    /(^|\D)401(\D|$)/.test(normalizedMessage)
  ) {
    return '身份验证失败，请重新登录、重新授权或检查账号凭据。'
  }
  if (normalizedMessage.includes('forbidden') || /(^|\D)403(\D|$)/.test(normalizedMessage)) {
    return '没有执行此操作的权限。'
  }
  if (normalizedMessage.includes('not found') || /(^|\D)404(\D|$)/.test(normalizedMessage)) {
    return '请求的资源不存在或已被删除。'
  }
  if (normalizedMessage.includes('rate limit') || normalizedMessage.includes('too many requests') || /(^|\D)429(\D|$)/.test(normalizedMessage)) {
    return '请求过于频繁，请稍后重试。'
  }
  if (
    normalizedMessage.includes('internal server error') ||
    normalizedMessage.includes('bad gateway') ||
    normalizedMessage.includes('service unavailable') ||
    normalizedMessage.includes('gateway timeout') ||
    /(^|\D)5\d{2}(\D|$)/.test(normalizedMessage)
  ) {
    return '上游服务暂时异常，请稍后重试。'
  }
  if (
    normalizedMessage.includes('request failed') ||
    normalizedMessage.includes('api returned') ||
    normalizedMessage.includes('upstream') ||
    normalizedMessage.includes('http error')
  ) {
    return '请求失败，请检查服务端日志中的详细原因。'
  }
  if (normalizedMessage.includes('unknown error')) {
    return '发生未知错误，请稍后重试。'
  }

  // 中文或纯数字状态说明可直接呈现；其他英文内部错误统一隐藏细节。
  if (!/[A-Za-z]/.test(message)) return message
  return '操作失败，详细原因请查看服务端日志。'
}

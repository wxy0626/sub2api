/**
 * withDetailedErrorMessage 以中文说明错误原因，并保留后端返回的原始技术字段供排查。
 * 凭据保护由管理员后端在响应前处理，客户端前端不再自行删改错误详情。
 */
function withDetailedErrorMessage(chineseReason: string, rawMessage: string): string {
  const technicalDetails = rawMessage.trim()
  if (!technicalDetails || technicalDetails === chineseReason) return chineseReason
  return `${chineseReason}\n技术详情：${technicalDetails}`
}

/**
 * normalizeDisplayErrorMessage 将原始错误统一转换为可直接展示给用户的中文详细提示。
 * 常规说明使用中文；HTTP 状态、错误码和上游字段保留原始英文。
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
    return withDetailedErrorMessage('ChatGPT 工作区已停用（HTTP 402），请恢复该工作区后再测试。', message)
  }
  if (normalizedMessage.includes('backend-api/codex/responses') && normalizedMessage.includes('eof')) {
    return withDetailedErrorMessage('请求失败：与 ChatGPT 上游服务的连接意外中断（EOF），请检查代理、网络和上游服务后重试。', message)
  }
  if (normalizedMessage.includes('eof')) {
    return withDetailedErrorMessage('请求失败：上游服务连接意外中断（EOF），请检查网络或代理后重试。', message)
  }
  if (
    normalizedMessage.includes('network error') ||
    normalizedMessage.includes('failed to fetch') ||
    normalizedMessage.includes('err_network') ||
    normalizedMessage.includes('econnrefused') ||
    normalizedMessage.includes('connection refused') ||
    normalizedMessage.includes('connection reset')
  ) {
    return withDetailedErrorMessage('网络连接失败，请检查网络、代理和服务状态后重试。', message)
  }
  if (normalizedMessage.includes('timeout') || normalizedMessage.includes('timed out')) {
    return withDetailedErrorMessage('请求超时，请检查网络、代理或上游响应耗时后重试。', message)
  }
  // 上游额度不足是账号可维护状态，管理员需要直接看到而不是被 403 通用提示掩盖。
  if (
    normalizedMessage.includes('insufficient_balance') ||
    normalizedMessage.includes('insufficient balance') ||
    normalizedMessage.includes('insufficient_user_quota') ||
    normalizedMessage.includes('额度不足')
  ) {
    return withDetailedErrorMessage('上游账号额度不足，请充值或更换账号后重试。', message)
  }
  if (
    normalizedMessage.includes('authentication failed') ||
    normalizedMessage.includes('unauthorized') ||
    normalizedMessage.includes('invalid credentials') ||
    /(^|\D)401(\D|$)/.test(normalizedMessage)
  ) {
    return withDetailedErrorMessage('身份验证失败，请重新登录、重新授权或检查账号凭据。', message)
  }
  if (normalizedMessage.includes('forbidden') || /(^|\D)403(\D|$)/.test(normalizedMessage)) {
    return withDetailedErrorMessage('没有执行此操作的权限，请确认当前账号权限和上游账号订阅状态。', message)
  }
  if (normalizedMessage.includes('not found') || /(^|\D)404(\D|$)/.test(normalizedMessage)) {
    return withDetailedErrorMessage('请求的资源不存在或已被删除，请检查账号、模型或资源 ID。', message)
  }
  if (normalizedMessage.includes('rate limit') || normalizedMessage.includes('too many requests') || /(^|\D)429(\D|$)/.test(normalizedMessage)) {
    return withDetailedErrorMessage('请求过于频繁（HTTP 429），请等待限流恢复后重试。', message)
  }
  if (
    normalizedMessage.includes('internal server error') ||
    normalizedMessage.includes('bad gateway') ||
    normalizedMessage.includes('service unavailable') ||
    normalizedMessage.includes('gateway timeout') ||
    /(^|\D)5\d{2}(\D|$)/.test(normalizedMessage)
  ) {
    return withDetailedErrorMessage('上游服务暂时异常，请稍后重试。', message)
  }
  if (
    normalizedMessage.includes('request failed') ||
    normalizedMessage.includes('api returned') ||
    normalizedMessage.includes('upstream') ||
    normalizedMessage.includes('http error')
  ) {
    return withDetailedErrorMessage('上游请求失败，请根据下方技术详情检查账号、模型、代理或上游服务。', message)
  }
  if (normalizedMessage.includes('unknown error')) {
    return '发生未知错误，请稍后重试。'
  }

  // 已中文化的服务端错误可直接呈现；其他错误保留原始技术详情。
  if (!/[A-Za-z]/.test(message)) return message
  return withDetailedErrorMessage('操作失败，请根据下方技术详情定位原因。', message)
}

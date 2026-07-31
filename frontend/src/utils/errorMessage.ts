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
    normalizedMessage.includes('insufficient quota') ||
    normalizedMessage.includes('insufficient credit') ||
    normalizedMessage.includes('quota exceeded') ||
    normalizedMessage.includes('quota exhausted') ||
    normalizedMessage.includes('credit balance') ||
    normalizedMessage.includes('balance too low') ||
    normalizedMessage.includes('额度不足') ||
    normalizedMessage.includes('配额不足') ||
    normalizedMessage.includes('余额不足') ||
    normalizedMessage.includes('额度耗尽')
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

/**
 * extractErrorStatusCode 从错误详情中提取最后一个 HTTP 错误状态码。
 * 账号错误详情可能同时包含网关和上游状态码，末尾状态码通常是最终上游结果。
 */
function extractErrorStatusCode(message: string): number | null {
  const matches = message.match(/\b[45]\d{2}\b/g)
  if (!matches || matches.length === 0) return null
  return Number(matches[matches.length - 1])
}

/**
 * getErrorStatusSummary 生成状态列下方的短错误说明，详细原文仍由问号图标展示。
 * 摘要只保留状态码和可执行方向，避免把上游长错误直接挤进账号列表。
 */
export function getErrorStatusSummary(rawMessage: unknown): string {
  // 保留原始错误文本用于提取状态码，空值由下方通用原因兜底。
  const message = typeof rawMessage === 'string' ? rawMessage.trim() : ''
  // 复用详细错误的统一分类结果，确保短说明与问号详情的原因一致。
  const displayMessage = normalizeDisplayErrorMessage(message, '操作失败，请稍后重试。').toLowerCase()
  // 状态码从后端返回的技术详情中提取，未返回时只显示原因。
  const statusCode = extractErrorStatusCode(message)
  // 默认原因覆盖未识别的错误类型，保证每个错误状态都有可读摘要。
  let reason = '请求失败'

  if (displayMessage.includes('额度不足')) {
    reason = '额度不足'
  } else if (displayMessage.includes('上游服务暂时异常')) {
    reason = '上游异常'
  } else if (displayMessage.includes('工作区已停用')) {
    reason = '工作区停用'
  } else if (displayMessage.includes('身份验证失败')) {
    reason = '认证失败'
  } else if (displayMessage.includes('权限')) {
    reason = '权限不足'
  } else if (displayMessage.includes('资源不存在')) {
    reason = '资源不存在'
  } else if (displayMessage.includes('请求过于频繁')) {
    reason = '请求频繁'
  } else if (displayMessage.includes('网络连接失败') || displayMessage.includes('连接意外中断')) {
    reason = '网络异常'
  } else if (displayMessage.includes('请求超时')) {
    reason = '请求超时'
  } else if (displayMessage.includes('上游请求失败')) {
    reason = '上游请求失败'
  } else if (displayMessage.includes('未知错误')) {
    reason = '未知错误'
  }

  return statusCode == null ? reason : `${statusCode} ${reason}`
}

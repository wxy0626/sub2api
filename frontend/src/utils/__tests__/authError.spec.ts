import { describe, expect, it } from 'vitest'
import { buildAuthErrorMessage } from '@/utils/authError'

describe('buildAuthErrorMessage', () => {
  it('将 response detail 中的英文错误转换为中文概述', () => {
    const message = buildAuthErrorMessage(
      {
        response: {
          data: {
            detail: 'detailed message',
            message: 'plain message'
          }
        },
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('操作失败，详细原因请查看服务端日志。')
  })

  it('将 response message 中的英文错误转换为中文概述', () => {
    const message = buildAuthErrorMessage(
      {
        response: {
          data: {
            message: 'plain message'
          }
        },
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('操作失败，详细原因请查看服务端日志。')
  })

  it('将 Error.message 中的英文错误转换为中文概述', () => {
    const message = buildAuthErrorMessage(
      {
        message: 'error message'
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('操作失败，详细原因请查看服务端日志。')
  })

  it('uses fallback when no message can be extracted', () => {
    expect(buildAuthErrorMessage({}, { fallback: 'fallback' })).toBe('fallback')
  })
})

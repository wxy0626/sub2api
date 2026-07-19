import { describe, expect, it } from 'vitest'
import { buildAuthErrorMessage } from '@/utils/authError'

describe('buildAuthErrorMessage', () => {
  it('将 response detail 中的英文错误显示为中文说明与技术详情', () => {
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
    expect(message).toContain('操作失败，请根据下方技术详情定位原因。')
    expect(message).toContain('技术详情：detailed message')
  })

  it('将 response message 中的英文错误显示为中文说明与技术详情', () => {
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
    expect(message).toContain('操作失败，请根据下方技术详情定位原因。')
    expect(message).toContain('技术详情：plain message')
  })

  it('将 Error.message 中的英文错误显示为中文说明与技术详情', () => {
    const message = buildAuthErrorMessage(
      {
        message: 'error message'
      },
      { fallback: 'fallback' }
    )
    expect(message).toContain('操作失败，请根据下方技术详情定位原因。')
    expect(message).toContain('技术详情：error message')
  })

  it('uses fallback when no message can be extracted', () => {
    expect(buildAuthErrorMessage({}, { fallback: 'fallback' })).toBe('fallback')
  })
})

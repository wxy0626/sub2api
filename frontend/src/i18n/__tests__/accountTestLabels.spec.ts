import { describe, expect, it } from 'vitest'

import en from '../locales/en/admin/accounts'
import zh from '../locales/zh/admin/accounts'

describe('account test and Compact status locales', () => {
  it('uses the Chat Completions label without changing the Responses test label', () => {
    expect(zh.accounts.openai.testModeDefault).toBe('/chat/completions测试')
    expect(en.accounts.openai.testModeDefault).toBe('/chat/completions test')
    expect(zh.accounts.openai.testModeResponses).toBe('/responses 测试')
    expect(en.accounts.openai.testModeResponses).toBe('/responses test')
  })

  it('provides a distinct Responses unsupported Compact status', () => {
    expect(zh.accounts.openai.compactResponsesUnsupported).toBe('不支持 Responses')
    expect(en.accounts.openai.compactResponsesUnsupported).toBe('Responses unsupported')
    expect(zh.accounts.openai.compactAuto).toBe('Compact Auto')
    expect(en.accounts.openai.compactAuto).toBe('Compact Auto')
  })
})

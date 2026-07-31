import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)

describe('CreateAccountModal DeepSeek account', () => {
  it('offers DeepSeek as a separate platform and only exposes API Key', () => {
    expect(source).toContain('data-testid="deepseek-platform-option"')
    expect(source).toContain("@click=\"form.platform = 'deepseek'\"")
    expect(source).toContain('data-testid="deepseek-account-type-api-key"')
    expect(source).toContain("const baseUrlHint = computed")
    expect(source).toContain("if (form.platform === 'deepseek') return t('admin.accounts.deepseek.baseUrlHint')")
    expect(source).not.toContain("form.platform === 'deepseek' && isOAuthFlow")
  })

  it('uses the official DeepSeek URL and keeps model mapping empty by default', () => {
    expect(source).toContain("? 'https://api.deepseek.com'")
    expect(source).toContain("form.platform === 'deepseek'")
    expect(source).toContain('base_url: apiKeyBaseUrl.value.trim() || defaultBaseUrl')
    expect(source).toContain('api_key: apiKeyValue.value.trim()')
    expect(source).toContain("allowedModels.value = form.platform === 'deepseek' ? [] : [...getModelsByPlatform(form.platform)]")
    expect(source).toContain("if (newMode === 'whitelist' && form.platform !== 'deepseek')")
    expect(source).toContain('credentials.model_mapping = modelMapping')
  })
})

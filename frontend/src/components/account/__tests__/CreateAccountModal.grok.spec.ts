import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)

describe('CreateAccountModal Grok account types', () => {
  it('offers API-key setup alongside OAuth with the official xAI default', () => {
    expect(source).toContain('data-testid="grok-account-type-api-key"')
    expect(source).toContain("@click=\"accountCategory = 'apikey'\"")
    expect(source).toContain("newPlatform === 'grok'")
    expect(source).toContain("? 'https://api.x.ai/v1'")
    expect(source).toContain("form.platform === 'grok'")
    expect(source).toContain("? 'xai-...'")
  })

  it('persists OpenAI upstream relay mode separately from passthrough and TTFT', () => {
    expect(source).toContain('data-testid="create-openai-upstream-relay-toggle"')
    expect(source).toContain('extra.openai_upstream_relay = true')
    expect(source).toContain("accountCategory.value === 'apikey'")
    expect(source).not.toContain('extra.openai_first_response_enabled')
  })
})

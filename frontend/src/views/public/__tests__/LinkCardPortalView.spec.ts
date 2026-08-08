import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LinkCardPortalView from '../LinkCardPortalView.vue'

const { activate, getMe, listUsage, showSuccess, showError, writeText } = vi.hoisted(() => ({
  activate: vi.fn(),
  getMe: vi.fn(),
  listUsage: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  writeText: vi.fn(),
}))

vi.mock('@/api/linkCards', () => ({
  publicLinkCardsAPI: { activate, getMe, listUsage },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const card = {
  masked_key: 'sk-card-***0008',
  status: 'active',
  group_name: 'Link Group',
  issued_quota: 1250,
  used_quota: 20,
  remaining_quota: 1230,
  request_count: 3,
  activated_at: '2026-08-07T00:05:00Z',
  created_at: '2026-08-06T00:05:00Z',
}
const fullKey = 'sk-card-live-00000008a1b2c3d4e5f60718'

describe('LinkCardPortalView', () => {
  beforeEach(() => {
    sessionStorage.clear()
    activate.mockReset()
    getMe.mockReset()
    listUsage.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    writeText.mockReset().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    activate.mockResolvedValue({ session_token: 'session-token', expires_at: '2026-08-08T00:00:00Z' })
    getMe.mockResolvedValue({ card, key: fullKey, api_base_url: 'https://api.52token.org/' })
    listUsage.mockResolvedValue({
      items: [{
        request_id: 'req-1', model: 'MODEL',
        request_type: 'stream', stream: true, billing_mode: 'token', input_tokens: 100, output_tokens: 20,
        cache_creation_tokens: 30, cache_creation_5m_tokens: 10, cache_creation_1h_tokens: 20,
        cache_read_tokens: 40, image_input_tokens: 5, image_output_tokens: 2, total_tokens: 217,
        input_cost: 0.01, output_cost: 0.02, cache_creation_cost: 0.003, cache_read_cost: 0.004,
        image_input_cost: 0.005, image_output_cost: 0.006, total_cost: 0.048, actual_cost: 0.048,
        rate_multiplier: 1, service_tier: 'priority', duration_ms: 700, first_token_ms: 100,
        created_at: '2026-08-07T00:05:00Z',
      }],
      total: 1, page: 1, page_size: 10, pages: 1,
    })
  })

  it('shows only the activation workflow until a complete Key succeeds', async () => {
    const wrapper = mount(LinkCardPortalView, { global: { stubs: { Icon: true } } })

    expect(wrapper.find('section.tech-panel').exists()).toBe(false)
    await wrapper.get('#quota-card-key').setValue(fullKey)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(activate).toHaveBeenCalledWith({ key: fullKey })
    expect(getMe).toHaveBeenCalledWith('session-token')
    expect(listUsage).toHaveBeenCalledWith('session-token', { page: 1, page_size: 10 })
    expect(sessionStorage.getItem('link_card_portal_session')).toBe('session-token')
    expect(wrapper.text()).toContain('linkCards.integration')
    expect(wrapper.text()).toContain('https://api.52token.org/v1/responses')
    expect(wrapper.get('[data-testid="link-card-api-endpoint"]').text()).toBe('https://api.52token.org/v1')
    expect(wrapper.find('[data-testid="link-card-key-panel"]').exists()).toBe(false)
    expect(wrapper.text()).toContain(`${card.masked_key} · ${card.group_name}`)
    expect(wrapper.get('[data-testid="copy-link-card-key"]').attributes('aria-label')).toBe('linkCards.copyFullKey')
    expect(wrapper.text()).not.toContain(fullKey)
    expect(wrapper.get('pre').text()).not.toContain('\n+')
    expect(wrapper.text()).toContain('usage.endpoint')
    expect(wrapper.text()).toContain('admin.usage.billingMode')
    expect(wrapper.findAll('.group.relative').length).toBeGreaterThanOrEqual(2)
    await wrapper.findAll('.group.relative')[0].trigger('mouseenter')
    await flushPromises()
    expect(document.body.textContent).toContain('usage.tokenDetails')
    expect(document.body.textContent).toContain('admin.usage.cacheCreation5mTokens')
    expect(document.body.textContent).toContain('admin.usage.cacheCreation1hTokens')
    expect(document.body.textContent).toContain('usage.imageInputTokens')
    await wrapper.findAll('.group.relative')[0].trigger('mouseleave')
    await wrapper.findAll('.group.relative')[1].trigger('mouseenter')
    await flushPromises()
    expect(document.body.textContent).toContain('usage.costDetails')
    expect(document.body.textContent).toContain('usage.imageInputCost')
    expect(document.body.textContent).toContain('usage.original')
    expect(document.body.textContent).toContain('usage.userBilled')
    expect(document.body.textContent).toContain('usage.rate')
    expect(document.body.textContent).toContain('usage.serviceTier')

    await wrapper.get('[data-testid="copy-link-card-key"]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith(fullKey)
    expect(showSuccess).toHaveBeenLastCalledWith('linkCards.keyCopied')

    await wrapper.get('[data-testid="copy-link-card-api-endpoint"]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('https://api.52token.org/v1')
    expect(showSuccess).toHaveBeenLastCalledWith('linkCards.endpointCopied')
  })

  it('shows CCSwitch values and copyable configuration files for every protocol', async () => {
    sessionStorage.setItem('link_card_portal_session', 'saved-session')
    const wrapper = mount(LinkCardPortalView, { global: { stubs: { Icon: true } } })
    await flushPromises()

    let configText = wrapper.findAll('.config-file pre code').map((node) => node.text()).join('\n')
    expect(configText).toContain('model_provider = "custom"')
    expect(configText).toContain('base_url = "https://api.52token.org/v1"')
    expect(configText).toContain('wire_api = "responses"')
    expect(configText).toContain('"OPENAI_API_KEY": "CARD_KEY"')
    expect(wrapper.get('[data-testid="link-card-ccswitch-config"]').text()).toContain('Codex / Responses')
    expect(wrapper.get('[data-testid="link-card-ccswitch-config"]').text()).toContain('https://api.52token.org/v1')

    const claudeTab = wrapper.findAll('button').find((button) => button.text() === 'linkCards.claude')
    expect(claudeTab).toBeDefined()
    await claudeTab!.trigger('click')

    configText = wrapper.findAll('.config-file pre code').map((node) => node.text()).join('\n')
    expect(wrapper.text()).toContain('~/.claude/settings.json')
    expect(configText).toContain('"ANTHROPIC_BASE_URL": "https://api.52token.org/v1"')
    expect(configText).toContain('"ANTHROPIC_AUTH_TOKEN": "CARD_KEY"')
    expect(configText).toContain('"ANTHROPIC_MODEL": "MODEL"')
    expect(wrapper.get('[data-testid="link-card-ccswitch-config"]').text()).toContain('Claude / Messages')

    await wrapper.get('[data-testid="copy-link-card-config-0"]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith(expect.stringContaining('ANTHROPIC_AUTH_TOKEN'))
    expect(showSuccess).toHaveBeenLastCalledWith('linkCards.configurationCopied')

    const openAITab = wrapper.findAll('button').find((button) => button.text() === 'linkCards.openaiCompatible')
    expect(openAITab).toBeDefined()
    await openAITab!.trigger('click')

    configText = wrapper.findAll('.config-file pre code').map((node) => node.text()).join('\n')
    expect(configText).toContain('OPENAI_BASE_URL=https://api.52token.org/v1')
    expect(configText).toContain('OPENAI_API_KEY=CARD_KEY')
    expect(wrapper.get('[data-testid="link-card-ccswitch-config"]').text()).toContain('OpenAI compatible')
  })

  it('does not duplicate v1 when the API Base already includes it', async () => {
    sessionStorage.setItem('link_card_portal_session', 'saved-session')
    getMe.mockResolvedValueOnce({ card, key: fullKey, api_base_url: 'https://api.52token.org/v1/' })
    const wrapper = mount(LinkCardPortalView, { global: { stubs: { Icon: true } } })
    await flushPromises()

    expect(wrapper.get('[data-testid="link-card-api-endpoint"]').text()).toBe('https://api.52token.org/v1')
    expect(wrapper.text()).not.toContain('/v1/v1')
  })

  it('refreshes the profile metrics and usage rows together', async () => {
    sessionStorage.setItem('link_card_portal_session', 'saved-session')
    getMe
      .mockResolvedValueOnce({ card, key: fullKey, api_base_url: 'https://api.52token.org/v1' })
      .mockResolvedValueOnce({
        card: { ...card, remaining_quota: 1190, used_quota: 60, request_count: 4 },
        key: fullKey,
        api_base_url: 'https://api.52token.org/v1',
      })
    const wrapper = mount(LinkCardPortalView, { global: { stubs: { Icon: true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('$1230')
    await wrapper.get('[data-testid="refresh-link-card-details"]').trigger('click')
    await flushPromises()

    expect(getMe).toHaveBeenCalledTimes(2)
    expect(listUsage).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('$1190')
    expect(wrapper.text()).toContain('60')
    expect(wrapper.get('[data-testid="refresh-link-card-details"]').attributes('disabled')).toBeUndefined()
  })

  it('rejects partial Keys without contacting the server', async () => {
    const wrapper = mount(LinkCardPortalView, { global: { stubs: { Icon: true } } })
    await wrapper.get('#quota-card-key').setValue('sk-short')
    await wrapper.get('form').trigger('submit')

    expect(activate).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('linkCards.invalidKey')
  })

  it('keeps the activation page visible when the backend locks invalid attempts', async () => {
    activate.mockRejectedValueOnce({
      status: 429,
      reason: 'LINK_CARD_ACTIVATION_LOCKED',
      metadata: { retry_after_seconds: '300' },
    })
    const wrapper = mount(LinkCardPortalView, { global: { stubs: { Icon: true } } })
    await wrapper.get('#quota-card-key').setValue(fullKey)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('linkCards.activationLocked')
    expect(wrapper.find('#quota-card-key').exists()).toBe(true)
    expect(getMe).not.toHaveBeenCalled()
  })

  it('restores a short-lived details session and copies the exact Key', async () => {
    sessionStorage.setItem('link_card_portal_session', 'saved-session')
    const wrapper = mount(LinkCardPortalView, { global: { stubs: { Icon: true } } })
    await flushPromises()

    expect(getMe).toHaveBeenCalledWith('saved-session')
    await wrapper.get('[data-testid="copy-link-card-key"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith(fullKey)
    expect(writeText).not.toHaveBeenCalledWith(card.masked_key)
  })

  it('reports clipboard failures without changing the details session', async () => {
    sessionStorage.setItem('link_card_portal_session', 'saved-session')
    writeText.mockRejectedValueOnce(new Error('clipboard denied'))
    const wrapper = mount(LinkCardPortalView, { global: { stubs: { Icon: true } } })
    await flushPromises()

    await wrapper.get('[data-testid="copy-link-card-key"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('common.copyFailed')
    expect(sessionStorage.getItem('link_card_portal_session')).toBe('saved-session')
  })
})

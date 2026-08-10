import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import enLanding from '@/i18n/locales/en/landing'
import zhLanding from '@/i18n/locales/zh/landing'
import HomeView from '../HomeView.vue'

const stores = vi.hoisted(() => ({
  app: {
    cachedPublicSettings: null as Record<string, unknown> | null,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
  auth: {
    isAuthenticated: false,
    isAdmin: false,
    checkAuth: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => stores.app,
  useAuthStore: () => stores.auth,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const RouterLinkStub = {
  props: ['to'],
  template: '<a :data-to="typeof to === \'string\' ? to : JSON.stringify(to)"><slot /></a>',
}

function publicSettings(overrides: Record<string, unknown> = {}) {
  return {
    site_name: 'Example Service',
    site_logo: '/brand.svg',
    site_subtitle: 'A clear service subtitle',
    doc_url: 'https://docs.example.com/start',
    home_content: '',
    registration_enabled: true,
    ...overrides,
  }
}

function mountHome() {
  return mount(HomeView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-locale-switcher />' },
        Icon: { props: ['name'], template: '<i :data-icon="name" />' },
      },
    },
  })
}

describe('HomeView', () => {
  beforeEach(() => {
    document.documentElement.classList.remove('dark')
    localStorage.clear()
    stores.app.cachedPublicSettings = publicSettings()
    stores.app.siteName = 'Sub2API'
    stores.app.siteLogo = ''
    stores.app.docUrl = ''
    stores.app.publicSettingsLoaded = true
    stores.app.fetchPublicSettings.mockReset().mockResolvedValue(null)
    stores.auth.isAuthenticated = false
    stores.auth.isAdmin = false
    stores.auth.checkAuth.mockReset()
  })

  it('renders the complete default public-home structure', () => {
    const wrapper = mountHome()

    expect(wrapper.get('[data-testid="default-home"]')).toBeTruthy()
    expect(wrapper.get('[data-testid="home-hero"]')).toBeTruthy()
    expect(wrapper.find('.home-scene-frame').exists()).toBe(false)
    expect(wrapper.find('.home-data-visual-svg').exists()).toBe(true)
    expect(wrapper.get('[data-testid="home-overview"]')).toBeTruthy()
    expect(wrapper.get('[data-testid="home-reliability"]')).toBeTruthy()
    expect(wrapper.get('[data-testid="home-guide"]')).toBeTruthy()
    expect(wrapper.get('[data-testid="home-faq"]')).toBeTruthy()

    const desktopNav = wrapper.get('[data-testid="home-desktop-nav"]')
    for (const href of ['#overview', '#reliability', '#guide', '#faq']) {
      expect(desktopNav.find(`[href="${href}"]`).exists()).toBe(true)
    }
    expect(desktopNav.find('[href="https://docs.example.com/start"]').exists()).toBe(true)
  })

  it('uses public settings for brand, subtitle, logo, and documentation links', () => {
    const wrapper = mountHome()

    expect(wrapper.get('h1').text()).toBe('Example Service')
    expect(wrapper.text()).toContain('A clear service subtitle')
    expect(wrapper.findAll('img').some((image) => image.attributes('src') === '/brand.svg')).toBe(true)
    expect(
      wrapper.findAll('a').some((link) => link.attributes('href') === 'https://docs.example.com/start'),
    ).toBe(true)
  })

  it('uses registration as the primary guest entry only when registration is enabled', () => {
    let wrapper = mountHome()

    expect(wrapper.get('[data-testid="home-register-link"]').attributes('data-to')).toBe('/register')
    expect(wrapper.get('[data-testid="home-primary-entry"]').attributes('data-to')).toBe('/register')

    wrapper.unmount()
    stores.app.cachedPublicSettings = publicSettings({ registration_enabled: false })
    wrapper = mountHome()

    expect(wrapper.find('[data-testid="home-register-link"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="home-primary-entry"]').attributes('data-to')).toBe('/login')
  })

  it('returns authenticated administrators to their dashboard', () => {
    stores.auth.isAuthenticated = true
    stores.auth.isAdmin = true
    const wrapper = mountHome()

    expect(wrapper.find('[data-testid="home-login-link"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="home-register-link"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="home-primary-entry"]').attributes('data-to')).toBe('/admin/dashboard')
  })

  it('exposes all public anchors through the mobile menu', async () => {
    const wrapper = mountHome()

    expect(wrapper.find('[data-testid="home-mobile-menu"]').exists()).toBe(false)
    await wrapper.get('[data-testid="home-mobile-menu-button"]').trigger('click')

    const menu = wrapper.get('[data-testid="home-mobile-menu"]')
    for (const href of ['#overview', '#reliability', '#guide', '#faq']) {
      expect(menu.find(`[href="${href}"]`).exists()).toBe(true)
    }
    expect(wrapper.get('[data-testid="home-mobile-menu-button"]').attributes('aria-expanded')).toBe('true')
  })

  it('keeps sanitized custom HTML as a full replacement for the default home', () => {
    stores.app.cachedPublicSettings = publicSettings({
      home_content: '<h1>Custom welcome</h1><script>window.bad = true</script>',
    })
    const wrapper = mountHome()

    expect(wrapper.find('[data-testid="default-home"]').exists()).toBe(false)
    expect(wrapper.get('.safe-home-content').text()).toContain('Custom welcome')
    expect(wrapper.find('script').exists()).toBe(false)
  })

  it('keeps the restricted HTTPS iframe mode as a full replacement', () => {
    stores.app.cachedPublicSettings = publicSettings({
      home_content: 'https://landing.example.com/home',
    })
    const wrapper = mountHome()

    expect(wrapper.find('[data-testid="default-home"]').exists()).toBe(false)
    const frame = wrapper.get('iframe')
    expect(frame.attributes('src')).toBe('https://landing.example.com/home')
    expect(frame.attributes('sandbox')).toBe('')
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
  })

  it('keeps public-home copy free of provider-specific promotional terms', () => {
    const localizedHomeCopy = JSON.stringify([zhLanding.home, enLanding.home]).toLowerCase()
    const disallowedTerms = ['openai', 'claude', 'fastaitoken', '中转']

    for (const term of disallowedTerms) {
      expect(localizedHomeCopy).not.toContain(term)
    }
  })
})

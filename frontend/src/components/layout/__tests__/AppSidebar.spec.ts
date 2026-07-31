import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineComponent, h } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const storeState = vi.hoisted(() => ({
  app: {
    sidebarCollapsed: false,
    mobileOpen: false,
    sidebarScrollTop: 0,
    backendModeEnabled: false,
    publicSettingsLoaded: true,
    cachedPublicSettings: {
      points_system_enabled: false,
      custom_menu_items: [],
    } as Record<string, unknown>,
    siteName: 'Sub2API',
    siteLogo: '',
    siteVersion: 'test',
    toggleSidebar: vi.fn(),
    setMobileOpen: vi.fn(),
  },
  auth: {
    isAdmin: false,
    isSimpleMode: false,
    user: null as null | { id: number; points_system_access?: boolean },
    refreshUser: vi.fn(),
  },
  onboarding: {
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  },
  adminSettings: {
    opsMonitoringEnabled: false,
    paymentEnabled: false,
    customMenuItems: [],
    fetch: vi.fn(),
  },
}))

const routerState = vi.hoisted(() => ({
  route: { path: '/dashboard' },
  push: vi.fn(),
}))

const batchImageState = vi.hoisted(() => ({
  canUseBatchImage: { value: false },
  refreshBatchImageAccess: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => routerState.route,
  useRouter: () => ({ push: routerState.push }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => storeState.app,
  useAuthStore: () => storeState.auth,
  useOnboardingStore: () => storeState.onboarding,
  useAdminSettingsStore: () => storeState.adminSettings,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => storeState.app,
}))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => batchImageState,
}))

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

let AppSidebar: typeof import('../AppSidebar.vue')['default']

const RouterLinkStub = defineComponent({
  name: 'RouterLink',
  props: {
    to: {
      type: String,
      required: true,
    },
  },
  setup(props, { slots }) {
    return () => h('a', { 'data-to': props.to }, slots.default?.())
  },
})

function mountSidebar(): VueWrapper {
  return mount(AppSidebar, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        VersionBadge: true,
      },
    },
  })
}

function navigationPaths(wrapper: VueWrapper): string[] {
  return wrapper.findAll('[data-to]').map((link) => link.attributes('data-to'))
}

beforeAll(async () => {
  if (!window.matchMedia) {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn(() => ({ matches: false })),
    })
  }
  AppSidebar = (await import('../AppSidebar.vue')).default
})

beforeEach(() => {
  storeState.auth.isAdmin = false
  storeState.auth.isSimpleMode = false
  storeState.auth.user = null
  storeState.auth.refreshUser.mockReset()
  storeState.app.cachedPublicSettings = {
    points_system_enabled: false,
    custom_menu_items: [],
  }
  storeState.adminSettings.fetch.mockClear()
  batchImageState.refreshBatchImageAccess.mockClear()
  routerState.push.mockClear()
  localStorage.clear()
  document.documentElement.classList.remove('dark')
})

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar points navigation', () => {
  it.each([
    ['normal', false],
    ['simple', true],
  ])('keeps the admin settings entry visible in %s mode while points are disabled', (_mode, simpleMode) => {
    storeState.auth.isAdmin = true
    storeState.auth.isSimpleMode = simpleMode

    const wrapper = mountSidebar()

    expect(navigationPaths(wrapper)).toContain('/admin/settings/points')
    wrapper.unmount()
  })

  it('hides the regular user points entry while points are disabled', () => {
    const wrapper = mountSidebar()

    expect(navigationPaths(wrapper)).not.toContain('/points')
    expect(navigationPaths(wrapper)).not.toContain('/admin/settings/points')
    wrapper.unmount()
  })

  it('shows the points entry only to a user with preview access while globally disabled', () => {
    storeState.auth.user = { id: 1, points_system_access: true }

    const wrapper = mountSidebar()

    expect(navigationPaths(wrapper)).toContain('/points')
    expect(navigationPaths(wrapper)).not.toContain('/admin/settings/points')
    wrapper.unmount()
  })

  it('refreshes a legacy session once so the server can resolve preview access', () => {
    storeState.auth.user = { id: 1 }
    storeState.auth.refreshUser.mockResolvedValue({ id: 1, points_system_access: true })

    const wrapper = mountSidebar()

    expect(storeState.auth.refreshUser).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('does not infer preview access from user ID alone', () => {
    storeState.auth.user = { id: 1, points_system_access: false }

    const wrapper = mountSidebar()

    expect(navigationPaths(wrapper)).not.toContain('/points')
    wrapper.unmount()
  })
})

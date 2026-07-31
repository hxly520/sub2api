import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
  routes: [] as Array<Record<string, any>>,
}))

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: true,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
  user: null as null | { id: number; points_system_access?: boolean },
  refreshUser: vi.fn(),
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: false,
  cachedPublicSettings: null as null | {
    payment_enabled?: boolean
    risk_control_enabled?: boolean
    points_system_enabled?: boolean
    custom_menu_items?: []
  },
  fetchPublicSettings: vi.fn(),
}))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn((options: { routes: Array<Record<string, any>> }) => {
    routerHarness.routes = options.routes
    return {
      beforeEach: vi.fn((guard: NavigationGuard) => {
        routerHarness.guard = guard
      }),
      afterEach: vi.fn(),
      onError: vi.fn(),
    }
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn(),
  }),
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function runGuard(meta: Record<string, unknown>, path: string) {
  if (!routerHarness.guard) {
    throw new Error('router guard was not registered')
  }

  const next = vi.fn()
  const navigation = routerHarness.guard(
    {
      path,
      fullPath: path,
      name: 'FeatureRoute',
      params: {},
      meta: { requiresAuth: true, ...meta },
    },
    {},
    next
  )
  return { navigation, next }
}

describe('feature route guard', () => {
  beforeAll(async () => {
    await import('@/router')
  })

  beforeEach(() => {
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    authStore.user = null
    authStore.refreshUser.mockReset()
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    appStore.fetchPublicSettings.mockReset()
  })

  it('waits for the first public-settings request before deciding payment access', async () => {
    const deferred = createDeferred<{ payment_enabled: boolean }>()
    appStore.fetchPublicSettings.mockImplementation(async () => {
      const settings = await deferred.promise
      appStore.cachedPublicSettings = settings
      appStore.publicSettingsLoaded = true
      return settings
    })

    const { navigation, next } = runGuard({ requiresPayment: true }, '/purchase')

    await vi.waitFor(() => expect(appStore.fetchPublicSettings).toHaveBeenCalledTimes(1))
    expect(next).not.toHaveBeenCalled()

    deferred.resolve({ payment_enabled: true })
    await navigation
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPayment: true }, '/purchase'],
    ['risk control', { requiresRiskControl: true }, '/admin/risk-control'],
    ['points system', { requiresPointsSystem: true }, '/points'],
  ])('does not treat a failed %s settings load as explicitly disabled', async (_name, meta, path) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next } = runGuard(meta, path)
    await navigation

    expect(appStore.publicSettingsLoaded).toBe(false)
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPayment: true }, { payment_enabled: false }, '/dashboard'],
    [
      'risk control',
      { requiresRiskControl: true },
      { risk_control_enabled: false },
      '/admin/settings',
    ],
    [
      'points system',
      { requiresPointsSystem: true },
      { points_system_enabled: false },
      '/dashboard',
    ],
  ])('redirects when loaded settings explicitly disable %s', async (_name, meta, settings, target) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.cachedPublicSettings = settings
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard(meta, '/feature')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith(target)
  })

  it('allows the admin points settings route while the user feature is disabled', async () => {
    const route = routerHarness.routes.find((item) => item.path === '/admin/settings/points')
    expect(route?.meta).toMatchObject({ requiresAuth: true, requiresAdmin: true })
    expect(route?.meta?.requiresPointsSystem).toBeUndefined()

    authStore.isAdmin = true
    appStore.cachedPublicSettings = { points_system_enabled: false }
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard(route?.meta ?? {}, route?.path ?? '/admin/settings/points')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it('allows a user with preview access to open points while globally disabled', async () => {
    authStore.user = { id: 1, points_system_access: true }
    appStore.cachedPublicSettings = { points_system_enabled: false }
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard({ requiresPointsSystem: true }, '/points')
    await navigation

    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it('refreshes a legacy session before deciding preview access', async () => {
    authStore.user = { id: 1 }
    authStore.refreshUser.mockImplementation(async () => {
      authStore.user = { id: 1, points_system_access: true }
      return authStore.user
    })
    appStore.cachedPublicSettings = { points_system_enabled: false }
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard({ requiresPointsSystem: true }, '/points')
    await navigation

    expect(authStore.refreshUser).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it('does not infer points preview access from user ID alone', async () => {
    authStore.user = { id: 1, points_system_access: false }
    appStore.cachedPublicSettings = { points_system_enabled: false }
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard({ requiresPointsSystem: true }, '/points')
    await navigation

    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith('/dashboard')
  })

  it('honors an explicit per-user denial when public settings fail to load', async () => {
    authStore.user = { id: 2, points_system_access: false }
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next } = runGuard({ requiresPointsSystem: true }, '/points')
    await navigation

    expect(appStore.publicSettingsLoaded).toBe(false)
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith('/dashboard')
  })

  it('lets an explicit per-user denial override a stale global enabled value', async () => {
    authStore.user = { id: 2, points_system_access: false }
    appStore.cachedPublicSettings = { points_system_enabled: true }
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard({ requiresPointsSystem: true }, '/points')
    await navigation

    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith('/dashboard')
  })
})

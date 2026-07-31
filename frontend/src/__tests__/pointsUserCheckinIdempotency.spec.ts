import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const userScript = readFileSync(
  resolve(
    dirname(fileURLToPath(import.meta.url)),
    '../../../points-system/internal/httpapi/web/assets/user.js',
  ),
  'utf8',
)

const pendingStorageKey = 'points.pending-checkin.v1'
const firstIdempotencyKey = '11111111-1111-4111-8111-111111111111'
const secondIdempotencyKey = '22222222-2222-4222-8222-222222222222'

type APIOptions = {
  headers?: HeadersInit
  method?: string
}

type PointsUI = {
  api: (path: string, options?: APIOptions) => Promise<unknown>
  byId: (id: string) => HTMLElement
  date: (value: unknown) => string
  dateTime: (value: unknown) => string
  idempotencyKey: () => string
  kindText: (value: unknown) => string
  logout: () => Promise<void>
  money: (value: unknown) => string
  notifyReady: () => void
  notice: (message: string, isError?: boolean) => void
  number: (value: unknown) => number
  points: (value: unknown) => string
  renderRows: (target: string) => void
  setButtonBusy: (button: HTMLButtonElement, busy: boolean, busyText?: string) => void
  setButtonLabel: (button: HTMLButtonElement, text: string) => void
  setSession: (data: unknown) => void
  shortDate: (value: unknown) => string
  statusChip: (value: unknown) => HTMLElement
  statusText: (value: unknown) => string
}

function installDashboardDOM(): void {
  document.body.innerHTML = `
    <main class="dashboard-page" aria-busy="true">
      <div id="notice" class="hidden"></div>
      <section id="dashboard-error" class="hidden">
        <span id="dashboard-error-message"></span>
        <button id="retry-dashboard" type="button"><span data-button-label>重新加载</span></button>
      </section>
      <button id="logout" type="button"><span data-button-label>退出</span></button>
      <button id="refresh-dashboard" type="button"><span data-button-label>刷新数据</span></button>
      <span id="dashboard-sync-mark" data-state="loading"></span>
      <button id="refresh-ledger" type="button"><span data-button-label>刷新</span></button>
      <button id="refresh-grants" type="button"><span data-button-label>刷新</span></button>
      <span id="login-email"></span>
      <span id="total-points"></span>
      <span id="today-rewards"></span>
      <span id="total-checkin-rewards"></span>
      <span id="yesterday-points"></span>
      <span id="snapshot-date"></span>
      <section class="checkin-band" data-status="off">
        <button id="checkin" type="button" disabled><span data-button-label>立即签到</span></button>
        <span id="checkin-count"></span>
      </section>
      <section class="chart-panel" aria-busy="false">
        <canvas id="points-chart"></canvas>
        <div id="chart-empty" class="hidden"></div>
        <div id="chart-tooltip" class="hidden"></div>
        <div id="chart-live"></div>
        <span id="period-points"></span>
        <span id="average-points"></span>
        <span id="active-days"></span>
        <table><tbody id="chart-data-body"></tbody></table>
      </section>
      <table><tbody id="ledger-body"></tbody></table>
      <button id="ledger-prev" type="button"></button>
      <span id="ledger-page"></span>
      <button id="ledger-next" type="button"></button>
      <table><tbody id="grants-body"></tbody></table>
      <button id="grants-prev" type="button"></button>
      <span id="grants-page"></span>
      <button id="grants-next" type="button"></button>
    </main>
  `
}

function profile(checkinCount: number) {
  return {
    role: 'user',
    login_email: 'member@example.com',
    business_date: '2026-08-01',
    csrf_token: 'csrf-token',
    account: {
      total_points_hundredths: 1000,
      settled_checkin_reward_microusd: 0,
    },
    checkin: {
      count: checkinCount,
      awarded_microusd: checkinCount * 10_000,
    },
    yesterday_snapshot: {
      business_date: '2026-07-31',
      awarded_points_hundredths: 1000,
    },
    features: {
      points_enabled: true,
      checkin_enabled: true,
      checkin_daily_limit: 2,
      checkin_available: true,
    },
  }
}

function installPointsUI(
  api: PointsUI['api'],
  idempotencyKey: PointsUI['idempotencyKey'],
): void {
  const setButtonLabel: PointsUI['setButtonLabel'] = (button, text) => {
    const label = button.querySelector<HTMLElement>('[data-button-label]')
    if (label) label.textContent = text
    else button.textContent = text
  }
  const buttonLabel = (button: HTMLButtonElement) =>
    button.querySelector<HTMLElement>('[data-button-label]')?.textContent || button.textContent || ''

  const pointsUI: PointsUI = {
    api,
    byId(id) {
      const element = document.getElementById(id)
      if (!element) throw new Error(`missing test element: ${id}`)
      return element
    },
    date: (value) => String(value || ''),
    dateTime: (value) => String(value || ''),
    idempotencyKey,
    kindText: (value) => String(value || ''),
    logout: async () => undefined,
    money: (value) => String(value || 0),
    notifyReady: vi.fn(),
    notice: vi.fn(),
    number(value) {
      const result = Number(value || 0)
      return Number.isFinite(result) ? result : 0
    },
    points: (value) => String(value || 0),
    renderRows(target) {
      document.getElementById(target)?.replaceChildren()
    },
    setButtonBusy(button, busy, busyText = '处理中') {
      if (busy) {
        button.dataset.label = buttonLabel(button)
        setButtonLabel(button, busyText)
        button.disabled = true
        return
      }
      if (button.dataset.label) setButtonLabel(button, button.dataset.label)
      delete button.dataset.label
      button.disabled = false
    },
    setButtonLabel,
    setSession: vi.fn(),
    shortDate: (value) => String(value || ''),
    statusChip(value) {
      const chip = document.createElement('span')
      chip.textContent = String(value || '')
      return chip
    },
    statusText: (value) => String(value || ''),
  }

  Object.defineProperty(window, 'PointsUI', {
    configurable: true,
    value: pointsUI,
  })
}

describe('points user check-in idempotency recovery', () => {
  beforeEach(() => {
    sessionStorage.clear()
    installDashboardDOM()
  })

  afterEach(() => {
    sessionStorage.clear()
    document.body.replaceChildren()
    delete (window as Window & { PointsUI?: PointsUI }).PointsUI
  })

  it('replays the original key until the profile confirms that the count increased', async () => {
    let profileReads = 0
    const submittedKeys: string[] = []
    const api = vi.fn<PointsUI['api']>(async (path, options = {}) => {
      if (path === '/api/v1/me') {
        profileReads += 1
        if (profileReads === 1) return profile(0)
        if (profileReads === 2 || profileReads === 3) throw new Error('profile unavailable')
        return profile(1)
      }
      if (path === '/api/v1/checkins') {
        submittedKeys.push(new Headers(options.headers).get('Idempotency-Key') || '')
        return { reward_microusd: 10_000, delivery_status: 'pending' }
      }
      if (path.startsWith('/api/v1/daily-points')) return []
      if (path.startsWith('/api/v1/ledger') || path.startsWith('/api/v1/balance-grants')) {
        return { items: [], next_cursor: '' }
      }
      throw new Error(`unexpected API path: ${path}`)
    })
    const idempotencyKey = vi.fn()
      .mockReturnValueOnce(firstIdempotencyKey)
      .mockReturnValueOnce(secondIdempotencyKey)
    installPointsUI(api, idempotencyKey)

    window.eval(userScript)

    const checkin = document.getElementById('checkin') as HTMLButtonElement
    const checkinLabel = () => checkin.querySelector('[data-button-label]')?.textContent
    await vi.waitFor(() => {
      expect(checkinLabel()).toBe('立即签到')
      expect(checkin.disabled).toBe(false)
    })

    checkin.click()
    await vi.waitFor(() => expect(checkinLabel()).toBe('确认签到结果'))

    expect(checkin.disabled).toBe(false)
    expect(idempotencyKey).toHaveBeenCalledTimes(1)
    expect(submittedKeys).toEqual([firstIdempotencyKey])
    expect(JSON.parse(sessionStorage.getItem(pendingStorageKey) || '{}')).toMatchObject({
      key: firstIdempotencyKey,
      count: 0,
      business_date: '2026-08-01',
      login_email: 'member@example.com',
    })

    checkin.click()
    await vi.waitFor(() => expect(document.getElementById('checkin-count')?.textContent).toBe('今日已签到 1 次'))

    expect(idempotencyKey).toHaveBeenCalledTimes(1)
    expect(submittedKeys).toEqual([firstIdempotencyKey, firstIdempotencyKey])
    expect(sessionStorage.getItem(pendingStorageKey)).toBeNull()
    expect(checkinLabel()).toBe('立即签到')
    expect(checkin.disabled).toBe(false)
  })
})

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const commonScript = readFileSync(
  resolve(
    dirname(fileURLToPath(import.meta.url)),
    '../../../points-system/internal/httpapi/web/assets/common.js',
  ),
  'utf8',
)

type PointsUI = {
  api: (path: string, options?: RequestInit) => Promise<unknown>
}

function loadPointsUI(): PointsUI {
  window.eval(commonScript)
  return (window as Window & { PointsUI: PointsUI }).PointsUI
}

describe('points shared API timeout', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    document.documentElement.removeAttribute('data-theme')
    document.body.className = 'user-shell'
    document.body.replaceChildren()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('aborts a hung request and returns a localized timeout error', async () => {
    const fetchMock = vi.fn((_input: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => {
          reject(new DOMException('Aborted', 'AbortError'))
        }, { once: true })
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const ui = loadPointsUI()

    const request = ui.api('/api/v1/hung')
    const assertion = expect(request).rejects.toMatchObject({
      code: 'request_timeout',
      status: 0,
      message: '请求超时，请检查网络后重试',
    })
    await vi.advanceTimersByTimeAsync(20_000)

    await assertion
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0]?.[1]?.signal?.aborted).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })
})

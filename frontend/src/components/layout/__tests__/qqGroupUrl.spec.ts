import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const headerSource = readFileSync(resolve(dir, '../AppHeader.vue'), 'utf8')

describe('qq_group_url navigation', () => {
  it('sanitizes the configured URL before rendering it', () => {
    expect(headerSource).toContain('sanitizeUrl(appStore.qqGroupUrl)')
    expect(headerSource).toContain('v-if="qqGroupUrl"')
    expect(headerSource).toContain(':href="qqGroupUrl"')
  })

  it('renders the QQ group entry before Docs', () => {
    expect(headerSource.indexOf('<!-- QQ Group Link -->')).toBeGreaterThan(-1)
    expect(headerSource.indexOf('<!-- QQ Group Link -->')).toBeLessThan(
      headerSource.indexOf('<!-- Docs Link -->'),
    )
  })
})

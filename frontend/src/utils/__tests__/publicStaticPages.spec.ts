import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { JSDOM } from 'jsdom'
import { describe, expect, it } from 'vitest'

function readPublicPage(relativePath: string): string {
  return readFileSync(resolve(process.cwd(), '..', relativePath), 'utf8')
}

describe.each([
  ['public landing page', 'deploy/public-landing/index.html'],
  ['public help page', 'deploy/public-help/index.html']
])('%s', (_name, relativePath) => {
  const html = readPublicPage(relativePath)
  const document = new JSDOM(html).window.document

  it('contains no executable or credential collection surface', () => {
    expect(document.querySelector('script, form, input, textarea, iframe')).toBeNull()
    expect(html).not.toMatch(/\son[a-z]+\s*=/i)
    expect(html).not.toMatch(/javascript:|data:text|\beval\s*\(|\bfetch\s*\(/i)
  })

  it('does not reference an external origin', () => {
    const resourceURLs = Array.from(document.querySelectorAll<HTMLElement>('[href], [src]'))
      .map((element) => element.getAttribute('href') || element.getAttribute('src'))
      .filter((value): value is string => value !== null)

    for (const value of resourceURLs) {
      const url = new URL(value, 'https://52token.org')
      expect(url.hostname === '52token.org' || url.hostname.endsWith('.52token.org')).toBe(true)
    }
  })

  it('does not reference the retired service domain', () => {
    expect(html).not.toContain('gpt-codex.top')
  })

  it('uses calc for subtraction inside CSS math functions', () => {
    expect(html).not.toMatch(/\b(?:min|max|clamp)\(\s*100%\s*-/i)
  })
})

describe('public landing page entry contract', () => {
  const html = readPublicPage('deploy/public-landing/index.html')
  const document = new JSDOM(html).window.document

  it('provides the same-origin documentation and account entry points', () => {
    const hrefs = new Set(
      Array.from(document.querySelectorAll<HTMLAnchorElement>('a[href]')).map((link) =>
        link.getAttribute('href'),
      ),
    )

    for (const entryPoint of ['/help/', '/login', '/register']) {
      expect(hrefs).toContain(entryPoint)
    }
  })

  it('keeps section navigation available without JavaScript on mobile', () => {
    const mobileNav = document.querySelector('details.mobile-nav')
    expect(mobileNav).not.toBeNull()
    const hrefs = Array.from(mobileNav?.querySelectorAll<HTMLAnchorElement>('a[href]') || []).map(
      (link) => link.getAttribute('href'),
    )
    expect(hrefs).toEqual(['#overview', '#reliability', '#guide', '#faq'])
  })

  it('keeps the public copy neutral and non-provider-specific', () => {
    const copy = document.body.textContent?.toLowerCase() || ''
    for (const term of ['openai', 'claude', 'anthropic', 'fastaitoken', '中转']) {
      expect(copy).not.toContain(term)
    }
  })

  it('keeps search indexing disabled and uses only the same-origin uploaded logo', () => {
    const robots = document.querySelector<HTMLMetaElement>('meta[name="robots"]')
    expect(robots?.content).toBe('noindex, nofollow, noarchive')
    expect(document.querySelector('link[rel="stylesheet"], video, audio')).toBeNull()

    const images = Array.from(document.querySelectorAll<HTMLImageElement>('img'))
    expect(images).toHaveLength(2)
    expect(images.every((image) => image.getAttribute('src') === '/api/v1/settings/logo')).toBe(
      true,
    )
  })

  it('ships the split hero and its transparent vector visual inside the exact-root HTML', () => {
    expect(document.querySelector('.hero-layout .hero-copy-block')).not.toBeNull()
    expect(document.querySelector('.hero-layout .hero-visual[role="img"]')).not.toBeNull()
    expect(document.querySelector('.hero-layout .home-data-visual-svg')).not.toBeNull()
    expect(html).not.toContain('data:image/webp;base64,')
  })

  it('does not reintroduce decorative hero pseudo-element borders', () => {
    expect(html).not.toContain('.hero::before')
    expect(html).not.toContain('.hero::after')
  })
})

describe('public static page Nginx security contract', () => {
  const rootLocations = readPublicPage('deploy/nginx/52token-public-root.inc.example')
  const securityHeaders = readPublicPage(
    'deploy/nginx/52token-static-security-headers.inc.example',
  )

  it('applies the static security headers to both exact root locations', () => {
    expect(rootLocations).toMatch(/location = \/\s*{/)
    expect(rootLocations).toMatch(/location = \/index\.html\s*{/)
    expect(rootLocations.match(/52token-static-security-headers\.inc/g)).toHaveLength(2)
  })

  it.each([
    "script-src 'none'",
    "connect-src 'none'",
    "object-src 'none'",
    "frame-src 'none'",
    "frame-ancestors 'none'",
  ])('retains the restrictive %s directive', (directive) => {
    expect(securityHeaders).toContain(directive)
  })
})

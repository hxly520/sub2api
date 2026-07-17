import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

function readPublicPage(relativePath: string): string {
  return readFileSync(resolve(process.cwd(), '..', relativePath), 'utf8')
}

describe.each([
  ['public landing page', 'deploy/public-landing/index.html'],
  ['public help page', 'deploy/public-help/index.html']
])('%s', (_name, relativePath) => {
  const html = readPublicPage(relativePath)

  it('contains no executable or credential collection surface', () => {
    expect(html).not.toMatch(/<script\b/i)
    expect(html).not.toMatch(/<(?:form|input|textarea|iframe)\b/i)
    expect(html).not.toMatch(/\son[a-z]+\s*=/i)
    expect(html).not.toMatch(/javascript:|data:text|\beval\s*\(|\bfetch\s*\(/i)
  })

  it('does not reference an external origin', () => {
    const urls = html.match(/https?:\/\/[^\s"'<>]+/gi) || []
    for (const value of urls) {
      const hostname = new URL(value.replace(/[),.;]+$/, '')).hostname
      expect(hostname === '52token.org' || hostname.endsWith('.52token.org')).toBe(true)
    }
  })

  it('does not reference the retired service domain', () => {
    expect(html).not.toContain('gpt-codex.top')
  })

  it('uses calc for subtraction inside CSS math functions', () => {
    expect(html).not.toMatch(/\b(?:min|max|clamp)\(\s*100%\s*-/i)
  })
})

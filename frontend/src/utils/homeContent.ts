import DOMPurify from 'dompurify'
import { sanitizeUrl } from '@/utils/url'

const HOME_CONTENT_TAGS = [
  'section',
  'div',
  'p',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'ul',
  'ol',
  'li',
  'strong',
  'em',
  'span',
  'a',
  'br',
  'hr',
  'blockquote',
  'code',
  'pre'
]

export function sanitizeHomeContentHtml(value: string): string {
  const sanitized = DOMPurify.sanitize(value, {
    ALLOWED_TAGS: HOME_CONTENT_TAGS,
    ALLOWED_ATTR: ['href', 'title', 'id'],
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
    SANITIZE_NAMED_PROPS: false
  })

  const documentFragment = new DOMParser().parseFromString(sanitized, 'text/html')
  documentFragment.querySelectorAll('[id]').forEach((element) => {
    const id = element.getAttribute('id') || ''
    // Keep only scoped presentation anchors; reject DOM-clobbering names and
    // arbitrary selectors from admin-provided HTML.
    if (!/^(?:custom|home)-[a-z0-9_-]{1,62}$/i.test(id)) {
      element.removeAttribute('id')
    }
  })
  documentFragment.querySelectorAll('a').forEach((link) => {
    const href = sanitizeUrl(link.getAttribute('href') || '', { allowRelative: true })
    if (!href) {
      link.removeAttribute('href')
      return
    }

    link.setAttribute('href', href)
    link.setAttribute('target', '_blank')
    link.setAttribute('rel', 'noopener noreferrer')
  })
  return documentFragment.body.innerHTML
}

export function sanitizeHomeContentFrameUrl(value: string): string {
  const sanitized = sanitizeUrl(value)
  if (!sanitized) return ''

  const parsed = new URL(sanitized)
  const isLocalDevelopment = parsed.hostname === 'localhost' || parsed.hostname === '127.0.0.1'
  if (parsed.protocol !== 'https:' && !(parsed.protocol === 'http:' && isLocalDevelopment)) {
    return ''
  }

  parsed.username = ''
  parsed.password = ''
  return parsed.toString()
}

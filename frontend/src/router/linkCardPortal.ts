const LINK_CARD_PORTAL_HOST = 'key.52token.org'
const LINK_CARD_PORTAL_PATH = '/card'
const LINK_CARD_PORTAL_ALIASES = new Set([
  LINK_CARD_PORTAL_PATH,
  '/link-card',
  '/quota-card',
])

export function resolveLinkCardPortalRedirect(hostname: string, path: string): string | null {
  if (hostname.trim().toLowerCase() !== LINK_CARD_PORTAL_HOST) return null
  return LINK_CARD_PORTAL_ALIASES.has(path) ? null : LINK_CARD_PORTAL_PATH
}


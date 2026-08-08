import { describe, expect, it } from 'vitest'
import { resolveLinkCardPortalRedirect } from '@/router/linkCardPortal'

describe('quota-card portal host routing', () => {
  it.each(['/card', '/link-card', '/quota-card'])(
    'keeps the public portal route %s reachable',
    (path) => {
      expect(resolveLinkCardPortalRedirect('key.52token.org', path)).toBeNull()
    },
  )

  it.each(['/', '/login', '/dashboard', '/missing'])(
    'returns non-portal route %s to the card entry page',
    (path) => {
      expect(resolveLinkCardPortalRedirect('KEY.52TOKEN.ORG', path)).toBe('/card')
    },
  )

  it('does not affect the regular Sub2API host', () => {
    expect(resolveLinkCardPortalRedirect('api.52token.org', '/login')).toBeNull()
  })
})

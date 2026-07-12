const TOKEN_AAD = 'sub2api-video-proxy-v1'
const TOKEN_VERSION = 1
const MAX_TOKEN_BYTES = 16 * 1024
const MAX_REDIRECTS = 4

const CLIENT_REQUEST_HEADERS = [
  'range',
  'if-range',
  'if-none-match',
  'if-modified-since',
]

const TOKEN_REQUEST_HEADERS = new Set([
  'authorization',
  'api-key',
  'x-api-key',
  'x-goog-api-key',
])

const UPSTREAM_RESPONSE_HEADERS = [
  'content-type',
  'content-length',
  'content-range',
  'accept-ranges',
  'etag',
  'last-modified',
  'cache-control',
]

function jsonError(status, message) {
  return new Response(JSON.stringify({
    error: {
      message,
      type: 'media_proxy_error',
      param: null,
      code: null,
    },
  }), {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
      ...corsHeaders(),
    },
  })
}

function corsHeaders() {
  return {
    'access-control-allow-origin': '*',
    'access-control-allow-methods': 'GET, HEAD, OPTIONS',
    'access-control-allow-headers': 'Range, If-Range, If-None-Match, If-Modified-Since',
    'access-control-expose-headers': 'Content-Type, Content-Length, Content-Range, Accept-Ranges, ETag, Last-Modified',
    'x-content-type-options': 'nosniff',
    'referrer-policy': 'no-referrer',
  }
}

function decodeBase64URL(value) {
  if (typeof value !== 'string' || value.length === 0 || value.length > MAX_TOKEN_BYTES * 2) {
    throw new Error('invalid token')
  }
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padding = '='.repeat((4 - (normalized.length % 4)) % 4)
  const binary = atob(normalized + padding)
  if (binary.length > MAX_TOKEN_BYTES) {
    throw new Error('invalid token')
  }
  return Uint8Array.from(binary, (character) => character.charCodeAt(0))
}

function decodeHexKey(value) {
  const normalized = String(value || '').trim()
  if (!/^[a-fA-F0-9]{64}$/.test(normalized)) {
    throw new Error('invalid encryption key')
  }
  const bytes = new Uint8Array(32)
  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = Number.parseInt(normalized.slice(index * 2, index * 2 + 2), 16)
  }
  return bytes
}

export async function decryptVideoToken(token, keyHex, nowSeconds = Math.floor(Date.now() / 1000)) {
  const sealed = decodeBase64URL(token)
  if (sealed.length <= 12 + 16) {
    throw new Error('invalid token')
  }
  const key = await crypto.subtle.importKey(
    'raw',
    decodeHexKey(keyHex),
    { name: 'AES-GCM' },
    false,
    ['decrypt'],
  )
  const plaintext = await crypto.subtle.decrypt({
    name: 'AES-GCM',
    iv: sealed.slice(0, 12),
    additionalData: new TextEncoder().encode(TOKEN_AAD),
    tagLength: 128,
  }, key, sealed.slice(12))
  const payload = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(plaintext))
  if (payload?.v !== TOKEN_VERSION || !Number.isSafeInteger(payload?.e) || payload.e <= nowSeconds) {
    throw new Error('expired token')
  }
  if (payload.e > nowSeconds + 86400 + 60) {
    throw new Error('invalid token lifetime')
  }
  return payload
}

function parseAllowedHosts(value) {
  return String(value || '')
    .split(',')
    .map((entry) => entry.trim().toLowerCase())
    .filter(Boolean)
}

function hostMatches(hostname, pattern) {
  if (pattern.startsWith('*.')) {
    const suffix = pattern.slice(2)
    return hostname === suffix || hostname.endsWith(`.${suffix}`)
  }
  return hostname === pattern
}

function isBlockedIPv4(hostname) {
  const parts = hostname.split('.')
  if (parts.length !== 4 || parts.some((part) => !/^\d{1,3}$/.test(part))) {
    return false
  }
  const values = parts.map(Number)
  if (values.some((value) => value < 0 || value > 255)) {
    return true
  }
  const [a, b] = values
  return a === 0 || a === 10 || a === 127 ||
    (a === 100 && b >= 64 && b <= 127) ||
    (a === 169 && b === 254) ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168) ||
    a >= 224
}

function isBlockedHostname(hostname) {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, '')
  if (host === 'localhost' || host.endsWith('.localhost') || host.endsWith('.local') || host.endsWith('.internal')) {
    return true
  }
  if (isBlockedIPv4(host)) {
    return true
  }
  if (host.includes(':')) {
    return host === '::' || host === '::1' || host.startsWith('fc') || host.startsWith('fd') || host.startsWith('fe8') || host.startsWith('fe9') || host.startsWith('fea') || host.startsWith('feb')
  }
  return false
}

export function validateTargetURL(rawURL, allowedHostsValue = '', mediaType = 'video') {
  if (typeof rawURL !== 'string' || rawURL.length === 0 || rawURL.length > 8192 || /[\r\n]/.test(rawURL)) {
    throw new Error('invalid target')
  }
  const target = new URL(rawURL)
  const allowHTTP = mediaType === 'image'
  const validScheme = target.protocol === 'https:' || (allowHTTP && target.protocol === 'http:')
  const validPort = !target.port || (target.protocol === 'https:' && target.port === '443') || (target.protocol === 'http:' && target.port === '80')
  if (!validScheme || !validPort || target.username || target.password) {
    throw new Error('invalid target')
  }
  const hostname = target.hostname.toLowerCase()
  if (!hostname || isBlockedHostname(hostname)) {
    throw new Error('blocked target')
  }
  const allowedHosts = parseAllowedHosts(allowedHostsValue)
  if (allowedHosts.length > 0 && !allowedHosts.some((pattern) => hostMatches(hostname, pattern))) {
    throw new Error('target host is not allowed')
  }
  target.hash = ''
  return target
}

function buildUpstreamHeaders(request, tokenHeaders, mediaType) {
  const headers = new Headers()
  for (const name of CLIENT_REQUEST_HEADERS) {
    const value = request.headers.get(name)
    if (value) {
      headers.set(name, value)
    }
  }
  if (tokenHeaders && typeof tokenHeaders === 'object' && !Array.isArray(tokenHeaders)) {
    for (const [rawName, rawValue] of Object.entries(tokenHeaders)) {
      const name = rawName.trim().toLowerCase()
      const value = typeof rawValue === 'string' ? rawValue.trim() : ''
      if (TOKEN_REQUEST_HEADERS.has(name) && value && value.length <= 8192 && !/[\r\n]/.test(value)) {
        headers.set(name, value)
      }
    }
  }
  headers.set('accept', mediaType === 'image'
    ? 'image/*, application/octet-stream;q=0.8, */*;q=0.1'
    : 'video/*, application/octet-stream;q=0.9, */*;q=0.1')
  return headers
}

async function fetchFollowingRedirects(target, init, allowedHostsValue, mediaType) {
  let current = target
  for (let redirects = 0; redirects <= MAX_REDIRECTS; redirects += 1) {
    const response = await fetch(current.toString(), {
      ...init,
      redirect: 'manual',
      cf: { cacheEverything: false, cacheTtl: 0 },
    })
    if (response.status < 300 || response.status >= 400) {
      return response
    }
    const location = response.headers.get('location')
    if (!location || redirects === MAX_REDIRECTS) {
      response.body?.cancel()
      throw new Error('redirect unavailable')
    }
    const next = validateTargetURL(new URL(location, current).toString(), allowedHostsValue, mediaType)
    response.body?.cancel()
    current = next
  }
  throw new Error('redirect unavailable')
}

function isAllowedContentType(value, mediaType) {
  const contentMediaType = String(value || '').split(';', 1)[0].trim().toLowerCase()
  if (mediaType === 'image') {
    return contentMediaType.startsWith('image/') || contentMediaType === 'application/octet-stream' || contentMediaType === 'binary/octet-stream'
  }
  return contentMediaType.startsWith('video/') || contentMediaType === 'application/octet-stream' || contentMediaType === 'binary/octet-stream' || contentMediaType === 'application/mp4'
}

function responseHeadersFromUpstream(response, headRangeFallback = false) {
  const headers = new Headers(corsHeaders())
  for (const name of UPSTREAM_RESPONSE_HEADERS) {
    const value = response.headers.get(name)
    if (value && !/[\r\n]/.test(value)) {
      headers.set(name, value)
    }
  }
  if (headRangeFallback) {
    const contentRange = response.headers.get('content-range') || ''
    const match = /\/([0-9]+)$/.exec(contentRange)
    headers.delete('content-range')
    if (match) {
      headers.set('content-length', match[1])
    } else {
      headers.delete('content-length')
    }
  }
  if (!headers.has('cache-control')) {
    headers.set('cache-control', 'private, max-age=60')
  }
  return headers
}

async function proxyMedia(request, env, payload, mediaType) {
  const allowedHosts = env.ALLOWED_MEDIA_HOSTS || ''
  const target = validateTargetURL(payload.u, allowedHosts, mediaType)
  const headers = buildUpstreamHeaders(request, payload.h, mediaType)
  let method = request.method
  let headRangeFallback = false
  let response = await fetchFollowingRedirects(target, { method, headers }, allowedHosts, mediaType)

  if (method === 'HEAD' && (response.status === 405 || response.status === 501)) {
    response.body?.cancel()
    const fallbackHeaders = new Headers(headers)
    fallbackHeaders.set('range', 'bytes=0-0')
    response = await fetchFollowingRedirects(target, { method: 'GET', headers: fallbackHeaders }, allowedHosts, mediaType)
    headRangeFallback = true
  }
  if (response.status >= 400) {
    response.body?.cancel()
    return jsonError(502, 'Media content is unavailable')
  }
  if (response.status !== 304 && !isAllowedContentType(response.headers.get('content-type'), mediaType)) {
    response.body?.cancel()
    return jsonError(502, 'Media content is unavailable')
  }
  const responseHeaders = responseHeadersFromUpstream(response, headRangeFallback)
  const status = headRangeFallback && response.status === 206 ? 200 : response.status
  const body = method === 'HEAD' || status === 304 ? null : response.body
  return new Response(body, { status, headers: responseHeaders })
}

export default {
  async fetch(request, env) {
    if (request.method === 'OPTIONS') {
      return new Response(null, { status: 204, headers: corsHeaders() })
    }
    if (request.method !== 'GET' && request.method !== 'HEAD') {
      return jsonError(405, 'Method not allowed')
    }
    const requestURL = new URL(request.url)
    const route = [
      { prefix: '/v1/video-content/', mediaType: 'video' },
      { prefix: '/v1/image-content/', mediaType: 'image' },
    ].find((candidate) => requestURL.pathname.startsWith(candidate.prefix))
    if (!route || requestURL.search || requestURL.hash) {
      return jsonError(404, 'Media content not found')
    }
    const token = requestURL.pathname.slice(route.prefix.length)
    if (!token || token.includes('/')) {
      return jsonError(404, 'Media content not found')
    }
    try {
      const payload = await decryptVideoToken(token, env.VIDEO_PROXY_KEY_HEX)
      const tokenMediaType = payload.m || 'video'
      if (tokenMediaType !== route.mediaType) {
        throw new Error('invalid media type')
      }
      return await proxyMedia(request, env, payload, route.mediaType)
    } catch (error) {
      const status = error instanceof Error && error.message === 'expired token' ? 410 : 404
      return jsonError(status, status === 410 ? 'Media link has expired' : 'Media content not found')
    }
  },
}

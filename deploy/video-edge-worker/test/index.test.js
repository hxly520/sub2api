import assert from 'node:assert/strict'
import test from 'node:test'
import { webcrypto } from 'node:crypto'

if (!globalThis.crypto) {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto })
}
if (!globalThis.atob) {
  Object.defineProperty(globalThis, 'atob', {
    value: (value) => Buffer.from(value, 'base64').toString('binary'),
  })
}

const { default: worker, decryptVideoToken, validateTargetURL } = await import('../src/index.js')

const KEY_HEX = '11'.repeat(32)
const AAD = new TextEncoder().encode('sub2api-video-proxy-v1')

function hexBytes(value) {
  return Uint8Array.from(value.match(/../g), (part) => Number.parseInt(part, 16))
}

function encodeBase64URL(value) {
  return Buffer.from(value).toString('base64url')
}

async function encryptToken(payload, keyHex = KEY_HEX) {
  const key = await crypto.subtle.importKey('raw', hexBytes(keyHex), { name: 'AES-GCM' }, false, ['encrypt'])
  const nonce = Uint8Array.from({ length: 12 }, (_, index) => index + 1)
  const ciphertext = new Uint8Array(await crypto.subtle.encrypt({
    name: 'AES-GCM',
    iv: nonce,
    additionalData: AAD,
    tagLength: 128,
  }, key, new TextEncoder().encode(JSON.stringify(payload))))
  const sealed = new Uint8Array(nonce.length + ciphertext.length)
  sealed.set(nonce)
  sealed.set(ciphertext, nonce.length)
  return encodeBase64URL(sealed)
}

test('decryptVideoToken accepts the Go-compatible AES-GCM envelope', async () => {
  const now = 1_800_000_000
  const payload = {
    v: 1,
    u: 'https://1.1.1.1/video.mp4?signature=secret',
    e: now + 3600,
    h: { Authorization: 'Bearer upstream-secret' },
  }
  const token = await encryptToken(payload)
  assert.deepEqual(await decryptVideoToken(token, KEY_HEX, now), payload)
})

test('validateTargetURL rejects private and non-allowlisted targets', () => {
  assert.throws(() => validateTargetURL('https://127.0.0.1/video.mp4'))
  assert.throws(() => validateTargetURL('http://1.1.1.1/video.mp4'))
  assert.equal(validateTargetURL('http://1.1.1.1/image.png', '', 'image').hostname, '1.1.1.1')
  assert.throws(() => validateTargetURL('http://127.0.0.1/image.png', '', 'image'))
  assert.throws(() => validateTargetURL('https://8.8.8.8/video.mp4', '1.1.1.1'))
  assert.equal(validateTargetURL('https://cdn.example.com/video.mp4', '*.example.com').hostname, 'cdn.example.com')
})

test('worker proxies image tokens on the dedicated media domain', { concurrency: false }, async () => {
  const originalFetch = globalThis.fetch
  let observedURL = ''
  globalThis.fetch = async (url) => {
    observedURL = String(url)
    return new Response('image', {
      status: 200,
      headers: { 'content-type': 'image/png', 'content-length': '5' },
    })
  }
  try {
    const token = await encryptToken({
      v: 1,
      m: 'image',
      u: 'http://1.1.1.1/generated.png?signature=secret',
      e: Math.floor(Date.now() / 1000) + 3600,
    })
    const response = await worker.fetch(new Request(`https://video.52token.org/v1/image-content/${token}`), {
      VIDEO_PROXY_KEY_HEX: KEY_HEX,
      ALLOWED_MEDIA_HOSTS: '1.1.1.1',
    })

    assert.equal(response.status, 200)
    assert.equal(await response.text(), 'image')
    assert.equal(observedURL, 'http://1.1.1.1/generated.png?signature=secret')
    assert.equal(response.headers.get('content-type'), 'image/png')
    assert.equal(response.headers.get('access-control-allow-origin'), '*')
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('worker accepts image downloads served as generic binary content', { concurrency: false }, async () => {
  const originalFetch = globalThis.fetch
  globalThis.fetch = async () => new Response('image', {
    status: 200,
    headers: { 'content-type': 'application/octet-stream' },
  })
  try {
    const token = await encryptToken({
      v: 1,
      m: 'image',
      u: 'https://1.1.1.1/generated.webp',
      e: Math.floor(Date.now() / 1000) + 3600,
    })
    const response = await worker.fetch(new Request(`https://video.52token.org/v1/image-content/${token}`), {
      VIDEO_PROXY_KEY_HEX: KEY_HEX,
      ALLOWED_MEDIA_HOSTS: '1.1.1.1',
    })

    assert.equal(response.status, 200)
    assert.equal(await response.text(), 'image')
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('worker streams ranges and encrypted authorization without leaking upstream metadata', { concurrency: false }, async () => {
  const originalFetch = globalThis.fetch
  let observedURL = ''
  let observedHeaders
  globalThis.fetch = async (url, init) => {
    observedURL = String(url)
    observedHeaders = new Headers(init.headers)
    return new Response('data', {
      status: 206,
      headers: {
        'content-type': 'video/mp4',
        'content-length': '4',
        'content-range': 'bytes 10-13/100',
        'accept-ranges': 'bytes',
        'server': 'private-upstream',
        'x-upstream-request-id': 'secret-id',
      },
    })
  }
  try {
    const token = await encryptToken({
      v: 1,
      u: 'https://1.1.1.1/private/video.mp4?signature=secret',
      e: Math.floor(Date.now() / 1000) + 3600,
      h: { Authorization: 'Bearer upstream-secret' },
    })
    const response = await worker.fetch(new Request(`https://video.52token.org/v1/video-content/${token}`, {
      headers: { Range: 'bytes=10-13' },
    }), {
      VIDEO_PROXY_KEY_HEX: KEY_HEX,
      ALLOWED_MEDIA_HOSTS: '1.1.1.1',
    })

    assert.equal(response.status, 206)
    assert.equal(await response.text(), 'data')
    assert.equal(observedURL, 'https://1.1.1.1/private/video.mp4?signature=secret')
    assert.equal(observedHeaders.get('range'), 'bytes=10-13')
    assert.equal(observedHeaders.get('authorization'), 'Bearer upstream-secret')
    assert.equal(response.headers.get('server'), null)
    assert.equal(response.headers.get('x-upstream-request-id'), null)
    assert.equal(response.headers.get('access-control-allow-origin'), '*')
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('worker does not impose a generated media size limit', { concurrency: false }, async () => {
  const originalFetch = globalThis.fetch
  globalThis.fetch = async () => new Response(null, {
    status: 200,
    headers: {
      'content-type': 'video/mp4',
      'content-length': String(8 * 1024 * 1024 * 1024),
      'accept-ranges': 'bytes',
    },
  })
  try {
    const token = await encryptToken({
      v: 1,
      m: 'video',
      u: 'https://1.1.1.1/large-video.mp4',
      e: Math.floor(Date.now() / 1000) + 3600,
    })
    const response = await worker.fetch(new Request(`https://video.52token.org/v1/video-content/${token}`, { method: 'HEAD' }), {
      VIDEO_PROXY_KEY_HEX: KEY_HEX,
      ALLOWED_MEDIA_HOSTS: '1.1.1.1',
    })

    assert.equal(response.status, 200)
    assert.equal(response.headers.get('content-length'), String(8 * 1024 * 1024 * 1024))
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('worker handles HEAD when the upstream only supports ranged GET', { concurrency: false }, async () => {
  const originalFetch = globalThis.fetch
  const methods = []
  globalThis.fetch = async (_url, init) => {
    methods.push(init.method)
    if (init.method === 'HEAD') {
      return new Response(null, { status: 405 })
    }
    assert.equal(new Headers(init.headers).get('range'), 'bytes=0-0')
    return new Response('x', {
      status: 206,
      headers: {
        'content-type': 'video/mp4',
        'content-length': '1',
        'content-range': 'bytes 0-0/9876',
      },
    })
  }
  try {
    const token = await encryptToken({
      v: 1,
      u: 'https://1.1.1.1/video.mp4',
      e: Math.floor(Date.now() / 1000) + 3600,
    })
    const response = await worker.fetch(new Request(`https://video.52token.org/v1/video-content/${token}`, { method: 'HEAD' }), {
      VIDEO_PROXY_KEY_HEX: KEY_HEX,
      ALLOWED_MEDIA_HOSTS: '1.1.1.1',
    })
    assert.equal(response.status, 200)
    assert.equal(response.body, null)
    assert.equal(response.headers.get('content-length'), '9876')
    assert.deepEqual(methods, ['HEAD', 'GET'])
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('worker follows redirects internally and never returns Location', { concurrency: false }, async () => {
  const originalFetch = globalThis.fetch
  const urls = []
  globalThis.fetch = async (url) => {
    urls.push(String(url))
    if (urls.length === 1) {
      return new Response(null, { status: 302, headers: { location: 'https://8.8.8.8/final.mp4?token=hidden' } })
    }
    return new Response('video', { status: 200, headers: { 'content-type': 'video/mp4' } })
  }
  try {
    const token = await encryptToken({
      v: 1,
      u: 'https://1.1.1.1/start',
      e: Math.floor(Date.now() / 1000) + 3600,
    })
    const response = await worker.fetch(new Request(`https://video.52token.org/v1/video-content/${token}`), {
      VIDEO_PROXY_KEY_HEX: KEY_HEX,
      ALLOWED_MEDIA_HOSTS: '1.1.1.1,8.8.8.8',
    })
    assert.equal(response.status, 200)
    assert.equal(response.headers.get('location'), null)
    assert.equal(await response.text(), 'video')
    assert.deepEqual(urls, ['https://1.1.1.1/start', 'https://8.8.8.8/final.mp4?token=hidden'])
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('worker returns a generic terminal error for expired tokens', async () => {
  const token = await encryptToken({
    v: 1,
    u: 'https://1.1.1.1/video.mp4',
    e: Math.floor(Date.now() / 1000) - 1,
  })
  const response = await worker.fetch(new Request(`https://video.52token.org/v1/video-content/${token}`), {
    VIDEO_PROXY_KEY_HEX: KEY_HEX,
  })
  assert.equal(response.status, 410)
  assert.doesNotMatch(await response.text(), /1\.1\.1\.1|video\.mp4/)
})

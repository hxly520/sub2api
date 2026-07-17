import type { GroupPlatform } from '@/types'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-5.5'

export type CcSwitchClientType = 'claude' | 'gemini'

export interface CcSwitchImportConfig {
  app: string
  endpoint: string
  model?: string
}

export interface CcSwitchImportDeeplinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  clientType: CcSwitchClientType
  providerName: string
  apiKey: string
  usageScriptBase64: string
  usageAutoIntervalMinutes: number
}

function normalizeCcSwitchBaseUrl(value: string): string {
  const parsed = new URL(value)
  const isLocalDevelopment = parsed.hostname === 'localhost' || parsed.hostname === '127.0.0.1'

  if (parsed.protocol !== 'https:' && !(parsed.protocol === 'http:' && isLocalDevelopment)) {
    throw new Error('CC Switch imports require an HTTPS endpoint')
  }

  parsed.username = ''
  parsed.password = ''
  parsed.search = ''
  parsed.hash = ''
  parsed.pathname = parsed.pathname.replace(/\/+$/, '')
  return parsed.toString().replace(/\/$/, '')
}

function normalizeCcSwitchValue(value: string, field: string): string {
  const normalized = Array.from(value, character => {
    const code = character.charCodeAt(0)
    return code <= 31 || code === 127 ? ' ' : character
  }).join('').trim()
  if (!normalized) {
    throw new Error(`CC Switch ${field} is required`)
  }
  return normalized
}

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string
): CcSwitchImportConfig {
  switch (platform || 'anthropic') {
    case 'antigravity':
      return {
        app: clientType === 'gemini' ? 'gemini' : 'claude',
        endpoint: `${baseUrl}/antigravity`
      }
    case 'openai':
      return {
        app: 'codex',
        endpoint: baseUrl,
        model: OPENAI_CC_SWITCH_CODEX_MODEL
      }
    case 'gemini':
      return {
        app: 'gemini',
        endpoint: baseUrl
      }
    default:
      return {
        app: 'claude',
        endpoint: baseUrl
      }
  }
}

export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const baseUrl = normalizeCcSwitchBaseUrl(input.baseUrl)
  const providerName = normalizeCcSwitchValue(input.providerName, 'provider name').slice(0, 80)
  const apiKey = normalizeCcSwitchValue(input.apiKey, 'API key')
  const usageScriptBase64 = normalizeCcSwitchValue(input.usageScriptBase64, 'usage template')
  if (!Number.isInteger(input.usageAutoIntervalMinutes) || input.usageAutoIntervalMinutes < 1) {
    throw new Error('CC Switch usage interval must be a positive integer')
  }
  const config = resolveCcSwitchImportConfig(input.platform, input.clientType, baseUrl)
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', providerName],
    ['homepage', new URL(baseUrl).origin],
    ['endpoint', config.endpoint],
    ['apiKey', apiKey],
    ['configFormat', 'json'],
    ['enabled', 'true'],
    ['usageEnabled', 'true'],
    ['usageScript', usageScriptBase64],
    ['usageAutoInterval', String(input.usageAutoIntervalMinutes)]
  ]

  if (config.model) {
    entries.splice(2, 0, ['model', config.model])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}

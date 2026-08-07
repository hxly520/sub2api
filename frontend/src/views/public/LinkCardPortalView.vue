<template>
  <main class="quota-portal min-h-[100dvh]" data-testid="link-card-portal">
    <div class="quota-grid" aria-hidden="true" />

    <section v-if="portalState === 'activate'" class="relative z-10 flex min-h-[100dvh] items-center justify-center px-4 py-10">
      <form class="activation-panel w-full max-w-xl" @submit.prevent="activateCard">
        <div class="mb-8 flex justify-center">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-11 w-11 object-contain" />
        </div>
        <h1 class="text-center text-2xl font-semibold text-white sm:text-3xl">{{ t('linkCards.activateTitle') }}</h1>
        <p class="mx-auto mt-3 max-w-md text-center text-sm leading-6 text-zinc-400">{{ t('linkCards.activateSubtitle') }}</p>
        <div class="mt-8 flex flex-col gap-3 sm:flex-row">
          <label class="sr-only" for="quota-card-key">{{ t('linkCards.fullKey') }}</label>
          <input
            id="quota-card-key"
            v-model.trim="keyInput"
            type="password"
            autocomplete="off"
            spellcheck="false"
            :placeholder="t('linkCards.keyPlaceholder')"
            class="tech-input min-w-0 flex-1 font-mono"
            :aria-invalid="Boolean(activationError)"
          />
          <button type="submit" class="activate-button inline-flex min-h-12 shrink-0 items-center justify-center gap-2 px-6" :disabled="activating">
            <Icon name="arrowRight" size="sm" />
            {{ activating ? t('linkCards.activating') : t('linkCards.activate') }}
          </button>
        </div>
        <p v-if="activationError" class="mt-3 text-sm text-rose-400" role="alert">{{ activationError }}</p>
      </form>
    </section>

    <section v-else class="relative z-10 mx-auto w-full max-w-[1480px] px-3 py-4 sm:px-5 sm:py-6">
      <header class="flex min-h-14 items-center justify-between gap-4 border-b border-white/10 pb-4">
        <div class="flex items-center gap-3">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-9 w-9 object-contain" />
          <h1 class="text-base font-semibold text-white sm:text-lg">{{ t('linkCards.cardPortal') }}</h1>
        </div>
        <button type="button" class="tech-button inline-flex items-center gap-2" @click="clearSession"><Icon name="swap" size="sm" />{{ t('linkCards.clearSession') }}</button>
      </header>

      <div v-if="portalState === 'loading'" class="grid gap-3 py-5 sm:grid-cols-2 xl:grid-cols-4" role="status">
        <div v-for="index in 4" :key="index" class="tech-panel h-28 animate-pulse bg-white/5 motion-reduce:animate-none" />
        <span class="sr-only">{{ t('linkCards.loading') }}</span>
      </div>

      <template v-else-if="profile">
        <div class="grid grid-cols-2 gap-3 py-5 xl:grid-cols-4">
          <article v-for="metric in metrics" :key="metric.key" class="tech-panel min-h-[108px] p-4 sm:p-5">
            <div class="flex items-start justify-between gap-3">
              <p class="text-xs font-medium text-zinc-400">{{ metric.label }}</p>
              <Icon :name="metric.icon" size="sm" class="text-cyan-300" />
            </div>
            <p class="mt-4 text-xl font-semibold tabular-nums text-white sm:text-2xl">{{ metric.value }}</p>
          </article>
        </div>

        <section class="tech-panel overflow-hidden">
          <div class="flex flex-col gap-2 border-b border-white/10 px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-5">
            <div><h2 class="text-sm font-semibold text-white">{{ t('linkCards.usage') }}</h2><p class="mt-1 text-xs text-zinc-500">{{ profile.card.masked_key }} · {{ profile.card.group_name }}</p></div>
            <button type="button" class="tech-icon-button" :title="t('common.refresh')" :disabled="usageLoading" @click="loadUsage"><Icon name="refresh" size="sm" :class="usageLoading ? 'animate-spin motion-reduce:animate-none' : ''" /></button>
          </div>

          <div class="overflow-x-auto">
            <table class="w-full min-w-[980px]">
              <thead><tr><th>时间</th><th>{{ t('linkCards.model') }}</th><th>{{ t('linkCards.requestType') }}</th><th>{{ t('linkCards.tokens') }}</th><th>缓存</th><th>{{ t('linkCards.cost') }}</th><th>{{ t('linkCards.latency') }}</th><th>{{ t('linkCards.requestId') }}</th></tr></thead>
              <tbody>
                <tr v-for="row in usageRows" :key="row.request_id">
                  <td>{{ formatDate(row.created_at) }}</td>
                  <td><span class="block max-w-[170px] truncate text-zinc-200" :title="row.model">{{ row.model }}</span></td>
                  <td><span class="tech-tag">{{ requestType(row) }}</span></td>
                  <td>
                    <div class="detail-tip" tabindex="0">
                      <span class="cursor-help tabular-nums text-zinc-200">{{ totalTokens(row).toLocaleString() }}</span>
                      <div class="detail-panel">
                        <p><span>输入</span><strong>{{ row.input_tokens.toLocaleString() }}</strong></p>
                        <p><span>输出</span><strong>{{ row.output_tokens.toLocaleString() }}</strong></p>
                        <p v-if="row.cache_creation_5m_tokens > 0"><span>缓存写入 5m</span><strong>{{ row.cache_creation_5m_tokens.toLocaleString() }}</strong></p>
                        <p v-if="row.cache_creation_1h_tokens > 0"><span>缓存写入 1h</span><strong>{{ row.cache_creation_1h_tokens.toLocaleString() }}</strong></p>
                        <p v-if="row.cache_creation_5m_tokens === 0 && row.cache_creation_1h_tokens === 0"><span>缓存写入</span><strong>{{ row.cache_creation_tokens.toLocaleString() }}</strong></p>
                        <p><span>缓存读取</span><strong>{{ row.cache_read_tokens.toLocaleString() }}</strong></p>
                        <p v-if="row.image_input_tokens > 0"><span>图像输入</span><strong>{{ row.image_input_tokens.toLocaleString() }}</strong></p>
                        <p v-if="row.image_output_tokens > 0"><span>图像输出</span><strong>{{ row.image_output_tokens.toLocaleString() }}</strong></p>
                        <p class="detail-total"><span>Token 合计</span><strong>{{ totalTokens(row).toLocaleString() }}</strong></p>
                      </div>
                    </div>
                  </td>
                  <td><span :class="row.cache_read_tokens > 0 ? 'text-emerald-300' : 'text-zinc-500'">{{ row.cache_read_tokens > 0 ? `命中 ${row.cache_read_tokens.toLocaleString()}` : '未命中' }}</span></td>
                  <td>
                    <div class="detail-tip" tabindex="0">
                      <span class="cursor-help tabular-nums text-cyan-300">{{ money(row.total_cost) }}</span>
                      <div class="detail-panel">
                        <p><span>输入费用</span><strong>{{ money(row.input_cost) }}</strong></p>
                        <p><span>输出费用</span><strong>{{ money(row.output_cost) }}</strong></p>
                        <p><span>缓存写入</span><strong>{{ money(row.cache_creation_cost) }}</strong></p>
                        <p><span>缓存读取</span><strong>{{ money(row.cache_read_cost) }}</strong></p>
                        <p v-if="Number(row.image_input_cost) > 0"><span>图像输入费用</span><strong>{{ money(row.image_input_cost) }}</strong></p>
                        <p v-if="Number(row.image_output_cost) > 0"><span>图像输出费用</span><strong>{{ money(row.image_output_cost) }}</strong></p>
                        <p class="detail-total"><span>原始费用</span><strong>{{ money(row.total_cost) }}</strong></p>
                        <p><span>实扣费用</span><strong>{{ money(row.actual_cost) }}</strong></p>
                        <p><span>计费倍率</span><strong>{{ row.rate_multiplier }}x</strong></p>
                        <p v-if="row.service_tier"><span>服务等级</span><strong>{{ row.service_tier }}</strong></p>
                      </div>
                    </div>
                  </td>
                  <td><span class="tabular-nums text-zinc-300">{{ duration(row.first_token_ms) }} / {{ duration(row.duration_ms) }}</span></td>
                  <td><code class="block max-w-[160px] truncate text-xs text-zinc-500" :title="row.request_id">{{ row.request_id }}</code></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="!usageLoading && usageRows.length === 0" class="px-5 py-12 text-center text-sm text-zinc-500">{{ t('linkCards.noUsage') }}</div>
          <div v-if="usageTotal > 0" class="flex items-center justify-between border-t border-white/10 px-4 py-3 text-sm sm:px-5">
            <span class="text-zinc-500">{{ usageTotal.toLocaleString() }} 条</span>
            <div class="flex items-center gap-2"><button class="tech-page-button" :disabled="usagePage <= 1" @click="changePage(usagePage - 1)"><Icon name="chevronLeft" size="sm" /></button><span class="min-w-20 text-center tabular-nums text-zinc-400">{{ usagePage }} / {{ totalPages }}</span><button class="tech-page-button" :disabled="usagePage >= totalPages" @click="changePage(usagePage + 1)"><Icon name="chevronRight" size="sm" /></button></div>
          </div>
        </section>

        <section class="tech-panel mt-5 overflow-hidden">
          <div class="border-b border-white/10 px-4 pt-4 sm:px-5">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
              <div><h2 class="text-sm font-semibold text-white">{{ t('linkCards.integration') }}</h2><p class="mt-1 text-xs text-zinc-500">{{ t('linkCards.streamRequired') }} · {{ apiBase }}</p></div>
              <div class="flex gap-1 overflow-x-auto" role="tablist">
                <button v-for="guide in guides" :key="guide.key" type="button" class="guide-tab" :class="activeGuide === guide.key ? 'guide-tab-active' : ''" @click="activeGuide = guide.key">{{ guide.label }}</button>
              </div>
            </div>
          </div>
          <div class="relative bg-[#090b0e] p-4 sm:p-5">
            <button type="button" class="absolute right-3 top-3 tech-icon-button" :title="t('linkCards.copy')" @click="copyGuide"><Icon name="copy" size="sm" /></button>
            <pre class="overflow-x-auto pr-10 text-xs leading-6 text-zinc-300 sm:text-sm"><code>{{ currentGuide.code }}</code></pre>
          </div>
        </section>
      </template>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { sanitizeUrl } from '@/utils/url'
import { publicLinkCardsAPI, type PublicLinkCardProfile, type PublicLinkCardUsageLog } from '@/api/linkCards'

type PortalState = 'activate' | 'loading' | 'details'
type GuideKey = 'codex' | 'claude' | 'openai'
const SESSION_KEY = 'link_card_portal_session'
const FALLBACK_API_BASE = 'https://api.52token.org/v1'

const { t } = useI18n()
const appStore = useAppStore()
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const portalState = ref<PortalState>('activate')
const keyInput = ref('')
const activating = ref(false)
const activationError = ref('')
const sessionToken = ref('')
const profile = ref<PublicLinkCardProfile | null>(null)
const usageRows = ref<PublicLinkCardUsageLog[]>([])
const usageLoading = ref(false)
const usagePage = ref(1)
const usageTotal = ref(0)
const activeGuide = ref<GuideKey>('codex')
const apiBase = computed(() => profile.value?.api_base_url?.replace(/\/$/, '') || FALLBACK_API_BASE)
const totalPages = computed(() => Math.max(1, Math.ceil(usageTotal.value / 10)))
const metrics = computed(() => profile.value ? [
  { key: 'balance', label: t('linkCards.availableBalance'), value: money(profile.value.card.remaining_quota), icon: 'creditCard' as const },
  { key: 'used', label: t('linkCards.usedQuota'), value: money(profile.value.card.used_quota), icon: 'chart' as const },
  { key: 'requests', label: t('linkCards.requestCount'), value: profile.value.card.request_count.toLocaleString(), icon: 'database' as const },
  { key: 'status', label: t('linkCards.cardStatus'), value: statusLabel(profile.value.card.status), icon: 'checkCircle' as const },
] : [])
const guides = computed(() => [
  { key: 'codex' as const, label: t('linkCards.codex'), code: `curl ${apiBase.value}/responses \\\n  -H "Authorization: Bearer CARD_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"MODEL","input":"Hello","stream":true}'` },
  { key: 'claude' as const, label: t('linkCards.claude'), code: `curl ${apiBase.value}/messages \\\n  -H "x-api-key: CARD_KEY" \\\n  -H "anthropic-version: 2023-06-01" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"MODEL","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}],"stream":true}'` },
  { key: 'openai' as const, label: t('linkCards.openaiCompatible'), code: `curl ${apiBase.value}/chat/completions \\\n  -H "Authorization: Bearer CARD_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"MODEL","messages":[{"role":"user","content":"Hello"}],"stream":true}'` },
])
const currentGuide = computed(() => guides.value.find((guide) => guide.key === activeGuide.value) || guides.value[0])

function isFullKey(value: string): boolean { return /^sk-[A-Za-z0-9_-]{20,}$/.test(value) }
function money(value: unknown): string { const number = Number(value); return `$${Number.isFinite(number) ? number.toFixed(6).replace(/0+$/, '').replace(/\.$/, '') : '0'}` }
function totalTokens(row: PublicLinkCardUsageLog): number { return row.total_tokens || row.input_tokens + row.output_tokens + row.cache_creation_tokens + row.cache_read_tokens }
function duration(value: number | null): string { if (value == null) return '-'; return value < 1000 ? `${value}ms` : `${(value / 1000).toFixed(2)}s` }
function requestType(row: PublicLinkCardUsageLog): string { if (row.request_type === 'ws_v2') return 'WebSocket'; if (row.request_type === 'stream' || row.stream) return 'Stream'; return row.request_type || 'Sync' }
function formatDate(value: string): string { return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value)) }
function statusLabel(status: string): string { if (status === 'active') return t('linkCards.active'); if (status === 'frozen') return t('linkCards.frozen'); if (status === 'depleted') return t('linkCards.exhausted'); return status }

async function activateCard(): Promise<void> {
  activationError.value = ''
  if (!isFullKey(keyInput.value)) { activationError.value = t('linkCards.invalidKey'); return }
  activating.value = true
  try {
    const result = await publicLinkCardsAPI.activate({ key: keyInput.value })
    sessionToken.value = result.session_token
    sessionStorage.setItem(SESSION_KEY, result.session_token)
    keyInput.value = ''
    await loadProfile()
  } catch { activationError.value = t('linkCards.activationFailed') } finally { activating.value = false }
}

async function loadProfile(): Promise<void> {
  if (!sessionToken.value) { portalState.value = 'activate'; return }
  portalState.value = 'loading'
  try { profile.value = await publicLinkCardsAPI.getMe(sessionToken.value); usagePage.value = 1; await loadUsage(); portalState.value = 'details' } catch { clearSession(); activationError.value = t('linkCards.sessionExpired') }
}

async function loadUsage(): Promise<void> {
  if (!sessionToken.value) return
  usageLoading.value = true
  try { const result = await publicLinkCardsAPI.listUsage(sessionToken.value, { page: usagePage.value, page_size: 10 }); usageRows.value = result.items || []; usageTotal.value = result.total } catch { clearSession(); activationError.value = t('linkCards.sessionExpired') } finally { usageLoading.value = false }
}

function changePage(page: number): void { if (page < 1 || page > totalPages.value) return; usagePage.value = page; void loadUsage() }
function clearSession(): void { sessionStorage.removeItem(SESSION_KEY); sessionToken.value = ''; profile.value = null; usageRows.value = []; usageTotal.value = 0; portalState.value = 'activate' }
async function copyGuide(): Promise<void> { try { await navigator.clipboard.writeText(currentGuide.value.code); appStore.showSuccess(t('linkCards.copied')) } catch { appStore.showError(t('common.copyFailed')) } }

onMounted(() => { const saved = sessionStorage.getItem(SESSION_KEY); if (saved) { sessionToken.value = saved; void loadProfile() } })
</script>

<style scoped>
.quota-portal { position: relative; overflow-x: hidden; background: #080a0d; color: #f4f4f5; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; }
.quota-grid { position: fixed; inset: 0; pointer-events: none; opacity: .18; background-image: linear-gradient(rgba(255,255,255,.055) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,.055) 1px, transparent 1px); background-size: 32px 32px; mask-image: linear-gradient(to bottom, black, transparent 82%); }
.activation-panel { border: 1px solid rgba(255,255,255,.1); border-radius: 8px; background: rgba(13,16,20,.94); padding: clamp(24px, 6vw, 48px); box-shadow: 0 28px 90px rgba(0,0,0,.42); }
.tech-input { min-height: 48px; border: 1px solid rgba(255,255,255,.14); border-radius: 6px; background: #0a0c0f; padding: 0 14px; color: white; outline: none; transition: border-color .18s, box-shadow .18s; }
.tech-input::placeholder { color: #52525b; }
.tech-input:focus { border-color: #67e8f9; box-shadow: 0 0 0 3px rgba(103,232,249,.12); }
.activate-button { border-radius: 6px; background: #d9f99d; color: #18230b; font-weight: 700; transition: transform .12s, background .18s; }
.activate-button:hover { background: #bef264; }
.activate-button:active { transform: translateY(1px); }
.activate-button:disabled { cursor: not-allowed; opacity: .55; }
.tech-panel { border: 1px solid rgba(255,255,255,.1); border-radius: 8px; background: rgba(15,18,22,.9); box-shadow: 0 18px 50px rgba(0,0,0,.2); }
.tech-button, .tech-icon-button, .tech-page-button { border: 1px solid rgba(255,255,255,.13); border-radius: 6px; background: rgba(255,255,255,.04); color: #d4d4d8; transition: background .16s, border-color .16s, color .16s; }
.tech-button { min-height: 38px; padding: 0 12px; font-size: 12px; font-weight: 600; }
.tech-icon-button, .tech-page-button { display: inline-flex; height: 34px; width: 34px; align-items: center; justify-content: center; }
.tech-button:hover, .tech-icon-button:hover, .tech-page-button:hover { border-color: rgba(103,232,249,.42); background: rgba(103,232,249,.08); color: #a5f3fc; }
.tech-button:disabled, .tech-icon-button:disabled, .tech-page-button:disabled { cursor: not-allowed; opacity: .35; }
table th { border-bottom: 1px solid rgba(255,255,255,.08); background: rgba(255,255,255,.025); padding: 11px 16px; color: #71717a; font-size: 11px; font-weight: 600; text-align: left; white-space: nowrap; }
table td { border-bottom: 1px solid rgba(255,255,255,.065); padding: 13px 16px; color: #a1a1aa; font-size: 12px; white-space: nowrap; }
table tbody tr:hover { background: rgba(255,255,255,.025); }
.tech-tag { display: inline-flex; border: 1px solid rgba(103,232,249,.18); border-radius: 4px; background: rgba(103,232,249,.07); padding: 2px 6px; color: #a5f3fc; font-size: 10px; }
.detail-tip { position: relative; display: inline-block; outline: none; }
.detail-panel { position: absolute; bottom: calc(100% + 8px); left: 50%; z-index: 30; display: none; min-width: 210px; transform: translateX(-50%); border: 1px solid rgba(255,255,255,.14); border-radius: 6px; background: #15181d; padding: 10px; box-shadow: 0 16px 45px rgba(0,0,0,.55); }
.detail-tip:hover .detail-panel, .detail-tip:focus .detail-panel { display: block; }
.detail-panel p { display: flex; justify-content: space-between; gap: 20px; padding: 3px 0; color: #a1a1aa; font-size: 11px; }
.detail-panel strong { color: #f4f4f5; font-weight: 600; font-variant-numeric: tabular-nums; }
.detail-panel .detail-total { margin-top: 5px; border-top: 1px solid rgba(255,255,255,.1); padding-top: 7px; }
.guide-tab { min-height: 38px; border-bottom: 2px solid transparent; padding: 0 12px; color: #71717a; font-size: 12px; font-weight: 600; white-space: nowrap; transition: color .16s, border-color .16s; }
.guide-tab:hover { color: #d4d4d8; }
.guide-tab-active { border-color: #a3e635; color: #d9f99d; }
@media (max-width: 640px) { .activation-panel { padding: 24px 18px; } .quota-grid { background-size: 24px 24px; } }
@media (prefers-reduced-motion: reduce) { .activate-button, .tech-button, .tech-icon-button, .tech-page-button, .guide-tab { transition: none; } }
</style>

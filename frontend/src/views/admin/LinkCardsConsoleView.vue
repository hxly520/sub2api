<template>
  <AppLayout>
    <div class="mx-auto max-w-[1600px] space-y-5" data-testid="admin-link-cards-view">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('linkCards.adminTitle') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.adminSubtitle') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2 self-start sm:self-auto" :disabled="loading" @click="loadAll">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin motion-reduce:animate-none' : ''" />
          {{ t('common.refresh') }}
        </button>
      </header>

      <nav class="flex gap-1 overflow-x-auto border-b border-gray-200 dark:border-dark-700" aria-label="Link card admin sections">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          class="-mb-px inline-flex min-h-11 shrink-0 items-center gap-2 border-b-2 px-4 text-sm font-medium transition-colors"
          :class="activeTab === tab.key ? 'border-primary-500 text-primary-600 dark:text-primary-400' : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="activeTab = tab.key"
        >
          <Icon :name="tab.icon" size="sm" />{{ tab.label }}
        </button>
      </nav>

      <div v-if="loading && !settings" class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4" role="status">
        <div v-for="index in 4" :key="index" class="card h-28 animate-pulse bg-gray-100 motion-reduce:animate-none dark:bg-dark-800" />
      </div>
      <div v-else-if="loadFailed" class="card flex min-h-64 flex-col items-center justify-center gap-4 p-8 text-center">
        <Icon name="exclamationTriangle" size="xl" class="text-amber-500" />
        <button type="button" class="btn btn-secondary" @click="loadAll">{{ t('linkCards.retry') }}</button>
      </div>

      <template v-else>
        <section v-show="activeTab === 'overview'" class="space-y-5">
          <div class="grid grid-cols-2 gap-3 xl:grid-cols-4">
            <div v-for="metric in metrics" :key="metric.key" class="card min-h-28 p-4 sm:p-5">
              <div class="flex items-start justify-between gap-3">
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</p>
                <Icon :name="metric.icon" size="sm" class="text-gray-400 dark:text-gray-500" />
              </div>
              <p class="mt-4 text-xl font-semibold tabular-nums text-gray-900 dark:text-white sm:text-2xl">{{ metric.value }}</p>
            </div>
          </div>

          <form v-if="settings" class="card overflow-hidden" @submit.prevent="saveSettings">
            <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-6">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('linkCards.adminTitle') }}</h2>
            </div>
            <div class="divide-y divide-gray-100 dark:divide-dark-700">
              <div class="flex flex-col gap-3 px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6">
                <div class="max-w-2xl"><p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('linkCards.featureSwitch') }}</p><p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('linkCards.featureHint') }}</p></div>
                <Toggle v-model="settingsForm.enabled" data-testid="link-card-feature-toggle" />
              </div>
              <div class="flex flex-col gap-3 px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6">
                <div class="max-w-2xl"><p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('linkCards.previewOnly') }}</p><p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('linkCards.previewHint') }}</p></div>
                <Toggle v-model="settingsForm.development_mode" data-testid="link-card-preview-toggle" />
              </div>
              <div class="grid gap-4 px-5 py-5 sm:grid-cols-2 sm:px-6 lg:grid-cols-3">
                <label class="sm:col-span-2 lg:col-span-3"><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.developmentUsers') }}</span><input v-model="settingsForm.development_user_ids" class="input" data-testid="link-card-development-users" inputmode="numeric" :disabled="!settingsForm.development_mode" :placeholder="t('linkCards.developmentUsersPlaceholder')" /><span class="mt-1.5 block text-xs text-gray-500 dark:text-gray-400">{{ t('linkCards.developmentUsersHint') }}</span></label>
                <label><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.defaultConcurrency') }}</span><input v-model.number="settingsForm.default_concurrency" type="number" min="1" max="100" class="input" /></label>
                <label><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.defaultRPM') }}</span><input v-model.number="settingsForm.default_rpm_limit" type="number" min="0" class="input" /></label>
                <label><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.batchLimit') }}</span><input v-model.number="settingsForm.max_batch_size" type="number" min="1" max="1000" class="input" /></label>
                <label><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.minimumDeposit') }}</span><input v-model.trim="settingsForm.minimum_deposit" class="input" inputmode="decimal" :placeholder="t('linkCards.noMinimumDeposit')" /></label>
                <label><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.sessionTTL') }}</span><input v-model.number="settingsForm.public_session_ttl_seconds" type="number" min="300" max="86400" class="input" /></label>
                <label class="sm:col-span-2 lg:col-span-3"><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.publicPortalURL') }}</span><input v-model.trim="settingsForm.public_portal_url" type="url" class="input" autocomplete="url" /></label>
                <label class="sm:col-span-2 lg:col-span-3"><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.apiBaseURL') }}</span><input v-model.trim="settingsForm.api_base_url" type="url" class="input" autocomplete="url" /></label>
              </div>
            </div>
            <div class="flex justify-end border-t border-gray-100 bg-gray-50 px-5 py-4 dark:border-dark-700 dark:bg-dark-800 sm:px-6"><button type="submit" class="btn btn-primary" :disabled="savingSettings">{{ t('linkCards.saveSettings') }}</button></div>
          </form>
        </section>

        <section v-show="activeTab === 'usage'" class="card overflow-hidden">
          <div class="grid gap-3 border-b border-gray-100 p-4 dark:border-dark-700 md:grid-cols-2">
            <SearchInput v-model="usageKeySearch" :placeholder="t('linkCards.searchKey')" @search="reloadUsage" />
            <SearchInput v-model="usageOwnerSearch" placeholder="搜索创建用户邮箱" @search="reloadUsage" />
          </div>
          <LinkCardUsageTable :rows="usageRows" :loading="usageLoading" show-owner />
          <div v-if="!usageLoading && usageRows.length === 0" class="p-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.noUsage') }}</div>
          <Pagination v-if="usagePage.total > 0" :page="usagePage.page" :page-size="10" :total="usagePage.total" :show-page-size-selector="false" @update:page="changeUsagePage" />
        </section>

        <section v-show="activeTab === 'groups'" class="space-y-4">
          <div class="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end">
            <label class="min-w-0 flex-1"><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.authorizeGroup') }}</span><select v-model="authorizeGroupId" class="input" data-testid="authorize-group-select"><option :value="null" disabled>{{ t('linkCards.selectGroup') }}</option><option v-for="group in availableToAuthorize" :key="group.id" :value="group.id">{{ group.name }} · {{ group.rate_multiplier }}x · {{ group.platform }}</option></select></label>
            <button type="button" class="btn btn-primary inline-flex shrink-0 items-center justify-center gap-2" :disabled="!authorizeGroupId || authorizing" @click="authorizeGroup"><Icon name="plus" size="sm" />{{ t('linkCards.authorizeGroup') }}</button>
          </div>

          <div class="card overflow-hidden">
            <div class="divide-y divide-gray-100 dark:divide-dark-700">
              <div v-for="group in authorizedGroups" :key="group.group_id" class="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
                <div class="min-w-0"><div class="flex flex-wrap items-center gap-2"><p class="font-medium text-gray-900 dark:text-white">{{ group.name }}</p><span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ group.rate_multiplier }}x</span></div><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ group.platform || '-' }} · {{ t('linkCards.concurrency', { value: group.default_concurrency }) }}</p></div>
                <button type="button" class="btn btn-ghost btn-sm self-start text-red-600 dark:text-red-400 sm:self-auto" @click="removeAuthorization(group)">{{ t('linkCards.removeAuthorization') }}</button>
              </div>
              <div v-if="authorizedGroups.length === 0" class="p-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.noGroups') }}</div>
            </div>
          </div>
        </section>

        <section v-show="activeTab === 'cards'" class="card overflow-hidden">
          <div class="grid gap-3 border-b border-gray-100 p-4 dark:border-dark-700 sm:grid-cols-[minmax(0,1fr)_200px]">
            <SearchInput v-model="cardSearch" :placeholder="t('linkCards.searchKey')" @search="reloadCards" />
            <select v-model="cardStatus" class="input" @change="reloadCards"><option value="">{{ t('linkCards.allStatus') }}</option><option v-for="status in statusOptions" :key="status" :value="status">{{ statusLabel(status) }}</option></select>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full min-w-[1200px] divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800"><tr><th v-for="head in cardHeaders" :key="head" class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ head }}</th></tr></thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
                <tr v-for="card in cards" :key="card.id" class="hover:bg-gray-50 dark:hover:bg-dark-800">
                  <td class="px-4 py-3"><div class="flex max-w-[300px] items-center gap-2"><code class="truncate text-xs text-gray-700 dark:text-gray-300" :title="card.key">{{ card.key }}</code><button class="icon-button" @click="copyKey(card.key)"><Icon name="copy" size="sm" /></button></div></td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-300"><span class="block">{{ card.creator_email || '-' }}</span><span class="text-xs text-gray-400">#{{ card.creator_user_id }}</span></td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">{{ card.group_name }}</td>
                  <td class="px-4 py-3 text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ money(card.remaining_quota) }}</td>
                  <td class="px-4 py-3 text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ money(card.used_quota) }}</td>
                  <td class="px-4 py-3"><span class="status-badge" :class="statusClass(card.status)">{{ statusLabel(card.status) }}</span></td>
                  <td class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{{ formatDate(card.created_at) }}</td>
                  <td class="px-4 py-3"><button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1.5" @click="openAction(card)"><Icon name="cog" size="sm" />{{ t('linkCards.adminAction') }}</button></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="!cardsLoading && cards.length === 0" class="p-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.noCards') }}</div>
          <Pagination v-if="cardPage.total > 0" :page="cardPage.page" :page-size="10" :total="cardPage.total" :show-page-size-selector="false" @update:page="changeCardPage" />
        </section>
      </template>
    </div>

    <BaseDialog :show="!!actionTarget" :title="t('linkCards.adminAction')" width="normal" @close="closeAction">
      <div v-if="actionTarget" class="space-y-4">
        <div class="rounded-md bg-gray-50 p-3 text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300"><code class="break-all">{{ actionTarget.key }}</code></div>
        <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
          <button v-for="action in actionOptions" :key="action.value" type="button" class="btn btn-secondary btn-sm" :class="actionForm.action === action.value ? 'ring-2 ring-primary-500' : ''" @click="actionForm.action = action.value">{{ action.label }}</button>
        </div>
        <label v-if="actionForm.action === 'recharge'"><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.actionAmount') }}</span><input v-model="actionForm.amount" class="input" inputmode="decimal" /></label>
        <div v-if="actionForm.action === 'set_limits'" class="grid gap-3 sm:grid-cols-2"><label><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.defaultConcurrency') }}</span><input v-model.number="actionForm.concurrency" type="number" min="1" class="input" /></label><label><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">RPM</span><input v-model.number="actionForm.rpm_limit" type="number" min="0" class="input" /></label></div>
        <label><span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.actionReason') }}</span><textarea v-model="actionForm.reason" class="input min-h-20 resize-y" maxlength="500" /></label>
      </div>
      <template #footer><div class="flex justify-end gap-3"><button class="btn btn-secondary" @click="closeAction">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="runningAction || !canRunAction" @click="runAction">{{ t('common.confirm') }}</button></div></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import LinkCardUsageTable, { type LinkCardUsageRow } from '@/components/link-cards/LinkCardUsageTable.vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import {
  adminLinkCardsAPI,
  type AdminLinkCardActionRequest,
  type AdminLinkCardSummary,
  type AdminLinkCardSettings,
  type LinkCard,
  type LinkCardAdminAction,
  type LinkCardGroup,
  type LinkCardStatus,
  type LinkCardUsageLog,
} from '@/api/linkCards'
import type { AdminGroup } from '@/types'

type AdminTab = 'overview' | 'usage' | 'groups' | 'cards'
type AdminSummary = AdminLinkCardSummary

const { t } = useI18n()
const appStore = useAppStore()
const activeTab = ref<AdminTab>('overview')
const loading = ref(false)
const loadFailed = ref(false)
const settings = ref<AdminLinkCardSettings | null>(null)
const settingsForm = reactive({
  enabled: false,
  development_mode: true,
  development_user_ids: '1',
  default_concurrency: 5,
  default_rpm_limit: 0,
  max_batch_size: 100,
  minimum_deposit: '',
  public_portal_url: 'https://key.52token.org',
  api_base_url: 'https://api.52token.org/v1',
  public_session_ttl_seconds: 3600,
})
const savingSettings = ref(false)
const summary = reactive<AdminSummary>({ total_cards: 0, active_cards: 0, total_reserved: 0, total_consumed: 0 })
const authorizedGroups = ref<LinkCardGroup[]>([])
const allGroups = ref<AdminGroup[]>([])
const authorizeGroupId = ref<number | null>(null)
const authorizing = ref(false)
const cards = ref<LinkCard[]>([])
const cardsLoading = ref(false)
const cardSearch = ref('')
const cardStatus = ref<LinkCardStatus | ''>('')
const cardPage = reactive({ page: 1, total: 0 })
const usageRows = ref<LinkCardUsageRow[]>([])
const usageLoading = ref(false)
const usageKeySearch = ref('')
const usageOwnerSearch = ref('')
const usagePage = reactive({ page: 1, total: 0 })
const actionTarget = ref<LinkCard | null>(null)
const runningAction = ref(false)
const actionForm = reactive<{ action: LinkCardAdminAction; amount: string; concurrency: number; rpm_limit: number; reason: string }>({ action: 'freeze', amount: '', concurrency: 5, rpm_limit: 0, reason: '' })

const tabs = computed(() => [
  { key: 'overview' as const, label: t('linkCards.overview'), icon: 'grid' as const },
  { key: 'usage' as const, label: t('linkCards.usage'), icon: 'chart' as const },
  { key: 'groups' as const, label: t('linkCards.groups'), icon: 'database' as const },
  { key: 'cards' as const, label: t('linkCards.cards'), icon: 'key' as const },
])
const metrics = computed(() => [
  { key: 'total', label: t('linkCards.totalCards'), value: summary.total_cards.toLocaleString(), icon: 'key' as const },
  { key: 'active', label: t('linkCards.activeCards'), value: summary.active_cards == null ? '-' : summary.active_cards.toLocaleString(), icon: 'checkCircle' as const },
  { key: 'reserved', label: t('linkCards.totalReserved'), value: summary.total_reserved == null ? '-' : money(summary.total_reserved), icon: 'creditCard' as const },
  { key: 'used', label: t('linkCards.totalConsumed'), value: summary.total_consumed == null ? '-' : money(summary.total_consumed), icon: 'chart' as const },
])
const availableToAuthorize = computed(() => {
  const ids = new Set(authorizedGroups.value.map((group) => group.group_id))
  return allGroups.value.filter((group) => group.status === 'active' && group.subscription_type === 'standard' && group.rate_multiplier > 0 && !ids.has(group.id))
})
const statusOptions: LinkCardStatus[] = ['pending_activation', 'active', 'frozen', 'depleted', 'refunded', 'revoked']
const cardHeaders = computed(() => [t('linkCards.fullKey'), t('linkCards.owner'), t('admin.usage.group'), t('linkCards.remainingQuota'), t('linkCards.usedQuota'), t('linkCards.status'), t('linkCards.createdAt'), t('linkCards.actions')])
const actionOptions = computed(() => [
  { value: 'recharge' as const, label: t('linkCards.recharge') }, { value: 'refund' as const, label: t('linkCards.adminRefund') }, { value: 'freeze' as const, label: t('linkCards.freeze') }, { value: 'unfreeze' as const, label: t('linkCards.unfreeze') }, { value: 'set_limits' as const, label: t('linkCards.limit') }, { value: 'revoke' as const, label: t('linkCards.destroy') },
])
const canRunAction = computed(() => Boolean(actionTarget.value) && actionForm.reason.trim().length >= 2 && (actionForm.action !== 'recharge' || finitePositive(actionForm.amount) > 0) && (actionForm.action !== 'set_limits' || actionForm.concurrency >= 1))

const operationKeys = new Map<string, { fingerprint: string; key: string }>()

function idempotencyKey(scope: string, fingerprint: string): string {
  const existing = operationKeys.get(scope)
  if (existing?.fingerprint === fingerprint) return existing.key
  const key = typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  operationKeys.set(scope, { fingerprint, key })
  return key
}

function clearIdempotencyKey(scope: string, fingerprint: string): void {
  if (operationKeys.get(scope)?.fingerprint === fingerprint) operationKeys.delete(scope)
}
function finitePositive(value: unknown): number { const number = Number(value); return Number.isFinite(number) && number > 0 ? number : 0 }
function parseDevelopmentUserIDs(value: string): number[] {
  return [...new Set(value.split(/[\s,;]+/).map(Number).filter((id) => Number.isSafeInteger(id) && id > 0))]
}
function money(value: unknown): string { const number = Number(value); return `$${Number.isFinite(number) ? number.toFixed(4).replace(/0+$/, '').replace(/\.$/, '') : '0'}` }
function formatDate(value: string): string { return new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)) }
function statusLabel(status: LinkCardStatus): string { return ({ pending_activation: t('linkCards.pending'), active: t('linkCards.active'), frozen: t('linkCards.frozen'), depleted: t('linkCards.exhausted'), refunded: t('linkCards.revoked'), revoked: t('linkCards.revoked') })[status] }
function statusClass(status: LinkCardStatus): string { if (status === 'active') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'; if (status === 'pending_activation') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'; if (status === 'frozen') return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'; return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' }
function toUsageRow(row: LinkCardUsageLog): LinkCardUsageRow { return { ...row, user_id: row.creator_user_id, user_email: row.creator_email, card_id: row.link_card_id, card_key: row.masked_key || row.key_prefix, group_name: row.group_name || undefined, input_cost: Number(row.input_cost), output_cost: Number(row.output_cost), cache_creation_cost: Number(row.cache_creation_cost), cache_read_cost: Number(row.cache_read_cost), image_input_cost: Number(row.image_input_cost), image_output_cost: Number(row.image_output_cost), total_cost: Number(row.total_cost), actual_cost: Number(row.actual_cost) } }

async function loadAll(): Promise<void> {
  loading.value = true; loadFailed.value = false
  try {
    const [nextSettings, nextAuthorized, nextAllGroups] = await Promise.all([adminLinkCardsAPI.getSettings(), adminLinkCardsAPI.listGroups(), adminAPI.groups.getAll()])
    settings.value = nextSettings
    Object.assign(settingsForm, {
      enabled: nextSettings.enabled,
      development_mode: nextSettings.development_mode,
      development_user_ids: (nextSettings.development_user_ids || [1]).join(', '),
      default_concurrency: nextSettings.default_concurrency,
      default_rpm_limit: nextSettings.default_rpm_limit ?? 0,
      max_batch_size: nextSettings.max_batch_size,
      minimum_deposit: nextSettings.minimum_deposit == null ? '' : String(nextSettings.minimum_deposit),
      public_portal_url: nextSettings.public_portal_url,
      api_base_url: nextSettings.api_base_url,
      public_session_ttl_seconds: nextSettings.public_session_ttl_seconds,
    })
    authorizedGroups.value = nextAuthorized.filter((group) => group.enabled); allGroups.value = nextAllGroups
    await Promise.all([loadCards(), loadUsage()])
  } catch { loadFailed.value = true } finally { loading.value = false }
}

async function loadCards(): Promise<void> {
  cardsLoading.value = true
  try {
    const result = await adminLinkCardsAPI.listCards({ page: cardPage.page, page_size: 10, search: cardSearch.value.trim() || undefined, status: cardStatus.value || undefined, sort_by: 'created_at', sort_order: 'desc' })
    cards.value = result.items || []; cardPage.total = result.total
    if (result.summary) Object.assign(summary, result.summary)
  } catch { cards.value = []; cardPage.total = 0 } finally { cardsLoading.value = false }
}

async function loadUsage(): Promise<void> {
  usageLoading.value = true
  try { const result = await adminLinkCardsAPI.listUsage({ page: usagePage.page, page_size: 10, key: usageKeySearch.value.trim() || undefined, creator_email: usageOwnerSearch.value.trim() || undefined, sort_by: 'created_at', sort_order: 'desc' }); usageRows.value = (result.items || []).map(toUsageRow); usagePage.total = result.total } catch { usageRows.value = []; usagePage.total = 0 } finally { usageLoading.value = false }
}

async function saveSettings(): Promise<void> {
  if (savingSettings.value) return; savingSettings.value = true
  const developmentUserIDs = parseDevelopmentUserIDs(settingsForm.development_user_ids)
  const minimumDeposit = settingsForm.minimum_deposit.trim()
  try {
    const saved = await adminLinkCardsAPI.updateSettings({
      enabled: settingsForm.enabled,
      development_mode: settingsForm.development_mode,
      development_user_ids: developmentUserIDs,
      default_concurrency: settingsForm.default_concurrency,
      default_rpm_limit: settingsForm.default_rpm_limit,
      max_batch_size: settingsForm.max_batch_size,
      ...(minimumDeposit ? { minimum_deposit: minimumDeposit } : { clear_minimum_deposit: true }),
      public_portal_url: settingsForm.public_portal_url.trim(),
      api_base_url: settingsForm.api_base_url.trim(),
      public_session_ttl_seconds: settingsForm.public_session_ttl_seconds,
    })
    settings.value = saved
    appStore.showSuccess(t('linkCards.settingsSaved'))
  } catch { appStore.showError(t('linkCards.settingsFailed')) } finally { savingSettings.value = false }
}

async function authorizeGroup(): Promise<void> { if (!authorizeGroupId.value || authorizing.value) return; authorizing.value = true; try { await adminLinkCardsAPI.authorizeGroup({ group_id: authorizeGroupId.value, default_concurrency: settingsForm.default_concurrency }); authorizedGroups.value = (await adminLinkCardsAPI.listGroups()).filter((group) => group.enabled); authorizeGroupId.value = null; appStore.showSuccess(t('linkCards.groupAuthorized')) } catch { appStore.showError(t('linkCards.actionFailed')) } finally { authorizing.value = false } }
async function removeAuthorization(group: LinkCardGroup): Promise<void> { try { await adminLinkCardsAPI.removeGroup(group.group_id); authorizedGroups.value = authorizedGroups.value.filter((item) => item.group_id !== group.group_id); appStore.showSuccess(t('linkCards.groupRemoved')) } catch { appStore.showError(t('linkCards.actionFailed')) } }
function reloadCards(): void { cardPage.page = 1; void loadCards() }
function reloadUsage(): void { usagePage.page = 1; void loadUsage() }
function changeCardPage(page: number): void { cardPage.page = page; void loadCards() }
function changeUsagePage(page: number): void { usagePage.page = page; void loadUsage() }
async function copyKey(key: string): Promise<void> { try { await navigator.clipboard.writeText(key); appStore.showSuccess(t('linkCards.copied')) } catch { appStore.showError(t('common.copyFailed')) } }
function openAction(card: LinkCard): void { actionTarget.value = card; Object.assign(actionForm, { action: card.status === 'frozen' ? 'unfreeze' : 'freeze', amount: '', concurrency: card.concurrency || settingsForm.default_concurrency, rpm_limit: card.rpm_limit || 0, reason: '' }) }
function closeAction(): void { if (!runningAction.value) actionTarget.value = null }
async function runAction(): Promise<void> { const target = actionTarget.value; if (!target || !canRunAction.value || runningAction.value) return; runningAction.value = true; const request: AdminLinkCardActionRequest = { action: actionForm.action, reason: actionForm.reason.trim(), ...(actionForm.action === 'recharge' ? { amount: finitePositive(actionForm.amount) } : {}), ...(actionForm.action === 'set_limits' ? { concurrency: actionForm.concurrency, rpm_limit: actionForm.rpm_limit } : {}) }; const fingerprint = JSON.stringify({ card_id: target.id, ...request }); const key = idempotencyKey('admin-action', fingerprint); try { await adminLinkCardsAPI.runCardAction(target.id, request, key); clearIdempotencyKey('admin-action', fingerprint); actionTarget.value = null; appStore.showSuccess(t('linkCards.actionSuccess')); await loadCards() } catch { appStore.showError(t('linkCards.actionFailed')) } finally { runningAction.value = false } }

watch(activeTab, (tab) => { if (tab === 'cards') void loadCards(); if (tab === 'usage') void loadUsage() })
onMounted(() => { void loadAll() })
</script>

<style scoped>
.icon-button { @apply inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-100; }
.status-badge { @apply inline-flex whitespace-nowrap rounded px-2 py-1 text-xs font-medium; }
</style>

<template>
  <AppLayout>
    <div class="mx-auto max-w-[1500px] space-y-5" data-testid="link-cards-view">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('linkCards.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.subtitle') }}</p>
        </div>
        <div class="text-left sm:text-right">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('linkCards.balance') }}</p>
          <p class="mt-0.5 text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ money(userBalance) }}</p>
        </div>
      </header>

      <nav class="flex gap-1 overflow-x-auto border-b border-gray-200 dark:border-dark-700" aria-label="Link card sections">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          class="-mb-px inline-flex min-h-11 shrink-0 items-center gap-2 border-b-2 px-4 text-sm font-medium transition-colors"
          :class="activeTab === tab.key
            ? 'border-primary-500 text-primary-600 dark:text-primary-400'
            : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="activeTab = tab.key"
        >
          <Icon :name="tab.icon" size="sm" />
          {{ tab.label }}
        </button>
      </nav>

      <section v-if="initialLoading" class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]" role="status">
        <div class="card h-[420px] animate-pulse bg-gray-100 motion-reduce:animate-none dark:bg-dark-800" />
        <div class="card h-[250px] animate-pulse bg-gray-100 motion-reduce:animate-none dark:bg-dark-800" />
      </section>

      <section v-else-if="loadError" class="card flex min-h-64 flex-col items-center justify-center gap-4 p-8 text-center">
        <Icon name="exclamationTriangle" size="xl" class="text-amber-500" />
        <p class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('linkCards.accessDenied') }}</p>
        <button type="button" class="btn btn-secondary" @click="loadInitial">{{ t('linkCards.retry') }}</button>
      </section>

      <template v-else>
        <section v-show="activeTab === 'create'" class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
          <form class="card overflow-hidden" @submit.prevent="submitCreate">
            <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-6">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('linkCards.selectGroup') }}</h2>
            </div>

            <div v-if="groups.length" class="grid grid-cols-1 divide-y divide-gray-100 dark:divide-dark-700 sm:grid-cols-2 sm:divide-x sm:divide-y-0 lg:grid-cols-3 xl:grid-cols-4">
              <label
                v-for="group in groups"
                :key="group.group_id"
                class="relative flex min-h-[116px] cursor-pointer flex-col justify-between gap-3 border-b border-gray-100 p-4 transition-colors last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800"
                :class="selectedGroupId === group.group_id ? 'bg-primary-50/70 ring-1 ring-inset ring-primary-400 dark:bg-primary-900/20' : ''"
              >
                <input v-model="selectedGroupId" type="radio" name="link-card-group" :value="group.group_id" class="sr-only" />
                <div class="flex items-start justify-between gap-3">
                  <span class="line-clamp-2 text-sm font-semibold text-gray-900 dark:text-white">{{ group.name }}</span>
                  <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium tabular-nums text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ group.rate_multiplier }}x</span>
                </div>
                <div class="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                  <span>{{ group.platform || '-' }}</span>
                  <span>{{ t('linkCards.concurrency', { value: group.default_concurrency }) }}</span>
                </div>
              </label>
            </div>
            <div v-else class="flex min-h-52 items-center justify-center p-8 text-sm text-gray-500 dark:text-gray-400">
              {{ t('linkCards.noGroups') }}
            </div>

            <div class="grid gap-4 border-t border-gray-100 p-5 dark:border-dark-700 sm:grid-cols-2 sm:p-6">
              <label class="block">
                <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.deposit') }}</span>
                <div class="relative">
                  <span class="pointer-events-none absolute inset-y-0 left-3 flex items-center text-sm text-gray-400">$</span>
                  <input v-model="depositInput" class="input pl-7 tabular-nums" inputmode="decimal" min="0.01" step="0.01" required />
                </div>
              </label>
              <label class="block">
                <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.quantity') }}</span>
                <input v-model="quantityInput" class="input tabular-nums" type="number" min="1" :max="settings?.max_batch_size || 100" step="1" required />
              </label>
            </div>
          </form>

          <aside class="card h-fit overflow-hidden xl:sticky xl:top-6">
            <dl class="divide-y divide-gray-100 dark:divide-dark-700">
              <div class="flex items-center justify-between gap-4 px-5 py-4">
                <dt class="text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.deposit') }}</dt>
                <dd class="font-medium tabular-nums text-gray-900 dark:text-white">{{ money(validDeposit) }}</dd>
              </div>
              <div class="flex items-center justify-between gap-4 px-5 py-4">
                <dt class="text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.quantity') }}</dt>
                <dd class="font-medium tabular-nums text-gray-900 dark:text-white">{{ validQuantity }}</dd>
              </div>
              <div class="flex items-center justify-between gap-4 px-5 py-4">
                <dt class="text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.baseQuota') }}</dt>
                <dd class="font-medium tabular-nums text-gray-900 dark:text-white">{{ money(baseQuotaPerCard) }} / {{ t('linkCards.quantity') }}</dd>
              </div>
              <div class="flex items-center justify-between gap-4 bg-gray-50 px-5 py-5 dark:bg-dark-800">
                <dt class="font-medium text-gray-700 dark:text-gray-200">{{ t('linkCards.totalDebit') }}</dt>
                <dd class="text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ money(totalDebit) }}</dd>
              </div>
            </dl>
            <div class="p-4">
              <p v-if="totalDebit > userBalance" class="mb-3 text-xs font-medium text-red-600 dark:text-red-400">{{ t('linkCards.insufficientBalance') }}</p>
              <button
                type="button"
                class="btn btn-primary flex w-full items-center justify-center gap-2"
                :disabled="!canCreate || creating"
                @click="submitCreate"
              >
                <Icon name="plus" size="sm" />
                {{ creating ? t('linkCards.creating') : t('linkCards.createAction') }}
              </button>
            </div>
          </aside>
        </section>

        <section v-show="activeTab === 'cards'" class="card overflow-hidden">
          <div class="flex flex-col gap-3 border-b border-gray-100 p-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
            <SearchInput v-model="cardSearch" :placeholder="t('linkCards.searchKey')" class="sm:max-w-md" @search="reloadCards" />
            <select v-model="cardStatus" class="input sm:w-48" @change="reloadCards">
              <option value="">{{ t('linkCards.allStatus') }}</option>
              <option v-for="status in statusOptions" :key="status" :value="status">{{ statusLabel(status) }}</option>
            </select>
          </div>

          <div class="hidden overflow-x-auto md:block">
            <table class="w-full min-w-[1050px] divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th v-for="head in cardHeaders" :key="head" class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ head }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
                <tr v-for="card in cards" :key="card.id" class="hover:bg-gray-50 dark:hover:bg-dark-800">
                  <td class="px-4 py-3">
                    <div class="flex max-w-[330px] items-center gap-2">
                      <code class="truncate text-xs text-gray-700 dark:text-gray-300" :title="card.key">{{ card.key }}</code>
                      <button type="button" class="icon-button shrink-0" :title="t('linkCards.copy')" @click="copyKey(card.key)"><Icon name="copy" size="sm" /></button>
                    </div>
                  </td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">{{ card.group_name }}</td>
                  <td class="px-4 py-3 text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ money(card.remaining_quota) }}</td>
                  <td class="px-4 py-3 text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ money(card.used_quota) }}</td>
                  <td class="px-4 py-3"><span class="status-badge" :class="statusClass(card.status)">{{ statusLabel(card.status) }}</span></td>
                  <td class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{{ formatDate(card.created_at) }}</td>
                  <td class="px-4 py-3">
                    <div class="flex items-center gap-2">
                      <button type="button" class="btn btn-ghost btn-sm" @click="openRecharge(card)">{{ t('linkCards.recharge') }}</button>
                      <button v-if="canRefund(card)" type="button" class="btn btn-ghost btn-sm text-red-600 dark:text-red-400" @click="refundTarget = card">{{ t('linkCards.refund') }}</button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="divide-y divide-gray-100 dark:divide-dark-700 md:hidden">
            <article v-for="card in cards" :key="card.id" class="space-y-3 p-4">
              <div class="flex items-start justify-between gap-3">
                <code class="min-w-0 flex-1 truncate text-xs text-gray-700 dark:text-gray-300">{{ card.key }}</code>
                <button type="button" class="icon-button" @click="copyKey(card.key)"><Icon name="copy" size="sm" /></button>
              </div>
              <div class="grid grid-cols-2 gap-3 text-sm">
                <div><p class="text-xs text-gray-400">{{ t('admin.usage.group') }}</p><p class="mt-0.5 text-gray-800 dark:text-gray-200">{{ card.group_name }}</p></div>
                <div><p class="text-xs text-gray-400">{{ t('linkCards.remainingQuota') }}</p><p class="mt-0.5 tabular-nums text-gray-800 dark:text-gray-200">{{ money(card.remaining_quota) }}</p></div>
              </div>
              <div class="flex items-center justify-between gap-3">
                <span class="status-badge" :class="statusClass(card.status)">{{ statusLabel(card.status) }}</span>
                <div class="flex gap-2"><button class="btn btn-ghost btn-sm" @click="openRecharge(card)">{{ t('linkCards.recharge') }}</button><button v-if="canRefund(card)" class="btn btn-ghost btn-sm text-red-600" @click="refundTarget = card">{{ t('linkCards.refund') }}</button></div>
              </div>
            </article>
          </div>

          <div v-if="!cardsLoading && cards.length === 0" class="p-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.noCards') }}</div>
          <Pagination v-if="cardPage.total > 0" :page="cardPage.page" :page-size="10" :total="cardPage.total" :show-page-size-selector="false" @update:page="changeCardPage" />
        </section>

        <section v-show="activeTab === 'usage'" class="card overflow-hidden">
          <LinkCardUsageTable :rows="usageRows" :loading="usageLoading" />
          <div v-if="!usageLoading && usageRows.length === 0" class="p-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('linkCards.noUsage') }}</div>
          <Pagination v-if="usagePage.total > 0" :page="usagePage.page" :page-size="10" :total="usagePage.total" :show-page-size-selector="false" @update:page="changeUsagePage" />
        </section>
      </template>
    </div>

    <BaseDialog :show="!!rechargeTarget" :title="t('linkCards.recharge')" width="narrow" @close="closeRecharge">
      <label class="block">
        <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('linkCards.rechargeAmount') }}</span>
        <input v-model="rechargeInput" class="input" inputmode="decimal" min="0.01" step="0.01" autofocus />
      </label>
      <template #footer>
        <div class="flex justify-end gap-3"><button class="btn btn-secondary" @click="closeRecharge">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="recharging || parsedRecharge <= 0" @click="submitRecharge">{{ t('common.confirm') }}</button></div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="!!refundTarget"
      :title="t('linkCards.refundTitle')"
      :message="t('linkCards.refundMessage')"
      danger
      @confirm="submitRefund"
      @cancel="refundTarget = null"
    />
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
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LinkCardUsageTable, { type LinkCardUsageRow } from '@/components/link-cards/LinkCardUsageTable.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import {
  linkCardsAPI,
  type LinkCard,
  type LinkCardGroup,
  type LinkCardSettings,
  type LinkCardStatus,
  type LinkCardUsageLog,
} from '@/api/linkCards'

type TabKey = 'create' | 'cards' | 'usage'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const activeTab = ref<TabKey>('create')
const initialLoading = ref(true)
const loadError = ref(false)
const creating = ref(false)
const settings = ref<LinkCardSettings | null>(null)
const groups = ref<LinkCardGroup[]>([])
const selectedGroupId = ref<number | null>(null)
const depositInput = ref('10.00')
const quantityInput = ref('1')
const cards = ref<LinkCard[]>([])
const cardsLoading = ref(false)
const cardSearch = ref('')
const cardStatus = ref<LinkCardStatus | ''>('')
const cardPage = reactive({ page: 1, total: 0 })
const usageRows = ref<LinkCardUsageRow[]>([])
const usageLoading = ref(false)
const usagePage = reactive({ page: 1, total: 0 })
const rechargeTarget = ref<LinkCard | null>(null)
const rechargeInput = ref('10.00')
const recharging = ref(false)
const refundTarget = ref<LinkCard | null>(null)

const tabs = computed(() => [
  { key: 'create' as const, label: t('linkCards.create'), icon: 'plus' as const },
  { key: 'cards' as const, label: t('linkCards.myCards'), icon: 'key' as const },
  { key: 'usage' as const, label: t('linkCards.usage'), icon: 'chart' as const },
])
const statusOptions: LinkCardStatus[] = ['pending_activation', 'active', 'frozen', 'depleted', 'refunded', 'revoked']
const userBalance = computed(() => Number(authStore.user?.balance || 0))
const validDeposit = computed(() => finitePositive(depositInput.value))
const validQuantity = computed(() => Math.max(0, Math.floor(Number(quantityInput.value) || 0)))
const totalDebit = computed(() => validDeposit.value * validQuantity.value)
const selectedGroup = computed(() => groups.value.find((group) => group.group_id === selectedGroupId.value))
const baseQuotaPerCard = computed(() => selectedGroup.value && selectedGroup.value.rate_multiplier > 0 ? validDeposit.value / selectedGroup.value.rate_multiplier : 0)
const canCreate = computed(() => Boolean(selectedGroup.value) && validDeposit.value > 0 && validQuantity.value > 0 && validQuantity.value <= (settings.value?.max_batch_size || 100) && totalDebit.value <= userBalance.value)
const parsedRecharge = computed(() => finitePositive(rechargeInput.value))
const cardHeaders = computed(() => [t('linkCards.fullKey'), t('admin.usage.group'), t('linkCards.remainingQuota'), t('linkCards.usedQuota'), t('linkCards.status'), t('linkCards.createdAt'), t('linkCards.actions')])

function finitePositive(value: unknown): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

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

function money(value: unknown): string {
  const parsed = Number(value)
  return `$${Number.isFinite(parsed) ? parsed.toFixed(4).replace(/0+$/, '').replace(/\.$/, '') : '0'}`
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))
}

function statusLabel(status: LinkCardStatus): string {
  const map: Record<LinkCardStatus, string> = {
    pending_activation: t('linkCards.pending'), active: t('linkCards.active'), frozen: t('linkCards.frozen'), depleted: t('linkCards.exhausted'), refunded: t('linkCards.revoked'), revoked: t('linkCards.revoked'),
  }
  return map[status]
}

function statusClass(status: LinkCardStatus): string {
  if (status === 'active') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (status === 'pending_activation') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (status === 'frozen') return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function canRefund(card: LinkCard): boolean {
  return card.status === 'pending_activation' && Number(card.used_quota) === 0 && Number(card.in_flight_quota || 0) === 0
}

function toUsageRow(row: LinkCardUsageLog): LinkCardUsageRow {
  return {
    ...row,
    card_id: row.link_card_id,
    card_key: row.masked_key || row.key_prefix,
    group_name: row.group_name || undefined,
    input_cost: Number(row.input_cost), output_cost: Number(row.output_cost), cache_creation_cost: Number(row.cache_creation_cost), cache_read_cost: Number(row.cache_read_cost), image_input_cost: Number(row.image_input_cost), image_output_cost: Number(row.image_output_cost), total_cost: Number(row.total_cost), actual_cost: Number(row.actual_cost),
  }
}

async function loadInitial(): Promise<void> {
  initialLoading.value = true
  loadError.value = false
  try {
    const [access, nextSettings, nextGroups] = await Promise.all([linkCardsAPI.getAccess(), linkCardsAPI.getSettings(), linkCardsAPI.listGroups()])
    if (access.allowed !== true) throw new Error('LINK_CARDS_ACCESS_DENIED')
    settings.value = nextSettings
    groups.value = nextGroups.filter((group) => group.enabled)
    selectedGroupId.value = groups.value[0]?.group_id ?? null
    await Promise.all([loadCards(), loadUsage()])
  } catch {
    loadError.value = true
  } finally {
    initialLoading.value = false
  }
}

async function loadCards(): Promise<void> {
  cardsLoading.value = true
  try {
    const result = await linkCardsAPI.listCards({ page: cardPage.page, page_size: 10, search: cardSearch.value.trim() || undefined, status: cardStatus.value || undefined, sort_by: 'created_at', sort_order: 'desc' })
    cards.value = result.items || []
    cardPage.total = result.total
  } catch {
    cards.value = []
    cardPage.total = 0
  } finally {
    cardsLoading.value = false
  }
}

async function loadUsage(): Promise<void> {
  usageLoading.value = true
  try {
    const result = await linkCardsAPI.listUsage({ page: usagePage.page, page_size: 10, sort_by: 'created_at', sort_order: 'desc' })
    usageRows.value = (result.items || []).map(toUsageRow)
    usagePage.total = result.total
  } catch {
    usageRows.value = []
    usagePage.total = 0
  } finally {
    usageLoading.value = false
  }
}

function reloadCards(): void { cardPage.page = 1; void loadCards() }
function changeCardPage(page: number): void { cardPage.page = page; void loadCards() }
function changeUsagePage(page: number): void { usagePage.page = page; void loadUsage() }

async function submitCreate(): Promise<void> {
  if (!canCreate.value || !selectedGroupId.value || creating.value) return
  creating.value = true
  const request = { group_id: selectedGroupId.value, quantity: validQuantity.value, amount: validDeposit.value }
  const fingerprint = JSON.stringify(request)
  const key = idempotencyKey('create', fingerprint)
  try {
    const result = await linkCardsAPI.createCards(request, key)
    clearIdempotencyKey('create', fingerprint)
    if (authStore.user) authStore.user.balance = Number(result.remaining_user_balance)
    appStore.showSuccess(t('linkCards.createSuccess'))
    cardPage.page = 1
    await loadCards()
    activeTab.value = 'cards'
  } catch {
    appStore.showError(t('linkCards.createFailed'))
  } finally {
    creating.value = false
  }
}

async function copyKey(key: string): Promise<void> {
  try { await navigator.clipboard.writeText(key); appStore.showSuccess(t('linkCards.copied')) } catch { appStore.showError(t('common.copyFailed')) }
}

function openRecharge(card: LinkCard): void { rechargeTarget.value = card; rechargeInput.value = '10.00' }
function closeRecharge(): void { if (!recharging.value) rechargeTarget.value = null }

async function submitRecharge(): Promise<void> {
  if (!rechargeTarget.value || parsedRecharge.value <= 0 || recharging.value) return
  const cardID = rechargeTarget.value.id
  const request = { amount: parsedRecharge.value }
  const fingerprint = JSON.stringify({ card_id: cardID, ...request })
  const key = idempotencyKey('recharge', fingerprint)
  recharging.value = true
  try {
    const result = await linkCardsAPI.rechargeCard(cardID, request, key)
    clearIdempotencyKey('recharge', fingerprint)
    if (authStore.user) authStore.user.balance = Number(result.remaining_user_balance)
    rechargeTarget.value = null
    appStore.showSuccess(t('linkCards.rechargeSuccess'))
    await loadCards()
  } catch { appStore.showError(t('linkCards.rechargeFailed')) } finally { recharging.value = false }
}

async function submitRefund(): Promise<void> {
  const card = refundTarget.value
  refundTarget.value = null
  if (!card || !canRefund(card)) return
  const fingerprint = JSON.stringify({ card_id: card.id })
  const key = idempotencyKey('refund', fingerprint)
  try {
    const result = await linkCardsAPI.refundCard(card.id, key)
    clearIdempotencyKey('refund', fingerprint)
    if (authStore.user) authStore.user.balance = Number(result.user_balance)
    appStore.showSuccess(t('linkCards.refundSuccess'))
    await loadCards()
  } catch { appStore.showError(t('linkCards.refundFailed')) }
}

watch(activeTab, (tab) => { if (tab === 'cards') void loadCards(); if (tab === 'usage') void loadUsage() })
onMounted(() => { void loadInitial() })
</script>

<style scoped>
.icon-button { @apply inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-100; }
.status-badge { @apply inline-flex whitespace-nowrap rounded px-2 py-1 text-xs font-medium; }
</style>

<template>
  <UsageTable
    :data="normalizedRows"
    :loading="loading"
    :columns="columns"
    :show-account-billing="false"
    :show-upstream-endpoint="false"
    flat
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import UsageTable from '@/components/admin/usage/UsageTable.vue'
import type { AdminUsageLog, UsageRequestType } from '@/types'
import type { Column } from '@/components/common/types'

export interface LinkCardUsageRow {
  id: number
  user_id?: number
  user_email?: string
  creator_email?: string
  card_id?: number
  card_key?: string
  key_prefix?: string
  request_id: string
  model: string
  group_id?: number | null
  group_name?: string
  input_tokens?: number
  output_tokens?: number
  cache_creation_tokens?: number
  cache_read_tokens?: number
  cache_creation_5m_tokens?: number
  cache_creation_1h_tokens?: number
  input_cost?: number
  output_cost?: number
  cache_creation_cost?: number
  cache_read_cost?: number
  total_cost?: number
  actual_cost?: number
  rate_multiplier?: number
  request_type?: UsageRequestType
  stream?: boolean
  duration_ms?: number | null
  first_token_ms?: number | null
  billing_mode?: string | null
  created_at: string
  inbound_endpoint?: string | null
  [key: string]: unknown
}

const props = withDefaults(defineProps<{
  rows: LinkCardUsageRow[]
  loading?: boolean
  showOwner?: boolean
}>(), {
  loading: false,
  showOwner: false,
})

const { t } = useI18n()

const columns = computed<Column[]>(() => [
  ...(props.showOwner ? [{ key: 'user', label: t('linkCards.owner') }] : []),
  { key: 'api_key', label: t('linkCards.fullKey') },
  { key: 'model', label: t('linkCards.model') },
  { key: 'group', label: t('admin.usage.group') },
  { key: 'stream', label: t('linkCards.requestType') },
  { key: 'tokens', label: t('linkCards.tokens') },
  { key: 'cost', label: t('linkCards.cost') },
  { key: 'latency', label: t('linkCards.latency') },
  { key: 'request_id', label: t('linkCards.requestId'), class: 'max-w-[170px] truncate' },
  { key: 'created_at', label: t('linkCards.createdAt') },
])

const normalizedRows = computed<AdminUsageLog[]>(() => props.rows.map((row) => ({
  id: row.id,
  user_id: row.user_id ?? 0,
  api_key_id: row.card_id ?? 0,
  account_id: null,
  request_id: row.request_id,
  model: row.model,
  inbound_endpoint: row.inbound_endpoint ?? null,
  upstream_endpoint: null,
  group_id: row.group_id ?? null,
  subscription_id: null,
  input_tokens: row.input_tokens ?? 0,
  output_tokens: row.output_tokens ?? 0,
  cache_creation_tokens: row.cache_creation_tokens ?? 0,
  cache_read_tokens: row.cache_read_tokens ?? 0,
  cache_creation_5m_tokens: row.cache_creation_5m_tokens ?? 0,
  cache_creation_1h_tokens: row.cache_creation_1h_tokens ?? 0,
  input_cost: row.input_cost ?? 0,
  output_cost: row.output_cost ?? 0,
  cache_creation_cost: row.cache_creation_cost ?? 0,
  cache_read_cost: row.cache_read_cost ?? 0,
  total_cost: row.total_cost ?? 0,
  actual_cost: row.actual_cost ?? row.total_cost ?? 0,
  rate_multiplier: row.rate_multiplier ?? 1,
  long_context_billing_applied: false,
  billing_type: 0,
  request_type: row.request_type,
  stream: row.stream ?? row.request_type === 'stream',
  duration_ms: row.duration_ms ?? null,
  first_token_ms: row.first_token_ms ?? null,
  image_count: 0,
  image_size: null,
  image_input_size: null,
  image_output_size: null,
  image_size_source: null,
  image_size_breakdown: null,
  image_input_tokens: 0,
  image_input_cost: 0,
  image_output_tokens: 0,
  image_output_cost: 0,
  user_agent: null,
  cache_ttl_overridden: false,
  billing_mode: row.billing_mode ?? 'token',
  created_at: row.created_at,
  user: props.showOwner ? {
    id: row.user_id ?? 0,
    email: row.user_email ?? row.creator_email ?? '-',
    username: row.user_email ?? row.creator_email ?? '-',
  } as AdminUsageLog['user'] : undefined,
  api_key: {
    id: row.card_id ?? 0,
    name: row.card_key ?? row.key_prefix ?? `#${row.card_id ?? '-'}`,
  } as AdminUsageLog['api_key'],
  group: row.group_name ? { id: row.group_id ?? 0, name: row.group_name } as AdminUsageLog['group'] : undefined,
})))
</script>

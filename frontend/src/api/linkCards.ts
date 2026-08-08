import type { PaginatedResponse, UsageRequestType } from '@/types'
import { apiClient } from './client'

/** Monetary values are normally numbers; string keeps fixed-point backend responses lossless. */
export type LinkCardAmount = number | string

export type LinkCardStatus =
  | 'pending_activation'
  | 'active'
  | 'frozen'
  | 'depleted'
  | 'refunded'
  | 'revoked'

export type LinkCardAdminAction =
  | 'recharge'
  | 'refund'
  | 'freeze'
  | 'unfreeze'
  | 'set_limits'
  | 'revoke'

export interface LinkCardAccess {
  enabled: boolean
  allowed: boolean
  development_mode: boolean
  reason?: string | null
}

export interface LinkCardSettings {
  enabled: boolean
  public_portal_url: string
  api_base_url: string
  default_concurrency: number
  max_batch_size: number
  minimum_deposit: LinkCardAmount | null
}

export interface AdminLinkCardSettings extends LinkCardSettings {
  development_mode: boolean
  development_user_ids: number[]
  default_rpm_limit: number
  public_session_ttl_seconds: number
}

export interface UpdateAdminLinkCardSettingsRequest {
  enabled?: boolean
  public_portal_url?: string
  api_base_url?: string
  default_concurrency?: number
  default_rpm_limit?: number
  max_batch_size?: number
  minimum_deposit?: LinkCardAmount | null
  clear_minimum_deposit?: boolean
  development_mode?: boolean
  development_user_ids?: number[]
  public_session_ttl_seconds?: number
}

export interface LinkCardGroup {
  id: number
  group_id: number
  name: string
  platform?: string | null
  description?: string | null
  rate_multiplier: number
  default_concurrency: number
  models?: string[]
  capabilities?: string[]
  enabled: boolean
  created_at?: string
  updated_at?: string
}

export interface AuthorizeLinkCardGroupRequest {
  group_id: number
  default_concurrency?: number
}

export interface UpdateLinkCardGroupRequest {
  group_id: number
  default_concurrency?: number
  enabled?: boolean
}

export interface LinkCard {
  id: number
  api_key_id: number
  creator_user_id: number
  creator_email?: string
  key: string
  key_prefix?: string
  masked_key?: string
  group_id: number
  group_name: string
  issue_rate_multiplier: number
  status: LinkCardStatus
  original_deposit_amount: LinkCardAmount
  total_deposit_amount: LinkCardAmount
  refundable_amount: LinkCardAmount
  issued_quota: LinkCardAmount
  used_quota: LinkCardAmount
  remaining_quota: LinkCardAmount
  in_flight_quota?: LinkCardAmount
  request_count: number
  concurrency: number
  rpm_limit?: number | null
  activated_at?: string | null
  frozen_at?: string | null
  revoked_at?: string | null
  created_at: string
  updated_at: string
}

export interface LinkCardListParams {
  page: number
  page_size: number
  search?: string
  status?: LinkCardStatus
  group_id?: number
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface AdminLinkCardListParams extends LinkCardListParams {
  creator_user_id?: number
  creator_email?: string
}

export interface CreateLinkCardsRequest {
  group_id: number
  quantity: number
  amount: LinkCardAmount
}

export interface CreateLinkCardsResponse {
  cards: LinkCard[]
  quantity: number
  amount_per_card: LinkCardAmount
  total_debited: LinkCardAmount
  remaining_user_balance: LinkCardAmount
}

export interface RechargeLinkCardRequest {
  amount: LinkCardAmount
}

export interface RechargeLinkCardResponse {
  card: LinkCard
  debited_amount: LinkCardAmount
  remaining_user_balance: LinkCardAmount
  ledger_id?: number
}

/** Refund value is deliberately absent: the server derives it from the locked funding ledger. */
export interface RefundLinkCardRequest {
  reason?: string
}

export interface RefundLinkCardResponse {
  card: LinkCard
  refunded_amount: LinkCardAmount
  user_balance: LinkCardAmount
  ledger_id?: number
}

export interface LinkCardUsageLog {
  id: number
  link_card_id: number
  api_key_id: number
  request_id: string
  model: string
  inbound_endpoint?: string | null
  group_id: number | null
  group_name?: string | null
  request_type?: UsageRequestType
  billing_mode?: string | null
  stream: boolean
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_creation_5m_tokens: number
  cache_creation_1h_tokens: number
  cache_read_tokens: number
  image_input_tokens: number
  image_output_tokens: number
  total_tokens: number
  input_cost: LinkCardAmount
  output_cost: LinkCardAmount
  cache_creation_cost: LinkCardAmount
  cache_read_cost: LinkCardAmount
  image_input_cost: LinkCardAmount
  image_output_cost: LinkCardAmount
  total_cost: LinkCardAmount
  actual_cost: LinkCardAmount
  rate_multiplier: number
  service_tier?: string | null
  duration_ms: number | null
  first_token_ms: number | null
  created_at: string
  creator_user_id?: number
  creator_email?: string
  key_prefix?: string
  masked_key?: string
}

export interface AdminLinkCardSummary {
  total_cards: number
  active_cards: number
  total_reserved: LinkCardAmount
  total_consumed: LinkCardAmount
}

export interface AdminLinkCardPage extends PaginatedResponse<LinkCard> {
  summary?: AdminLinkCardSummary
}

export interface LinkCardUsageParams {
  page: number
  page_size: number
  card_id?: number
  request_id?: string
  model?: string
  group_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  start_date?: string
  end_date?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface AdminLinkCardUsageParams extends LinkCardUsageParams {
  creator_user_id?: number
  creator_email?: string
  key?: string
}

export interface AdminLinkCardActionRequest {
  action: LinkCardAdminAction
  amount?: LinkCardAmount
  concurrency?: number
  rpm_limit?: number | null
  reason?: string
}

export interface AdminLinkCardActionResponse {
  card: LinkCard
  action: LinkCardAdminAction
  debited_amount?: LinkCardAmount
  refunded_amount?: LinkCardAmount
  ledger_id?: number
}

export interface PublicLinkCard {
  masked_key: string
  status: LinkCardStatus
  group_name: string
  issued_quota: LinkCardAmount
  used_quota: LinkCardAmount
  remaining_quota: LinkCardAmount
  request_count: number
  activated_at: string
  created_at: string
}

export interface ActivateLinkCardRequest {
  key: string
}

export interface ActivateLinkCardResponse {
  session_token: string
  expires_at: string
  card?: PublicLinkCard
}

export interface PublicLinkCardProfile {
  card: PublicLinkCard
  key: string
  api_base_url: string
}

export type PublicLinkCardUsageLog = Omit<
  LinkCardUsageLog,
  | 'id'
  | 'link_card_id'
  | 'api_key_id'
  | 'creator_user_id'
  | 'creator_email'
  | 'key_prefix'
  | 'masked_key'
  | 'group_id'
  | 'group_name'
>

function idempotencyConfig(idempotencyKey: string) {
  return {
    headers: {
      'Idempotency-Key': idempotencyKey
    }
  }
}

function publicSessionConfig(sessionToken: string) {
  return {
    headers: {
      'X-Link-Card-Session': sessionToken
    }
  }
}

async function getAccess(): Promise<LinkCardAccess> {
  const { data } = await apiClient.get<LinkCardAccess>('/link-cards/access')
  return data
}

// Named export keeps route guards and sidebar feature checks independent of the API object.
export const getLinkCardAccess = getAccess

async function getSettings(): Promise<LinkCardSettings> {
  const { data } = await apiClient.get<LinkCardSettings>('/link-cards/settings')
  return data
}

async function listGroups(): Promise<LinkCardGroup[]> {
  const { data } = await apiClient.get<LinkCardGroup[]>('/link-cards/groups')
  return data
}

async function listCards(params: LinkCardListParams): Promise<PaginatedResponse<LinkCard>> {
  const { data } = await apiClient.get<PaginatedResponse<LinkCard>>('/link-cards/cards', { params })
  return data
}

async function createCards(
  request: CreateLinkCardsRequest,
  idempotencyKey: string
): Promise<CreateLinkCardsResponse> {
  const { data } = await apiClient.post<CreateLinkCardsResponse>(
    '/link-cards/cards',
    request,
    idempotencyConfig(idempotencyKey)
  )
  return data
}

async function rechargeCard(
  id: number,
  request: RechargeLinkCardRequest,
  idempotencyKey: string
): Promise<RechargeLinkCardResponse> {
  const { data } = await apiClient.post<RechargeLinkCardResponse>(
    `/link-cards/cards/${id}/recharge`,
    request,
    idempotencyConfig(idempotencyKey)
  )
  return data
}

async function refundCard(
  id: number,
  idempotencyKey: string,
  request: RefundLinkCardRequest = {}
): Promise<RefundLinkCardResponse> {
  const { data } = await apiClient.post<RefundLinkCardResponse>(
    `/link-cards/cards/${id}/refund`,
    request,
    idempotencyConfig(idempotencyKey)
  )
  return data
}

async function listUsage(
  params: LinkCardUsageParams
): Promise<PaginatedResponse<LinkCardUsageLog>> {
  const { data } = await apiClient.get<PaginatedResponse<LinkCardUsageLog>>('/link-cards/usage', {
    params
  })
  return data
}

async function getAdminSettings(): Promise<AdminLinkCardSettings> {
  const { data } = await apiClient.get<AdminLinkCardSettings>('/admin/link-cards/settings')
  return data
}

async function updateAdminSettings(
  request: UpdateAdminLinkCardSettingsRequest
): Promise<AdminLinkCardSettings> {
  const { data } = await apiClient.put<AdminLinkCardSettings>(
    '/admin/link-cards/settings',
    request
  )
  return data
}

async function listAdminGroups(): Promise<LinkCardGroup[]> {
  const { data } = await apiClient.get<LinkCardGroup[]>('/admin/link-cards/groups')
  return data
}

async function authorizeGroup(request: AuthorizeLinkCardGroupRequest): Promise<LinkCardGroup> {
  const { data } = await apiClient.post<LinkCardGroup>('/admin/link-cards/groups', request)
  return data
}

async function updateGroup(request: UpdateLinkCardGroupRequest): Promise<LinkCardGroup> {
  const { data } = await apiClient.put<LinkCardGroup>('/admin/link-cards/groups', request)
  return data
}

async function removeGroup(groupId: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>('/admin/link-cards/groups', {
    data: { group_id: groupId }
  })
  return data
}

async function listAdminCards(
  params: AdminLinkCardListParams
): Promise<AdminLinkCardPage> {
  const { data } = await apiClient.get<AdminLinkCardPage>('/admin/link-cards/cards', {
    params
  })
  return data
}

async function listAdminUsage(
  params: AdminLinkCardUsageParams
): Promise<PaginatedResponse<LinkCardUsageLog>> {
  const { data } = await apiClient.get<PaginatedResponse<LinkCardUsageLog>>(
    '/admin/link-cards/usage',
    { params }
  )
  return data
}

async function runCardAction(
  id: number,
  request: AdminLinkCardActionRequest,
  idempotencyKey: string
): Promise<AdminLinkCardActionResponse> {
  const { data } = await apiClient.post<AdminLinkCardActionResponse>(
    `/admin/link-cards/cards/${id}/actions`,
    request,
    idempotencyConfig(idempotencyKey)
  )
  return data
}

async function activate(request: ActivateLinkCardRequest): Promise<ActivateLinkCardResponse> {
  const { data } = await apiClient.post<ActivateLinkCardResponse>(
    '/public/link-cards/activate',
    request
  )
  return data
}

async function getPublicProfile(sessionToken: string): Promise<PublicLinkCardProfile> {
  const { data } = await apiClient.get<PublicLinkCardProfile>(
    '/public/link-cards/me',
    publicSessionConfig(sessionToken)
  )
  return data
}

async function listPublicUsage(
  sessionToken: string,
  params: Pick<LinkCardUsageParams, 'page' | 'page_size' | 'start_date' | 'end_date'>
): Promise<PaginatedResponse<PublicLinkCardUsageLog>> {
  const { data } = await apiClient.get<PaginatedResponse<PublicLinkCardUsageLog>>(
    '/public/link-cards/usage',
    {
      ...publicSessionConfig(sessionToken),
      params
    }
  )
  return data
}

export const linkCardsAPI = {
  getAccess,
  getSettings,
  listGroups,
  listCards,
  createCards,
  rechargeCard,
  refundCard,
  listUsage
}

export const adminLinkCardsAPI = {
  getSettings: getAdminSettings,
  updateSettings: updateAdminSettings,
  listGroups: listAdminGroups,
  authorizeGroup,
  updateGroup,
  removeGroup,
  listCards: listAdminCards,
  listUsage: listAdminUsage,
  runCardAction
}

export const publicLinkCardsAPI = {
  activate,
  getMe: getPublicProfile,
  listUsage: listPublicUsage
}

export default linkCardsAPI

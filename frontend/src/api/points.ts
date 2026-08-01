import { apiClient } from './client'

export interface PointsLaunchRequest {
  theme: 'light' | 'dark'
  language: string
}

export interface PointsLaunchResponse {
  launch_url: string
}

export interface PointsBridgeStatus {
  enabled: boolean
  configured: boolean
  active: boolean
  public_url: string
  menu_label: string
  launch_key_id: string
  launch_secret_configured: boolean
  credit_key_id: string
  credit_secret_configured: boolean
  launch_ttl_seconds: number
  clock_skew_seconds: number
}

export interface PointsUserAccess {
  allowed: boolean
}

export async function getPointsUserAccess(): Promise<PointsUserAccess> {
  const { data } = await apiClient.get<PointsUserAccess>('/points/access')
  return data
}

export async function getPointsBridgeStatus(): Promise<PointsBridgeStatus> {
  const { data } = await apiClient.get<PointsBridgeStatus>('/admin/points/status')
  return data
}

export async function createPointsLaunch(
  role: 'user' | 'admin',
  request: PointsLaunchRequest,
): Promise<PointsLaunchResponse> {
  const path = role === 'admin' ? '/admin/points/launch' : '/points/launch'
  const { data } = await apiClient.post<PointsLaunchResponse>(path, request)
  return data
}

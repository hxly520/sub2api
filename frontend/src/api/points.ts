import { apiClient } from './client'

export interface PointsLaunchRequest {
  theme: 'light' | 'dark'
  language: string
}

export interface PointsLaunchResponse {
  launch_url: string
}

export async function createPointsLaunch(
  role: 'user' | 'admin',
  request: PointsLaunchRequest,
): Promise<PointsLaunchResponse> {
  const path = role === 'admin' ? '/admin/points/launch' : '/points/launch'
  const { data } = await apiClient.post<PointsLaunchResponse>(path, request)
  return data
}

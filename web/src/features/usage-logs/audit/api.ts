/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { t } from 'i18next'

import type { ApiResponse } from '@/features/profile/types'
import { api } from '@/lib/api'
import { createServerError } from '@/lib/server-error-message'

export interface AuditLog {
  event_id: string
  user_id: number
  username: string
  actor_role: number
  created_at: number
  category: string
  action: string
  token_ref: string
  auth_method?: string
  ip: string
  user_agent: string
  method: string
  route: string
  status: number
  success: boolean
  request_id: string
  content: string
  other: Record<string, unknown> | null
}
export interface AuditFilters {
  p: number
  page_size: number
  start_timestamp?: number
  end_timestamp?: number
  success?: string
  category?: string
  action?: string
  token_ref?: string
  exclude_token_ref?: string
  username?: string
  request_id?: string
}

export type AbuseReviewStatus = 'pending' | 'resolved' | 'false_positive'

export interface AbuseReview {
  id: number
  event_id: string
  user_id: number
  token_id: number
  action: string
  status: AbuseReviewStatus
  risk_score: number
  disposition?: string
  score_version: string
  evidence_json: string
  reviewer_id: number
  review_note: string
  created_at: number
  updated_at: number
  resolved_at: number
}

export interface AbuseReviewSummary {
  total: number
  pending: number
  resolved: number
  false_positive: number
  last_24_hours: number
  review_priority: number
  manual_priority: number
  content_audit: number
  keyword_match: number
  generated_at: number
}

export interface ContentAuditHealth {
  enabled: boolean
  configured: boolean
  model: string
  timeout_ms: number
  sample_rate: number
  requests: number
  flagged: number
  errors: number
  dropped: number
  in_flight: number
  last_error_at: number
}

export interface AbuseReviewSummaryResponse {
  reviews: AbuseReviewSummary
  content_audit: ContentAuditHealth
}
export async function getAuditLogs(
  scope: 'all' | 'self',
  params: AuditFilters
): Promise<{ items: AuditLog[]; total: number }> {
  const response = await api.get<
    ApiResponse<{ items: AuditLog[]; total: number }>
  >(scope === 'all' ? '/api/audit' : '/api/audit/self', { params })
  if (!response.data.success || !response.data.data) {
    throw createServerError(response.data, t('Failed to load audit records'))
  }
  return response.data.data
}

export async function getAbuseReviews(params: {
  p: number
  page_size: number
  status?: AbuseReviewStatus
  action?: string
}): Promise<{ items: AbuseReview[]; total: number }> {
  const response = await api.get<
    ApiResponse<{ items: AbuseReview[]; total: number }>
  >('/api/abuse_reviews', { params })
  if (!response.data.success || !response.data.data) {
    throw createServerError(response.data, t('Failed to load abuse reviews'))
  }
  return response.data.data
}

export async function getAbuseReviewSummary(): Promise<AbuseReviewSummaryResponse> {
  const response = await api.get<ApiResponse<AbuseReviewSummaryResponse>>(
    '/api/abuse_reviews/summary'
  )
  if (!response.data.success || !response.data.data) {
    throw createServerError(response.data, t('Failed to load abuse review summary'))
  }
  return response.data.data
}

export async function updateAbuseReview(
  id: number,
  status: Exclude<AbuseReviewStatus, 'pending'>,
  note: string
): Promise<AbuseReview> {
  const response = await api.patch<ApiResponse<AbuseReview>>(
    `/api/abuse_reviews/${id}`,
    { status, note }
  )
  if (!response.data.success || !response.data.data) {
    throw createServerError(response.data, t('Failed to update abuse review'))
  }
  return response.data.data
}

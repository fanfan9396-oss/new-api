const ACTION_LABELS: Record<string, string> = {
  'abuse.failure_burst': '连续失败',
  'abuse.provider_error': '上游服务错误',
  'abuse.token_churn': '密钥频繁轮换',
  'abuse.long_request': '请求持续时间过长',
  'abuse.content_audit': '内容审计命中',
  'abuse.keyword_match': '高风险行为关键词命中',
}

const CATEGORY_LABELS: Record<string, string> = {
  cyber_abuse: '网络滥用',
  credential_theft: '凭据窃取',
  security_bypass: '安全机制绕过',
  provider_policy_evasion: '上游政策规避',
  bulk_account_key_rotation: '批量账号或密钥轮换',
  bulk_registration_evasion: '批量注册规避',
  quota_billing_evasion: '额度或计费规避',
  none: '未分类',
}

const REASON_LABELS: Record<string, string> = {
  third_party_attack: '检测到针对第三方系统的攻击意图',
  bulk_account_key_rotation: '检测到批量账号或密钥轮换以规避检测',
  bulk_registration_evasion: '检测到批量注册以规避检测',
  quota_billing_evasion: '检测到额度或计费规避意图',
  sensitive_keyword: '命中高风险行为规则',
}

const FIELD_LABELS: Record<string, string> = {
  failure_count: '失败次数',
  status: '上游 HTTP 状态',
  token_id: '关联密钥编号',
  user_id: '关联用户编号',
  window_seconds: '统计窗口',
  duration_ms: '请求耗时',
  category: '审计类别',
  confidence: '审计置信度',
  reason_code: '审计原因',
  model_name: '审计模型',
  input_length: '输入长度',
  match_count: '命中规则数',
  rule_source: '规则来源',
  action: '处置建议',
}

function labelForValue(value: unknown, labels: Record<string, string>) {
  if (typeof value !== 'string') return value
  return labels[value] ?? value
}

function formatValue(key: string, value: unknown): string {
  if (key === 'category') return String(labelForValue(value, CATEGORY_LABELS))
  if (key === 'reason_code') return String(labelForValue(value, REASON_LABELS))
  if (key === 'action') {
    return value === 'review' ? '进入人工复核' : value === 'allow' ? '放行并记录' : String(value)
  }
  if (key === 'confidence' && typeof value === 'number') return `${Math.round(value * 100)}%`
  if (key === 'duration_ms' && typeof value === 'number') return `${value} 毫秒`
  if (key === 'window_seconds' && typeof value === 'number') return `${value} 秒`
  if (key === 'input_length' && typeof value === 'number') return `${value} 字符`
  if (typeof value === 'object' && value !== null) return JSON.stringify(value)
  return String(value)
}

function reasonFor(action: string, fields: Record<string, unknown>): string {
  const failureCount = fields.failure_count
  const status = fields.status
  const duration = fields.duration_ms
  if (action === 'abuse.failure_burst' && failureCount !== undefined) return `短时间内连续失败请求达到 ${failureCount} 次`
  if (action === 'abuse.provider_error' && status !== undefined) return `上游服务返回 HTTP ${status} 错误`
  if (action === 'abuse.token_churn') return '同一用户在短时间内频繁创建或轮换 API 密钥'
  if (action === 'abuse.long_request' && duration !== undefined) return `请求持续 ${duration} 毫秒，超过长请求告警阈值`
  if (action === 'abuse.content_audit') {
    const category = labelForValue(fields.category, CATEGORY_LABELS)
    const reason = labelForValue(fields.reason_code, REASON_LABELS)
    return `内容审计识别为${category}${reason ? `：${reason}` : ''}`
  }
  if (action === 'abuse.keyword_match') return '请求命中高风险行为关键词规则，进入人工复核'
  return '检测到需要人工复核的异常行为信号'
}

export function formatAbuseAction(action: string): string {
  return ACTION_LABELS[action] ?? action
}

export function formatAbuseEvidence(action: string, raw: string): string {
  if (!raw) return '暂无结构化证据'
  let parsed: Record<string, unknown>
  try {
    parsed = JSON.parse(raw) as Record<string, unknown>
  } catch {
    return raw
  }
  const fields = parsed.root_info && typeof parsed.root_info === 'object'
    ? { ...(parsed.root_info as Record<string, unknown>), ...parsed }
    : parsed
  const lines = [`原因：${reasonFor(action, fields)}`]
  for (const [key, value] of Object.entries(fields)) {
    if (key === 'root_info' || key === 'action' && action === 'abuse.content_audit') continue
    const label = FIELD_LABELS[key] ?? key
    lines.push(`${label}：${formatValue(key, value)}`)
  }
  return lines.join('\n')
}

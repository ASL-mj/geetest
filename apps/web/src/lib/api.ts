// Typed client for the CaptchaFlow platform API. Session cookies travel
// automatically (same-origin via the dev proxy, or same-site deployments);
// credentials are never stored in localStorage.

export interface Envelope<T> {
  success: boolean;
  request_id: string;
  data?: T;
  error?: { code: string; message: string; retryable: boolean };
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string;

  constructor(status: number, code: string, message: string, requestId: string) {
    super(message);
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
  const response = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: {
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...(init.headers ?? {}),
    },
  });
  let payload: Envelope<T>;
  try {
    payload = (await response.json()) as Envelope<T>;
  } catch {
    throw new ApiError(response.status, 'NETWORK_ERROR', '响应不是有效的 JSON。', '');
  }
  if (!response.ok || !payload.success) {
    // Session expiry mid-console: broadcast so the shell can re-probe and
    // route the user back to activation instead of endless load errors.
    if (response.status === 401) {
      window.dispatchEvent(new CustomEvent('captchaflow:unauthorized'));
    }
    const error = payload.error ?? { code: 'UNKNOWN', message: '未知错误。', retryable: false };
    throw new ApiError(response.status, error.code, error.message, payload.request_id ?? '');
  }
  return payload;
}

// ---------- shared shapes ----------

export interface APIKey {
  id: string;
  name: string;
  prefix: string;
  last4: string;
  status: string;
  quota_limit: number | null;
  allowed_ips: string | null;
  total_calls: number;
  created_at: string;
  last_used_at: string | null;
}

export interface CDKSummary {
  code_prefix: string;
  status: string;
  activated_at: string | null;
  expires_at: string | null;
  quota_total: number;
  quota_used: number;
  quota_reserved: number;
  quota_remaining: number;
}

export interface UsageReport {
  quota_total: number;
  quota_used: number;
  quota_reserved: number;
  quota_remaining: number;
  calls_today: number;
  calls_total: number;
  success_total: number;
  failed_total: number;
  rejected_total: number;
  success_rate: number;
}

export interface CallRecord {
  request_id: string;
  api_key_name: string;
  api_key_prefix: string;
  captcha_id: string;
  risk_type: string;
  status: string;
  http_status: number;
  error_code: string | null;
  accepted_at: string;
  completed_at: string | null;
  duration_ms: number | null;
  quota_reserved: boolean;
  quota_refunded: boolean;
  client_ip: string;
  user_agent: string;
}

export interface ActivationResult {
  default_api_key?: string;
  user: { id: string; cdk_prefix: string };
}

/**
 * Resolves the base URL shown in docs and snippets: the operator-configured
 * display address when present, otherwise the current origin. The result is
 * cached for the tab lifetime; the value is display-only.
 */
let docsBaseURL: string | null = null;
export async function getDocsBaseURL(): Promise<string> {
  // Only successful lookups are cached; a transient meta failure falls back
  // to the current origin and retries on the next call.
  if (docsBaseURL !== null) return docsBaseURL;
  try {
    const envelope = await meta();
    const configured = envelope.data?.api_base_url?.trim();
    docsBaseURL = configured ? configured : window.location.origin;
  } catch {
    return window.location.origin;
  }
  return docsBaseURL;
}

/** Clears the cached display URL after an operator updates it. */
export function resetDocsBaseURLCache(): void {
  docsBaseURL = null;
}

// ---------- user session ----------

export const api = {
  activate: (cdk: string) =>
    request<ActivationResult>('/v1/auth/activate', {
      method: 'POST',
      body: JSON.stringify({ cdk }),
    }),

  logout: () => request<Record<string, never>>('/v1/auth/logout', { method: 'POST' }),

  account: () => request<{ cdk: CDKSummary }>('/v1/account'),

  usage: () => request<UsageReport>('/v1/usage'),

  listKeys: () => request<{ items: APIKey[] }>('/v1/keys'),

  createKey: (name: string) =>
    request<APIKey & { secret: string }>('/v1/keys', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),

  updateKey: (keyId: string, patch: { name?: string; status?: string; quota_limit?: number; allowed_ips?: string }) =>
    request<APIKey>(`/v1/keys/${keyId}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),

  revealKeySecret: (keyId: string) =>
    request<{ secret: string }>(`/v1/keys/${keyId}/secret`),

  deleteKey: (keyId: string) =>
    request<APIKey>(`/v1/keys/${keyId}`, { method: 'DELETE' }),

  listCalls: (cursor?: string, limit = 20) => {
    const query = new URLSearchParams({ limit: String(limit) });
    if (cursor) query.set('cursor', cursor);
    return request<{ items: CallRecord[]; next_cursor: string | null }>(`/v1/calls?${query}`);
  },

  getCall: (requestId: string) => request<CallRecord>(`/v1/calls/${requestId}`),

  consoleSolve: (keyId: string, captchaId: string) =>
    request<{ captcha_id: string; lot_number: string; captcha_output: string; pass_token: string; gen_time: string }>(
      '/v1/tools/captcha/solve',
      { method: 'POST', body: JSON.stringify({ key_id: keyId, captcha_id: captchaId }) },
    ),
};

// Public, unauthenticated display metadata (see /v1/meta).
export const meta = () => request<PublicMeta>('/v1/meta');

// ---------- admin ----------

export interface AdminDashboard {
  users_total: number;
  users_active: number;
  cdks_active: number;
  cdks_unactivated: number;
  cdks_exhausted: number;
  calls_today: number;
  calls_rejected_today: number;
  calls_failed_today: number;
  calls_succeeded_today: number;
  quota_consumed: number;
  success_rate: number;
}

export interface AdminBatch {
  id: string;
  name: string;
  default_quota: number;
  total_cdks: number;
  active_cdks: number;
  created_at: string;
}

export interface AdminCdk {
  id: string;
  batch_id: string;
  batch_name: string;
  code_prefix: string;
  status: string;
  bound_user_id: string | null;
  bound_user_state: string | null;
  remark: string | null;
  quota_total: number;
  quota_used: number;
  quota_remaining: number;
  activated_at: string | null;
  created_at: string;
}

export interface SystemConfig {
  api_base_url: string;
  has_override: boolean;
}

export interface PublicMeta {
  api_base_url: string;
}

export interface AdminUser {
  id: string;
  status: string;
  created_at: string;
  cdk_prefix: string | null;
  cdk_status: string | null;
  cdk_remaining: number | null;
}

export interface AdminAuditEntry {
  id: string;
  admin_user_id: string;
  action: string;
  target_type: string;
  target_id: string;
  reason: string;
  ip_masked: string;
  created_at: string;
}

export const adminApi = {
  login: (username: string, password: string) =>
    request<Record<string, never>>('/admin/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  logout: () => request<Record<string, never>>('/admin/v1/auth/logout', { method: 'POST' }),

  dashboard: () => request<AdminDashboard>('/admin/v1/dashboard'),

  createBatch: (input: { name: string; description?: string; quota: number; count: number; service_duration_days?: number; reason: string }) =>
    request<{ batch_id: string; codes: string[] }>('/admin/v1/cdk-batches', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  listBatches: () => request<{ items: AdminBatch[] }>('/admin/v1/cdk-batches'),

  listCdks: () => request<{ items: AdminCdk[] }>('/admin/v1/cdks'),

  adjustQuota: (cdkId: string, delta: number, reason: string) =>
    request<{ quota_remaining: number }>(`/admin/v1/cdks/${cdkId}/quota-adjustments`, {
      method: 'POST',
      body: JSON.stringify({ delta, reason }),
    }),

  revealCdkCode: (cdkId: string) =>
    request<{ code: string }>(`/admin/v1/cdks/${cdkId}/code`),

  setCdkRemark: (cdkId: string, remark: string) =>
    request<{ remark: string }>(`/admin/v1/cdks/${cdkId}/remark`, {
      method: 'PATCH',
      body: JSON.stringify({ remark }),
    }),

  getSystemConfig: () => request<SystemConfig>('/admin/v1/system/config'),

  setSystemConfig: (input: { api_base_url: string; reason: string }) =>
    request<{ api_base_url: string }>('/admin/v1/system/config', {
      method: 'PUT',
      body: JSON.stringify(input),
    }),

  setCdkStatus: (cdkId: string, status: string, reason: string) =>
    request<{ status: string }>(`/admin/v1/cdks/${cdkId}`, {
      method: 'PATCH',
      body: JSON.stringify({ status, reason }),
    }),

  listUsers: () => request<{ items: AdminUser[] }>('/admin/v1/users'),

  setUserStatus: (userId: string, status: string, reason: string) =>
    request<{ status: string }>(`/admin/v1/users/${userId}`, {
      method: 'PATCH',
      body: JSON.stringify({ status, reason }),
    }),

  listAuditLogs: () => request<{ items: AdminAuditEntry[] }>('/admin/v1/audit-logs'),

  solverHealth: () =>
    request<{ status: string; latency_ms: number; checked_at: string; failure?: string }>(
      '/admin/v1/solver-health',
    ),
};

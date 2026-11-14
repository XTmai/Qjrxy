// API 端点集合，与 internal/server/router.go 的 34 条路由一一对应。
import { getToken, http } from './client';
import type {
  Account,
  AccountPhoto,
  AdminConfig,
  AccountInput,
  AuditRow,
  Dashboard,
  DayStat,
  LoginChallenge,
  ManualCaptcha,
  Plan,
  PlanInput,
  ProbeResult,
  RunLog,
  RunOutcome,
  School,
  SignRange,
  Stats7d,
  SystemInfo,
  Task,
  User,
} from '../lib/types';

export interface AuthPayload {
  user: User;
  token?: string;
}

export const api = {
  // ---- 认证 ----
  register: (username: string, password: string, nickname?: string) =>
    http.post<AuthPayload>('/auth/register', { username, password, nickname }, { keepAuth: true }),
  login: (username: string, password: string) =>
    http.post<AuthPayload>('/auth/login', { username, password }, { keepAuth: true }),
  logout: () => http.post<null>('/auth/logout', {}),
  me: () => http.get<{ user: User }>('/auth/me', { keepAuth: true }),
  changePassword: (oldPassword: string, newPassword: string) =>
    http.post<{ message: string }>('/auth/password', { oldPassword, newPassword }),
  systemInfo: () => http.get<SystemInfo>('/system/info'),

  // ---- 学校 ----
  schools: (q: string) => http.get<{ schools: School[]; total: number }>(`/schools?q=${encodeURIComponent(q)}`),

  // ---- 账号 ----
  listAccounts: () => http.get<{ accounts: Account[] }>('/accounts'),
  account: (id: number) => http.get<{ account: Account; tasks?: Task[] }>(`/accounts/${id}`),
  // existed=true：该学校下学号本来就已绑定，后端复用并更新了密码/设备信息，并未新建。
  createAccount: (data: AccountInput) =>
    http.post<{ account: Account; existed?: boolean }>('/accounts', data),
  updateAccount: (id: number, data: Partial<AccountInput>) =>
    http.patch<{ account: Account }>(`/accounts/${id}`, data),
  deleteAccount: (id: number) => http.del<{ message: string }>(`/accounts/${id}`),
  accountLogin: (id: number) => http.post<LoginChallenge>(`/accounts/${id}/login`, {}),
  smsSend: (id: number, phone: string) =>
    http.post<{ done: boolean; ticket: string; phone: string; expiresAt: string; message: string }>(
      `/accounts/${id}/sms/send`,
      { phone },
    ),
  smsLogin: (id: number, ticket: string, code: string) =>
    http.post<{ done: boolean; message: string; account?: Account }>(`/accounts/${id}/sms/login`, { ticket, code }),
  submitCaptcha: (id: number, ticket: string, code: string) =>
    http.post<{ done: boolean; message?: string }>(`/accounts/${id}/captcha`, { ticket, code }),
  verifyAccount: (id: number) =>
    http.post<{ loginStatus: string; message: string }>(`/accounts/${id}/verify`, {}),
  probeAccounts: (accountId = 0) => http.post<{ results: ProbeResult[] }>('/accounts/probe', { accountId }),
  uploadPhoto: (id: number, formData: FormData) =>
    http.post<{ account: Account }>(`/accounts/${id}/photo`, formData),
  photoUrl: (id: number) => `/api/accounts/${id}/photo`,
  accountPhotos: (id: number) => http.get<{ photos: AccountPhoto[] }>(`/accounts/${id}/photos`),
  uploadAccountPhotos: (id: number, formData: FormData) =>
    http.post<{ photos: AccountPhoto[]; failed: { name: string; error: string }[] }>(
      `/accounts/${id}/photos`,
      formData,
    ),
  deleteAccountPhoto: (id: number, photoId: number) =>
    http.del<{ message: string }>(`/accounts/${id}/photos/${photoId}`),

  // ---- 设备扫码 ----
  deviceTicket: (id: number) =>
    http.post<{ ticket: string; url: string; expiresAt: string; qrcode: string }>(
      `/accounts/${id}/device-ticket`,
      {},
    ),
  devicePoll: (ticket: string) =>
    http.get<{
      pending: boolean;
      collected?: boolean;
      deviceId?: string;
      deviceModel?: string;
      systemVersion?: string;
      buildId?: string;
      platform?: string;
      wisDeviceId?: string;
      uuid?: string;
    }>(`/device/poll/${encodeURIComponent(ticket)}`),

  // ---- 扫码获取登录态（与设备扫码是两个独立入口） ----
  loginScanTicket: (id: number) =>
    http.post<{
      ticket: string;
      url: string;
      expiresAt: string;
      qrcode: string;
      clientIds: string[];
      target: { schoolName: string; username: string; alias: string; host: string };
    }>(`/accounts/${id}/login-scan`, {}),
  loginScanPoll: (id: number, ticket: string) =>
    http.get<{ pending: boolean; collected?: boolean; account?: Account }>(
      `/accounts/${id}/login-scan/poll?t=${encodeURIComponent(ticket)}`,
    ),

  // ---- 签到位置 ----
  signRange: (id: number) => http.get<{ range: SignRange }>(`/accounts/${id}/sign-range`),
  applySignRange: (id: number) =>
    http.post<{ account: Account; range: SignRange; message: string }>(`/accounts/${id}/sign-range`, {}),

  // ---- 任务 ----
  tasks: () => http.get<{ tasks: Task[] }>('/tasks'),
  syncTasks: (accountId: number) =>
    http.post<{ tasks: Task[]; count: number }>('/tasks/sync', { accountId }),
  runTask: (accountId: number, taskId: number, kind: string) =>
    http.post<RunOutcome>('/tasks/run', { accountId, taskId, kind }),

  // ---- 验证码人工兜底 ----
  // 自动识别失败时签到会挂起，这里取当前待处理的题目；刷新页面后也能恢复。
  captchaPending: () => http.get<ManualCaptcha[]>('/captcha/pending'),
  captchaSubmit: (token: string, body: { cells?: number[]; action?: 'skip' | 'cancel' }) =>
    http.post<{ submitted: boolean }>(`/captcha/${token}`, body),

  // ---- 账号导入导出 ----
  // 导出的文件含登录态凭证，走原生 fetch 拿 blob（Authorization 头无法用 <a> 直接带）。
  exportAccounts: async (opts: { format: 'json' | 'csv'; withSession: boolean; withPassword: boolean }) => {
    const qs = new URLSearchParams({
      format: opts.format,
      withSession: opts.withSession ? '1' : '0',
      withPassword: opts.withPassword ? '1' : '0',
    });
    const res = await fetch(`/api/accounts/export?${qs.toString()}`, {
      headers: getToken() ? { Authorization: `Bearer ${getToken()}` } : undefined,
      credentials: 'same-origin',
    });
    if (!res.ok) throw new Error('导出失败');
    return await res.blob();
  },
  importAccounts: (accounts: unknown[]) =>
    http.post<{ created: number; updated: number; failed: number; items: any[] }>('/accounts/import', { accounts }),

  // ---- 计划 ----
  plans: () => http.get<{ plans: Plan[] }>('/plans'),
  createPlan: (data: PlanInput) => http.post<{ plan: Plan }>('/plans', data),
  updatePlan: (id: number, data: PlanInput) => http.patch<{ plan: Plan }>(`/plans/${id}`, data),
  deletePlan: (id: number) => http.del<{ message: string }>(`/plans/${id}`),
  togglePlan: (id: number, enabled: boolean) =>
    http.post<{ enabled: boolean }>(`/plans/${id}/toggle`, { enabled }),
  resetPlan: (id: number) => http.post<{ message: string }>(`/plans/${id}/reset`, {}),

  // ---- 记录 ----
  logs: (params: { scope?: string; status?: string; kind?: string; limit?: number; offset?: number }) => {
    const qs = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') qs.set(k, String(v));
    });
    return http.get<{ logs: RunLog[]; total: number }>(`/logs${qs.toString() ? `?${qs}` : ''}`);
  },
  log: (id: number) => http.get<{ log: RunLog }>(`/logs/${id}`),
  auditLogs: (params: { status?: string; scope?: string; kind?: string; limit?: number; offset?: number }) => {
    const qs = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') qs.set(k, String(v));
    });
    return http.get<{ logs: AuditRow[]; total: number }>(`/logs/audit${qs.toString() ? `?${qs}` : ''}`);
  },
  stats7d: () => http.get<Stats7d>('/stats/7d'),
  logTrend: (days = 14) => http.get<{ days: number; trend: DayStat[] }>(`/logs/trend?days=${days}`),

  // ---- 工作台 ----
  dashboard: () => http.get<Dashboard>('/dashboard'),

  // ---- 额度（QINGJIN_QUOTA_ENABLED=true 时可用） ----
  redeem: (code: string) =>
    http.post<{ user: User; granted: number; message: string }>('/quota/redeem', { code }),

  // ---- 管理员（role=admin 或单用户模式；其他人 403） ----
  getAdminConfig: () => http.get<AdminConfig>('/admin/config'),
  saveAdminConfig: (payload: {
    diagEnabled?: boolean;
    diagKeepDays?: number;
    visionApiKey?: string;
    visionBaseURL?: string;
    visionModel?: string;
    multiUser?: boolean;
    adminPassword?: string;
    registerOpen?: boolean;
    quotaEnabled?: boolean;
    tlsInsecureHosts?: string;
    photoPoolLimitBytes?: number;
  }) => http.post<{ message: string; token?: string }>('/admin/config', payload),
};

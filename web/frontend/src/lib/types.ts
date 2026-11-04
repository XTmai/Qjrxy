// API 数据模型，与 Go 后端 store 层的 JSON 序列化一一对应。

export interface User {
  id: number;
  username: string;
  nickname: string;
  role: string;
  quota: number;
  quotaUsed: number;
  theme: string;
  createdAt: string;
}

export interface Account {
  id: number;
  userId: number;
  alias: string;
  schoolName: string;
  username: string;
  tenantId: string;
  host: string;
  joinType: string;
  loginStatus: 'authorized' | 'expired' | 'unknown' | string;
  lastLoginAt: string;
  lastProbeAt: string;
  lastProbeMsg: string;
  deviceId: string;
  deviceModel: string;
  systemVersion: string;
  platform: 'android' | 'ios' | string;
  wisDeviceId: string;
  userAgent: string;
  appVersion: string;
  buildId: string;
  address: string;
  lon: number;
  lat: number;
  abnormal: string;
  photoPath: string;
  photoUrl?: string;
  hasCookie?: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface School {
  id: string;
  name: string;
}

/** 创建/编辑账号的输入（AccountInput） */
export interface AccountInput {
  alias?: string;
  schoolName: string;
  username: string;
  password?: string;
  deviceId?: string;
  deviceModel?: string;
  systemVersion?: string;
  platform?: string;
  wisDeviceId?: string;
  userAgent?: string;
  appVersion?: string;
  buildId?: string;
  address?: string;
  lon?: number;
  lat?: number;
  abnormalReason?: string;
}

export type TaskKind = 'attendance' | 'sign' | 'collect';

export interface Task {
  id: number;
  accountId: number;
  accountAlias?: string;
  kind: TaskKind | string;
  kindLabel?: string;
  signInstanceWid: string;
  signWid: string;
  taskName: string;
  sender: string;
  startTime: string;
  endTime: string;
  status: 'pending' | 'done' | 'leave' | string;
  stuSignWid: string;
  signDate: string;
  beginClock: string;
  endClock: string;
  signStatus: string;
  signStatusText: string;
  fetchedAt: string;
}

export interface Plan {
  id: number;
  userId: number;
  accountId: number;
  name: string;
  kind: string;
  mode: 'school' | 'custom' | string;
  windowStart: string;
  windowEnd: string;
  days: number[];
  enabled: boolean;
  lastRunAt: string;
  lastResult: string;
  createdAt: string;
  updatedAt: string;
  accountAlias?: string;
  schoolName?: string;
  accountUsername?: string;
}

export interface PlanInput {
  accountId: number;
  name: string;
  kind: string;
  mode: string;
  windowStart: string;
  windowEnd: string;
  days: number[];
  enabled: boolean;
}

export interface RunLog {
  id: number;
  userId: number;
  accountId: number;
  planId: number;
  taskName: string;
  kind: string;
  source: 'auto' | 'manual' | string;
  status: 'success' | 'failed' | 'pending' | 'executing' | string;
  message: string;
  detail: string;
  createdAt: string;
  accountAlias?: string;
  schoolName?: string;
  accountUsername?: string;
}

export interface AuditRow extends RunLog {
  traceId: string;
  latencyMs: number;
  geoOffsetM: number;
  inRange?: boolean | null;
}

export interface Stats7d {
  total: number;
  success: number;
  successRate: number;
  avgLatencyMs: number;
  avgGeoOffsetM: number;
}

export interface LogDetail {
  code?: string;
  message?: string;
  lon?: number;
  lat?: number;
  address?: string;
  device?: string;
  isPhoto?: boolean;
  isNeedExtra?: boolean;
  inRange?: boolean;
  places?: Array<{ name: string; distance: number; radius: number; inRange: boolean }>;
  error?: string;
}

export interface TodayStats {
  todayTotal: number;
  todaySuccess: number;
  todayFailed: number;
  todayPending: number;
}

export interface Dashboard {
  stats: TodayStats;
  accounts: Account[];
  tasks: Task[];
  pendingTask: number;
  plans: number;
  activePlans: number;
  recentLogs: RunLog[];
  user: User;
  today: string;
}

export interface DayStat {
  date: string;
  success: number;
  failed: number;
  pending: number;
}

export interface ProbeResult {
  accountId: number;
  accountName: string;
  status: 'authorized' | 'relogged' | 'expired' | string;
  message: string;
  taskCount: number;
  relogged: boolean;
  error?: string;
}

export interface SystemInfo {
  appVersion: string;
  apiVersion: string;
  serverTime: string;
  tokenTTLDay: number;
  quotaEnabled: boolean;
  features: string[];
}

export interface SignRange {
  taskName: string;
  signInstanceWid: string;
  lon: number;
  lat: number;
  radius: number;
  address: string;
  beginTime: string;
  endTime: string;
  applied: boolean;
}

export interface RunOutcome {
  logId: number;
  status: string;
  message: string;
  hint?: string;
  taskName: string;
}

/** 登录接口返回：done=true 直接成功；否则带验证码票据 */
export interface LoginChallenge {
  done: boolean;
  ticket?: string;
  needCaptcha?: boolean;
  captcha?: string;
  message?: string;
}

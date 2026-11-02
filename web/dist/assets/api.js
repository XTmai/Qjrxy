/* =========================================================
   小麦校园 · API 客户端
   ========================================================= */

const TOKEN_KEY = 'wheat_token';

export function getToken() {
  try { return localStorage.getItem(TOKEN_KEY) || ''; } catch { return ''; }
}

export function setToken(t) {
  try {
    if (t) localStorage.setItem(TOKEN_KEY, t);
    else localStorage.removeItem(TOKEN_KEY);
  } catch { /* 忽略 */ }
}

export class ApiError extends Error {
  constructor(message, status, code) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function request(method, path, body, opts = {}) {
  const headers = { Accept: 'application/json' };
  const token = getToken();
  if (token) headers.Authorization = 'Bearer ' + token;

  let payload;
  if (body instanceof FormData) {
    payload = body;
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    payload = JSON.stringify(body);
  }

  let resp;
  try {
    resp = await fetch('/api' + path, { method, headers, body: payload, credentials: 'same-origin' });
  } catch {
    throw new ApiError('网络请求失败，请检查服务是否运行', 0, 'network');
  }

  const text = await resp.text();
  let data = null;
  if (text) {
    try { data = JSON.parse(text); } catch { /* 非 JSON */ }
  }

  if (!resp.ok || (data && data.ok === false)) {
    const err = (data && data.error) || {};
    if (resp.status === 401 && !opts.keepAuth) {
      setToken('');
      window.dispatchEvent(new CustomEvent('wheat:logout'));
    }
    throw new ApiError(err.message || '请求失败（HTTP ' + resp.status + '）', resp.status, err.code);
  }
  return data ? data.data : null;
}

export const api = {
  // 认证
  login: (username, password) =>
    request('POST', '/auth/login', { username, password }, { keepAuth: true }),
  register: (username, password, nickname) =>
    request('POST', '/auth/register', { username, password, nickname }, { keepAuth: true }),
  logout: () => request('POST', '/auth/logout', {}),
  me: () => request('GET', '/auth/me', undefined, { keepAuth: true }),
  changePassword: (oldPassword, newPassword) =>
    request('POST', '/auth/password', { oldPassword, newPassword }),

  // 概览
  dashboard: () => request('GET', '/dashboard'),
  systemInfo: () => request('GET', '/system/info'),

  // 账号
  accounts: () => request('GET', '/accounts'),
  account: (id) => request('GET', '/accounts/' + id),
  createAccount: (d) => request('POST', '/accounts', d),
  updateAccount: (id, d) => request('PATCH', '/accounts/' + id, d),
  deleteAccount: (id) => request('DELETE', '/accounts/' + id),
  accountLogin: (id) => request('POST', '/accounts/' + id + '/login', {}),
  accountCaptcha: (id, ticket, code) =>
    request('POST', '/accounts/' + id + '/captcha', { ticket, code }),
  importCookies: (id, cookies) =>
    request('POST', '/accounts/' + id + '/cookies', { cookies }),
  verifyAccount: (id) => request('POST', '/accounts/' + id + '/verify', {}),
  uploadPhoto: (id, formData) => request('POST', '/accounts/' + id + '/photo', formData),

  // 设备信息（扫码）
  deviceTicket: (id) => request('POST', '/accounts/' + id + '/device-ticket', {}),
  devicePoll: (ticket) => request('GET', '/device/poll/' + encodeURIComponent(ticket)),

  // 签到位置（自动获取）
  signRange: (id) => request('GET', '/accounts/' + id + '/sign-range'),
  applySignRange: (id) => request('POST', '/accounts/' + id + '/sign-range', {}),

  // 任务
  tasks: () => request('GET', '/tasks'),
  syncTasks: (accountId) => request('POST', '/tasks/sync', { accountId }),
  runTask: (accountId, taskId, kind) =>
    request('POST', '/tasks/run', { accountId, taskId, kind }),

  // 计划
  plans: () => request('GET', '/plans'),
  createPlan: (d) => request('POST', '/plans', d),
  updatePlan: (id, d) => request('PATCH', '/plans/' + id, d),
  deletePlan: (id) => request('DELETE', '/plans/' + id),
  togglePlan: (id, enabled) => request('POST', '/plans/' + id + '/toggle', { enabled }),

  // 记录
  logs: (params = {}) => {
    const qs = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') qs.set(k, v);
    });
    return request('GET', '/logs' + (qs.toString() ? '?' + qs : ''));
  },
  log: (id) => request('GET', '/logs/' + id),

  // 其它
  schools: (q) => request('GET', '/schools?q=' + encodeURIComponent(q || '')),
  redeem: (code) => request('POST', '/quota/redeem', { code }),
};

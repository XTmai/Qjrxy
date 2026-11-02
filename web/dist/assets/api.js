/* =========================================================
   青衿前端 · 轻量 API 客户端
   ========================================================= */

const TOKEN_KEY = 'qj_token';

export function getToken() {
  try { return localStorage.getItem(TOKEN_KEY) || ''; } catch { return ''; }
}

export function setToken(token) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch { /* 隐私模式忽略 */ }
}

export class ApiError extends Error {
  constructor(message, status, code) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function request(method, path, body, options = {}) {
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
    resp = await fetch('/api' + path, {
      method, headers, body: payload,
      credentials: 'same-origin',
      signal: options.signal,
    });
  } catch (err) {
    if (err.name === 'AbortError') throw err;
    throw new ApiError('网络请求失败，请检查服务是否运行', 0, 'network');
  }

  const text = await resp.text();
  let json = null;
  if (text) {
    try { json = JSON.parse(text); } catch { /* 非 JSON */ }
  }

  if (!resp.ok || (json && json.ok === false)) {
    const err = (json && json.error) || {};
    if (resp.status === 401 && !options.keepAuth) {
      setToken('');
      window.dispatchEvent(new CustomEvent('qj:unauthorized'));
    }
    throw new ApiError(err.message || `请求失败（HTTP ${resp.status}）`, resp.status, err.code);
  }
  return json ? json.data : null;
}

export const api = {
  get: (path, options) => request('GET', path, undefined, options),
  post: (path, body, options) => request('POST', path, body, options),
  patch: (path, body, options) => request('PATCH', path, body, options),
  del: (path, options) => request('DELETE', path, undefined, options),
  upload: (path, formData, options) => request('POST', path, formData, options),

  // 认证
  me: () => request('GET', '/auth/me', undefined, { keepAuth: true }),
  login: (username, password) => request('POST', '/auth/login', { username, password }, { keepAuth: true }),
  register: (username, password, nickname) =>
    request('POST', '/auth/register', { username, password, nickname }, { keepAuth: true }),
  logout: () => request('POST', '/auth/logout', {}),

  // 工作台
  dashboard: () => request('GET', '/dashboard'),
  systemInfo: () => request('GET', '/system/info'),

  // 账号
  accounts: () => request('GET', '/accounts'),
  account: (id) => request('GET', '/accounts/' + id),
  createAccount: (data) => request('POST', '/accounts', data),
  updateAccount: (id, data) => request('PATCH', '/accounts/' + id, data),
  deleteAccount: (id) => request('DELETE', '/accounts/' + id),
  accountLogin: (id) => request('POST', `/accounts/${id}/login`, {}),
  accountCaptcha: (id, ticket, code) => request('POST', `/accounts/${id}/captcha`, { ticket, code }),
  importCookies: (id, cookies) => request('POST', `/accounts/${id}/cookies`, { cookies }),
  verifyAccount: (id) => request('POST', `/accounts/${id}/verify`, {}),
  deviceTicket: (id) => request('POST', `/accounts/${id}/device-ticket`, {}),
  devicePoll: (ticket) => request('GET', '/device/poll/' + encodeURIComponent(ticket)),
  signRange: (id) => request('GET', `/accounts/${id}/sign-range`),
  applySignRange: (id) => request('POST', `/accounts/${id}/sign-range`, {}),

  // 任务
  tasks: () => request('GET', '/tasks'),
  syncTasks: (accountId) => request('POST', '/tasks/sync', { accountId }),
  runTask: (accountId, taskId, kind) => request('POST', '/tasks/run', { accountId, taskId, kind }),

  // 计划
  plans: () => request('GET', '/plans'),
  createPlan: (data) => request('POST', '/plans', data),
  updatePlan: (id, data) => request('PATCH', '/plans/' + id, data),
  deletePlan: (id) => request('DELETE', '/plans/' + id),
  togglePlan: (id, enabled) => request('POST', `/plans/${id}/toggle`, { enabled }),

  // 记录
  logs: (params = {}) => {
    const qs = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') qs.set(k, v);
    });
    return request('GET', '/logs' + (qs.toString() ? '?' + qs : ''));
  },
  log: (id) => request('GET', '/logs/' + id),

  // 其他
  schools: (q) => request('GET', '/schools?q=' + encodeURIComponent(q || '')),
  redeem: (code) => request('POST', '/quota/redeem', { code }),
  changePassword: (oldPassword, newPassword) =>
    request('POST', '/auth/password', { oldPassword, newPassword }),
};

// 轻量 API 客户端：token 注入、统一错误信封、401 全局事件。

const TOKEN_KEY = 'qjrxy_token';

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) || '';
  } catch {
    return '';
  }
}

export function setToken(token: string) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* 隐私模式忽略 */
  }
}

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(message: string, status: number, code: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

/** 会话失效时派发；AuthProvider 监听并清空会话 */
export const UNAUTHORIZED_EVENT = 'qjrxy:unauthorized';

interface RequestOptions {
  keepAuth?: boolean;
  signal?: AbortSignal;
}

async function request<T>(method: string, path: string, body?: unknown, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;

  let payload: BodyInit | undefined;
  if (body instanceof FormData) {
    payload = body;
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    payload = JSON.stringify(body);
  }

  let resp: Response;
  try {
    resp = await fetch(`/api${path}`, { method, headers, body: payload, credentials: 'same-origin', signal: options.signal });
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err;
    throw new ApiError('网络请求失败，请检查服务是否运行', 0, 'network');
  }

  const text = await resp.text();
  let json: { ok?: boolean; data?: T; error?: { message?: string; code?: string } } | null = null;
  if (text) {
    try {
      json = JSON.parse(text);
    } catch {
      /* 非 JSON（如登录页 HTML） */
    }
  }

  if (!resp.ok || json?.ok === false) {
    const err = json?.error ?? {};
    if (resp.status === 401 && !options.keepAuth) {
      setToken('');
      window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT));
    }
    throw new ApiError(err.message || `请求失败（HTTP ${resp.status}）`, resp.status, err.code ?? '');
  }
  return (json ? json.data : null) as T;
}

export const http = {
  get: <T>(path: string, options?: RequestOptions) => request<T>('GET', path, undefined, options),
  post: <T>(path: string, body?: unknown, options?: RequestOptions) => request<T>('POST', path, body, options),
  patch: <T>(path: string, body?: unknown, options?: RequestOptions) => request<T>('PATCH', path, body, options),
  del: <T>(path: string, options?: RequestOptions) => request<T>('DELETE', path, undefined, options),
};

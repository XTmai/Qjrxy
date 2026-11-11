// SSE 客户端：用 fetch 读流式响应，才能带上 Authorization 头
//（浏览器原生 EventSource 不支持自定义请求头）。
//
// 只负责「连接 + 解析 + 退避重连」，事件语义由 use-live-updates 处理。
import { UNAUTHORIZED_EVENT, getToken } from "./client";
import type { LiveStatus } from "@/stores/live-store";

export interface LiveReadyInfo {
  appVersion?: string;
  serverTime?: string;
  retryMs?: number;
}

export interface LiveHandlers {
  /** 收到变更通知（主题名列表） */
  onInvalidate: (topics: string[]) => void;
  /** 每次连接就绪时触发；重连成功后可用于补齐断线期间漏掉的数据 */
  onReady?: (info: LiveReadyInfo) => void;
  onStatus?: (status: LiveStatus) => void;
}

const MAX_BACKOFF_MS = 30_000;
// 建连阶段最多等这么久拿响应头；一旦开始推流就不再计时。
const HANDSHAKE_TIMEOUT_MS = 12_000;

/** 订阅服务端变更流，返回取消函数。 */
export function subscribeLiveEvents(handlers: LiveHandlers): () => void {
  let stopped = false;
  let controller: AbortController | null = null;
  let retryTimer: number | undefined;
  let attempt = 0;

  const emitStatus = (status: LiveStatus) => handlers.onStatus?.(status);

  const scheduleReconnect = () => {
    if (stopped) return;
    attempt += 1;
    // 1s、2s、4s…… 封顶 30s，加一点抖动避免多标签页同时重连。
    const backoff = Math.min(MAX_BACKOFF_MS, 1000 * 2 ** Math.min(attempt - 1, 5));
    const delay = backoff + Math.random() * 400;
    emitStatus("offline");
    retryTimer = window.setTimeout(() => void connect(), delay);
  };

  const connect = async () => {
    if (stopped) return;
    emitStatus("connecting");

    const ac = new AbortController();
    controller = ac;
    let timedOut = false;
    const handshakeTimer = window.setTimeout(() => {
      timedOut = true;
      ac.abort();
    }, HANDSHAKE_TIMEOUT_MS);

    const headers: Record<string, string> = { Accept: "text/event-stream" };
    const token = getToken();
    if (token) headers.Authorization = `Bearer ${token}`;

    let resp: Response;
    try {
      resp = await fetch("/api/events", {
        headers,
        credentials: "same-origin",
        signal: ac.signal,
      });
    } catch {
      window.clearTimeout(handshakeTimer);
      if (stopped) return;
      scheduleReconnect();
      return;
    }
    window.clearTimeout(handshakeTimer);

    // 会话失效：交给全局 401 逻辑（清缓存、回登录页），不再重连。
    if (resp.status === 401) {
      window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT));
      emitStatus("offline");
      return;
    }
    if (!resp.ok || !resp.body) {
      if (timedOut || !stopped) scheduleReconnect();
      return;
    }

    attempt = 0;
    emitStatus("live");

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let boundary = buffer.indexOf("\n\n");
        while (boundary >= 0) {
          const frame = buffer.slice(0, boundary);
          buffer = buffer.slice(boundary + 2);
          handleFrame(frame, handlers);
          boundary = buffer.indexOf("\n\n");
        }
      }
    } catch {
      // 读流中断（服务重启、网络切换、代理超时）统一走重连。
    }

    if (stopped) return;
    controller = null;
    scheduleReconnect();
  };

  void connect();

  return () => {
    stopped = true;
    if (retryTimer !== undefined) window.clearTimeout(retryTimer);
    controller?.abort();
  };
}

/** 解析一个 SSE 事件块（event/data 字段，忽略注释行）。 */
function handleFrame(frame: string, handlers: LiveHandlers) {
  let event = "message";
  const data: string[] = [];

  for (const rawLine of frame.split("\n")) {
    const line = rawLine.endsWith("\r") ? rawLine.slice(0, -1) : rawLine;
    if (!line || line.startsWith(":")) continue;
    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field === "event") event = value;
    else if (field === "data") data.push(value);
  }
  if (!data.length) return;

  let payload: unknown;
  try {
    payload = JSON.parse(data.join("\n"));
  } catch {
    return;
  }

  if (event === "ready") {
    handlers.onReady?.((payload ?? {}) as LiveReadyInfo);
    return;
  }
  if (event === "invalidate") {
    const topics = (payload as { topics?: unknown })?.topics;
    if (Array.isArray(topics)) {
      handlers.onInvalidate(topics.filter((t): t is string => typeof t === "string" && t !== ""));
    }
  }
}

/* =========================================================
   青衿前端 · 通用工具（DOM、格式化、Toast、Modal）
   ========================================================= */

/** 创建元素：h('div', {class:'x'}, 'text' | node | [nodes]) */
export function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  if (attrs && typeof attrs === 'object' && !(attrs instanceof Node)) {
    for (const [key, value] of Object.entries(attrs)) {
      if (value === null || value === undefined || value === false) continue;
      if (key === 'class') el.className = value;
      else if (key === 'html') el.innerHTML = value;
      else if (key === 'text') el.textContent = value;
      else if (key === 'dataset') Object.assign(el.dataset, value);
      else if (key.startsWith('on') && typeof value === 'function') {
        el.addEventListener(key.slice(2).toLowerCase(), value);
      } else if (key === 'style' && typeof value === 'object') {
        Object.assign(el.style, value);
      } else if (value === true) el.setAttribute(key, '');
      else el.setAttribute(key, value);
    }
  }
  for (const child of children.flat(Infinity)) {
    if (child === null || child === undefined || child === false) continue;
    el.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return el;
}

/** 内联图标（Feather 风格 24 网格） */
const ICONS = {
  refresh: 'M21 12a9 9 0 1 1-2.6-6.4M21 4v5h-5',
  play: 'M8 5.2v13.6L19 12z',
  plus: 'M12 5v14M5 12h14',
  check: 'M4.5 12.5 9 17 19.5 6.5',
  x: 'M6 6l12 12M18 6 6 18',
  clock: 'M12 7v5l3.2 2M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z',
  shield: 'M12 3l7 3v6c0 4.4-3 8-7 9-4-1-7-4.6-7-9V6z',
  key: 'M15 7a4 4 0 1 1-3.5 6L9 15.5H7v2H5v2H3v-3.5l6.5-6.5A4 4 0 0 1 15 7z',
  camera: 'M4 8h3l1.5-2h7L17 8h3v11H4zM12 16.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7z',
  trash: 'M5 7h14M9 7V5h6v2M7 7l1 13h8l1-13',
  edit: 'M4 20h4L20 8l-4-4L4 16z',
  login: 'M14 5h4a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-4M10 8l-4 4 4 4M6 12h9',
  qr: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h2v2h-2zM18 14h2v2h-2zM14 18h2v2h-2zM18 18h2v2h-2z',
  users: 'M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM2 21c0-3.9 3.1-7 7-7s7 3.1 7 7M17 4.5a4 4 0 0 1 0 7.7M18 14.2c2.4.8 4 3 4 5.8',
  list: 'M5 6h14M5 12h14M5 18h9',
  pin: 'M12 21s7-5.4 7-11a7 7 0 1 0-14 0c0 5.6 7 11 7 11zM12 12.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z',
  link: 'M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1',
  alert: 'M12 8v5M12 17h.01M12 3l9 17H3z',
  info: 'M12 11v6M12 8h.01M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z',
  arrow: 'M5 12h14M13 6l6 6-6 6',
  logout: 'M14 5h4a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-4M10 8l-4 4 4 4M6 12h9',
  book: 'M4 5.5A2.5 2.5 0 0 1 6.5 3H20v18H6.5A2.5 2.5 0 0 1 4 18.5zM4 18.5A2.5 2.5 0 0 1 6.5 16H20',
};

export function icon(name, cls) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  if (cls) svg.setAttribute('class', cls);
  svg.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', ICONS[name] || ICONS.info);
  svg.append(path);
  return svg;
}

/* ---------------- Toast ---------------- */

let toastTimer = null;
export function toast(message, kind = '') {
  const el = document.getElementById('toast');
  if (!el) return;
  el.textContent = message;
  el.className = 'toast show ' + kind;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.className = 'toast ' + kind; }, 3200);
}

/* ---------------- Modal ---------------- */

const modalEl = () => document.getElementById('modal');

export function openModal({ title, body, footer, wide = false, onClose }) {
  const modal = modalEl();
  const card = modal.querySelector('.modal-card');
  card.classList.toggle('wide', wide);
  modal.querySelector('#modalTitle').textContent = title || '';
  const bodyEl = modal.querySelector('#modalBody');
  bodyEl.replaceChildren();
  if (body) bodyEl.append(body instanceof Node ? body : document.createTextNode(body));
  const footEl = modal.querySelector('#modalFoot');
  footEl.replaceChildren();
  if (footer) footEl.append(...(Array.isArray(footer) ? footer : [footer]));

  modal.hidden = false;
  modal.dataset.onClose = onClose ? '1' : '';
  modal._onClose = onClose || null;
  const focusable = card.querySelector('input, select, textarea, button:not([data-close])');
  if (focusable) setTimeout(() => focusable.focus(), 60);
}

export function closeModal() {
  const modal = modalEl();
  if (!modal || modal.hidden) return;
  const cb = modal._onClose;
  modal.hidden = true;
  modal._onClose = null;
  if (cb) cb();
}

export function initModal() {
  const modal = modalEl();
  if (!modal) return;
  // 用 closest 向上找带 data-close 的元素：
  // 点「×」时实际命中的是里面的 svg/path，直接读 e.target.dataset 会失效。
  modal.addEventListener('click', (e) => {
    const trigger = e.target instanceof Element ? e.target.closest('[data-close]') : null;
    if (trigger) {
      e.preventDefault();
      closeModal();
    }
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !modal.hidden) closeModal();
  });
}

/** 确认框，返回 Promise<boolean> */
export function confirmDialog({ title, message, confirmText = '确定', danger = false }) {
  return new Promise((resolve) => {
    let done = false;
    const finish = (value) => {
      if (done) return;
      done = true;
      resolve(value);
      closeModal();
    };
    const btn = h('button', {
      class: 'btn ' + (danger ? 'danger' : 'primary'),
      onclick: () => finish(true),
    }, confirmText);
    openModal({
      title,
      body: h('p', { class: 'hint-text' }, message),
      footer: [
        h('button', { class: 'btn ghost', onclick: () => finish(false) }, '取消'),
        btn,
      ],
      onClose: () => finish(false),
    });
  });
}

/* ---------------- 格式化 ---------------- */

const WEEK = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
export const WEEK_NAMES = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];

export function formatDate(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function formatDateTime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function formatClock(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function relativeTime(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const diff = Date.now() - d.getTime();
  const min = Math.floor(diff / 60000);
  if (min < 1) return '刚刚';
  if (min < 60) return `${min} 分钟前`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour} 小时前`;
  const day = Math.floor(hour / 24);
  if (day < 30) return `${day} 天前`;
  return formatDate(iso);
}

export function todayLabel(date = new Date()) {
  const m = date.getMonth() + 1;
  const d = date.getDate();
  return `${m}月${d}日${WEEK[date.getDay()]}`;
}

/** 后端返回的时间可能是 RFC3339 或 "2026-10-26 20:00:00"，统一成短格式 */
export function shortRange(start, end) {
  const norm = (v) => {
    if (!v) return '';
    const s = String(v).replace('T', ' ').replace(/\.\d+/, '').replace(/(Z|[+-]\d{2}:?\d{2})$/, '');
    return s.slice(0, 16);
  };
  const s = norm(start);
  const e = norm(end);
  if (!s && !e) return '时间待定';
  if (!e) return s;
  return `${s} → ${e}`;
}

const STATUS_MAP = {
  success: { label: '成功', cls: 'ok' },
  failed: { label: '失败', cls: 'err' },
  pending: { label: '结果待核对', cls: 'warn' },
  executing: { label: '执行中', cls: 'info' },
  done: { label: '已处理', cls: 'ok' },
  leave: { label: '已请假', cls: 'plain' },
};

export function statusBadge(status) {
  const meta = STATUS_MAP[status] || { label: status || '未知', cls: 'plain' };
  return h('span', { class: 'badge ' + meta.cls }, meta.label);
}

export function statusText(status) {
  const meta = STATUS_MAP[status];
  return meta ? meta.label : status;
}

const LOGIN_MAP = {
  authorized: { label: '已登录', cls: 'ok' },
  expired: { label: '需重新登录', cls: 'err' },
  unknown: { label: '未登录', cls: 'plain' },
  unauthorized: { label: '未授权', cls: 'warn' },
};

export function loginBadge(status) {
  const meta = LOGIN_MAP[status] || LOGIN_MAP.unknown;
  return h('span', { class: 'badge ' + meta.cls }, meta.label);
}

export function kindLabel(kind) {
  return { attendance: '查寝', sign: '签到', collect: '信息收集' }[kind] || kind || '任务';
}

/* ---------------- 其它 ---------------- */

export function initials(text) {
  const s = String(text || '?').trim();
  return s ? s.slice(0, 1) : '?';
}

export function debounce(fn, wait = 260) {
  let timer = null;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), wait);
  };
}

/** 复制到剪贴板，失败时回退到 execCommand */
export async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* 回退 */ }
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.style.position = 'fixed';
  ta.style.opacity = '0';
  document.body.append(ta);
  ta.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch { ok = false; }
  ta.remove();
  return ok;
}

export function skeletonCard(title = true) {
  const wrap = h('div', { class: 'card' });
  if (title) {
    wrap.append(h('div', { class: 'card-head' }, h('div', { class: 'skeleton', style: { width: '120px' } })));
  }
  wrap.append(h('div', { class: 'loading-block' },
    h('div', { class: 'skeleton', style: { width: '100%' } }),
    h('div', { class: 'skeleton', style: { width: '86%' } }),
    h('div', { class: 'skeleton', style: { width: '64%' } }),
  ));
  return wrap;
}

export function emptyState(title, hint, action) {
  return h('div', { class: 'empty' },
    h('strong', null, title),
    hint ? h('span', null, hint) : null,
    action ? h('div', { style: { marginTop: '16px' } }, action) : null,
  );
}

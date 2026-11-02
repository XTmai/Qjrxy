/* =========================================================
   小麦校园 · 基础工具
   ========================================================= */

/** 创建元素：el('div', {class:'x'}, 'text', child) */
export function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  if (attrs && typeof attrs === 'object' && !(attrs instanceof Node)) {
    for (const [key, val] of Object.entries(attrs)) {
      if (val === null || val === undefined || val === false) continue;
      if (key === 'class') node.className = val;
      else if (key === 'text') node.textContent = val;
      else if (key === 'html') node.innerHTML = val;
      else if (key === 'dataset') Object.assign(node.dataset, val);
      else if (key === 'style' && typeof val === 'object') Object.assign(node.style, val);
      else if (key.startsWith('on') && typeof val === 'function') {
        node.addEventListener(key.slice(2).toLowerCase(), val);
      } else if (val === true) node.setAttribute(key, '');
      else node.setAttribute(key, val);
    }
  }
  for (const child of children.flat(Infinity)) {
    if (child === null || child === undefined || child === false) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return node;
}

/* ---------------- 图标 ---------------- */

const PATHS = {
  refresh: 'M20 12a8 8 0 1 1-2.4-5.7M20 4v4.5h-4.5',
  play: 'M8 5.3v13.4L19 12z',
  plus: 'M12 5v14M5 12h14',
  check: 'M4.8 12.4 9 16.6 19.2 6.4',
  x: 'M6 6l12 12M18 6 6 18',
  clock: 'M12 7.5V12l3 2M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z',
  shield: 'M12 3.2 19 6v6c0 4.3-3 7.8-7 8.8-4-1-7-4.5-7-8.8V6z',
  key: 'M15.2 7a3.8 3.8 0 1 1-3.4 5.3L9 15.1H7.2v1.8H5.4V18.7H3.2v-3.2l6.4-6.4A3.8 3.8 0 0 1 15.2 7z',
  camera: 'M4 8.2h3l1.5-2h7l1.5 2h3v10.6H4zM12 16.6a3.4 3.4 0 1 0 0-6.8 3.4 3.4 0 0 0 0 6.8z',
  trash: 'M5 7h14M9.5 7V5h5v2M7.5 7l.9 12h7.2l.9-12',
  edit: 'M4 20h4L20 8l-4-4L4 16z',
  login: 'M14 5h4a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-4M10 8l-4 4 4 4M6 12h9',
  qr: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14.5 14.5h2v2h-2zM17.5 17.5h2v2h-2z',
  users: 'M9 11a3.8 3.8 0 1 0 0-7.6 3.8 3.8 0 0 0 0 7.6zM2.5 20.5c0-3.6 2.9-6.5 6.5-6.5s6.5 2.9 6.5 6.5',
  pin: 'M12 21s7-5.5 7-11a7 7 0 1 0-14 0c0 5.5 7 11 7 11zM12 12.4a2.4 2.4 0 1 0 0-4.8 2.4 2.4 0 0 0 0 4.8z',
  alert: 'M12 8.5v5M12 17h.01M12 3.2 21 20H3z',
  info: 'M12 11v5.5M12 8h.01M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z',
  logout: 'M14 5h4a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-4M10 8l-4 4 4 4M6 12h9',
  wheat: 'M12 21V9M12 9c0-2.5 1.5-4.5 3.5-5.5C15.5 6 14 8 12 9zM12 9c0-2.5-1.5-4.5-3.5-5.5C8.5 6 10 8 12 9z',
  link: 'M10 13.5a4.5 4.5 0 0 0 6.4 0l1.8-1.8a4.5 4.5 0 0 0-6.4-6.4l-1 1M14 10.5a4.5 4.5 0 0 0-6.4 0l-1.8 1.8a4.5 4.5 0 0 0 6.4 6.4l1-1',
  copy: 'M9 9V5.5h9.5V15H15M5.5 9H15v9.5H5.5z',
  arrow: 'M5 12h13M13 6.5l5.5 5.5-5.5 5.5',
};

export function ico(name, cls) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  if (cls) svg.setAttribute('class', cls);
  svg.setAttribute('aria-hidden', 'true');
  const p = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  p.setAttribute('d', PATHS[name] || PATHS.info);
  svg.append(p);
  return svg;
}

/* ---------------- Toast ---------------- */

let toastTimer = null;
export function toast(msg, kind = '') {
  const node = document.getElementById('toast');
  if (!node) return;
  node.textContent = msg;
  node.className = 'toast show ' + kind;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { node.className = 'toast ' + kind; }, 3000);
}

/* ---------------- 弹窗 ---------------- */

let modalCloseCb = null;

export function openModal({ title, sub, body, actions, wide = false, onClose }) {
  const modal = document.getElementById('modal');
  const box = modal.querySelector('.modal-box');
  box.classList.toggle('wide', wide);
  document.getElementById('modalTitle').textContent = title || '';
  const subEl = document.getElementById('modalSub');
  subEl.textContent = sub || '';
  subEl.hidden = !sub;

  const bodyEl = document.getElementById('modalBody');
  bodyEl.replaceChildren();
  if (body) bodyEl.append(...(Array.isArray(body) ? body : [body]));

  const actsEl = document.getElementById('modalActs');
  actsEl.replaceChildren();
  if (actions) actsEl.append(...(Array.isArray(actions) ? actions : [actions]));

  modal.hidden = false;
  modalCloseCb = onClose || null;
  const focusTarget = bodyEl.querySelector('input:not([type=file]), select, textarea');
  if (focusTarget) setTimeout(() => focusTarget.focus(), 80);
}

export function closeModal() {
  const modal = document.getElementById('modal');
  if (!modal || modal.hidden) return;
  const cb = modalCloseCb;
  modal.hidden = true;
  modalCloseCb = null;
  if (cb) cb();
}

export function initModal() {
  const modal = document.getElementById('modal');
  if (!modal) return;
  // 用 closest 向上找，避免点在 svg 上时读不到 data-close
  modal.addEventListener('click', (e) => {
    const target = e.target instanceof Element ? e.target.closest('[data-close]') : null;
    if (target) {
      e.preventDefault();
      closeModal();
    }
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !modal.hidden) closeModal();
  });
}

/** 确认框 */
export function confirmBox({ title, message, okText = '确定', danger = false }) {
  return new Promise((resolve) => {
    let settled = false;
    const done = (val) => {
      if (settled) return;
      settled = true;
      resolve(val);
      closeModal();
    };
    openModal({
      title,
      body: el('p', { class: 'confirm-text' }, message),
      actions: [
        el('button', { class: 'btn plain', onclick: () => done(false) }, '取消'),
        el('button', {
          class: 'btn ' + (danger ? 'danger' : 'main'),
          onclick: () => done(true),
        }, okText),
      ],
      onClose: () => done(false),
    });
  });
}

/* ---------------- 格式化 ---------------- */

const WEEK = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];

export function fmtDate(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export function fmtTime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

export function todayText(d = new Date()) {
  return `${d.getMonth() + 1}月${d.getDate()}日 ${WEEK[d.getDay()]}`;
}

/** 时间范围：优先用日期+时段，否则用起止时间 */
export function timeRange(item) {
  if (item.signDate || item.beginClock) {
    const date = (item.signDate || '').split(' ')[0] || '';
    const clock = item.beginClock && item.endClock
      ? `${item.beginClock} – ${item.endClock}`
      : (item.beginClock || '');
    return [date, clock].filter(Boolean).join('  ');
  }
  const s = (item.startTime || '').replace('T', ' ').slice(0, 16);
  const e = (item.endTime || '').replace('T', ' ').slice(0, 16);
  if (!s && !e) return '时间待定';
  if (!e) return s;
  return `${s} → ${e.slice(-5)}`;
}

/* ---------------- 状态 ---------------- */

const LOG_STATE = {
  success: { text: '成功', cls: 'ok' },
  failed: { text: '失败', cls: 'err' },
  pending: { text: '待核对', cls: 'warn' },
  executing: { text: '执行中', cls: 'info' },
  done: { text: '已处理', cls: 'ok' },
  leave: { text: '已请假', cls: 'plain' },
};

export function logTag(status) {
  const m = LOG_STATE[status] || { text: status || '未知', cls: 'plain' };
  return el('span', { class: 'tag ' + m.cls }, m.text);
}

const ACCT_STATE = {
  authorized: { text: '已登录', cls: 'ok' },
  expired: { text: '需重新登录', cls: 'err' },
  unknown: { text: '未登录', cls: 'plain' },
};

export function accountTag(status) {
  const m = ACCT_STATE[status] || ACCT_STATE.unknown;
  return el('span', { class: 'tag ' + m.cls }, m.text);
}

/** 任务可执行状态（只做查寝） */
export function taskState(task) {
  if (task.status === 'done') return { text: '已处理', cls: 'ok', runnable: false };
  const st = String(task.signStatus || '').trim();
  if (st === '2') return { text: '未到时间', cls: 'warn', runnable: false };
  if (st === '3') return { text: '已过期', cls: 'plain', runnable: false };
  if (st === '4') return { text: '已签到', cls: 'ok', runnable: false };
  return { text: '可执行', cls: 'info', runnable: true };
}

/* ---------------- 其它 ---------------- */

export function initial(text) {
  const s = String(text || '小').trim();
  return s ? s.slice(0, 1) : '小';
}

export function debounce(fn, wait = 260) {
  let t = null;
  return (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), wait);
  };
}

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

export function skeleton() {
  return el('div', { class: 'card' },
    el('div', { class: 'skel-group' },
      el('div', { class: 'skel', style: { width: '32%' } }),
      el('div', { class: 'skel', style: { width: '100%' } }),
      el('div', { class: 'skel', style: { width: '78%' } }),
    ),
  );
}

export function emptyBox(title, hint, action) {
  return el('div', { class: 'empty' },
    el('div', { class: 'empty-icon' }, ico('wheat')),
    el('strong', null, title),
    hint ? el('div', null, hint) : null,
    action ? el('div', { style: { marginTop: '14px' } }, action) : null,
  );
}

/** 安全替换子节点：过滤 null/false，避免变成字符串 "null" */
export function setChildren(node, ...children) {
  node.replaceChildren();
  for (const c of children.flat(Infinity)) {
    if (c === null || c === undefined || c === false) continue;
    node.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return node;
}

/** 只读键值列表 */
export function kvList(rows) {
  return el('div', { class: 'kv-list' },
    rows.filter(Boolean).map(([k, v, mono]) =>
      el('div', { class: 'kv-row' },
        el('span', { class: 'kv-key' }, k),
        el('b', { class: 'kv-val' + (mono ? ' mono' : ''), title: String(v) }, String(v)),
      ),
    ),
  );
}

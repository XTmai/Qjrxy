/* =========================================================
   青衿 · 组件层
   元素工厂 + 页面级组件 + 统一状态词汇表
   ========================================================= */

/* ---------------- 元素工厂 ---------------- */

/** h('div', {class:'x'}, 'text' | node | [nodes]) */
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
  appendChildren(el, children);
  return el;
}

/** 追加子节点，自动过滤 null/undefined/false（避免渲染出 "null" 文本） */
export function appendChildren(el, children) {
  for (const child of children.flat(Infinity)) {
    if (child === null || child === undefined || child === false) continue;
    el.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return el;
}

/** 替换子节点，同样过滤空值 */
export function setChildren(el, ...children) {
  el.replaceChildren();
  return appendChildren(el, children);
}

/* ---------------- 图标 ---------------- */

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

/**
 * 提示条
 * kind: '' | 'ok' | 'err' | 'info'
 */
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

/**
 * 打开弹窗
 * @param {object} opt
 * @param {string} opt.title
 * @param {string} [opt.sub]      标题下的说明
 * @param {Node|Node[]} [opt.body]
 * @param {Node|Node[]} [opt.footer]
 * @param {boolean} [opt.wide]
 * @param {Function} [opt.onClose]
 */
export function openModal({ title, sub, body, footer, wide = false, onClose }) {
  const modal = modalEl();
  if (!modal) return;
  const card = modal.querySelector('.modal-card');
  card.classList.toggle('wide', wide);

  modal.querySelector('#modalTitle').textContent = title || '';
  const subEl = modal.querySelector('#modalSub');
  if (subEl) {
    subEl.textContent = sub || '';
    subEl.hidden = !sub;
  }

  const bodyEl = modal.querySelector('#modalBody');
  bodyEl.replaceChildren();
  if (body) appendChildren(bodyEl, Array.isArray(body) ? body : [body]);

  const footEl = modal.querySelector('#modalFoot');
  footEl.replaceChildren();
  if (footer) appendChildren(footEl, Array.isArray(footer) ? footer : [footer]);

  modal.hidden = false;
  modal._onClose = onClose || null;

  const focusable = card.querySelector('input:not([type=file]), select, textarea, button:not([data-close])');
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
  // 用 closest 向上找：点「×」时实际命中的是内部 svg/path
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
    openModal({
      title,
      body: h('p', { class: 'confirm-text' }, message),
      footer: [
        h('button', { class: 'btn ghost', onclick: () => finish(false) }, '取消'),
        h('button', {
          class: 'btn ' + (danger ? 'danger' : 'primary'),
          onclick: () => finish(true),
        }, confirmText),
      ],
      onClose: () => finish(false),
    });
  });
}

/* ---------------- 状态词汇表（单一来源） ---------------- */

/**
 * 执行记录状态
 * 后端 status: success / failed / pending / executing
 */
const RUN_STATE = {
  success: { text: '成功', tone: 'ok' },
  failed: { text: '失败', tone: 'err' },
  pending: { text: '待核对', tone: 'warn' },
  executing: { text: '执行中', tone: 'info' },
};

/**
 * 任务状态
 * 后端 task.status: pending / done / leave
 * 后端 task.signStatus: 0/1 可签到 | 2 未到时间 | 3 已过期 | 4 已签到 | 空 无限制
 */
export function taskState(task) {
  if (!task) return { text: '未知', tone: 'plain', runnable: false };
  if (task.status === 'done') return { text: '已处理', tone: 'ok', runnable: false };
  if (task.status === 'leave') return { text: '已请假', tone: 'plain', runnable: false };
  const st = String(task.signStatus || '').trim();
  if (st === '2') return { text: '未到时间', tone: 'warn', runnable: false };
  if (st === '3') return { text: '已过期', tone: 'plain', runnable: false };
  if (st === '4') return { text: '已签到', tone: 'ok', runnable: false };
  return { text: '可执行', tone: 'info', runnable: true };
}

/** 账号登录状态 */
const LOGIN_STATE = {
  authorized: { text: '已登录', tone: 'ok' },
  expired: { text: '需重新登录', tone: 'err' },
  unknown: { text: '未登录', tone: 'plain' },
  unauthorized: { text: '未授权', tone: 'warn' },
};

/** 徽标：badge('run'|'task'|'login', value) */
export function badge(type, value) {
  let meta;
  if (type === 'task') meta = taskState(value).text
    ? { text: taskState(value).text, tone: taskState(value).tone }
    : { text: '未知', tone: 'plain' };
  else if (type === 'login') meta = LOGIN_STATE[value] || LOGIN_STATE.unknown;
  else meta = RUN_STATE[value] || { text: value || '未知', tone: 'plain' };
  return h('span', { class: 'badge ' + meta.tone }, meta.text);
}

/* 兼容旧调用 */
export function statusBadge(status) { return badge('run', status); }
export function loginBadge(status) { return badge('login', status); }
export function statusText(status) {
  return (RUN_STATE[status] || {}).text || status;
}

export function kindLabel(kind) {
  return { attendance: '查寝', sign: '签到', collect: '信息收集' }[kind] || kind || '任务';
}

/* ---------------- 页面级组件 ---------------- */

/**
 * 页头
 * @param {object} opt
 * @param {string} opt.title
 * @param {string} [opt.sub]
 * @param {Node[]} [opt.actions]
 */
export function pageHeader({ title, sub, actions }) {
  return h('div', { class: 'page-head' },
    h('div', { class: 'page-head-text' },
      h('h1', null, title),
      sub ? h('p', { class: 'page-sub' }, sub) : null,
    ),
    actions && actions.length ? h('div', { class: 'page-actions' }, actions) : null,
  );
}

/**
 * 卡片
 * @param {object} opt
 * @param {string} [opt.title]
 * @param {string} [opt.note]     标题下的说明
 * @param {Node}   [opt.action]   头部右侧操作
 * @param {Node|Node[]} [opt.body]
 * @param {boolean}[opt.pad]      body 自带内边距（默认 false，用 card-body）
 * @param {Node[]} [opt.tools]    头部右侧多个操作
 */
export function card({ title, note, action, tools, body, pad = false }) {
  const wrap = h('div', { class: 'card' });
  if (title) {
    wrap.append(h('div', { class: 'card-head' },
      h('div', null,
        h('h2', null, title),
        note ? h('p', { class: 'hint' }, note) : null,
      ),
      action || (tools && tools.length ? h('div', { class: 'card-tools' }, tools) : null),
    ));
  }
  if (body) {
    const nodes = Array.isArray(body) ? body : [body];
    if (pad) wrap.append(h('div', { class: 'card-body' }, nodes));
    else appendChildren(wrap, nodes);
  }
  return wrap;
}

/**
 * 统计卡组
 * @param {Array<{label,value,note,tone}>} items
 * @param {number} [cols]
 */
export function statGrid(items, cols) {
  const grid = h('div', { class: 'stat-row' });
  if (cols) grid.style.gridTemplateColumns = 'repeat(' + cols + ', minmax(0, 1fr))';
  items.forEach((it) => {
    grid.append(h('div', { class: 'stat' },
      h('div', { class: 'stat-label' },
        h('span', null, it.label),
        h('span', { class: 'stat-dot ' + (it.tone || '') }),
      ),
      h('div', { class: 'stat-value' }, String(it.value ?? 0)),
      it.note ? h('div', { class: 'stat-foot' }, it.note) : null,
    ));
  });
  return grid;
}

/**
 * 键值列表
 * @param {Array<[string, Node|string, boolean?]>} rows  [label, value, mono?]
 */
export function kvList(rows) {
  const list = h('dl', { class: 'kv-list' });
  rows.filter(Boolean).forEach(([label, value, mono]) => {
    list.append(h('div', { class: 'kv-row' },
      h('dt', null, label),
      h('dd', { class: mono ? 'mono' : '' }, value instanceof Node ? value : String(value)),
    ));
  });
  return list;
}

/**
 * 数据表格
 * @param {object} opt
 * @param {Array<{key,label,align?,render?}>} opt.columns
 * @param {Array<object>} opt.rows
 * @param {Node|string} [opt.empty]
 * @param {boolean} [opt.stack]  移动端堆叠为卡片（默认 true）
 */
export function dataTable({ columns, rows, empty, stack = true }) {
  if (!rows || rows.length === 0) {
    return h('div', null, empty || emptyState('暂无数据'));
  }

  const table = h('table', { class: 'data' + (stack ? ' stack' : '') });
  table.append(h('thead', null, h('tr', null,
    columns.map((c) => h('th', { class: c.align === 'right' ? 'col-right' : '' }, c.label)),
  )));

  const tbody = h('tbody');
  rows.forEach((row) => {
    const tr = h('tr');
    columns.forEach((c) => {
      const content = c.render ? c.render(row) : row[c.key];
      tr.append(h('td', {
        class: c.align === 'right' ? 'col-right' : '',
        'data-label': c.label,
      }, content instanceof Node ? content : String(content ?? '—')));
    });
    tbody.append(tr);
  });
  table.append(tbody);

  return h('div', { class: 'table-wrap' }, table);
}

/**
 * 工具栏
 * @param {Node|Node[]} left
 * @param {Node|Node[]} [right]
 */
export function toolbar(left, right) {
  return h('div', { class: 'toolbar' },
    h('div', { class: 'toolbar-left' }, left),
    right ? h('div', { class: 'toolbar-right' }, right) : null,
  );
}

/**
 * 表单字段
 * @param {object} opt
 * @param {string} [opt.label]
 * @param {Node} opt.input
 * @param {string} [opt.hint]
 * @param {boolean} [opt.required]
 */
export function formField({ label, input, hint, required }) {
  let hintNode = null;
  if (hint) {
    // hint 可以是字符串，也可以是外部维护的 Node（便于动态更新文案）
    hintNode = hint instanceof Node ? hint : h('span', { class: 'hint' }, hint);
  }
  return h('label', { class: 'field' },
    label ? h('span', null, label, required ? h('i', { class: 'req' }, '*') : null) : null,
    input,
    hintNode,
  );
}

/**
 * 标签页
 * @param {object} opt
 * @param {Array<[string,string]>} opt.items  [[key, label]]
 * @param {string} opt.active
 * @param {Function} opt.onSelect
 */
export function tabs({ items, active, onSelect }) {
  const bar = h('div', { class: 'tabs' });
  items.forEach(([key, label]) => {
    bar.append(h('button', {
      class: active === key ? 'active' : '',
      dataset: { tab: key },
      onclick: () => onSelect(key),
    }, label));
  });
  return bar;
}

/**
 * 分段控件
 * @param {object} opt
 * @param {Array<[string,string]>} opt.items
 * @param {string} opt.active
 * @param {Function} opt.onSelect
 */
export function segmented({ items, active, onSelect }) {
  const wrap = h('div', { class: 'segmented' });
  items.forEach(([key, label]) => {
    wrap.append(h('button', {
      class: active === key ? 'active' : '',
      dataset: { seg: key },
      onclick: () => onSelect(key),
    }, label));
  });
  return wrap;
}

/**
 * 三态：加载 / 空 / 错误
 */
export function loadingState(text) {
  return h('div', { class: 'state-box' },
    h('div', { class: 'spinner' }),
    text ? h('p', null, text) : null,
  );
}

export function emptyState(title, hint, action) {
  return h('div', { class: 'state-box' },
    h('div', { class: 'state-icon' }, icon('info')),
    h('strong', null, title),
    hint ? h('p', null, hint) : null,
    action ? h('div', { class: 'state-action' }, action) : null,
  );
}

export function errorState(title, err, action) {
  return h('div', { class: 'state-box err' },
    h('div', { class: 'state-icon' }, icon('alert')),
    h('strong', null, title),
    h('p', null, err?.message || String(err || '未知错误')),
    action ? h('div', { class: 'state-action' }, action) : null,
  );
}

/* ---------------- 格式化 ---------------- */

const WEEK = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
export const WEEK_NAMES = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];

const pad2 = (n) => String(n).padStart(2, '0');

export function formatDate(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

export function formatDateTime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

export function formatClock(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
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
  return `${date.getMonth() + 1}月${date.getDate()}日${WEEK[date.getDay()]}`;
}

/**
 * 任务时间范围
 * 优先用「日期 + 时段」（周期性任务），否则用起止时间
 */
export function timeRange(item) {
  if (!item) return '—';
  if (item.signDate || item.beginClock) {
    const date = String(item.signDate || '').split(' ')[0];
    const clock = item.beginClock && item.endClock
      ? `${item.beginClock} – ${item.endClock}`
      : (item.beginClock || '');
    return [date, clock].filter(Boolean).join('  ') || '时间待定';
  }
  return shortRange(item.startTime, item.endTime);
}

/** 兼容旧调用 */
export function shortRange(start, end) {
  const norm = (v) => {
    if (!v) return '';
    return String(v).replace('T', ' ').replace(/\.\d+/, '')
      .replace(/(Z|[+-]\d{2}:?\d{2})$/, '').slice(0, 16);
  };
  const s = norm(start);
  const e = norm(end);
  if (!s && !e) return '时间待定';
  if (!e) return s;
  return `${s} → ${e}`;
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

/** 兼容旧调用 */
export function skeletonCard() {
  return loadingState();
}

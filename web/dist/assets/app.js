/* =========================================================
   小麦校园 · 主入口
   ========================================================= */
import { api, getToken, setToken } from './api.js';
import { el, toast, initModal, closeModal, initial } from './ui.js';

import { pageHome } from './page-home.js';
import { pageAccounts } from './page-accounts.js';
import { pageTasks } from './page-tasks.js';
import { pagePlans } from './page-plans.js';
import { pageLogs } from './page-logs.js';
import { pageQuota } from './page-quota.js';
import { pageSettings } from './page-settings.js';

const ROUTES = {
  home: { title: '概览', page: pageHome },
  accounts: { title: '校园账号', page: pageAccounts },
  tasks: { title: '查寝任务', page: pageTasks },
  plans: { title: '自动计划', page: pagePlans },
  logs: { title: '执行记录', page: pageLogs },
  quota: { title: '额度', page: pageQuota },
  settings: { title: '设置', page: pageSettings },
};

const THEME_KEY = 'wheat_theme';

const dom = {
  gate: document.getElementById('gate'),
  app: document.getElementById('app'),
  page: document.getElementById('page'),
  nav: document.getElementById('nav'),
  tabbar: document.getElementById('tabbar'),
  topTitle: document.getElementById('topTitle'),
  topSub: document.getElementById('topSub'),
  sideName: document.getElementById('sideName'),
  sideQuota: document.getElementById('sideQuota'),
  sideAvatar: document.getElementById('sideAvatar'),
  topAvatar: document.getElementById('topAvatar'),
};

let currentUser = null;

const ctx = {
  go(path) {
    const hash = path.startsWith('#') ? path : '#' + path;
    if (location.hash === hash) route();
    else location.hash = hash;
  },
  logout,
  setTheme,
  refreshUser,
  user: () => currentUser,
};

/* ---------------- 主题 ---------------- */

function firstTheme() {
  let saved = '';
  try { saved = localStorage.getItem(THEME_KEY) || ''; } catch { /* 忽略 */ }
  if (saved === 'light' || saved === 'dark') return saved;
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function setTheme(mode) {
  document.documentElement.dataset.theme = mode;
  const label = document.getElementById('themeText');
  if (label) label.textContent = mode === 'dark' ? '深色' : '浅色';
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', mode === 'dark' ? '#1a1713' : '#f7f3ec');
  try { localStorage.setItem(THEME_KEY, mode); } catch { /* 忽略 */ }
}

/* ---------------- 登录 ---------------- */

function showGate() {
  dom.gate.hidden = false;
  dom.app.hidden = true;
}

function showApp(user) {
  currentUser = user;
  dom.gate.hidden = true;
  dom.app.hidden = false;
  paintUser(user);
}

function paintUser(user) {
  if (!user) return;
  const name = user.nickname || user.username || '用户';
  const left = Math.max(0, (user.quota || 0) - (user.quotaUsed || 0));
  if (dom.sideName) dom.sideName.textContent = name;
  if (dom.sideQuota) dom.sideQuota.textContent = left + ' 次可用';
  if (dom.sideAvatar) dom.sideAvatar.textContent = initial(name);
  if (dom.topAvatar) dom.topAvatar.textContent = initial(name);
}

async function refreshUser() {
  try {
    const { user } = await api.me();
    currentUser = user;
    paintUser(user);
  } catch { /* 忽略 */ }
}

async function logout() {
  try { await api.logout(); } catch { /* 忽略 */ }
  setToken('');
  currentUser = null;
  closeModal();
  showGate();
  toast('已退出登录');
}

/* ---------------- 路由 ---------------- */

function currentRoute() {
  const raw = location.hash.replace(/^#\/?/, '').split('?')[0].toLowerCase();
  return ROUTES[raw] ? raw : 'home';
}

async function route() {
  closeModal();
  const name = currentRoute();
  const def = ROUTES[name];

  document.querySelectorAll('[data-route]').forEach((node) => {
    node.classList.toggle('on', node.dataset.route === name);
  });
  if (dom.topTitle) dom.topTitle.textContent = def.title;
  document.title = def.title + ' · 小麦校园';

  dom.page.replaceChildren();
  try {
    await def.page(dom.page, ctx);
  } catch (err) {
    dom.page.replaceChildren(
      el('div', { class: 'head' }, el('h1', null, def.title)),
      el('div', { class: 'empty' },
        el('strong', null, '页面加载失败'),
        el('div', null, err.message || String(err)),
      ),
    );
  }
  window.scrollTo({ top: 0 });
}

function setSub(text) {
  if (dom.topSub) dom.topSub.textContent = text || '';
}

/* ---------------- 登录表单 ---------------- */

function initGate() {
  const form = document.getElementById('gateForm');
  const err = document.getElementById('gateErr');
  const submit = document.getElementById('gateSubmit');
  const nickField = document.getElementById('gateNickField');
  const pass = document.getElementById('gatePass');
  let mode = 'login';

  document.querySelectorAll('.gate-tabs button').forEach((btn) => {
    btn.addEventListener('click', () => {
      mode = btn.dataset.mode;
      document.querySelectorAll('.gate-tabs button').forEach((b) =>
        b.classList.toggle('on', b === btn));
      nickField.hidden = mode !== 'register';
      submit.textContent = mode === 'register' ? '注册并登录' : '登录';
      pass.autocomplete = mode === 'register' ? 'new-password' : 'current-password';
      err.textContent = '';
    });
  });

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    err.textContent = '';
    const username = document.getElementById('gateUser').value.trim();
    const password = pass.value;
    const nickname = document.getElementById('gateNick').value.trim();

    if (username.length < 3) { err.textContent = '用户名至少 3 位'; return; }
    if (password.length < 6) { err.textContent = '密码至少 6 位'; return; }

    submit.disabled = true;
    submit.textContent = '处理中…';
    try {
      const res = mode === 'register'
        ? await api.register(username, password, nickname)
        : await api.login(username, password);
      setToken(res.token);
      showApp(res.user);
      form.reset();
      if (!location.hash) location.hash = '#/home';
      await route();
      toast(mode === 'register' ? '注册成功' : '登录成功', 'ok');
    } catch (e2) {
      err.textContent = e2.message;
    } finally {
      submit.disabled = false;
      submit.textContent = mode === 'register' ? '注册并登录' : '登录';
    }
  });
}

/* ---------------- 启动 ---------------- */

async function boot() {
  setTheme(firstTheme());
  initModal();
  initGate();

  document.getElementById('themeBtn')?.addEventListener('click', () => {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
  });
  document.getElementById('logoutBtn')?.addEventListener('click', logout);

  window.addEventListener('hashchange', route);
  window.addEventListener('wheat:logout', () => {
    currentUser = null;
    showGate();
  });

  if (!getToken()) {
    showGate();
    return;
  }

  try {
    const { user } = await api.me();
    showApp(user);
    await route();
  } catch {
    setToken('');
    showGate();
  }
}

boot();

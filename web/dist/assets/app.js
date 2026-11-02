/* =========================================================
   青衿前端主入口：路由、会话、主题
   ========================================================= */
import { api, getToken, setToken } from './api.js';
import { h, toast, initModal, closeModal } from './ui.js';

import { renderDashboard } from './page-dashboard.js';
import { renderAccounts } from './page-accounts.js';
import { renderTasks } from './page-tasks.js';
import { renderLogs } from './page-logs.js';
import { renderQuota } from './page-quota.js';
import { renderSecurity } from './page-security.js';

const ROUTES = {
  dashboard: { title: '工作台', render: renderDashboard },
  accounts: { title: '学生账号', render: renderAccounts },
  tasks: { title: '任务执行', render: renderTasks },
  logs: { title: '执行记录', render: renderLogs },
  quota: { title: '额度与卡密', render: renderQuota },
  security: { title: '账号与安全', render: renderSecurity },
};

const THEME_KEY = 'qj_theme';
const contentEl = document.getElementById('content');
const appEl = document.getElementById('app');
const authEl = document.getElementById('authScreen');

let currentUser = null;
let currentRoute = '';

const ctx = {
  go(path) {
    const target = path.startsWith('#') ? path : '#' + path;
    if (location.hash === target) {
      route();
    } else {
      location.hash = target;
    }
  },
  logout,
  setTheme,
  refreshUser,
};

/* ---------------- 主题 ---------------- */

function initialTheme() {
  let saved = '';
  try { saved = localStorage.getItem(THEME_KEY) || ''; } catch { /* 忽略 */ }
  if (saved === 'light' || saved === 'dark') return saved;
  return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function setTheme(mode) {
  document.documentElement.dataset.theme = mode;
  document.body.dataset.theme = mode;
  try { localStorage.setItem(THEME_KEY, mode); } catch { /* 忽略 */ }
  const label = document.getElementById('themeLabel');
  if (label) label.textContent = mode === 'dark' ? '深色' : '浅色';
  const meta = document.querySelector('meta[name="color-scheme"]');
  if (meta) meta.setAttribute('content', mode === 'dark' ? 'dark' : 'light');
}

function toggleTheme() {
  const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
  setTheme(next);
}

/* ---------------- 认证 ---------------- */

function showAuth() {
  authEl.hidden = false;
  appEl.hidden = true;
  document.body.classList.add('auth-mode');
}

function showApp(user) {
  currentUser = user;
  authEl.hidden = true;
  appEl.hidden = false;
  document.body.classList.remove('auth-mode');
  updateUserChrome(user);
}

function updateUserChrome(user) {
  if (!user) return;
  const name = user.nickname || user.username;
  const remaining = Math.max(0, (user.quota || 0) - (user.quotaUsed || 0));
  setText('userName', name);
  setText('chipName', name);
  setText('userQuota', `${remaining} 次`);
  setText('userAvatar', name.slice(0, 1));
  setText('chipAvatar', name.slice(0, 1));
}

function setText(id, value) {
  const el = document.getElementById(id);
  if (el) el.textContent = value;
}

async function refreshUser() {
  try {
    const { user } = await api.me();
    updateUserChrome(user);
  } catch { /* 忽略 */ }
}

async function logout() {
  try { await api.logout(); } catch { /* 忽略 */ }
  setToken('');
  currentUser = null;
  showAuth();
  toast('已退出登录');
}

/* ---------------- 路由 ---------------- */

function parseHash() {
  const raw = location.hash.replace(/^#\/?/, '');
  const [path] = raw.split('?');
  return (path || 'dashboard').toLowerCase();
}

async function route() {
  closeModal();
  const name = ROUTES[parseHash()] ? parseHash() : 'dashboard';
  currentRoute = name;
  const { title, render } = ROUTES[name];

  document.querySelectorAll('[data-nav]').forEach((el) => {
    el.classList.toggle('active', el.dataset.nav === name);
  });
  setText('crumbCurrent', title);
  document.title = `${title} · 青衿`;

  contentEl.replaceChildren();
  try {
    await render(contentEl, ctx);
  } catch (err) {
    contentEl.replaceChildren(
      h('div', { class: 'page-head' }, h('div', null, h('h1', null, title))),
      h('div', { class: 'empty' },
        h('strong', null, '页面渲染出错'),
        h('span', null, err.message || String(err)),
      ),
    );
  }
  contentEl.scrollTop = 0;
  window.scrollTo({ top: 0, behavior: 'instant' in window ? 'instant' : 'auto' });
}

/* ---------------- 登录表单 ---------------- */

function initAuthForm() {
  const form = document.getElementById('authForm');
  const nicknameField = document.getElementById('nicknameField');
  const errorEl = document.getElementById('authError');
  const submitBtn = document.getElementById('authSubmit');
  let mode = 'login';

  document.querySelectorAll('[data-auth-tab]').forEach((btn) => {
    btn.addEventListener('click', () => {
      mode = btn.dataset.authTab;
      document.querySelectorAll('[data-auth-tab]').forEach((b) => {
        b.classList.toggle('active', b === btn);
      });
      nicknameField.hidden = mode !== 'register';
      submitBtn.textContent = mode === 'register' ? '注册并登录' : '登录';
      errorEl.textContent = '';
      const pwd = document.getElementById('authPassword');
      pwd.autocomplete = mode === 'register' ? 'new-password' : 'current-password';
    });
  });

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorEl.textContent = '';
    const data = new FormData(form);
    const username = String(data.get('username') || '').trim();
    const password = String(data.get('password') || '');
    const nickname = String(data.get('nickname') || '').trim();

    if (username.length < 3) { errorEl.textContent = '用户名至少 3 位'; return; }
    if (password.length < 6) { errorEl.textContent = '密码至少 6 位'; return; }

    submitBtn.disabled = true;
    submitBtn.textContent = '处理中…';
    try {
      const res = mode === 'register'
        ? await api.register(username, password, nickname)
        : await api.login(username, password);
      setToken(res.token);
      showApp(res.user);
      form.reset();
      if (!location.hash) location.hash = '#/dashboard';
      await route();
      toast(mode === 'register' ? '注册成功' : '登录成功', 'ok');
    } catch (err) {
      errorEl.textContent = err.message;
    } finally {
      submitBtn.disabled = false;
      submitBtn.textContent = mode === 'register' ? '注册并登录' : '登录';
    }
  });
}

/* ---------------- 启动 ---------------- */

async function boot() {
  setTheme(initialTheme());
  initModal();
  initAuthForm();

  document.getElementById('themeToggle')?.addEventListener('click', toggleTheme);
  document.getElementById('authThemeToggle')?.addEventListener('click', toggleTheme);
  document.getElementById('logoutBtn')?.addEventListener('click', logout);

  window.addEventListener('hashchange', route);
  window.addEventListener('qj:unauthorized', () => {
    currentUser = null;
    showAuth();
  });

  // 没有 token 直接展示登录页，避免多余请求
  if (!getToken()) {
    showAuth();
    return;
  }

  try {
    const { user } = await api.me();
    showApp(user);
    await route();
  } catch {
    setToken('');
    showAuth();
  }
}

boot();

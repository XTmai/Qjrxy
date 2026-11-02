/* 账号与安全页 */
import { api } from './api.js';
import {
  setChildren,
  h,
  icon,
  toast,
  kvList,
  pageHeader,
  card,
  formField,
  segmented,
  loadingState,
  errorState,
  formatDateTime,
  openModal,
  closeModal,
} from './ui.js';

const API_ENDPOINTS = [
  'GET    /api/dashboard            工作台数据',
  'GET    /api/accounts             账号列表',
  'POST   /api/tasks/sync           拉取今日任务',
  'POST   /api/tasks/run            执行任务',
  'GET    /api/plans                自动计划',
  'POST   /api/plans                创建计划',
  'GET    /api/logs                 执行记录',
  'GET    /api/selftest             加密与数据库自检',
];

export async function renderSecurity(root, ctx) {
  setChildren(root, loadingState('正在加载账户信息…'));

  let user;
  let info = null;
  try {
    ({ user } = await api.me());
    info = await api.systemInfo().catch(() => null);
  } catch (err) {
    setChildren(root, 
      pageHeader({ title: '账号与安全' }),
      errorState('加载失败', err,
        h('button', { class: 'btn primary sm', onclick: () => renderSecurity(root, ctx) }, '重试')),
    );
    return;
  }

  const head = pageHeader({
    title: '账号与安全',
    sub: `当前登录：${user.username}`,
  });

  /* 账户信息 */
  const profileCard = card({
    title: '账户信息',
    body: h('div', { class: 'card-body' },
      kvList([
        ['用户名', user.username],
        ['昵称', user.nickname || '—'],
        ['注册时间', formatDateTime(user.createdAt)],
        ['角色', user.role === 'admin' ? '管理员' : '普通用户'],
        ['额度', `${Math.max(0, (user.quota || 0) - (user.quotaUsed || 0))} / ${user.quota || 0} 次`],
      ]),
    ),
  });

  /* 修改密码 */
  const oldInput = h('input', {
    type: 'password',
    autocomplete: 'current-password',
    placeholder: '当前密码',
  });
  const newInput = h('input', {
    type: 'password',
    autocomplete: 'new-password',
    placeholder: '新密码（至少 6 位）',
  });
  const pwErr = h('p', { class: 'form-error' });

  const pwCard = card({
    title: '修改登录密码',
    body: h('div', { class: 'card-body' },
      formField({ label: '当前密码', input: oldInput }),
      formField({ label: '新密码', input: newInput }),
      pwErr,
      h('button', {
        class: 'btn primary',
        onclick: async (e) => {
          const btn = e.currentTarget;
          pwErr.textContent = '';
          if (!oldInput.value) { pwErr.textContent = '请输入当前密码'; return; }
          if (newInput.value.length < 6) { pwErr.textContent = '新密码至少 6 位'; return; }
          btn.disabled = true;
          try {
            await api.changePassword(oldInput.value, newInput.value);
            toast('密码已更新', 'ok');
            oldInput.value = '';
            newInput.value = '';
          } catch (e2) {
            pwErr.textContent = e2.message;
          } finally {
            btn.disabled = false;
          }
        },
      }, '更新密码'),
    ),
  });

  /* 外观 */
  const themeSeg = segmented({
    items: [['light', '浅色'], ['dark', '深色']],
    active: document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light',
    onSelect: (mode) => {
      ctx.setTheme(mode);
      themeSeg.querySelectorAll('button').forEach((b) =>
        b.classList.toggle('active', b.dataset.seg === mode));
    },
  });

  const themeCard = card({
    title: '外观',
    note: '偏好保存在本机浏览器',
    body: h('div', { class: 'card-body' }, themeSeg),
  });

  /* 开放 API */
  const apiCard = card({
    title: '开放 API',
    note: '全部界面能力都通过 REST 接口提供',
    body: h('div', { class: 'card-body' },
      h('p', { class: 'card-note', style: { marginBottom: '10px' } },
        '在请求头携带 Authorization: Bearer <token> 即可调用。'),
      h('pre', { class: 'code-block' }, API_ENDPOINTS.join('\n')),
      h('div', { class: 'inline-actions' },
        h('button', {
          class: 'btn ghost xs',
          onclick: () => {
            const url = location.origin + '/api/health';
            navigator.clipboard?.writeText(url).then(
              () => toast('服务地址已复制', 'ok'),
              () => toast('复制失败', 'err'),
            );
          },
        }, '复制服务地址'),
        h('a', {
          class: 'btn ghost xs',
          href: '/api/selftest',
          target: '_blank',
          rel: 'noopener',
        }, '打开自检'),
      ),
      info ? h('p', { class: 'card-note', style: { marginTop: '10px' } },
        `客户端协议 ${info.appVersion} · API ${info.apiVersion} · 会话 ${info.tokenTTLDay} 天`)
        : null,
    ),
  });

  /* 关于 */
  const aboutCard = card({
    title: '关于',
    body: h('div', { class: 'card-body' },
      h('p', { class: 'card-note' },
        '本项目仅供个人学习与自用。请在学校及平台允许的范围内，'
        + '使用本人或已获授权的账号，并如实确认任务信息。'),
    ),
  });

  /* 退出 */
  const logoutCard = card({
    body: h('div', { class: 'card-body' },
      h('button', {
        class: 'btn ghost-danger block',
        onclick: async () => {
          const ok = await confirmLogout();
          if (ok) ctx.logout();
        },
      }, icon('logout'), '退出登录'),
    ),
  });

  setChildren(root, head,
    h('div', { class: 'settings-grid' },
      h('div', { class: 'settings-col' },
        profileCard, pwCard, apiCard,
      ),
      h('div', { class: 'settings-col' },
        themeCard, aboutCard, logoutCard,
      ),
    ));
}

function confirmLogout() {
  return new Promise((resolve) => {
    let done = false;
    const finish = (v) => {
      if (done) return;
      done = true;
      closeModal();
      resolve(v);
    };
    openModal({
      title: '退出登录',
      body: h('p', { class: 'confirm-text' },
        '退出后当前浏览器的登录状态会被清除，服务器上的数据不受影响。'),
      footer: [
        h('button', { class: 'btn ghost', onclick: () => finish(false) }, '取消'),
        h('button', { class: 'btn danger', onclick: () => finish(true) }, '退出'),
      ],
      onClose: () => finish(false),
    });
  });
}

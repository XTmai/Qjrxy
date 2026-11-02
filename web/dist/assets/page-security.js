/* 账号与安全：主题偏好、修改密码、开放 API 说明 */
import { api } from './api.js';
import { h, icon, toast, emptyState, copyText, formatDateTime } from './ui.js';

export async function renderSecurity(root, ctx) {
  let user;
  let sysInfo = null;
  try {
    ({ user } = await api.me());
    sysInfo = await api.systemInfo().catch(() => null);
  } catch (err) {
    root.replaceChildren(emptyState('加载失败', err.message));
    return;
  }

  const head = h('div', { class: 'page-head' },
    h('div', null,
      h('h1', null, '账号与安全'),
      h('p', { class: 'page-sub' }, `当前登录：${user.username}`),
    ),
  );

  const profile = h('div', { class: 'card' },
    h('div', { class: 'card-head' }, h('h2', null, '账户信息')),
    h('div', { class: 'card-body' },
      h('dl', { class: 'kv-list' },
        kv('用户名', user.username),
        kv('昵称', user.nickname || '—'),
        kv('注册时间', formatDateTime(user.createdAt)),
        kv('角色', user.role === 'admin' ? '管理员' : '普通用户'),
        kv('额度', `${Math.max(0, user.quota - user.quotaUsed)} / ${user.quota} 次`),
      ),
    ),
  );

  const oldInput = h('input', { type: 'password', autocomplete: 'current-password', placeholder: '当前密码' });
  const newInput = h('input', { type: 'password', autocomplete: 'new-password', placeholder: '新密码（至少 6 位）' });
  const confirmInput = h('input', { type: 'password', autocomplete: 'new-password', placeholder: '确认新密码' });
  const error = h('p', { class: 'form-error' });

  const passwordCard = h('div', { class: 'card' },
    h('div', { class: 'card-head' }, h('h2', null, '修改登录密码')),
    h('div', { class: 'card-body' },
      h('label', { class: 'field' }, h('span', null, '当前密码'), oldInput),
      h('label', { class: 'field' }, h('span', null, '新密码'), newInput),
      h('label', { class: 'field' }, h('span', null, '确认新密码'), confirmInput),
      error,
      h('button', {
        class: 'btn primary',
        onclick: async (e) => {
          error.textContent = '';
          if (!oldInput.value) { error.textContent = '请输入当前密码'; return; }
          if (newInput.value.length < 6) { error.textContent = '新密码至少 6 位'; return; }
          if (newInput.value !== confirmInput.value) { error.textContent = '两次输入的新密码不一致'; return; }
          const btn = e.currentTarget;
          btn.disabled = true;
          try {
            await api.changePassword(oldInput.value, newInput.value);
            toast('密码已更新', 'ok');
            oldInput.value = newInput.value = confirmInput.value = '';
          } catch (err) {
            error.textContent = err.message;
          } finally {
            btn.disabled = false;
          }
        },
      }, '更新密码'),
    ),
  );

  const themeCard = h('div', { class: 'card' },
    h('div', { class: 'card-head' },
      h('div', null,
        h('h2', null, '外观'),
        h('p', { class: 'hint' }, '偏好保存在本机浏览器，并同步到账号'),
      ),
    ),
    h('div', { class: 'card-body' },
      h('div', { class: 'kind-toggle' },
        ['light', 'dark'].map((mode) => h('button', {
          class: document.documentElement.dataset.theme === mode ? 'active' : '',
          onclick: () => {
            ctx.setTheme(mode);
            root.querySelectorAll('.kind-toggle button').forEach((b, i) => {
              b.classList.toggle('active', ['light', 'dark'][i] === mode);
            });
          },
        }, mode === 'light' ? '浅色' : '深色')),
      ),
    ),
  );

  const apiCard = h('div', { class: 'card' },
    h('div', { class: 'card-head' },
      h('div', null,
        h('h2', null, '开放 API'),
        h('p', { class: 'hint' }, '全部界面能力都通过 REST 接口提供，可直接用于二次开发'),
      ),
    ),
    h('div', { class: 'card-body' },
      h('p', { class: 'hint-text', style: { marginBottom: '12px' } },
        '在请求头携带 Authorization: Bearer <token> 即可调用。下面是常用端点：'),
      h('div', { class: 'code-box', style: { whiteSpace: 'pre-wrap', lineHeight: '1.9' } },
        [
          'GET    /api/dashboard            工作台数据',
          'GET    /api/accounts             账号列表',
          'POST   /api/tasks/sync           拉取今日任务',
          'POST   /api/tasks/run            执行任务',
          'GET    /api/plans                自动计划',
          'POST   /api/plans                创建计划',
          'GET    /api/logs                 执行记录',
          'GET    /api/selftest             加密与数据库自检',
        ].join('\n')),
      h('div', { style: { display: 'flex', gap: '10px', marginTop: '14px', flexWrap: 'wrap' } },
        h('button', {
          class: 'btn ghost sm',
          onclick: async () => {
            const ok = await copyText(location.origin + '/api/health');
            toast(ok ? '已复制' : '复制失败');
          },
        }, '复制服务地址'),
        h('a', { class: 'btn ghost sm', href: '/api/selftest', target: '_blank', rel: 'noopener' }, '打开自检'),
      ),
      sysInfo ? h('p', { class: 'hint-text', style: { marginTop: '12px' } },
        `客户端协议版本 ${sysInfo.appVersion} · API ${sysInfo.apiVersion} · 会话有效期 ${sysInfo.tokenTTLDay} 天`) : null,
    ),
  );

  const logoutCard = h('div', { class: 'card' },
    h('div', { class: 'card-head' }, h('h2', null, '会话')),
    h('div', { class: 'card-body' },
      h('p', { class: 'hint-text', style: { marginBottom: '14px' } },
        '退出后当前浏览器的登录状态会被清除，服务器上的数据不受影响。'),
      h('button', {
        class: 'btn danger-ghost',
        onclick: () => ctx.logout(),
      }, icon('logout'), '退出登录'),
    ),
  );

  root.replaceChildren(head,
    h('div', { class: 'dash-grid' },
      h('div', { class: 'dash-col' }, profile, passwordCard, apiCard),
      h('div', { class: 'dash-col' }, themeCard, logoutCard),
    ),
  );
}

function kv(label, value) {
  return h('div', null, h('dt', null, label), h('dd', null, value));
}

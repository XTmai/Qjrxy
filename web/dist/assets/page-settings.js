/* 设置页 */
import { api } from './api.js';
import {
  setChildren,
  el,
  ico,
  toast,
  emptyBox,
  kvList,
  fmtTime,
} from './ui.js';

export async function pageSettings(root, ctx) {
  let user;
  let info = null;
  try {
    ({ user } = await api.me());
    info = await api.systemInfo().catch(() => null);
  } catch (err) {
    setChildren(root, emptyBox('加载失败', err.message));
    return;
  }

  const head = el('div', { class: 'head' },
    el('div', null,
      el('h1', null, '设置'),
      el('div', { class: 'sub' }, '账户与外观'),
    ),
  );

  /* 账户信息 */
  const infoCard = el('div', { class: 'card' },
    el('div', { class: 'card-head' }, el('h2', null, '账户')),
    el('div', { class: 'card-body' },
      kvList([
        ['用户名', user.username],
        ['昵称', user.nickname || '—'],
        ['注册时间', fmtTime(user.createdAt)],
        ['额度', Math.max(0, (user.quota || 0) - (user.quotaUsed || 0)) + ' / ' + (user.quota || 0)],
      ]),
    ),
  );

  /* 改密码 */
  const oldPw = el('input', { type: 'password', placeholder: '当前密码' });
  const newPw = el('input', { type: 'password', placeholder: '新密码（至少 6 位）' });
  const pwErr = el('p', { class: 'form-err' });

  const pwCard = el('div', { class: 'card' },
    el('div', { class: 'card-head' }, el('h2', null, '修改密码')),
    el('div', { class: 'card-body' },
      el('div', { class: 'field' }, el('label', null, '当前密码'), oldPw),
      el('div', { class: 'field' }, el('label', null, '新密码'), newPw),
      pwErr,
      el('button', {
        class: 'btn main',
        onclick: async (e) => {
          pwErr.textContent = '';
          if (!oldPw.value) { pwErr.textContent = '请输入当前密码'; return; }
          if (newPw.value.length < 6) { pwErr.textContent = '新密码至少 6 位'; return; }
          const btn = e.currentTarget;
          btn.disabled = true;
          try {
            await api.changePassword(oldPw.value, newPw.value);
            toast('密码已更新', 'ok');
            oldPw.value = newPw.value = '';
          } catch (e2) {
            pwErr.textContent = e2.message;
          } finally {
            btn.disabled = false;
          }
        },
      }, '更新密码'),
    ),
  );

  /* 外观 */
  const themeCard = el('div', { class: 'card' },
    el('div', { class: 'card-head' }, el('h2', null, '外观')),
    el('div', { class: 'card-body' },
      el('div', { class: 'seg' },
        [['light', '浅色'], ['dark', '深色']].map(([mode, label]) =>
          el('button', {
            class: document.documentElement.dataset.theme === mode ? 'on' : '',
            dataset: { theme: mode },
            onclick: (e) => {
              ctx.setTheme(mode);
              e.currentTarget.parentElement.querySelectorAll('button')
                .forEach((b) => b.classList.toggle('on', b === e.currentTarget));
            },
          }, label)),
      ),
    ),
  );

  /* 关于 */
  const aboutCard = el('div', { class: 'card' },
    el('div', { class: 'card-head' }, el('h2', null, '关于')),
    el('div', { class: 'card-body' },
      kvList([
        ['产品', '小麦校园'],
        ['协议版本', info?.appVersion || '—'],
        ['API', info?.apiVersion || '—'],
        ['会话有效期', (info?.tokenTTLDay || 30) + ' 天'],
      ]),
      el('div', { class: 'note-box info', style: { marginTop: '12px' } },
        '本项目仅供个人学习与自用。请在学校及平台允许范围内，使用本人或已获授权的账号。'),
    ),
  );

  /* 退出 */
  const logoutCard = el('div', { class: 'card' },
    el('div', { class: 'card-body' },
      el('button', {
        class: 'btn danger-text full',
        onclick: () => ctx.logout(),
      }, ico('logout'), '退出登录'),
    ),
  );

  setChildren(root, head,
    el('div', { style: { display: 'grid', gap: '16px', maxWidth: '720px' } },
      infoCard, pwCard, themeCard, aboutCard, logoutCard));
}

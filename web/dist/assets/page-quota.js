/* 额度与卡密页 */
import { api } from './api.js';
import { h, icon, toast, emptyState } from './ui.js';

export async function renderQuota(root, ctx) {
  let user;
  try {
    ({ user } = await api.me());
  } catch (err) {
    root.replaceChildren(emptyState('加载失败', err.message));
    return;
  }
  const remaining = Math.max(0, (user.quota || 0) - (user.quotaUsed || 0));

  const head = h('div', { class: 'page-head' },
    h('div', null,
      h('h1', null, '额度与卡密'),
      h('p', { class: 'page-sub' }, '每次手动执行扣 1 次额度；自动计划按执行次数计入'),
    ),
  );

  const stats = h('div', { class: 'stat-row', style: { gridTemplateColumns: 'repeat(3, minmax(0, 1fr))' } },
    h('div', { class: 'stat' },
      h('div', { class: 'stat-label' }, h('span', null, '剩余额度'), h('span', { class: 'stat-dot' })),
      h('div', { class: 'stat-value' }, String(remaining)),
      h('div', { class: 'stat-foot' }, '可用于手动与自动执行'),
    ),
    h('div', { class: 'stat' },
      h('div', { class: 'stat-label' }, h('span', null, '累计获得'), h('span', { class: 'stat-dot green' })),
      h('div', { class: 'stat-value' }, String(user.quota || 0)),
      h('div', { class: 'stat-foot' }, '含初始额度与卡密兑换'),
    ),
    h('div', { class: 'stat' },
      h('div', { class: 'stat-label' }, h('span', null, '累计使用'), h('span', { class: 'stat-dot amber' })),
      h('div', { class: 'stat-value' }, String(user.quotaUsed || 0)),
      h('div', { class: 'stat-foot' }, '按执行次数统计'),
    ),
  );

  const input = h('input', {
    placeholder: '输入卡密，如 QJ-A1B2-C3D4', autocomplete: 'off',
    style: { fontFamily: 'var(--mono)', textTransform: 'uppercase' },
  });
  const error = h('p', { class: 'form-error' });

  const redeemCard = h('div', { class: 'card' },
    h('div', { class: 'card-head' },
      h('div', null,
        h('h2', null, '兑换卡密'),
        h('p', { class: 'hint' }, '一张卡密可增加 30 次执行额度'),
      ),
    ),
    h('div', { class: 'card-body' },
      h('label', { class: 'field' }, h('span', null, '卡密'), input),
      h('button', {
        class: 'btn primary',
        onclick: async (e) => {
          const code = input.value.trim();
          if (!code) { error.textContent = '请输入卡密'; return; }
          const btn = e.currentTarget;
          btn.disabled = true;
          btn.textContent = '兑换中…';
          try {
            const res = await api.redeem(code);
            toast(`兑换成功，增加 ${res.granted} 次`, 'ok');
            error.textContent = '';
            input.value = '';
            await renderQuota(root, ctx);
            ctx.refreshUser?.();
          } catch (err) {
            error.textContent = err.message;
            btn.disabled = false;
            btn.textContent = '立即兑换';
          }
        },
      }, '立即兑换'),
      error,
      h('p', { class: 'hint-text' },
        '卡密由平台运营方发放。自建部署时，可在 internal/server/accounts.go 的 redeemAmount 中调整校验规则。'),
    ),
  );

  const rules = h('div', { class: 'card' },
    h('div', { class: 'card-head' }, h('h2', null, '额度规则')),
    h('div', { class: 'card-body' },
      h('div', { class: 'notice-list' },
        rule('扣减方式', '手动执行一次任务扣 1 次；自动计划每成功提交一次扣 1 次。'),
        rule('失败是否扣减', '提交失败同样计入一次执行，因为已经向学校系统发起请求。'),
        rule('额度不足', '剩余为 0 时任务会被拦截，兑换卡密后自动恢复。'),
        rule('自建部署', '你可以在数据库中直接调整 users.quota，把额度设为所需数量。'),
      ),
    ),
  );

  root.replaceChildren(head, stats, h('div', { class: 'dash-grid' }, redeemCard, rules));
}

function rule(title, text) {
  return h('div', { class: 'notice' },
    h('p', { class: 'notice-title' }, title),
    h('p', { class: 'notice-text' }, text),
  );
}

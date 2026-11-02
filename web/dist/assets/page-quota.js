/* 额度页 */
import { api } from './api.js';
import {
  setChildren,
  el,
  ico,
  toast,
  emptyBox,
} from './ui.js';

export async function pageQuota(root, ctx) {
  let user;
  try {
    ({ user } = await api.me());
  } catch (err) {
    setChildren(root, emptyBox('加载失败', err.message));
    return;
  }

  const left = Math.max(0, (user.quota || 0) - (user.quotaUsed || 0));

  const head = el('div', { class: 'head' },
    el('div', null,
      el('h1', null, '额度'),
      el('div', { class: 'sub' }, '每次执行查寝扣 1 次'),
    ),
  );

  const stats = el('div', { class: 'stats', style: { gridTemplateColumns: 'repeat(3,1fr)' } },
    statCard('剩余', left, '可用于执行'),
    statCard('累计获得', user.quota || 0, '含兑换'),
    statCard('累计使用', user.quotaUsed || 0, '按次统计'),
  );

  const codeInput = el('input', {
    placeholder: '输入卡密，如 QW-A1B2-C3D4',
    style: { fontFamily: 'var(--mono)', textTransform: 'uppercase' },
  });
  const err = el('p', { class: 'form-err' });

  const redeemCard = el('div', { class: 'card' },
    el('div', { class: 'card-head' }, el('h2', null, '兑换卡密')),
    el('div', { class: 'card-body' },
      el('div', { class: 'field' }, codeInput),
      err,
      el('button', {
        class: 'btn main full',
        onclick: async (e) => {
          const code = codeInput.value.trim();
          if (!code) { err.textContent = '请输入卡密'; return; }
          const btn = e.currentTarget;
          btn.disabled = true;
          btn.textContent = '兑换中…';
          try {
            const res = await api.redeem(code);
            toast('兑换成功，增加 ' + res.granted + ' 次', 'ok');
            codeInput.value = '';
            err.textContent = '';
            await pageQuota(root, ctx);
            ctx.refreshUser?.();
          } catch (e2) {
            err.textContent = e2.message;
          } finally {
            btn.disabled = false;
            btn.textContent = '立即兑换';
          }
        },
      }, '立即兑换'),
      el('div', { class: 'hint', style: { marginTop: '10px' } },
        '卡密由平台运营方提供。自建部署时可在后端调整校验规则。'),
    ),
  );

  const ruleCard = el('div', { class: 'card' },
    el('div', { class: 'card-head' }, el('h2', null, '额度规则')),
    el('div', { class: 'card-body' },
      el('div', { style: { display: 'flex', flexDirection: 'column', gap: '12px' } },
        rule('扣减方式', '手动执行一次扣 1 次；自动计划每成功提交一次扣 1 次。'),
        rule('失败是否扣', '同样计入一次，因为已经向学校系统发起请求。'),
        rule('额度不足', '为 0 时任务会被拦截，兑换后恢复。'),
      ),
    ),
  );

  setChildren(root, head, stats,
    el('div', { style: { display: 'grid', gap: '16px', maxWidth: '560px' } },
      redeemCard, ruleCard));
}

function statCard(label, value, note) {
  return el('div', { class: 'stat' },
    el('div', { class: 'stat-top' }, el('span', null, label), el('i', null)),
    el('div', { class: 'stat-num' }, String(value ?? 0)),
    el('div', { class: 'stat-note' }, note),
  );
}

function rule(title, text) {
  return el('div', null,
    el('div', { style: { fontSize: '13.5px', fontWeight: '600' } }, title),
    el('div', { style: { fontSize: '12.5px', color: 'var(--text-faint)', marginTop: '2px', lineHeight: '1.65' } }, text),
  );
}

/* 额度与卡密页 */
import { api } from './api.js';
import {
  setChildren,
  h,
  icon,
  toast,
  pageHeader,
  card,
  statGrid,
  formField,
  loadingState,
  emptyState,
  errorState,
} from './ui.js';

const RULES = [
  ['扣减方式', '手动执行一次任务扣 1 次额度；自动计划每成功提交一次扣 1 次。'],
  ['失败是否扣减', '提交失败同样计入一次，因为已经向学校系统发起了请求。'],
  ['额度不足', '剩余为 0 时任务会被拦截，兑换卡密后自动恢复。'],
  ['自建部署', '可在数据库中直接调整 users.quota 字段把额度设为所需数量。'],
];

export async function renderQuota(root, ctx) {
  setChildren(root, loadingState('正在加载额度…'));

  let user;
  try {
    ({ user } = await api.me());
  } catch (err) {
    setChildren(root, 
      pageHeader({ title: '额度与卡密' }),
      errorState('加载失败', err,
        h('button', { class: 'btn primary sm', onclick: () => renderQuota(root, ctx) }, '重试')),
    );
    return;
  }

  const remaining = Math.max(0, (user.quota || 0) - (user.quotaUsed || 0));

  const head = pageHeader({
    title: '额度与卡密',
    sub: '每次执行查寝扣 1 次额度',
  });

  const stats = statGrid([
    { label: '剩余额度', value: remaining, note: '可用于执行任务', tone: '' },
    { label: '累计获得', value: user.quota || 0, note: '含初始额度与兑换', tone: 'green' },
    { label: '累计使用', value: user.quotaUsed || 0, note: '按执行次数统计', tone: 'amber' },
  ], 3);

  const codeInput = h('input', {
    placeholder: '输入卡密，如 QJ-A1B2-C3D4',
    style: { fontFamily: 'var(--mono)', textTransform: 'uppercase' },
  });
  const err = h('p', { class: 'form-error' });

  const redeemBtn = h('button', {
    class: 'btn primary',
    onclick: async () => {
      const code = codeInput.value.trim();
      if (!code) { err.textContent = '请输入卡密'; return; }
      redeemBtn.disabled = true;
      redeemBtn.textContent = '兑换中…';
      try {
        const res = await api.redeem(code);
        toast(`兑换成功，增加 ${res.granted} 次`, 'ok');
        codeInput.value = '';
        err.textContent = '';
        await renderQuota(root, ctx);
        ctx.refreshUser?.();
      } catch (e) {
        err.textContent = e.message;
      } finally {
        redeemBtn.disabled = false;
        redeemBtn.textContent = '立即兑换';
      }
    },
  }, '立即兑换');

  const redeemCard = card({
    title: '兑换卡密',
    note: '一张卡密可增加 30 次执行额度',
    body: h('div', { class: 'card-body' },
      formField({ label: '卡密', input: codeInput }),
      err,
      redeemBtn,
      h('p', { class: 'card-note' },
        '卡密由平台运营方发放。自建部署时可调整后端校验规则。'),
    ),
  });

  const ruleCard = card({
    title: '额度规则',
    body: h('div', { class: 'card-body' },
      h('div', { class: 'rule-list' },
        RULES.map(([title, text]) => h('div', { class: 'rule-item' },
          h('div', { class: 'rule-title' }, title),
          h('div', { class: 'rule-text' }, text),
        )),
      ),
    ),
  });

  setChildren(root, head, stats,
    h('div', { class: 'narrow-grid' }, redeemCard, ruleCard));
}

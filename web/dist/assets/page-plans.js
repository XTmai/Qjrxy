/* 自动计划页 */
import { api } from './api.js';
import {
  setChildren,
  el,
  ico,
  toast,
  skeleton,
  emptyBox,
  fmtTime,
  openModal,
  closeModal,
  confirmBox,
} from './ui.js';

const WEEK = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];

export async function pagePlans(root, ctx) {
  setChildren(root, skeleton());
  let plans = [];
  let accounts = [];
  try {
    const [p, a] = await Promise.all([api.plans(), api.accounts()]);
    plans = p.plans;
    accounts = a.accounts;
  } catch (err) {
    setChildren(root, emptyBox('加载失败', err.message,
      el('button', { class: 'btn main', onclick: () => pagePlans(root, ctx) }, '重试')));
    return;
  }
  paint(root, ctx, plans, accounts);
}

function paint(root, ctx, plans, accounts) {
  const active = plans.filter((p) => p.enabled).length;

  const head = el('div', { class: 'head' },
    el('div', null,
      el('h1', null, '自动计划'),
      el('div', { class: 'sub' }, plans.length + ' 个计划，' + active + ' 个运行中'),
    ),
    el('div', { class: 'head-actions' },
      el('button', {
        class: 'btn main',
        disabled: !accounts.length,
        onclick: () => planForm(ctx, null, accounts, () => pagePlans(root, ctx)),
      }, ico('plus'), '新建计划'),
    ),
  );

  if (!accounts.length) {
    setChildren(root, head, emptyBox('还没有绑定账号', '先添加校园账号再创建计划',
      el('button', { class: 'btn main', onclick: () => ctx.go('/accounts') }, '去添加')));
    return;
  }

  if (!plans.length) {
    setChildren(root, head, emptyBox('还没有自动计划',
      '创建后，系统会按设定时段自动执行查寝',
      el('button', {
        class: 'btn main sm',
        onclick: () => planForm(ctx, null, accounts, () => pagePlans(root, ctx)),
      }, '新建计划')));
    return;
  }

  const list = el('div', { class: 'plan-list' });
  plans.forEach((p) => list.append(planCard(p, accounts, ctx, root)));

  setChildren(root, head, list);
}

function planCard(plan, accounts, ctx, root) {
  const toggle = el('button', {
    class: 'switch ' + (plan.enabled ? 'on' : ''),
    title: plan.enabled ? '点击暂停' : '点击启用',
    onclick: async (e) => {
      const btn = e.currentTarget;
      btn.disabled = true;
      try {
        await api.togglePlan(plan.id, !plan.enabled);
        toast(plan.enabled ? '已暂停' : '已启用', 'ok');
        await pagePlans(root, ctx);
      } catch (err) {
        toast(err.message, 'err');
        btn.disabled = false;
      }
    },
  });

  const modeText = plan.mode === 'school' ? '跟随学校下发时间' : '自定义时段';
  const daysText = describeDays(plan.days);

  return el('div', { class: 'plan' },
    el('div', { class: 'plan-top' },
      el('div', null,
        el('div', { class: 'plan-name' }, plan.name),
        el('div', { class: 'plan-account' },
          (plan.accountAlias || plan.accountUsername) + ' · ' + plan.schoolName),
      ),
      toggle,
    ),
    el('div', { class: 'plan-mode' }, modeText),
    el('div', { class: 'plan-meta' },
      metaRow('执行日期', daysText),
      plan.mode === 'custom' ? metaRow('时段', plan.windowStart + ' – ' + plan.windowEnd) : null,
      metaRow('最近执行', plan.lastRunAt ? fmtTime(plan.lastRunAt) : '尚未执行'),
      plan.lastResult ? metaRow('结果', plan.lastResult) : null,
    ),
    el('div', { class: 'plan-acts' },
      el('button', {
        class: 'btn plain xs',
        onclick: () => planForm(ctx, plan, accounts, () => pagePlans(root, ctx)),
      }, '编辑'),
      el('button', {
        class: 'btn danger-text xs',
        onclick: async () => {
          const ok = await confirmBox({
            title: '删除计划',
            message: '确定删除「' + plan.name + '」吗？历史记录会保留。',
            okText: '删除',
            danger: true,
          });
          if (!ok) return;
          try {
            await api.deletePlan(plan.id);
            toast('已删除', 'ok');
            await pagePlans(root, ctx);
          } catch (err) {
            toast(err.message, 'err');
          }
        },
      }, '删除'),
    ),
  );
}

function metaRow(k, v) {
  return el('div', { class: 'meta-row' },
    el('span', null, k), el('b', null, v),
  );
}

function describeDays(days) {
  const set = new Set(days || []);
  if (set.size === 7) return '每天';
  if (set.size === 5 && [0, 1, 2, 3, 4].every((d) => set.has(d))) return '工作日';
  if (set.size === 2 && set.has(5) && set.has(6)) return '周末';
  return [...set].sort((a, b) => a - b).map((d) => WEEK[d]).join('、') || '每天';
}

/* ---------------- 新建 / 编辑 ---------------- */

function planForm(ctx, existing, accounts, refresh) {
  const isEdit = Boolean(existing);
  const form = {
    kind: 'attendance',
    mode: existing?.mode || 'school',
    days: new Set(existing?.days?.length ? existing.days : [0, 1, 2, 3, 4, 5, 6]),
  };

  const accountSel = el('select', null,
    accounts.map((a) => el('option', {
      value: a.id,
      selected: existing ? a.id === existing.accountId : a.id === accounts[0]?.id,
    }, (a.alias || a.username) + ' · ' + a.schoolName)));

  const nameInput = el('input', {
    placeholder: '如：晚间查寝', value: existing?.name || '',
  });

  const modeSeg = el('div', { class: 'seg' },
    [['school', '跟随学校时间'], ['custom', '自定义时段']].map(([key, label]) =>
      el('button', {
        class: form.mode === key ? 'on' : '',
        dataset: { mode: key },
        onclick: () => {
          form.mode = key;
          modeSeg.querySelectorAll('button').forEach((b) =>
            b.classList.toggle('on', b.dataset.mode === key));
          customFields.hidden = key !== 'custom';
        },
      }, label)),
  );

  const startInput = el('input', { type: 'time', value: existing?.windowStart || '00:10' });
  const endInput = el('input', { type: 'time', value: existing?.windowEnd || '23:30' });
  const customFields = el('div', { class: 'grid2', hidden: form.mode !== 'custom' },
    el('div', { class: 'field' }, el('label', null, '开始'), startInput),
    el('div', { class: 'field' }, el('label', null, '结束'), endInput),
  );

  const dayBtns = WEEK.map((label, idx) => el('button', {
    class: 'day-chip' + (form.days.has(idx) ? ' on' : ''),
    dataset: { day: String(idx) },
    onclick: (e) => {
      if (form.days.has(idx)) form.days.delete(idx);
      else form.days.add(idx);
      e.currentTarget.classList.toggle('on', form.days.has(idx));
    },
  }, label));

  const err = el('p', { class: 'form-err' });

  const body = el('div', null,
    el('div', { class: 'field' }, el('label', null, '计划名称'), nameInput),
    el('div', { class: 'field' }, el('label', null, '选择账号'), accountSel),
    el('div', { class: 'field' },
      el('label', null, '执行方式'), modeSeg,
      el('div', { class: 'hint' }, '跟随学校时间：任务下发后尽快执行；自定义时段：只在设定区间内执行。')),
    customFields,
    el('div', { class: 'field' },
      el('label', null, '执行日期'),
      el('div', { class: 'day-row' }, dayBtns),
      el('div', { class: 'hint' }, '每天最多成功执行一次。')),
    err,
  );

  openModal({
    title: isEdit ? '编辑计划' : '新建计划',
    sub: '按设定时间自动执行查寝',
    body,
    wide: true,
    actions: [
      el('button', { class: 'btn plain', onclick: closeModal }, '取消'),
      el('button', {
        class: 'btn main',
        onclick: async (e) => {
          const btn = e.currentTarget;
          const payload = {
            accountId: Number(accountSel.value),
            name: nameInput.value.trim() || '查寝计划',
            kind: 'attendance',
            mode: form.mode,
            windowStart: startInput.value,
            windowEnd: endInput.value,
            days: [...form.days].sort((a, b) => a - b),
            enabled: true,
          };
          if (!payload.accountId) { err.textContent = '请选择账号'; return; }
          if (!payload.days.length) { err.textContent = '请至少选择一天'; return; }

          btn.disabled = true;
          btn.textContent = '保存中…';
          try {
            if (isEdit) await api.updatePlan(existing.id, payload);
            else await api.createPlan(payload);
            toast(isEdit ? '已保存' : '计划已创建', 'ok');
            closeModal();
            refresh();
          } catch (e2) {
            err.textContent = e2.message;
            btn.disabled = false;
            btn.textContent = '保存';
          }
        },
      }, '保存'),
    ],
  });
}

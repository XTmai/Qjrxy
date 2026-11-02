/* 任务执行页：手动任务 + 自动签到/查寝计划 */
import { api } from './api.js';
import {
  h, icon, toast, openModal, closeModal, confirmDialog, statusBadge, loginBadge,
  skeletonCard, emptyState, shortRange, kindLabel, formatDateTime, WEEK_NAMES, statusText,
} from './ui.js';

let cache = {
  accounts: [],
  tasks: [],
  plans: [],
  accountId: null,
  kind: 'attendance',
  statusTab: 'all',
  tab: 'manual',
  loaded: false,
};

export async function renderTasks(root, ctx) {
  const hashParts = location.hash.split('?');
  const params = new URLSearchParams(hashParts[1] || '');
  if (params.get('account')) cache.accountId = Number(params.get('account'));
  if (hashParts[0].includes('auto')) cache.tab = 'auto';

  root.replaceChildren(h('div', { class: 'loading-block' }, skeletonCard(false)));

  try {
    const [acct, taskRes, planRes] = await Promise.all([api.accounts(), api.tasks(), api.plans()]);
    cache.accounts = acct.accounts;
    cache.tasks = taskRes.tasks;
    cache.plans = planRes.plans;
    cache.loaded = true;
    if (!cache.accountId && cache.accounts.length) cache.accountId = cache.accounts[0].id;
  } catch (err) {
    root.replaceChildren(emptyState('加载失败', err.message,
      h('button', { class: 'btn primary', onclick: () => renderTasks(root, ctx) }, '重试')));
    return;
  }

  draw(root, ctx);
}

function draw(root, ctx) {
  const head = h('div', { class: 'page-head' },
    h('div', null,
      h('h1', null, '任务执行'),
      h('p', { class: 'page-sub' }, '查寝任务执行与自动计划'),
    ),
  );

  const segmented = h('div', { class: 'segmented', style: { marginBottom: '22px' } },
    h('button', {
      class: cache.tab === 'manual' ? 'active' : '',
      onclick: () => { cache.tab = 'manual'; draw(root, ctx); },
    }, '手动任务'),
    h('button', {
      class: cache.tab === 'auto' ? 'active' : '',
      onclick: () => { cache.tab = 'auto'; draw(root, ctx); },
    }, '自动签到'),
  );

  const body = cache.tab === 'manual'
    ? manualView(root, ctx)
    : autoView(root, ctx);

  root.replaceChildren(head, segmented, body);
}

/* ================= 手动任务 ================= */

function manualView(root, ctx) {
  const accountSelect = h('select', {
    onchange: (e) => { cache.accountId = Number(e.target.value); draw(root, ctx); },
  });
  if (cache.accounts.length === 0) {
    accountSelect.append(h('option', { value: '' }, '尚未添加账号'));
    accountSelect.disabled = true;
  } else {
    cache.accounts.forEach((a) => {
      accountSelect.append(h('option', {
        value: a.id, selected: a.id === cache.accountId,
      }, `${a.alias || a.username}（${a.schoolName}）`));
    });
  }

  // 本项目只处理查寝；签到类任务不在此展示与执行
  const kindButtons = [
    h('button', { class: 'active' }, '查寝 / 宿舍考勤'),
  ];

  const pullBtn = h('button', {
    class: 'btn primary block',
    disabled: !cache.accountId,
    onclick: async (e) => {
      const btn = e.currentTarget;
      if (!cache.accountId) { toast('请先添加并选择账号', 'err'); return; }
      btn.disabled = true;
      btn.textContent = '拉取中…';
      try {
        await api.syncTasks(cache.accountId);
        const { tasks } = await api.tasks();
        cache.tasks = tasks;
        toast('任务已更新', 'ok');
        draw(root, ctx);
      } catch (err) {
        toast(err.message, 'err');
        btn.disabled = false;
        btn.textContent = '拉取今日任务';
      }
    },
  }, '拉取今日任务');

  const panel = h('div', { class: 'filter-panel' },
    h('div', { class: 'card' },
      h('div', { class: 'card-body', style: { paddingTop: '20px' } },
        h('label', { class: 'field' }, h('span', null, '选择账号'), accountSelect),
        h('div', { class: 'field' }, h('span', null, '任务类型'), h('div', { class: 'kind-toggle' }, kindButtons)),
        pullBtn,
      ),
    ),
  );

  const accountTasks = cache.tasks.filter((t) => t.accountId === cache.accountId);
  // 只看查寝任务
  const byKind = accountTasks.filter((t) => t.kind === 'attendance');

  const pending = byKind.filter((t) => t.status !== 'done').length;
  const handled = byKind.filter((t) => t.status === 'done').length;

  const statusTabs = h('div', { class: 'status-tabs' },
    [['all', `全部 ${byKind.length}`], ['pending', `待执行 ${pending}`], ['done', `已处理 ${handled}`]]
      .map(([key, label]) => h('button', {
        class: cache.statusTab === key ? 'active' : '',
        onclick: () => { cache.statusTab = key; draw(root, ctx); },
      }, label)),
  );

  const shown = byKind.filter((t) => {
    if (cache.statusTab === 'pending') return t.status !== 'done';
    if (cache.statusTab === 'done') return t.status === 'done';
    return true;
  });

  const list = h('div', { class: 'task-list' });
  if (shown.length === 0) {
    list.append(emptyState(
      byKind.length === 0 ? `暂无${kindLabel(cache.kind)}任务` : '该分类下没有任务',
      byKind.length === 0 ? '点击左侧「拉取今日任务」从学校系统同步' : '切换上方分类查看其他任务',
    ));
  } else {
    shown.forEach((task) => list.append(buildTaskCard(task, ctx, root)));
  }

  const right = h('div', null,
    h('div', { class: 'task-count' },
      h('h2', null, `${kindLabel(cache.kind)} / 宿舍考勤`),
      h('span', { class: 'count' }, `${byKind.length} 条`),
    ),
    statusTabs,
    list,
  );

  return h('div', { class: 'task-layout' }, panel, right);
}

function buildTaskCard(task, ctx, root) {
  const isDone = task.status === 'done';
  const signable = taskSignable(task);
  const notYet = task.signStatus === '2';
  const runBtn = h('button', {
    class: 'btn ' + (isDone || !signable ? 'ghost' : 'soft') + ' sm',
    disabled: isDone || !signable,
    title: notYet ? '还没到签到时间' : (task.status === 'done' ? '已处理' : ''),
    onclick: async (e) => {
      const btn = e.currentTarget;
      const ok = await confirmDialog({
        title: '执行任务',
        message: `即将为「${task.accountAlias || task.accountId}」执行「${task.taskName || kindLabel(task.kind)}」。确认提交吗？`,
        confirmText: '立即执行',
      });
      if (!ok) return;
      btn.disabled = true;
      btn.textContent = '执行中…';
      try {
        const res = await api.runTask(task.accountId, task.id, task.kind);
        if (res.status === 'success') {
          toast('执行成功：' + (res.message || '已完成'), 'ok');
        } else if (res.status === 'pending') {
          toast('结果待核对：' + (res.hint || res.message || ''), '');
        } else {
          toast('执行失败：' + (res.hint || res.message || ''), 'err');
        }
        await renderTasks(root, ctx);
      } catch (err) {
        toast(err.message, 'err');
        btn.disabled = false;
        btn.textContent = '执行';
      }
    },
  }, isDone ? '已处理' : (notYet ? '未到时间' : '执行'));

  return h('div', { class: 'task-card' },
    h('div', { class: 'task-main' },
      h('div', { class: 'task-title' }, task.taskName || kindLabel(task.kind)),
      renderTaskTime(task),
      h('div', { class: 'task-tags' },
        taskStatusBadge(task),
        taskSignable(task) ? h('span', { class: 'badge ok' }, '可签到') : null,
        task.sender ? h('span', { class: 'badge plain' }, task.sender) : null,
        h('span', { class: 'badge info' }, task.kindLabel || kindLabel(task.kind)),
      ),
    ),
    runBtn,
  );
}

// renderTaskTime 渲染任务时间。周期性任务用「日期 + 时段」，单次任务用起止时间。
function renderTaskTime(task) {
  if (task.signDate || task.beginClock) {
    const date = task.signDate || '';
    const clock = task.beginClock && task.endClock
      ? `${task.beginClock} - ${task.endClock}`
      : (task.beginClock || '');
    return h('div', { class: 'task-time' },
      h('span', { class: 'task-date' }, date),
      clock ? h('span', { class: 'task-clock' }, clock) : null,
    );
  }
  return h('div', { class: 'task-time' }, shortRange(task.startTime, task.endTime));
}

// taskSignable 判断当前是否可以提交。
// signStatus: 0/1 可签，2 未到时间，3 已过期，4 已签到。
function taskSignable(task) {
  if (task.status === 'done') return false;
  const st = String(task.signStatus || '').trim();
  if (st === '' ) return true;          // 查寝等不带 signStatus 的任务
  return st === '0' || st === '1';
}

function taskStatusBadge(task) {
  if (task.status === 'done') return h('span', { class: 'badge ok' }, '已处理');
  const st = String(task.signStatus || '').trim();
  switch (st) {
    case '2': return h('span', { class: 'badge warn' }, '未到时间');
    case '3': return h('span', { class: 'badge plain' }, '已过期');
    case '4': return h('span', { class: 'badge ok' }, '已签到');
    case '0':
    case '1': return h('span', { class: 'badge info' }, '待签到');
  }
  return h('span', { class: 'badge plain' }, '待处理');
}

/* ================= 自动计划 ================= */

function autoView(root, ctx) {
  const enabled = cache.plans.filter((p) => p.enabled).length;
  const needAttention = cache.plans.filter((p) => p.lastResult && !/成功|已完成/.test(p.lastResult)).length;

  const hero = h('div', { class: 'plan-hero' },
    h('div', { class: 'plan-hero-icon' }, icon('clock')),
    h('div', { class: 'plan-hero-text' },
      h('div', { class: 'eyebrow' }, icon('shield'), '按时检查，安心完成'),
      h('h2', null, '把日常签到交给计划'),
      h('p', null, '选择自己的账号和执行时段，系统会在时段内持续检查新任务。关闭页面后也会继续执行。'),
    ),
    h('button', {
      class: 'btn primary',
      onclick: () => openPlanForm(ctx, null, root),
      disabled: cache.accounts.length === 0,
    }, '新增计划'),
  );

  const stats = h('div', { class: 'plan-stats' },
    planStat(cache.plans.length, '全部计划'),
    planStat(enabled, '已开启'),
    planStat(needAttention, '需要处理'),
  );

  const planList = h('div', null);
  if (cache.plans.length === 0) {
    planList.append(h('div', { class: 'card', style: { padding: '8px 0' } },
      emptyState('还没有自动计划', cache.accounts.length === 0
        ? '先添加账号，再创建自动签到或查寝计划'
        : '创建计划后，系统会按设定时段自动执行签到与查寝')));
  } else {
    cache.plans.forEach((plan) => planList.append(buildPlanCard(plan, ctx, root)));
  }

  const recentLogs = cache.plans
    .filter((p) => p.lastRunAt)
    .sort((a, b) => String(b.lastRunAt).localeCompare(String(a.lastRunAt)))
    .slice(0, 5);

  const recentSide = h('div', null,
    h('div', { class: 'plan-section-title' },
      h('h3', null, '最近自动任务'),
      h('button', { class: 'link-btn', onclick: () => ctx.go('/logs') }, '全部执行记录'),
    ),
    recentLogs.length === 0
      ? h('div', { class: 'card', style: { padding: '6px 0' } }, emptyState('暂无自动执行', '计划执行后会在这里显示'))
      : recentLogs.map((p) => h('div', { class: 'mini-log' },
        h('div', null,
          h('strong', null, p.name),
          h('span', null, `${p.accountAlias || p.accountUsername || '账号'} · ${kindLabel(p.kind)}`),
        ),
        h('span', {
          class: 'badge ' + (/成功|已完成/.test(p.lastResult) ? 'ok' : 'warn'),
        }, /成功|已完成/.test(p.lastResult) ? '已完成' : '需核对'),
      )),

    h('details', { class: 'disclosure' },
      h('summary', null, '执行前准备与说明'),
      h('ol', null,
        h('li', null, '确认账号已登录，设备 ID 与打卡手机一致。'),
        h('li', null, '确认默认位置坐标在签到点半径内。'),
        h('li', null, '任务需要照片时，先在账号中上传照片。'),
        h('li', null, '「跟随学校下发时间」会在任务出现后尽快执行；«自定义时段» 只在设定区间内执行。'),
        h('li', null, '每个计划每天最多成功执行一次。'),
      ),
    ),
  );

  return h('div', null,
    hero, stats,
    h('div', { class: 'plan-grid' },
      h('div', null,
        h('div', { class: 'plan-section-title' },
          h('h3', null, '我的计划'),
          h('button', { class: 'btn ghost sm', onclick: () => renderTasks(root, ctx) }, '刷新'),
        ),
        planList,
      ),
      recentSide,
    ),
  );
}

function planStat(value, label) {
  return h('div', { class: 'plan-stat' }, h('strong', null, String(value)), h('span', null, label));
}

function buildPlanCard(plan, ctx, root) {
  const toggle = h('button', {
    class: 'switch ' + (plan.enabled ? 'on' : ''),
    title: plan.enabled ? '点击暂停' : '点击启用',
    'aria-label': plan.enabled ? '暂停计划' : '启用计划',
    onclick: async (e) => {
      const btn = e.currentTarget;
      btn.disabled = true;
      try {
        await api.togglePlan(plan.id, !plan.enabled);
        plan.enabled = !plan.enabled;
        toast(plan.enabled ? '计划已启用' : '计划已暂停', 'ok');
        draw(root, ctx);
      } catch (err) {
        toast(err.message, 'err');
        btn.disabled = false;
      }
    },
  });

  const modeLabel = plan.mode === 'school' ? '跟随学校下发时间' : '自定义执行时段';
  const daysLabel = describeDays(plan.days);

  return h('div', { class: 'plan-card' },
    h('div', { class: 'plan-card-top' },
      h('div', null,
        h('h4', null, plan.name),
        h('div', { class: 'plan-account' },
          `${plan.accountAlias || plan.accountUsername} · ${plan.schoolName}`),
      ),
      toggle,
    ),
    h('div', { class: 'plan-big' }, modeLabel),
    h('div', { class: 'plan-meta' },
      h('div', null,
        h('span', null, daysLabel),
        h('span', null, kindLabel(plan.kind)),
      ),
      plan.mode === 'custom'
        ? h('div', null, h('span', null, '执行时段'),
            h('span', null, `${plan.windowStart} - ${plan.windowEnd}`))
        : null,
      h('div', null,
        h('span', null, '最近执行'),
        h('span', null, plan.lastRunAt ? formatDateTime(plan.lastRunAt) : '尚未执行'),
      ),
      h('div', null,
        h('span', null, '执行结果'),
        h('span', null, plan.lastResult || '—'),
      ),
    ),
    h('div', { class: 'plan-foot' },
      h('button', { class: 'btn ghost sm', onclick: () => openPlanForm(ctx, plan, root) }, '编辑'),
      h('button', { class: 'btn danger-ghost sm', onclick: () => removePlan(plan, ctx, root) }, '删除'),
    ),
  );
}

function describeDays(days) {
  const set = new Set(days || []);
  if (set.size === 7) return '每天';
  if (set.size === 5 && [0, 1, 2, 3, 4].every((d) => set.has(d))) return '工作日（周一至周五）';
  if (set.size === 6 && [0, 1, 2, 3, 4, 5].every((d) => set.has(d))) return '周一至周六';
  if (set.size === 2 && set.has(5) && set.has(6)) return '周末';
  return [...set].sort((a, b) => a - b).map((d) => WEEK_NAMES[d]).join('、') || '每天';
}

function openPlanForm(ctx, existing, root) {
  const isEdit = Boolean(existing);
  const accountSelect = h('select', null,
    cache.accounts.map((a) => h('option', {
      value: a.id,
      selected: existing ? a.id === existing.accountId : a.id === cache.accountId,
    }, `${a.alias || a.username}（${a.schoolName}）`)),
  );
  if (cache.accounts.length === 0) accountSelect.append(h('option', { value: '' }, '尚未添加账号'));

  const nameInput = h('input', {
    placeholder: '如：晚间查寝计划', value: existing?.name || '',
  });

  const form = { kind: existing?.kind || 'attendance', mode: existing?.mode || 'school' };

  const kindButtons = [h('button', { class: 'active' }, '查寝 / 宿舍考勤')];

  const modeButtons = ['school', 'custom'].map((mode) =>
    h('button', {
      class: form.mode === mode ? 'active' : '',
      onclick: () => {
        form.mode = mode;
        modeButtons.forEach((b, i) => b.classList.toggle('active', ['school', 'custom'][i] === mode));
        customFields.hidden = mode !== 'custom';
      },
    }, mode === 'school' ? '跟随学校时间' : '自定义时段'),
  );

  const startInput = h('input', { type: 'time', value: existing?.windowStart || '00:10' });
  const endInput = h('input', { type: 'time', value: existing?.windowEnd || '23:30' });
  const customFields = h('div', { class: 'field-row', hidden: (existing?.mode || 'school') !== 'custom' },
    h('label', { class: 'field' }, h('span', null, '开始时间'), startInput),
    h('label', { class: 'field' }, h('span', null, '结束时间'), endInput),
  );

  const selectedDays = new Set(existing?.days?.length ? existing.days : [0, 1, 2, 3, 4, 5, 6]);
  const dayChips = WEEK_NAMES.map((label, idx) =>
    h('button', {
      class: selectedDays.has(idx) ? 'active' : '',
      onclick: (e) => {
        if (selectedDays.has(idx)) selectedDays.delete(idx);
        else selectedDays.add(idx);
        e.currentTarget.classList.toggle('active', selectedDays.has(idx));
      },
    }, label));

  const error = h('p', { class: 'form-error' });

  const body = h('div', null,
    h('label', { class: 'field' }, h('span', null, '计划名称'), nameInput),
    h('label', { class: 'field' }, h('span', null, '选择账号'), accountSelect),
    h('div', { class: 'field' }, h('span', null, '任务类型'), h('div', { class: 'kind-toggle' }, kindButtons)),
    h('div', { class: 'field' }, h('span', null, '执行方式'), h('div', { class: 'kind-toggle' }, modeButtons)),
    customFields,
    h('div', { class: 'field' },
      h('span', null, '执行日期'),
      h('div', { class: 'kind-toggle', style: { gridTemplateColumns: 'repeat(4, 1fr)' } }, dayChips),
      h('span', { class: 'hint-text' }, '每天最多成功执行一次，重复提交会被自动跳过。'),
    ),
    error,
  );

  openModal({
    title: isEdit ? '编辑计划' : '新增计划',
    wide: true,
    body,
    footer: [
      h('button', { class: 'btn ghost', onclick: closeModal }, '取消'),
      h('button', {
        class: 'btn primary',
        onclick: async (e) => {
          const btn = e.currentTarget;
          const payload = {
            accountId: Number(accountSelect.value),
            name: nameInput.value.trim(),
            kind: form.kind,
            mode: form.mode,
            windowStart: startInput.value,
            windowEnd: endInput.value,
            days: [...selectedDays].sort((a, b) => a - b),
            enabled: true,
          };
          if (!payload.accountId) { error.textContent = '请选择账号'; return; }
          if (payload.days.length === 0) { error.textContent = '请至少选择一天'; return; }
          if (!payload.name) {
            payload.name = '自动' + kindLabel(payload.kind) + '计划';
          }
          btn.disabled = true;
          btn.textContent = '保存中…';
          try {
            if (isEdit) await api.updatePlan(existing.id, payload);
            else await api.createPlan(payload);
            toast(isEdit ? '计划已更新' : '计划已创建', 'ok');
            closeModal();
            await renderTasks(root, ctx);
          } catch (err) {
            error.textContent = err.message;
            btn.disabled = false;
            btn.textContent = '保存';
          }
        },
      }, '保存'),
    ],
  });
}

async function removePlan(plan, ctx, root) {
  const ok = await confirmDialog({
    title: '删除计划',
    message: `确定删除「${plan.name}」吗？已生成的历史记录会保留。`,
    confirmText: '删除',
    danger: true,
  });
  if (!ok) return;
  try {
    await api.deletePlan(plan.id);
    toast('计划已删除', 'ok');
    await renderTasks(root, ctx);
  } catch (err) {
    toast(err.message, 'err');
  }
}

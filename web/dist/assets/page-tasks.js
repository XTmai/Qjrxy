/* 任务执行页：手动任务 + 自动计划 */
import { api } from './api.js';
import {
  setChildren,
  h,
  icon,
  toast,
  badge,
  kvList,
  pageHeader,
  card,
  toolbar,
  tabs,
  segmented,
  formField,
  loadingState,
  emptyState,
  errorState,
  openModal,
  closeModal,
  confirmDialog,
  timeRange,
  taskState,
  formatDateTime,
  kindLabel,
} from './ui.js';

const state = {
  accounts: [],
  tasks: [],
  plans: [],
  accountId: null,
  statusTab: 'all',
  tab: 'manual',
};

const WEEK = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];

export async function renderTasks(root, ctx) {
  setChildren(root, loadingState('正在加载任务…'));

  const hash = location.hash;
  const qs = new URLSearchParams((hash.split('?')[1] || ''));
  if (qs.get('account')) state.accountId = Number(qs.get('account'));
  if (hash.includes('auto')) state.tab = 'auto';

  try {
    const [acct, taskRes, planRes] = await Promise.all([
      api.accounts(), api.tasks(), api.plans(),
    ]);
    state.accounts = acct.accounts;
    state.tasks = taskRes.tasks;
    state.plans = planRes.plans;
  } catch (err) {
    setChildren(root, 
      pageHeader({ title: '任务执行' }),
      errorState('加载失败', err,
        h('button', { class: 'btn primary sm', onclick: () => renderTasks(root, ctx) }, '重试')),
    );
    return;
  }

  if (state.accountId && !state.accounts.some((a) => a.id === state.accountId)) {
    state.accountId = null;
  }
  if (!state.accountId && state.accounts.length) state.accountId = state.accounts[0].id;

  draw(root, ctx);
}

function draw(root, ctx) {
  const head = pageHeader({
    title: '任务执行',
    sub: '手动完成今日查寝，或设置自己的自动执行计划',
  });

  const seg = segmented({
    items: [['manual', '手动任务'], ['auto', '自动计划']],
    active: state.tab,
    onSelect: (key) => { state.tab = key; draw(root, ctx); },
  });

  const body = state.tab === 'manual' ? manualView(root, ctx) : autoView(root, ctx);
  setChildren(root, head, h('div', { class: 'tab-switch' }, seg), body);
}

/* ================= 手动任务 ================= */

function manualView(root, ctx) {
  if (!state.accounts.length) {
    return emptyState('还没有绑定账号', '先添加校园账号才能查询查寝任务',
      h('button', { class: 'btn primary sm', onclick: () => ctx.go('/accounts') }, '去添加'));
  }

  const accountSelect = h('select', {
    onchange: (e) => {
      state.accountId = Number(e.target.value);
      draw(root, ctx);
    },
  }, state.accounts.map((a) => h('option', {
    value: a.id,
    selected: a.id === state.accountId,
  }, `${a.alias || a.username} · ${a.schoolName}`)));

  const pullBtn = h('button', {
    class: 'btn primary block',
    onclick: async (e) => {
      const btn = e.currentTarget;
      btn.disabled = true;
      btn.textContent = '拉取中…';
      try {
        await api.syncTasks(state.accountId);
        const { tasks } = await api.tasks();
        state.tasks = tasks;
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
    card({
      title: '筛选',
      pad: true,
      body: h('div', null,
        formField({ label: '选择账号', input: accountSelect }),
        pullBtn,
      ),
    }),
  );

  const mine = state.tasks.filter(
    (t) => t.accountId === state.accountId && t.kind === 'attendance');

  const runnable = mine.filter((t) => taskState(t).runnable);
  const finished = mine.filter((t) => !taskState(t).runnable);

  const tabBar = tabs({
    items: [
      ['all', `全部 ${mine.length}`],
      ['run', `可执行 ${runnable.length}`],
      ['done', `已结束 ${finished.length}`],
    ],
    active: state.statusTab,
    onSelect: (key) => { state.statusTab = key; draw(root, ctx); },
  });

  const shown = state.statusTab === 'run' ? runnable
    : state.statusTab === 'done' ? finished : mine;

  const list = h('div', { class: 'task-list' });
  if (!shown.length) {
    list.append(emptyState(
      mine.length ? '这个分类下没有任务' : '还没有查寝任务',
      mine.length ? '切换上方分类看看' : '点左侧「拉取今日任务」从学校同步',
      mine.length ? null : h('button', {
        class: 'btn primary sm',
        onclick: () => pullBtn.click(),
      }, '立即拉取'),
    ));
  } else {
    shown.forEach((t) => list.append(taskCard(t, ctx, root)));
  }

  const right = h('div', { class: 'task-panel' },
    h('div', { class: 'task-count' },
      h('h2', null, '查寝 / 宿舍考勤'),
      h('span', null, `${mine.length} 条`),
    ),
    tabBar,
    list,
  );

  return h('div', { class: 'task-layout' }, panel, right);
}

function taskCard(task, ctx, root) {
  const st = taskState(task);

  const runBtn = h('button', {
    class: 'btn ' + (st.runnable ? 'primary' : 'ghost') + ' sm',
    disabled: !st.runnable,
    title: st.runnable ? '' : st.text,
    onclick: async (e) => {
      const btn = e.currentTarget;
      const ok = await confirmDialog({
        title: '执行查寝',
        message: `即将为「${task.accountAlias || '当前账号'}」执行「${task.taskName || '查寝'}」。`
          + '提交前请确认坐标与照片符合学校要求。',
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
          toast('待核对：' + (res.hint || res.message || ''), 'info');
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
  }, st.runnable ? '执行' : st.text);

  return h('div', { class: 'task-card' },
    h('div', { class: 'task-main' },
      h('div', { class: 'task-title' }, task.taskName || '查寝'),
      h('div', { class: 'task-time' }, timeRange(task)),
      h('div', { class: 'task-tags' },
        h('span', { class: 'badge ' + st.tone }, st.text),
        task.sender ? h('span', { class: 'badge plain' }, task.sender) : null,
      ),
    ),
    runBtn,
  );
}

/* ================= 自动计划 ================= */

function autoView(root, ctx) {
  const enabled = state.plans.filter((p) => p.enabled).length;
  const attention = state.plans.filter(
    (p) => p.lastResult && !/成功|已完成/.test(p.lastResult)).length;

  const hero = h('div', { class: 'plan-hero' },
    h('div', { class: 'plan-hero-icon' }, icon('clock')),
    h('div', { class: 'plan-hero-text' },
      h('h2', null, '把日常查寝交给计划'),
      h('p', null, '选择账号和执行时段，系统会在时段内自动检查并提交。关闭页面后也会继续执行。'),
    ),
    h('button', {
      class: 'btn primary',
      disabled: !state.accounts.length,
      onclick: () => openPlanForm(ctx, null, root),
    }, icon('plus'), '新增计划'),
  );

  const stats = h('div', { class: 'plan-stats' },
    planStat(state.plans.length, '全部计划'),
    planStat(enabled, '已开启'),
    planStat(attention, '需要处理'),
  );

  const list = h('div', { class: 'plan-list' });
  if (!state.plans.length) {
    list.append(emptyState('还没有自动计划',
      state.accounts.length ? '创建计划后，系统会在设定时段自动执行查寝' : '先添加账号再创建计划',
      state.accounts.length
        ? h('button', { class: 'btn primary sm', onclick: () => openPlanForm(ctx, null, root) }, '新增计划')
        : h('button', { class: 'btn primary sm', onclick: () => ctx.go('/accounts') }, '去添加账号')));
  } else {
    state.plans.forEach((p) => list.append(planCard(p, ctx, root)));
  }

  const recent = state.plans
    .filter((p) => p.lastRunAt)
    .sort((a, b) => String(b.lastRunAt).localeCompare(String(a.lastRunAt)))
    .slice(0, 5);

  const side = h('div', null,
    h('div', { class: 'side-title' },
      h('h3', null, '最近自动执行'),
      h('button', { class: 'link-btn', onclick: () => ctx.go('/logs') }, '全部记录'),
    ),
    recent.length
      ? h('div', { class: 'mini-list' },
          recent.map((p) => h('div', { class: 'mini-row' },
            h('div', null,
              h('strong', null, p.name),
              h('span', null, `${p.accountAlias || p.accountUsername || '账号'} · ${kindLabel(p.kind)}`),
            ),
            h('span', {
              class: 'badge ' + (/成功|已完成/.test(p.lastResult) ? 'ok' : 'warn'),
            }, /成功|已完成/.test(p.lastResult) ? '已完成' : '需核对'),
          )),
        )
      : h('div', { class: 'box-empty' }, '暂无自动执行记录'),

    h('details', { class: 'disclosure' },
      h('summary', null, '执行前准备与说明'),
      h('ol', null,
        h('li', null, '确认账号已登录，设备 ID 与打卡手机一致。'),
        h('li', null, '确认签到位置已通过「自动获取」写入。'),
        h('li', null, '任务需要照片时，先在账号中上传照片。'),
        h('li', null, '「跟随学校时间」会在任务下发后尽快执行；「自定义时段」只在区间内执行。'),
        h('li', null, '每个计划每天最多成功执行一次。'),
      ),
    ),
  );

  return h('div', null,
    hero, stats,
    h('div', { class: 'plan-grid' },
      h('div', null,
        h('div', { class: 'side-title' },
          h('h3', null, '我的计划'),
          h('button', { class: 'btn ghost xs', onclick: () => renderTasks(root, ctx) }, '刷新'),
        ),
        list,
      ),
      side,
    ),
  );
}

function planStat(value, label) {
  return h('div', { class: 'plan-stat' },
    h('strong', null, String(value)),
    h('span', null, label),
  );
}

function planCard(plan, ctx, root) {
  const toggle = h('button', {
    class: 'switch ' + (plan.enabled ? 'on' : ''),
    title: plan.enabled ? '点击暂停' : '点击启用',
    'aria-label': plan.enabled ? '暂停计划' : '启用计划',
    onclick: async (e) => {
      const btn = e.currentTarget;
      btn.disabled = true;
      try {
        await api.togglePlan(plan.id, !plan.enabled);
        toast(plan.enabled ? '计划已暂停' : '计划已启用', 'ok');
        await renderTasks(root, ctx);
      } catch (err) {
        toast(err.message, 'err');
        btn.disabled = false;
      }
    },
  });

  const modeText = plan.mode === 'school' ? '跟随学校下发时间' : '自定义执行时段';

  return h('div', { class: 'plan-card' },
    h('div', { class: 'plan-top' },
      h('div', null,
        h('h4', null, plan.name),
        h('div', { class: 'plan-account' },
          `${plan.accountAlias || plan.accountUsername || '账号'} · ${plan.schoolName || ''}`),
      ),
      toggle,
    ),
    h('div', { class: 'plan-mode' }, modeText),
    h('div', { class: 'plan-meta' },
      metaRow('执行日期', describeDays(plan.days)),
      plan.mode === 'custom'
        ? metaRow('执行时段', `${plan.windowStart} – ${plan.windowEnd}`)
        : null,
      metaRow('最近执行', plan.lastRunAt ? formatDateTime(plan.lastRunAt) : '尚未执行'),
      plan.lastResult ? metaRow('执行结果', plan.lastResult) : null,
    ),
    h('div', { class: 'plan-acts' },
      h('button', {
        class: 'btn ghost xs',
        onclick: () => openPlanForm(ctx, plan, root),
      }, '编辑'),
      h('button', {
        class: 'btn ghost-danger xs',
        onclick: () => removePlan(plan, ctx, root),
      }, '删除'),
    ),
  );
}

function metaRow(label, value) {
  return h('div', { class: 'meta-row' },
    h('span', null, label),
    h('b', null, value),
  );
}

function describeDays(days) {
  const set = new Set(days || []);
  if (set.size === 7) return '每天';
  if (set.size === 5 && [0, 1, 2, 3, 4].every((d) => set.has(d))) return '工作日';
  if (set.size === 2 && set.has(5) && set.has(6)) return '周末';
  return [...set].sort((a, b) => a - b).map((d) => WEEK[d]).join('、') || '每天';
}

function openPlanForm(ctx, existing, root) {
  const isEdit = Boolean(existing);
  const form = {
    mode: existing?.mode || 'school',
    days: new Set(existing?.days?.length ? existing.days : [0, 1, 2, 3, 4, 5, 6]),
  };

  const accountSel = h('select', null, state.accounts.map((a) => h('option', {
    value: a.id,
    selected: existing ? a.id === existing.accountId : a.id === state.accountId,
  }, `${a.alias || a.username} · ${a.schoolName}`)));

  const nameInput = h('input', {
    placeholder: '如：晚间查寝计划',
    value: existing?.name || '',
  });

  const modeButtons = [
    ['school', '跟随学校时间'],
    ['custom', '自定义时段'],
  ].map(([key, label]) => h('button', {
    class: form.mode === key ? 'active' : '',
    dataset: { mode: key },
    onclick: () => {
      form.mode = key;
      modeButtons.forEach((b) => b.classList.toggle('active', b.dataset.mode === key));
      customFields.hidden = key !== 'custom';
    },
  }, label));

  const startInput = h('input', { type: 'time', value: existing?.windowStart || '00:10' });
  const endInput = h('input', { type: 'time', value: existing?.windowEnd || '23:30' });
  const customFields = h('div', { class: 'field-row', hidden: form.mode !== 'custom' },
    formField({ label: '开始时间', input: startInput }),
    formField({ label: '结束时间', input: endInput }),
  );

  const dayButtons = WEEK.map((label, idx) => h('button', {
    class: form.days.has(idx) ? 'active' : '',
    onclick: (e) => {
      if (form.days.has(idx)) form.days.delete(idx);
      else form.days.add(idx);
      e.currentTarget.classList.toggle('active', form.days.has(idx));
    },
  }, label));

  const err = h('p', { class: 'form-error' });

  const body = h('div', null,
    formField({ label: '计划名称', input: nameInput }),
    formField({ label: '选择账号', input: accountSel }),
    formField({
      label: '执行方式',
      input: h('div', { class: 'choice-group' }, modeButtons),
      hint: '跟随学校时间：任务下发后尽快执行；自定义时段：只在设定区间内执行。',
    }),
    customFields,
    formField({
      label: '执行日期',
      input: h('div', { class: 'choice-group days' }, dayButtons),
      hint: '每天最多成功执行一次，重复提交会被自动跳过。',
    }),
    err,
  );

  openModal({
    title: isEdit ? '编辑计划' : '新建计划',
    sub: '按设定时间自动执行查寝',
    body,
    wide: true,
    footer: [
      h('button', { class: 'btn ghost', onclick: closeModal }, '取消'),
      h('button', {
        class: 'btn primary',
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
            toast(isEdit ? '计划已更新' : '计划已创建', 'ok');
            closeModal();
            await renderTasks(root, ctx);
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

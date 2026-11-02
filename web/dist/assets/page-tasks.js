/* 查寝任务页 */
import { api } from './api.js';
import {
  setChildren,
  el,
  ico,
  toast,
  skeleton,
  emptyBox,
  timeRange,
  taskState,
  openModal,
  closeModal,
  confirmBox,
} from './ui.js';

const state = { accountId: null, tab: 'all' };

export async function pageTasks(root, ctx) {
  setChildren(root, skeleton());
  let accounts = [];
  let tasks = [];
  try {
    const [a, t] = await Promise.all([api.accounts(), api.tasks()]);
    accounts = a.accounts;
    tasks = t.tasks;
  } catch (err) {
    setChildren(root, emptyBox('加载失败', err.message,
      el('button', { class: 'btn main', onclick: () => pageTasks(root, ctx) }, '重试')));
    return;
  }

  if (state.accountId && !accounts.some((a) => a.id === state.accountId)) {
    state.accountId = null;
  }
  if (!state.accountId && accounts.length) state.accountId = accounts[0].id;

  paint(root, ctx, accounts, tasks);
}

function paint(root, ctx, accounts, tasks) {
  const head = el('div', { class: 'head' },
    el('div', null,
      el('h1', null, '查寝任务'),
      el('div', { class: 'sub' }, '查询并执行宿舍考勤任务'),
    ),
  );

  if (!accounts.length) {
    setChildren(root, head, emptyBox('还没有绑定账号', '先添加校园账号才能查询任务',
      el('button', { class: 'btn main', onclick: () => ctx.go('/accounts') }, '去添加')));
    return;
  }

  const select = el('select', {
    onchange: (e) => {
      state.accountId = Number(e.target.value);
      paint(root, ctx, accounts, tasks);
    },
  }, accounts.map((a) => el('option', {
    value: a.id,
    selected: a.id === state.accountId,
  }, (a.alias || a.username) + ' · ' + a.schoolName)));

  const syncBtn = el('button', {
    class: 'btn main',
    onclick: async (e) => {
      const btn = e.currentTarget;
      btn.disabled = true;
      btn.textContent = '拉取中…';
      try {
        await api.syncTasks(state.accountId);
        const { tasks: fresh } = await api.tasks();
        toast('任务已更新', 'ok');
        paint(root, ctx, accounts, fresh);
      } catch (err) {
        toast(err.message, 'err');
        btn.disabled = false;
        btn.textContent = '拉取任务';
      }
    },
  }, ico('refresh'), '拉取任务');

  const mine = tasks.filter((t) => t.accountId === state.accountId && t.kind === 'attendance');
  const runnable = mine.filter((t) => taskState(t).runnable);
  const finished = mine.filter((t) => !taskState(t).runnable);

  const tabs = [['all', '全部 ' + mine.length], ['run', '可执行 ' + runnable.length],
    ['done', '已结束 ' + finished.length]];
  const tabBar = el('div', { class: 'tabs' },
    tabs.map(([key, label]) => el('button', {
      class: state.tab === key ? 'on' : '',
      onclick: () => { state.tab = key; paint(root, ctx, accounts, tasks); },
    }, label)),
  );

  const shown = state.tab === 'run' ? runnable : state.tab === 'done' ? finished : mine;

  const list = el('div', { class: 'task-list' });
  if (!shown.length) {
    list.append(emptyBox(
      mine.length ? '这个分类下没有任务' : '还没有查寝任务',
      mine.length ? '切换上方分类看看' : '点「拉取任务」从学校同步',
    ));
  } else {
    shown.forEach((t) => list.append(taskCard(t, accounts, ctx, root)));
  }

  setChildren(root, 
    head,
    el('div', { class: 'toolbar' },
      el('div', { class: 'search-wrap' }, select),
      syncBtn,
    ),
    tabBar,
    list,
  );
}

function taskCard(task, accounts, ctx, root) {
  const st = taskState(task);
  const acc = accounts.find((a) => a.id === task.accountId);

  const runBtn = el('button', {
    class: 'btn ' + (st.runnable ? 'main' : 'plain') + ' sm',
    disabled: !st.runnable,
    onclick: async (e) => {
      const btn = e.currentTarget;
      const ok = await confirmBox({
        title: '执行查寝',
        message: '即将为「' + (acc?.alias || acc?.username) + '」执行「' +
          task.taskName + '」。确认提交吗？',
        okText: '立即执行',
      });
      if (!ok) return;
      btn.disabled = true;
      btn.textContent = '执行中…';
      try {
        const res = await api.runTask(task.accountId, task.id, task.kind);
        if (res.status === 'success') toast('执行成功：' + (res.message || '已完成'), 'ok');
        else if (res.status === 'pending') toast('待核对：' + (res.hint || res.message || ''), '');
        else toast('执行失败：' + (res.hint || res.message || ''), 'err');
        await pageTasks(root, ctx);
      } catch (err) {
        toast(err.message, 'err');
        btn.disabled = false;
        btn.textContent = '执行';
      }
    },
  }, st.runnable ? '执行' : st.text);

  return el('div', { class: 'task' },
    el('div', { class: 'task-main' },
      el('div', { class: 'task-name' }, task.taskName || '查寝'),
      el('div', { class: 'task-time' }, timeRange(task)),
      el('div', { class: 'task-tags' },
        el('span', { class: 'tag ' + st.cls }, st.text),
        task.sender ? el('span', { class: 'tag plain' }, task.sender) : null,
        task.stuSignWid ? el('span', { class: 'tag plain' }, '考勤 ' + task.stuSignWid) : null,
      ),
    ),
    runBtn,
  );
}

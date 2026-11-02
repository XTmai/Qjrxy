/* 概览页 */
import { api } from './api.js';
import {
  setChildren,
  el,
  ico,
  toast,
  skeleton,
  emptyBox,
  logTag,
  accountTag,
  fmtTime,
  todayText,
  timeRange,
  taskState,
  openModal,
  closeModal,
} from './ui.js';

export async function pageHome(root, ctx) {
  setChildren(root, skeleton());
  let data;
  try {
    data = await api.dashboard();
  } catch (err) {
    setChildren(root, 
      el('div', { class: 'head' }, el('h1', null, '概览')),
      emptyBox('加载失败', err.message,
        el('button', { class: 'btn main', onclick: () => pageHome(root, ctx) }, '重试')),
    );
    return;
  }

  const { stats, accounts, recentLogs, tasks, plans, activePlans, pendingTask, user } = data;
  const left = Math.max(0, (user?.quota || 0) - (user?.quotaUsed || 0));

  const head = el('div', { class: 'head' },
    el('div', null,
      el('h1', null, '概览'),
      el('div', { class: 'sub' }, todayText() + ' · 数据来自今日校园'),
    ),
    el('div', { class: 'head-actions' },
      el('button', { class: 'btn main', onclick: () => ctx.go('/tasks') },
        ico('play'), '执行查寝'),
      el('button', { class: 'btn plain', onclick: () => ctx.go('/plans') },
        ico('clock'), '自动计划'),
      el('button', { class: 'btn plain', onclick: () => pageHome(root, ctx) },
        ico('refresh'), '刷新'),
    ),
  );

  const statRow = el('div', { class: 'stats' },
    statCard('今日执行', stats.todayTotal, '按北京时间统计', ''),
    statCard('已完成', stats.todaySuccess, '学校已确认', 'ok'),
    statCard('失败', stats.todayFailed, '查看失败原因', 'err'),
    statCard('待核对', stats.todayPending, '需要人工确认', 'warn'),
  );

  // 待办摘要
  const pendingTasks = (tasks || []).filter((t) => t.kind === 'attendance' && taskState(t).runnable);
  const notice = pendingTasks.length
    ? el('div', { class: 'note-box warn' },
        '有 ' + pendingTasks.length + ' 个查寝任务可以执行，最早的是「' +
        pendingTasks[0].taskName + '」（' + timeRange(pendingTasks[0]) + '）')
    : null;

  const grid = el('div', { class: 'home-grid' },
    el('div', { class: 'home-main' },
      accountsCard(accounts, ctx),
      recentCard(recentLogs, ctx),
    ),
    el('div', { class: 'home-side' },
      quotaCard(user, left, ctx),
      planCard(plans, activePlans, pendingTask, ctx),
      helpCard(),
    ),
  );

  setChildren(root, head, notice, statRow, grid);
}

function statCard(label, value, note, tone) {
  return el('div', { class: 'stat' },
    el('div', { class: 'stat-top' },
      el('span', null, label),
      el('i', { class: tone || '' }),
    ),
    el('div', { class: 'stat-num' }, String(value ?? 0)),
    el('div', { class: 'stat-note' }, note),
  );
}

function accountsCard(accounts, ctx) {
  const card = el('div', { class: 'card' },
    el('div', { class: 'card-head' },
      el('div', null,
        el('h2', null, '校园账号 ' + accounts.length),
        el('div', { class: 'note' },
          accounts.filter((a) => a.loginStatus !== 'authorized').length
            ? accounts.filter((a) => a.loginStatus !== 'authorized').length + ' 个需要重新登录'
            : '全部登录正常'),
      ),
      el('button', { class: 'text-btn', onclick: () => ctx.go('/accounts') }, '管理'),
    ),
  );

  if (!accounts.length) {
    card.append(emptyBox('还没有绑定账号', '添加你的今日校园账号后即可查询查寝任务',
      el('button', { class: 'btn main sm', onclick: () => ctx.go('/accounts') }, '添加账号')));
    return card;
  }

  const rows = accounts.map((a) => el('tr', null,
    el('td', null,
      el('div', { class: 'cell-strong' }, a.alias || a.username),
      el('div', { class: 'cell-sub' }, a.username),
    ),
    el('td', null, a.schoolName),
    el('td', null, accountTag(a.loginStatus)),
    el('td', null,
      a.deviceId
        ? el('span', { class: 'tag plain' }, a.deviceModel || '已绑定')
        : el('span', { class: 'tag warn' }, '未绑设备'),
    ),
    el('td', { class: 'right' },
      el('div', { class: 'cell-acts' },
        el('button', { class: 'text-btn', onclick: () => ctx.go('/tasks') }, '任务'),
      ),
    ),
  ));

  card.append(el('div', { class: 'table-wrap' },
    el('table', { class: 'list' },
      el('thead', null, el('tr', null,
        el('th', null, '账号'), el('th', null, '学校'), el('th', null, '登录'),
        el('th', null, '设备'), el('th', { class: 'right' }, '操作'),
      )),
      el('tbody', null, rows),
    ),
  ));
  return card;
}

function recentCard(logs, ctx) {
  const card = el('div', { class: 'card' },
    el('div', { class: 'card-head' },
      el('h2', null, '最近执行'),
      el('button', { class: 'text-btn', onclick: () => ctx.go('/logs') }, '全部记录'),
    ),
  );

  if (!logs || !logs.length) {
    card.append(emptyBox('暂无执行记录', '执行一次查寝后这里会显示结果'));
    return card;
  }

  const rows = logs.map((l) => el('tr', null,
    el('td', null,
      el('div', { class: 'cell-strong' }, l.taskName || '查寝'),
      el('div', { class: 'cell-sub' }, l.source === 'auto' ? '自动执行' : '手动执行'),
    ),
    el('td', null, l.accountAlias || l.accountUsername || '—'),
    el('td', null, logTag(l.status)),
    el('td', { class: 'right cell-sub' }, fmtTime(l.createdAt)),
  ));

  card.append(el('div', { class: 'table-wrap' },
    el('table', { class: 'list' },
      el('thead', null, el('tr', null,
        el('th', null, '任务'), el('th', null, '账号'), el('th', null, '状态'),
        el('th', { class: 'right' }, '时间'),
      )),
      el('tbody', null, rows),
    ),
  ));
  return card;
}

function quotaCard(user, left, ctx) {
  return el('div', { class: 'card' },
    el('div', { class: 'card-head' },
      el('h2', null, '可用额度'),
      el('button', { class: 'text-btn', onclick: () => ctx.go('/quota') }, '兑换'),
    ),
    el('div', { class: 'card-body' },
      el('div', { style: { display: 'flex', alignItems: 'baseline', gap: '6px' } },
        el('span', { style: { fontSize: '30px', fontWeight: '700', letterSpacing: '-0.03em' } },
          String(left)),
        el('span', { style: { fontSize: '13px', color: 'var(--text-faint)' } }, '次'),
      ),
      el('div', { class: 'stat-note', style: { marginTop: '4px' } },
        '已使用 ' + (user?.quotaUsed || 0) + ' 次'),
    ),
  );
}

function planCard(total, active, pending, ctx) {
  const item = (num, label) => el('div', { style: { textAlign: 'center' } },
    el('div', { style: { fontSize: '20px', fontWeight: '700' } }, String(num || 0)),
    el('div', { style: { fontSize: '11.5px', color: 'var(--text-faint)' } }, label),
  );
  return el('div', { class: 'card' },
    el('div', { class: 'card-head' },
      el('h2', null, '自动计划'),
      el('button', { class: 'text-btn', onclick: () => ctx.go('/plans') }, '管理'),
    ),
    el('div', { class: 'card-body' },
      el('div', { style: { display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: '6px' } },
        item(total, '全部'), item(active, '运行中'), item(pending, '待办'),
      ),
    ),
  );
}

function helpCard() {
  return el('div', { class: 'card' },
    el('div', { class: 'card-head' }, el('h2', null, '使用提示')),
    el('div', { class: 'card-body' },
      el('div', { class: 'note-box info' },
        '第一次使用：添加账号 → 扫码获取设备信息 → 自动获取签到位置 → 登录 → 拉取任务。'),
      el('button', {
        class: 'btn plain sm full', style: { marginTop: '10px' },
        onclick: showHelp,
      }, '查看完整步骤'),
    ),
  );
}

function showHelp() {
  const steps = [
    ['添加账号', '填写学校全称、学号、密码。学校名称输入几个字会自动匹配。'],
    ['扫码获取设备信息', '用你平时打卡的那台手机打开今日校园 App，扫一扫生成的二维码。设备 ID、机型、系统版本、UA 会自动填入。'],
    ['自动获取签到位置', '直接读取学校设置的签到点，不需要手动选点或填坐标。'],
    ['登录学校账号', '点「账密登录」，输入学校密码。需要验证码时会弹窗输入。'],
    ['执行查寝', '拉取任务后点执行。任务未到签到时间时会被拦住，避免无效提交。'],
  ];
  openModal({
    title: '使用步骤',
    sub: '按顺序完成以下操作',
    body: el('ol', { style: { paddingLeft: '20px', lineHeight: '2', fontSize: '13.5px' } },
      steps.map(([t, d]) => el('li', null,
        el('b', null, t + '：'),
        el('span', { style: { color: 'var(--text-muted)' } }, d),
      )),
    ),
    actions: el('button', { class: 'btn main', onclick: closeModal }, '知道了'),
  });
}

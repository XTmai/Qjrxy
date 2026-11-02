/* 工作台：今日统计、账号概览、最近执行、额度与公告 */
import { api } from './api.js';
import {
  setChildren,
  h,
  icon,
  toast,
  formatDateTime,
  todayLabel,
  badge,
  kvList,
  pageHeader,
  card,
  statGrid,
  dataTable,
  loadingState,
  emptyState,
  errorState,
  openModal,
  closeModal,
  kindLabel,
  relativeTime,
} from './ui.js';

const TIPS = [
  {
    tag: '快速开始',
    title: '三步完成第一次查寝',
    text: '添加账号 → 扫码获取设备信息 → 自动获取签到位置。完成后即可查询并执行任务。',
  },
  {
    tag: '设备绑定',
    title: '设备 ID 必须来自你打卡的那台手机',
    text: '用今日校园 App 扫一扫生成的二维码，设备 ID、机型、系统版本会自动填入。换手机会导致设备 ID 变化。',
  },
  {
    tag: '任务时间',
    title: '任务未到签到时间会被自动拦截',
    text: '查寝任务通常只在固定时段内可签到。列表里会显示「未到时间」，到点后按钮才会变可用。',
  },
];

export async function renderDashboard(root, ctx) {
  setChildren(root, loadingState('正在加载工作台…'));

  let data;
  try {
    data = await api.dashboard();
  } catch (err) {
    setChildren(root, 
      pageHeader({ title: '工作台' }),
      errorState('加载失败', err,
        h('button', { class: 'btn primary sm', onclick: () => renderDashboard(root, ctx) }, '重试')),
    );
    return;
  }

  const { stats, accounts, recentLogs, tasks, plans, activePlans, pendingTask, user } = data;
  const remaining = Math.max(0, (user?.quota || 0) - (user?.quotaUsed || 0));
  const runnable = (tasks || []).filter((t) => t.kind === 'attendance' && !t.status?.includes('done'));

  const head = pageHeader({
    title: '工作台',
    sub: `${todayLabel()} · 更新于 ${new Date().toTimeString().slice(0, 5)}`,
    actions: [
      h('button', { class: 'btn primary', onclick: () => ctx.go('/tasks') }, icon('play'), '执行任务'),
      h('button', { class: 'btn ghost', onclick: () => ctx.go('/tasks#auto') }, icon('clock'), '自动计划'),
      h('button', { class: 'btn ghost', onclick: () => renderDashboard(root, ctx) }, icon('refresh'), '刷新'),
    ],
  });

  const statRow = statGrid([
    { label: '今日执行', value: stats.todayTotal, note: '按北京时间统计', tone: '' },
    { label: '已完成', value: stats.todaySuccess, note: '学校已确认结果', tone: 'green' },
    { label: '失败', value: stats.todayFailed, note: '查看失败原因', tone: 'red' },
    { label: '待核对', value: stats.todayPending, note: '正在确认结果', tone: 'amber' },
  ]);

  const grid = h('div', { class: 'dash-grid' },
    h('div', { class: 'dash-main' },
      accountsCard(accounts, ctx),
      recentCard(recentLogs, ctx),
    ),
    h('div', { class: 'dash-side' },
      quotaCard(user, remaining, ctx),
      planCard(plans, activePlans, runnable.length, ctx),
      tipsCard(),
    ),
  );

  setChildren(root, head, statRow, grid);
}

function accountsCard(accounts, ctx) {
  const needLogin = accounts.filter((a) => a.loginStatus !== 'authorized').length;

  return card({
    title: `学生账号 ${accounts.length}`,
    note: needLogin > 0 ? `${needLogin} 个账号需要重新登录` : '全部账号登录状态正常',
    action: h('button', { class: 'link-btn', onclick: () => ctx.go('/accounts') }, '管理账号'),
    body: dataTable({
      columns: [
        {
          key: 'account', label: '账号',
          render: (a) => h('div', null,
            h('div', { class: 'cell-main' }, a.alias || a.username),
            h('div', { class: 'cell-sub' }, a.username),
          ),
        },
        { key: 'schoolName', label: '学校' },
        { key: 'loginStatus', label: '登录', render: (a) => badge('login', a.loginStatus) },
        {
          key: 'device', label: '设备',
          render: (a) => a.deviceId
            ? h('span', { class: 'badge plain' }, a.deviceModel || '已绑定')
            : h('span', { class: 'badge warn' }, '未绑定'),
        },
        {
          key: 'actions', label: '操作', align: 'right',
          render: (a) => h('div', { class: 'cell-actions' },
            h('button', { class: 'link-btn', onclick: () => ctx.go(`/tasks?account=${a.id}`) }, '查看任务'),
            h('button', { class: 'link-btn', onclick: () => ctx.go(`/accounts?id=${a.id}`) }, '设置'),
          ),
        },
      ],
      rows: accounts,
      empty: emptyState('还没有绑定账号', '添加账号后即可查询查寝任务',
        h('button', { class: 'btn primary sm', onclick: () => ctx.go('/accounts') }, '添加账号')),
    }),
  });
}

function recentCard(logs, ctx) {
  return card({
    title: '最近执行',
    action: h('button', { class: 'link-btn', onclick: () => ctx.go('/logs') }, '全部记录'),
    body: dataTable({
      columns: [
        {
          key: 'task', label: '任务',
          render: (l) => h('div', null,
            h('div', { class: 'cell-main' }, l.taskName || kindLabel(l.kind)),
            h('div', { class: 'cell-sub' }, l.source === 'auto' ? '自动执行' : '手动执行'),
          ),
        },
        { key: 'account', label: '账号', render: (l) => l.accountAlias || l.accountUsername || '—' },
        { key: 'status', label: '状态', render: (l) => badge('run', l.status) },
        {
          key: 'time', label: '时间', align: 'right',
          render: (l) => h('span', { class: 'cell-sub', title: formatDateTime(l.createdAt) },
            relativeTime(l.createdAt)),
        },
      ],
      rows: logs || [],
      empty: emptyState('暂无执行记录', '执行一次查寝后这里会显示结果'),
    }),
  });
}

function quotaCard(user, remaining, ctx) {
  return card({
    title: '账户额度',
    action: h('button', { class: 'link-btn', onclick: () => ctx.go('/quota') }, '兑换卡密'),
    body: h('div', { class: 'card-body' },
      h('div', { class: 'quota-line' },
        h('strong', null, String(remaining)),
        h('span', null, '次可用'),
      ),
      h('div', { class: 'stat-foot' }, `累计使用 ${user?.quotaUsed || 0} 次`),
    ),
  });
}

function planCard(total, active, runnable, ctx) {
  const item = (num, label) => h('div', { class: 'mini-stat' },
    h('strong', null, String(num || 0)),
    h('span', null, label),
  );
  return card({
    title: '自动计划',
    action: h('button', { class: 'link-btn', onclick: () => ctx.go('/tasks#auto') }, '管理计划'),
    body: h('div', { class: 'card-body' },
      h('div', { class: 'mini-stat-row' },
        item(total, '全部'),
        item(active, '已开启'),
        item(runnable, '待执行'),
      ),
    ),
  });
}

function tipsCard() {
  return card({
    title: '使用提示',
    action: h('button', { class: 'link-btn', onclick: showTips }, '查看全部'),
    body: h('div', { class: 'card-body' },
      h('div', { class: 'tip-list' },
        TIPS.slice(0, 2).map((t) => h('div', { class: 'tip-item' },
          h('div', { class: 'tip-head' },
            h('span', { class: 'badge info' }, t.tag),
          ),
          h('div', { class: 'tip-title' }, t.title),
          h('div', { class: 'tip-text' }, t.text),
        )),
      ),
    ),
  });
}

export function showTips() {
  openModal({
    title: '使用提示',
    sub: '开始使用前建议先了解这几点',
    wide: true,
    body: h('div', { class: 'tip-list' },
      TIPS.map((t) => h('div', { class: 'tip-item' },
        h('div', { class: 'tip-head' }, h('span', { class: 'badge info' }, t.tag)),
        h('div', { class: 'tip-title' }, t.title),
        h('div', { class: 'tip-text' }, t.text),
      )),
      h('div', { class: 'notice warn' },
        '请在学校及平台允许的范围内，使用本人或已获授权的账号，并如实确认任务信息。'),
    ),
    footer: h('button', { class: 'btn primary', onclick: closeModal }, '知道了'),
  });
}

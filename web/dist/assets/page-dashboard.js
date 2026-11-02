/* 工作台：今日统计、账号概览、最近执行、额度与公告 */
import { api } from './api.js';
import {
  h, icon, formatDateTime, todayLabel, statusBadge, loginBadge,
  skeletonCard, emptyState, kindLabel, openModal, closeModal,
} from './ui.js';

const NOTICES = [
  {
    tag: '重要',
    date: '2026-10-26',
    title: '欢迎使用青衿',
    text: '从添加账号开始，按学校要求确认任务信息。首页「使用教程」可随时查看。',
  },
  {
    date: '2026-10-26',
    title: '自动计划，按你的节奏安排',
    text: '支持跟随学校时间或自定义执行时段，可随时暂停与调整。',
  },
  {
    date: '2026-10-26',
    title: '请先在今日校园完成一次手动签到',
    text: '首次使用新设备时，学校端可能要求设备授权；先在官方 App 完成一次签到成功率更高。',
  },
];

export async function renderDashboard(root, ctx) {
  root.replaceChildren(h('div', { class: 'loading-block' }, skeletonCard(false)));
  let data;
  try {
    data = await api.dashboard();
  } catch (err) {
    root.replaceChildren(
      h('div', { class: 'page-head' }, h('div', null, h('h1', null, '工作台'))),
      emptyState('加载失败', err.message,
        h('button', { class: 'btn primary', onclick: () => renderDashboard(root, ctx) }, '重试')),
    );
    return;
  }

  const { stats, accounts, recentLogs, plans, activePlans, pendingTask, user } = data;

  const head = h('div', { class: 'page-head' },
    h('div', null,
      h('h1', null, '工作台'),
      h('p', { class: 'page-sub' },
        `${todayLabel()} · 更新于 ${new Date().toTimeString().slice(0, 5)}`),
    ),
    h('div', { class: 'page-actions' },
      h('button', { class: 'btn primary', onclick: () => ctx.go('/tasks') }, '执行任务'),
      h('button', { class: 'btn ghost', onclick: () => ctx.go('/tasks#auto') }, '自动计划'),
      h('button', { class: 'btn ghost', onclick: showTutorial }, '使用教程'),
      h('button', {
        class: 'btn ghost', title: '刷新',
        onclick: () => renderDashboard(root, ctx),
      }, icon('refresh'), '刷新'),
    ),
  );

  const statRow = h('div', { class: 'stat-row' },
    statCard('今日执行', stats.todayTotal, '按北京时间统计', ''),
    statCard('已完成', stats.todaySuccess, '学校已确认结果', 'green'),
    statCard('失败', stats.todayFailed, '查看失败原因', 'red'),
    statCard('待核对', stats.todayPending, '正在确认结果', 'amber'),
  );

  const accountsCard = buildAccountsCard(accounts, ctx);
  const recentCard = buildRecentCard(recentLogs, ctx);

  const left = h('div', { class: 'dash-col' }, accountsCard, recentCard);
  const right = h('div', { class: 'dash-col' },
    buildQuotaCard(user, ctx),
    buildPlanCard(plans, activePlans, pendingTask, ctx),
    buildNoticeCard(),
  );

  root.replaceChildren(head, statRow, h('div', { class: 'dash-grid' }, left, right));
}

function statCard(label, value, foot, dot) {
  return h('div', { class: 'stat' },
    h('div', { class: 'stat-label' },
      h('span', null, label),
      h('span', { class: 'stat-dot ' + dot }),
    ),
    h('div', { class: 'stat-value' }, String(value ?? 0)),
    h('div', { class: 'stat-foot' }, foot),
  );
}

function buildAccountsCard(accounts, ctx) {
  const needLogin = accounts.filter((a) => a.loginStatus !== 'authorized').length;

  const card = h('div', { class: 'card' },
    h('div', { class: 'card-head' },
      h('div', null,
        h('h2', null, `账号 ${accounts.length}`),
        h('p', { class: 'hint' },
          needLogin > 0 ? `${needLogin} 个账号需要重新登录` : '全部账号登录状态正常'),
      ),
      h('button', { class: 'link-btn', onclick: () => ctx.go('/accounts') }, '管理账号'),
    ),
  );

  if (accounts.length === 0) {
    card.append(emptyState('还没有绑定账号', '添加今日校园账号后即可查看待办任务',
      h('button', { class: 'btn primary sm', onclick: () => ctx.go('/accounts') }, '添加账号')));
    return card;
  }

  const rows = accounts.map((acc) => h('tr', null,
    h('td', null,
      h('div', { class: 'cell-main' }, acc.alias || acc.username),
      h('div', { class: 'cell-sub' }, acc.username),
    ),
    h('td', null, acc.schoolName),
    h('td', null, loginBadge(acc.loginStatus)),
    h('td', null,
      acc.lon && acc.lat
        ? h('span', { class: 'badge plain' }, '已设置')
        : h('span', { class: 'badge plain' }, '未设置'),
    ),
    h('td', { class: 'col-right' },
      h('div', { class: 'cell-actions' },
        h('button', { class: 'link-btn', onclick: () => ctx.go('/tasks?account=' + acc.id) }, '查看任务'),
        h('button', { class: 'link-btn', onclick: () => ctx.go('/accounts?id=' + acc.id) }, '设置'),
      ),
    ),
  ));

  card.append(h('div', { class: 'table-wrap' },
    h('table', { class: 'data' },
      h('thead', null, h('tr', null,
        h('th', null, '账号'), h('th', null, '学校'), h('th', null, '登录状态'),
        h('th', null, '默认位置'), h('th', { class: 'col-right' }, '操作'),
      )),
      h('tbody', null, rows),
    ),
  ));
  return card;
}

function buildRecentCard(logs, ctx) {
  const card = h('div', { class: 'card' },
    h('div', { class: 'card-head' },
      h('h2', null, '最近执行'),
      h('button', { class: 'link-btn', onclick: () => ctx.go('/logs') }, '全部记录'),
    ),
  );
  if (!logs || logs.length === 0) {
    card.append(emptyState('暂无执行记录', '执行一次签到或查寝后即可在这里看到结果'));
    return card;
  }
  const rows = logs.map((log) => h('tr', null,
    h('td', null,
      h('div', { class: 'cell-main' }, log.taskName || kindLabel(log.kind)),
      h('div', { class: 'cell-sub' }, log.source === 'auto' ? '自动执行' : '手动执行'),
    ),
    h('td', null, log.accountAlias || log.accountUsername || '—'),
    h('td', null, statusBadge(log.status)),
    h('td', { class: 'col-right' }, formatDateTime(log.createdAt)),
  ));
  card.append(h('div', { class: 'table-wrap' },
    h('table', { class: 'data' },
      h('thead', null, h('tr', null,
        h('th', null, '任务'), h('th', null, '账号'), h('th', null, '状态'),
        h('th', { class: 'col-right' }, '时间'),
      )),
      h('tbody', null, rows),
    ),
  ));
  return card;
}

function buildQuotaCard(user, ctx) {
  const remaining = Math.max(0, (user.quota || 0) - (user.quotaUsed || 0));
  return h('div', { class: 'card' },
    h('div', { class: 'card-head' },
      h('h2', null, '账户额度'),
      h('button', { class: 'link-btn', onclick: () => ctx.go('/quota') }, '兑换卡密'),
    ),
    h('div', { class: 'card-body' },
      h('p', { class: 'hint' }, '可用次数'),
      h('div', { class: 'side-num' },
        h('strong', null, String(remaining)),
        h('span', null, '次'),
      ),
      h('div', { class: 'stat-foot', style: { marginTop: '8px' } },
        `累计使用 ${user.quotaUsed || 0} 次`),
    ),
  );
}

function buildPlanCard(plans, activePlans, pendingTask, ctx) {
  // 侧栏较窄，用紧凑的三列数字，避免卡片内的标签被挤成竖排
  const item = (value, label) => h('div', { style: { textAlign: 'center', minWidth: '0' } },
    h('strong', {
      style: { display: 'block', fontSize: '22px', fontWeight: '700', letterSpacing: '-0.02em' },
    }, String(value || 0)),
    h('span', {
      style: {
        display: 'block', marginTop: '2px', fontSize: '11.5px',
        color: 'var(--text-muted)', whiteSpace: 'nowrap',
      },
    }, label),
  );

  return h('div', { class: 'card' },
    h('div', { class: 'card-head' },
      h('h2', null, '自动计划'),
      h('button', { class: 'link-btn', onclick: () => ctx.go('/tasks#auto') }, '管理计划'),
    ),
    h('div', { class: 'card-body' },
      h('div', {
        style: {
          display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
          gap: '6px', padding: '2px 0',
        },
      },
        item(plans, '全部'),
        item(activePlans, '启用'),
        item(pendingTask, '待办'),
      ),
    ),
  );
}

function buildNoticeCard() {
  const card = h('div', { class: 'card' },
    h('div', { class: 'card-head' },
      h('h2', null, '通知公告'),
      h('button', { class: 'link-btn', onclick: showTutorial }, '使用教程'),
    ),
    h('div', { class: 'card-body' }, h('div', { class: 'notice-list' },
      NOTICES.map((n) => h('div', { class: 'notice' },
        h('div', { class: 'notice-meta' },
          n.tag ? h('span', { class: 'notice-tag' }, n.tag) : null,
          h('span', null, n.date),
        ),
        h('p', { class: 'notice-title' }, n.title),
        h('p', { class: 'notice-text' }, n.text),
      )),
    )),
  );
  return card;
}

export function showTutorial() {
  const steps = [
    '在「学生账号」中添加今日校园账号：填写学校全称、学号、密码，并按提示完成登录或导入 Cookie。',
    '用手机今日校园 App 扫描「设备码」，把扫码得到的设备 ID 绑定到账号——设备 ID 与打卡设备必须一致。',
    '设置默认位置：填写签到点附近的地址与经纬度，坐标需落在学校签到点半径内。',
    '在「任务执行」中拉取今日任务，确认任务要求后点击执行；也可在「自动签到」里配置计划。',
    '计划由服务端调度，关闭浏览器后继续运行。执行结果可在「执行记录」中核对。',
  ];
  const body = h('div', null,
    h('div', { class: 'info-box', style: { marginBottom: '14px' } },
      '首次使用同一台设备时，建议先在今日校园 App 里手动完成一次签到，通过学校端的设备授权校验。'),
    h('ol', { style: { paddingLeft: '20px', lineHeight: '1.95', fontSize: '14px', color: 'var(--text-soft)' } },
      steps.map((s) => h('li', null, s)),
    ),
    h('div', { class: 'warn-box', style: { marginTop: '16px' } },
      '请在学校及平台允许的范围内，使用本人或已获授权的账号，并如实确认任务信息。虚拟定位与代签可能违反校规。'),
  );
  openModal({
    title: '使用教程',
    body,
    footer: h('button', { class: 'btn primary', onclick: closeModal }, '我知道了'),
  });
}

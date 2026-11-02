/* 执行记录页 */
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
  dataTable,
  loadingState,
  emptyState,
  errorState,
  openModal,
  closeModal,
  formatDateTime,
  kindLabel,
  relativeTime,
} from './ui.js';

const state = { scope: 'today', status: 'all', page: 1, size: 20 };

const SCOPES = [['today', '今天'], ['week', '近 7 天'], ['all', '全部']];
const STATUS_TABS = [
  ['all', '全部'],
  ['success', '成功'],
  ['failed', '失败'],
  ['pending', '待核对'],
  ['executing', '执行中'],
];

export async function renderLogs(root, ctx) {
  setChildren(root, loadingState('正在加载记录…'));

  let data;
  try {
    data = await api.logs({
      scope: state.scope,
      status: state.status,
      limit: state.size,
      offset: (state.page - 1) * state.size,
    });
  } catch (err) {
    setChildren(root, 
      pageHeader({ title: '执行记录' }),
      errorState('加载记录失败', err,
        h('button', { class: 'btn primary sm', onclick: () => renderLogs(root, ctx) }, '重试')),
    );
    return;
  }

  const { logs, total } = data;
  const pages = Math.max(1, Math.ceil(total / state.size));

  const head = pageHeader({
    title: '执行记录',
    sub: `共 ${total} 条记录`,
    actions: [
      h('button', { class: 'btn ghost', onclick: () => renderLogs(root, ctx) },
        icon('refresh'), '刷新'),
    ],
  });

  const scopeSelect = h('select', {
    onchange: (e) => {
      state.scope = e.target.value;
      state.page = 1;
      renderLogs(root, ctx);
    },
  }, SCOPES.map(([v, l]) => h('option', { value: v, selected: state.scope === v }, l)));

  const bar = toolbar(
    h('div', { class: 'toolbar-left' },
      h('span', { class: 'filter-label' }, '查看范围'),
      scopeSelect,
    ),
  );

  const tabBar = tabs({
    items: STATUS_TABS,
    active: state.status,
    onSelect: (key) => {
      state.status = key;
      state.page = 1;
      renderLogs(root, ctx);
    },
  });

  const table = card({
    body: dataTable({
      columns: [
        {
          key: 'task', label: '任务',
          render: (l) => h('div', null,
            h('div', { class: 'cell-main' }, l.taskName || kindLabel(l.kind)),
            h('div', { class: 'cell-sub' }, l.message || '—'),
          ),
        },
        {
          key: 'account', label: '账号',
          render: (l) => h('div', null,
            h('div', null, l.accountAlias || l.accountUsername || '—'),
            h('div', { class: 'cell-sub' }, l.source === 'auto' ? '自动执行' : '手动执行'),
          ),
        },
        { key: 'status', label: '状态', render: (l) => badge('run', l.status) },
        { key: 'kind', label: '类型', render: (l) => kindLabel(l.kind) },
        {
          key: 'time', label: '执行时间',
          render: (l) => h('span', { class: 'cell-sub', title: formatDateTime(l.createdAt) },
            formatDateTime(l.createdAt)),
        },
        {
          key: 'actions', label: '操作', align: 'right',
          render: (l) => h('button', {
            class: 'btn ghost xs',
            onclick: () => openLogDetail(l.id),
          }, '查看详情'),
        },
      ],
      rows: logs,
      empty: emptyState('暂无记录', '调整筛选条件，或先执行一次查寝'),
    }),
  });

  const pager = logs.length ? h('div', { class: 'pager' },
    h('button', {
      class: 'btn ghost xs',
      disabled: state.page <= 1,
      onclick: () => { state.page--; renderLogs(root, ctx); },
    }, '上一页'),
    h('span', null, `第 ${state.page} 页 / 共 ${pages} 页`),
    h('button', {
      class: 'btn ghost xs',
      disabled: state.page >= pages,
      onclick: () => { state.page++; renderLogs(root, ctx); },
    }, '下一页'),
  ) : null;

  setChildren(root, head, bar, tabBar, table, pager);
}

/* ---------------- 详情 ---------------- */

async function openLogDetail(id) {
  let log;
  try {
    ({ log } = await api.log(id));
  } catch (err) {
    toast(err.message, 'err');
    return;
  }

  let detail = null;
  try {
    detail = log.detail ? JSON.parse(log.detail) : null;
  } catch {
    detail = null;
  }

  const groups = [];

  /* 基本 */
  groups.push(detailGroup('执行信息', kvList([
    ['任务', log.taskName || '查寝'],
    ['账号', log.accountAlias || log.accountUsername || '—'],
    ['学校', log.schoolName || '—'],
    ['类型', kindLabel(log.kind)],
    ['来源', log.source === 'auto' ? '自动计划' : '手动执行'],
    ['状态', badge('run', log.status)],
    ['时间', formatDateTime(log.createdAt)],
    ['结果', log.message || '—'],
  ])));

  /* 位置与设备（来自 detail） */
  if (detail) {
    const placeRows = [];
    if (detail.address) placeRows.push(['位置名称', detail.address]);
    if (detail.lon != null && detail.lat != null) {
      placeRows.push(['提交坐标', `${Number(detail.lon).toFixed(6)}, ${Number(detail.lat).toFixed(6)}`, true]);
    }
    if (detail.inRange != null) {
      placeRows.push(['是否在范围', detail.inRange ? '是' : '否']);
    }
    if (placeRows.length) groups.push(detailGroup('位置', kvList(placeRows)));

    if (detail.places && detail.places.length) {
      const placeNodes = detail.places.map((p) => h('div', { class: 'place-item' },
        h('div', { class: 'place-name' }, p.name),
        h('div', { class: 'place-meta' },
          h('span', null, `距离 ${Math.round(p.distance)} 米`),
          h('span', null, `半径 ${Math.round(p.radius)} 米`),
          h('span', { class: 'badge ' + (p.inRange ? 'ok' : 'warn') },
            p.inRange ? '范围内' : '范围外'),
        ),
      ));
      groups.push(detailGroup('签到点', h('div', { class: 'place-list' }, placeNodes)));
    }

    const devRows = [];
    if (detail.device) devRows.push(['设备 ID', detail.device, true]);
    if (detail.isPhoto != null) devRows.push(['需要照片', detail.isPhoto ? '是' : '否']);
    if (detail.isNeedExtra != null) devRows.push(['附加题目', detail.isNeedExtra ? '是' : '否']);
    if (devRows.length) groups.push(detailGroup('设备与要求', kvList(devRows)));

    if (detail.code != null || detail.error) {
      groups.push(detailGroup('服务端返回', kvList([
        detail.error ? ['错误', detail.error] : null,
        detail.code != null ? ['code', String(detail.code), true] : null,
      ].filter(Boolean))));
    }
  }

  openModal({
    title: '执行详情',
    sub: log.taskName || '查寝',
    body: h('div', { class: 'detail-groups' }, groups),
    wide: true,
    footer: h('button', { class: 'btn primary', onclick: closeModal }, '关闭'),
  });
}

function detailGroup(title, content) {
  return h('div', { class: 'detail-group' },
    h('h4', null, title),
    content,
  );
}

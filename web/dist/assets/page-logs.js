/* 执行记录页 */
import { api } from './api.js';
import {
  h, icon, toast, openModal, closeModal, statusBadge, skeletonCard, emptyState,
  formatDateTime, kindLabel, shortRange,
} from './ui.js';

const state = { scope: 'today', status: 'all', kind: 'all', page: 1, pageSize: 20 };

export async function renderLogs(root, ctx) {
  root.replaceChildren(h('div', { class: 'loading-block' }, skeletonCard(false)));

  let data;
  try {
    data = await api.logs({
      scope: state.scope,
      status: state.status,
      kind: state.kind,
      limit: state.pageSize,
      offset: (state.page - 1) * state.pageSize,
    });
  } catch (err) {
    root.replaceChildren(emptyState('加载记录失败', err.message,
      h('button', { class: 'btn primary', onclick: () => renderLogs(root, ctx) }, '重试')));
    return;
  }

  const { logs, total } = data;
  const totalPages = Math.max(1, Math.ceil(total / state.pageSize));

  const head = h('div', { class: 'page-head' },
    h('div', null,
      h('h1', null, '执行记录'),
      h('p', { class: 'page-sub' }, `共 ${total} 条记录`),
    ),
  );

  const scopeSelect = h('select', {
    onchange: (e) => { state.scope = e.target.value; state.page = 1; renderLogs(root, ctx); },
  },
    h('option', { value: 'today', selected: state.scope === 'today' }, '今日记录'),
    h('option', { value: 'week', selected: state.scope === 'week' }, '近 7 天'),
    h('option', { value: 'all', selected: state.scope === 'all' }, '全部记录'),
  );

  const kindSelect = h('select', {
    onchange: (e) => { state.kind = e.target.value; state.page = 1; renderLogs(root, ctx); },
  },
    h('option', { value: 'all', selected: state.kind === 'all' }, '全部类型'),
    h('option', { value: 'attendance', selected: state.kind === 'attendance' }, '查寝'),
    h('option', { value: 'sign', selected: state.kind === 'sign' }, '签到'),
    h('option', { value: 'collect', selected: state.kind === 'collect' }, '信息收集'),
  );

  const filters = h('div', { class: 'log-filters' },
    h('label', null, '查看范围', scopeSelect),
    h('label', null, '任务类型', kindSelect),
    h('button', { class: 'btn ghost sm', onclick: () => renderLogs(root, ctx) }, icon('refresh'), '刷新'),
  );

  const statusTabs = h('div', { class: 'status-tabs' },
    [['all', '全部'], ['success', '成功'], ['failed', '失败'], ['pending', '结果待核对'], ['executing', '执行中']]
      .map(([key, label]) => h('button', {
        class: state.status === key ? 'active' : '',
        onclick: () => { state.status = key; state.page = 1; renderLogs(root, ctx); },
      }, label)),
  );

  const tableCard = h('div', { class: 'card' });
  if (logs.length === 0) {
    tableCard.append(emptyState('暂无记录', '调整筛选条件，或先执行一次任务'));
  } else {
    const rows = logs.map((log) => h('tr', null,
      h('td', null,
        h('div', { class: 'cell-main' }, log.taskName || kindLabel(log.kind)),
        h('div', { class: 'cell-sub' }, log.message || '—'),
      ),
      h('td', null,
        h('div', null, log.accountAlias || log.accountUsername || '—'),
        h('div', { class: 'cell-sub' }, log.source === 'auto' ? '自动执行' : '手动执行'),
      ),
      h('td', null, statusBadge(log.status)),
      h('td', null, kindLabel(log.kind)),
      h('td', null, formatDateTime(log.createdAt)),
      h('td', { class: 'col-right' },
        h('button', { class: 'btn ghost xs', onclick: () => showLogDetail(log.id) }, '查看详情'),
      ),
    ));

    tableCard.append(h('div', { class: 'table-wrap' },
      h('table', { class: 'data' },
        h('thead', null, h('tr', null,
          h('th', null, '任务'), h('th', null, '账号'), h('th', null, '状态'),
          h('th', null, '类型'), h('th', null, '执行时间'), h('th', { class: 'col-right' }, '操作'),
        )),
        h('tbody', null, rows),
      ),
    ));
  }

  const pager = h('div', { class: 'pager' },
    h('button', {
      class: 'link-btn', disabled: state.page <= 1,
      style: { opacity: state.page <= 1 ? 0.4 : 1 },
      onclick: () => { if (state.page > 1) { state.page--; renderLogs(root, ctx); } },
    }, '上一页'),
    h('span', null, `第 ${state.page} 页 / 共 ${totalPages} 页`),
    h('button', {
      class: 'link-btn', disabled: state.page >= totalPages,
      style: { opacity: state.page >= totalPages ? 0.4 : 1 },
      onclick: () => { if (state.page < totalPages) { state.page++; renderLogs(root, ctx); } },
    }, '下一页'),
  );

  root.replaceChildren(head, filters, statusTabs, tableCard, pager);
}

async function showLogDetail(id) {
  let log;
  try {
    ({ log } = await api.log(id));
  } catch (err) {
    toast(err.message, 'err');
    return;
  }

  let detail = null;
  try { detail = log.detail ? JSON.parse(log.detail) : null; } catch { detail = null; }

  const body = h('div', null,
    h('dl', { class: 'kv-list', style: { marginBottom: '16px' } },
      row('任务', log.taskName || kindLabel(log.kind)),
      row('账号', `${log.accountAlias || '—'}（${log.schoolName || '—'}）`),
      row('类型', kindLabel(log.kind)),
      row('来源', log.source === 'auto' ? '自动计划' : '手动执行'),
      row('状态', statusBadge(log.status)),
      row('时间', formatDateTime(log.createdAt)),
    ),
    h('div', { class: 'warn-box', style: { marginBottom: '14px' } }, log.message || '无附加信息'),
    detail ? h('div', null,
      h('p', { class: 'hint-text', style: { marginBottom: '8px' } }, '执行详情'),
      h('pre', {
        class: 'code-box',
        style: { whiteSpace: 'pre-wrap', maxHeight: '260px', overflow: 'auto' },
      }, JSON.stringify(detail, null, 2)),
    ) : null,
  );

  openModal({
    title: '执行详情',
    wide: true,
    body,
    footer: h('button', { class: 'btn primary', onclick: closeModal }, '关闭'),
  });
}

function row(label, value) {
  return h('div', null, h('dt', null, label), h('dd', null, value));
}

/* 执行记录页 */
import { api } from './api.js';
import {
  setChildren,
  el,
  ico,
  toast,
  skeleton,
  emptyBox,
  logTag,
  fmtTime,
  openModal,
  closeModal,
  kvList,
} from './ui.js';

const state = { scope: 'today', status: 'all', page: 1, size: 20 };

export async function pageLogs(root, ctx) {
  setChildren(root, skeleton());
  let data;
  try {
    data = await api.logs({
      scope: state.scope,
      status: state.status,
      limit: state.size,
      offset: (state.page - 1) * state.size,
    });
  } catch (err) {
    setChildren(root, emptyBox('加载失败', err.message,
      el('button', { class: 'btn main', onclick: () => pageLogs(root, ctx) }, '重试')));
    return;
  }

  const { logs, total } = data;
  const pages = Math.max(1, Math.ceil(total / state.size));

  const head = el('div', { class: 'head' },
    el('div', null,
      el('h1', null, '执行记录'),
      el('div', { class: 'sub' }, '共 ' + total + ' 条'),
    ),
    el('div', { class: 'head-actions' },
      el('button', { class: 'btn plain', onclick: () => pageLogs(root, ctx) },
        ico('refresh'), '刷新'),
    ),
  );

  const scopeSel = el('select', {
    onchange: (e) => { state.scope = e.target.value; state.page = 1; pageLogs(root, ctx); },
  },
    [['today', '今天'], ['week', '近 7 天'], ['all', '全部']].map(([v, l]) =>
      el('option', { value: v, selected: state.scope === v }, l)));

  const tabs = el('div', { class: 'tabs' },
    [['all', '全部'], ['success', '成功'], ['failed', '失败'], ['pending', '待核对']]
      .map(([key, label]) => el('button', {
        class: state.status === key ? 'on' : '',
        onclick: () => { state.status = key; state.page = 1; pageLogs(root, ctx); },
      }, label)));

  const card = el('div', { class: 'card' });
  if (!logs.length) {
    card.append(emptyBox('暂无记录', '执行查寝后这里会显示结果'));
  } else {
    const rows = logs.map((l) => el('tr', null,
      el('td', null,
        el('div', { class: 'cell-strong' }, l.taskName || '查寝'),
        el('div', { class: 'cell-sub' }, l.message || '—'),
      ),
      el('td', null,
        el('div', null, l.accountAlias || l.accountUsername || '—'),
        el('div', { class: 'cell-sub' }, l.source === 'auto' ? '自动' : '手动'),
      ),
      el('td', null, logTag(l.status)),
      el('td', { class: 'cell-sub' }, fmtTime(l.createdAt)),
      el('td', { class: 'right' },
        el('button', { class: 'btn plain xs', onclick: () => logDetail(l.id) }, '详情'),
      ),
    ));

    card.append(el('div', { class: 'table-wrap' },
      el('table', { class: 'list' },
        el('thead', null, el('tr', null,
          el('th', null, '任务'), el('th', null, '账号'), el('th', null, '状态'),
          el('th', null, '时间'), el('th', { class: 'right' }, '操作'),
        )),
        el('tbody', null, rows),
      ),
    ));
  }

  const pager = el('div', { class: 'pager' },
    el('button', {
      class: 'btn plain xs', disabled: state.page <= 1,
      onclick: () => { state.page--; pageLogs(root, ctx); },
    }, '上一页'),
    el('span', null, state.page + ' / ' + pages),
    el('button', {
      class: 'btn plain xs', disabled: state.page >= pages,
      onclick: () => { state.page++; pageLogs(root, ctx); },
    }, '下一页'),
  );

  setChildren(root, 
    head,
    el('div', { class: 'toolbar' }, el('div', { class: 'search-wrap' }, scopeSel)),
    tabs,
    card,
    logs.length ? pager : null,
  );
}

async function logDetail(id) {
  let item;
  try {
    ({ log: item } = await api.log(id));
  } catch (err) {
    toast(err.message, 'err');
    return;
  }

  let detail = null;
  try { detail = item.detail ? JSON.parse(item.detail) : null; } catch { detail = null; }

  const rows = [
    ['任务', item.taskName || '查寝'],
    ['账号', item.accountAlias || item.accountUsername || '—'],
    ['来源', item.source === 'auto' ? '自动计划' : '手动执行'],
    ['状态', item.status === 'success' ? '成功'
      : item.status === 'failed' ? '失败'
      : item.status === 'pending' ? '待核对' : item.status],
    ['时间', fmtTime(item.createdAt)],
  ];

  if (detail?.places?.length) {
    const p = detail.places[0];
    rows.push(['签到点', p.name]);
    rows.push(['距离', Math.round(p.distance) + ' 米 / 半径 ' + Math.round(p.radius) + ' 米']);
    rows.push(['是否在范围', p.inRange ? '是' : '否']);
  }

  openModal({
    title: '执行详情',
    body: el('div', null,
      kvList(rows.map((r) => [r[0], r[1]])),
      item.message ? el('div', { class: 'note-box warn', style: { marginTop: '12px' } }, item.message) : null,
      detail ? el('pre', {
        style: {
          marginTop: '12px', padding: '11px 13px', background: 'var(--surface-2)',
          border: '1px solid var(--line)', borderRadius: '10px',
          fontSize: '11.5px', fontFamily: 'var(--mono)', whiteSpace: 'pre-wrap',
          maxHeight: '220px', overflow: 'auto', color: 'var(--text-muted)',
        },
      }, JSON.stringify(detail, null, 2)) : null,
    ),
    wide: true,
    actions: el('button', { class: 'btn main', onclick: closeModal }, '关闭'),
  });
}

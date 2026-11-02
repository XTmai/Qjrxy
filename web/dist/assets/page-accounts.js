/* 学生账号页：列表、筛选、添加/编辑、登录、Cookie 导入、扫码、自动定位 */
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
  dataTable,
  toolbar,
  segmented,
  formField,
  loadingState,
  emptyState,
  errorState,
  openModal,
  closeModal,
  confirmDialog,
  copyText,
  formatDateTime,
  initials,
  debounce,
} from './ui.js';

const state = { accounts: [], keyword: '', filter: 'all' };

export async function renderAccounts(root, ctx) {
  setChildren(root, loadingState('正在加载账号…'));

  let accounts = [];
  try {
    ({ accounts } = await api.accounts());
  } catch (err) {
    setChildren(root, 
      pageHeader({ title: '学生账号' }),
      errorState('加载账号失败', err,
        h('button', { class: 'btn primary sm', onclick: () => renderAccounts(root, ctx) }, '重试')),
    );
    return;
  }

  state.accounts = accounts;

  const head = pageHeader({
    title: '学生账号',
    sub: `已绑定 ${accounts.length} 个账号`,
    actions: [
      h('button', {
        class: 'btn primary',
        onclick: () => openAccountForm(ctx, null, () => renderAccounts(root, ctx)),
      }, icon('plus'), '添加账号'),
    ],
  });

  const search = h('input', {
    type: 'search',
    placeholder: '搜索账号、学号或学校',
    value: state.keyword,
    oninput: debounce((e) => {
      state.keyword = e.target.value.trim();
      paintList();
    }, 200),
  });

  const filterSeg = segmented({
    items: [['all', '全部'], ['authorized', '已登录'], ['attention', '需处理']],
    active: state.filter,
    onSelect: (key) => {
      state.filter = key;
      filterSeg.querySelectorAll('button').forEach((b) =>
        b.classList.toggle('active', b.dataset.seg === key));
      paintList();
    },
  });

  const bar = toolbar(
    h('div', { class: 'search-box' }, icon('users'), search),
    filterSeg,
  );

  const list = h('div', { class: 'account-list' });

  function paintList() {
    const kw = state.keyword.toLowerCase();
    const shown = accounts.filter((a) => {
      if (state.filter === 'authorized' && a.loginStatus !== 'authorized') return false;
      if (state.filter === 'attention' && a.loginStatus === 'authorized') return false;
      if (!kw) return true;
      return [a.alias, a.username, a.schoolName]
        .some((v) => String(v || '').toLowerCase().includes(kw));
    });

    if (!shown.length) {
      setChildren(list, emptyState(
        accounts.length ? '没有匹配的账号' : '还没有绑定账号',
        accounts.length ? '试试其他关键词' : '添加今日校园账号后即可查询查寝任务',
        accounts.length ? null : h('button', {
          class: 'btn primary sm',
          onclick: () => openAccountForm(ctx, null, () => renderAccounts(root, ctx)),
        }, '添加账号'),
      ));
      return;
    }

    setChildren(list, ...shown.map((a) =>
      accountRow(a, ctx, () => renderAccounts(root, ctx))));
  }

  paintList();
  setChildren(root, head, bar, list);

  // 支持 ?id= 直达
  const qs = new URLSearchParams((location.hash.split('?')[1] || ''));
  const focusId = Number(qs.get('id'));
  if (focusId) {
    const acc = accounts.find((a) => a.id === focusId);
    if (acc) openAccountDetail(acc, ctx, () => renderAccounts(root, ctx));
  }
}

function accountRow(acc, ctx, refresh) {
  const tags = [
    badge('login', acc.loginStatus),
    acc.deviceId
      ? h('span', { class: 'badge info' }, `${acc.deviceModel || '设备'} · ${acc.platform === 'ios' ? 'iOS' : 'Android'}`)
      : h('span', { class: 'badge warn' }, '未绑设备'),
    acc.lon && acc.lat
      ? h('span', { class: 'badge plain' }, '已设位置')
      : h('span', { class: 'badge warn' }, '未设位置'),
    acc.photoUrl ? h('span', { class: 'badge plain' }, '有照片') : null,
  ].filter(Boolean);

  return h('div', { class: 'account-row' },
    h('div', { class: 'avatar' }, initials(acc.alias || acc.username)),
    h('div', { class: 'account-info' },
      h('div', { class: 'account-name' }, acc.alias || acc.username),
      h('div', { class: 'account-school' }, `${acc.schoolName} · ${acc.username}`),
      h('div', { class: 'account-tags' }, tags),
    ),
    h('div', { class: 'account-acts' },
      h('button', { class: 'btn ghost sm', onclick: () => ctx.go(`/tasks?account=${acc.id}`) }, '任务'),
      h('button', { class: 'btn soft sm', onclick: () => openAccountDetail(acc, ctx, refresh) }, '管理'),
    ),
  );
}

/* ---------------- 账号详情 ---------------- */

function openAccountDetail(acc, ctx, refresh) {
  const deviceRows = acc.deviceId
    ? [
        ['设备 ID', acc.deviceId, true],
        ['机型', `${acc.deviceModel || '—'} · ${acc.systemVersion || '—'}`],
        ['平台', acc.platform === 'ios' ? 'iOS' : 'Android'],
        acc.buildId ? ['构建号', acc.buildId] : null,
        acc.appVersion ? ['App 版本', acc.appVersion] : null,
      ].filter(Boolean)
    : [['设备状态', '未绑定']];

  const body = h('div', { class: 'detail-groups' },
    h('div', { class: 'detail-group' },
      h('h4', null, '基本信息'),
      kvList([
        ['学校', acc.schoolName],
        ['学号', acc.username],
        ['登录状态', badge('login', acc.loginStatus)],
        ['最近登录', acc.lastLoginAt ? formatDateTime(acc.lastLoginAt) : '—'],
      ]),
    ),
    h('div', { class: 'detail-group' },
      h('h4', null, '设备信息'),
      kvList(deviceRows),
    ),
    h('div', { class: 'detail-group' },
      h('h4', null, '签到位置'),
      kvList([
        ['位置名称', acc.address || '未设置'],
        ['坐标', acc.lon && acc.lat
          ? `${Number(acc.lon).toFixed(6)}, ${Number(acc.lat).toFixed(6)}`
          : '未设置', true],
      ]),
    ),
    h('div', { class: 'detail-group' },
      h('h4', null, '签到照片'),
      kvList([[
        '照片',
        acc.photoUrl
          ? h('a', { href: acc.photoUrl, target: '_blank', rel: 'noopener' }, '查看照片')
          : '未上传',
      ]]),
    ),
  );

  openModal({
    title: acc.alias || acc.username,
    sub: acc.schoolName,
    body,
    wide: true,
    footer: [
      h('button', {
        class: 'btn primary sm',
        onclick: () => { closeModal(); doLogin(acc, refresh); },
      }, icon('login'), '账密登录'),
      h('button', {
        class: 'btn ghost sm',
        onclick: () => { closeModal(); openCookieForm(acc, refresh); },
      }, icon('link'), '导入 Cookie'),
      h('button', {
        class: 'btn ghost sm',
        onclick: () => { closeModal(); openAccountForm(ctx, acc, refresh); },
      }, icon('edit'), '编辑'),
      h('button', {
        class: 'btn ghost-danger sm',
        onclick: () => removeAccount(acc, refresh),
      }, icon('trash'), '删除'),
    ],
  });
}

/* ---------------- 登录 ---------------- */

async function doLogin(acc, refresh) {
  toast('正在登录，请稍候…');
  try {
    const res = await api.accountLogin(acc.id);
    if (res.done) {
      toast('登录成功', 'ok');
      refresh();
      return;
    }
    openCaptchaForm(acc, res, refresh);
  } catch (err) {
    toast(err.message, 'err');
  }
}

function openCaptchaForm(acc, res, refresh) {
  const input = h('input', {
    placeholder: '请输入图中字符',
    autocomplete: 'off',
    maxlength: '8',
  });
  const img = h('img', {
    class: 'captcha-img',
    src: res.captcha,
    alt: '验证码',
    title: '看不清？点击重新获取',
    onclick: () => { closeModal(); doLogin(acc, refresh); },
  });
  const err = h('p', { class: 'form-error' });

  const submit = async () => {
    const code = input.value.trim();
    if (!code) { err.textContent = '请输入验证码'; return; }
    try {
      await api.accountCaptcha(acc.id, res.ticket, code);
      toast('登录成功', 'ok');
      closeModal();
      refresh();
    } catch (e) {
      err.textContent = e.message;
    }
  };

  input.addEventListener('keydown', (e) => { if (e.key === 'Enter') submit(); });

  openModal({
    title: '输入验证码',
    sub: `正在为「${acc.alias || acc.username}」登录，学校要求图形验证码`,
    body: h('div', null, img, formField({ input }), err),
    footer: [
      h('button', { class: 'btn ghost', onclick: closeModal }, '取消'),
      h('button', { class: 'btn primary', onclick: submit }, '提交验证码'),
    ],
  });
  setTimeout(() => input.focus(), 100);
}

function openCookieForm(acc, refresh) {
  const area = h('textarea', {
    placeholder: 'MOD_AUTH_CAS=xxx; sessionToken=yyy',
  });
  const err = h('p', { class: 'form-error' });

  openModal({
    title: '导入 Cookie',
    sub: '账密登录失败时的备用方式',
    body: h('div', null,
      h('div', { class: 'notice info', style: { marginBottom: '14px' } },
        '在浏览器登录今日校园网页版，打开开发者工具 → Network → 任选一个请求，'
        + '复制请求头里的 Cookie 整段粘贴到下面。Cookie 会加密保存在你的服务器上。'),
      formField({ label: 'Cookie', input: area }),
      err,
    ),
    footer: [
      h('button', { class: 'btn ghost', onclick: closeModal }, '取消'),
      h('button', {
        class: 'btn primary',
        onclick: async () => {
          const raw = area.value.trim();
          if (!raw) { err.textContent = '请粘贴 Cookie'; return; }
          try {
            const res = await api.importCookies(acc.id, raw);
            toast(res.message || 'Cookie 已保存', 'ok');
            closeModal();
            refresh();
          } catch (e) {
            err.textContent = e.message;
          }
        },
      }, '保存并校验'),
    ],
  });
  setTimeout(() => area.focus(), 100);
}

/* ---------------- 新增 / 编辑 ---------------- */

function openAccountForm(ctx, existing, refresh) {
  const isEdit = Boolean(existing);

  /* 基础字段 */
  const schoolInput = h('input', {
    placeholder: '输入学校全称，如：三峡大学科技学院',
    value: existing?.schoolName || '',
    list: 'schoolOptions',
    autocomplete: 'off',
  });
  const datalist = h('datalist', { id: 'schoolOptions' });
  const schoolHint = h('span', { class: 'hint' }, '输入几个字会自动匹配，从下拉中选中即可。');

  schoolInput.addEventListener('input', debounce(async (e) => {
    const kw = e.target.value.trim();
    if (kw.length < 2) return;
    try {
      const { schools } = await api.schools(kw);
      setChildren(datalist, ...schools.slice(0, 20)
        .map((s) => h('option', { value: s.name })));
      schoolHint.textContent = schools.length
        ? `匹配到 ${schools.length} 所学校，请从下拉中选择`
        : '未匹配到学校，请检查名称是否为今日校园中的全称';
    } catch {
      schoolHint.textContent = '学校查询失败，可手动填写';
    }
  }, 320));

  const userInput = h('input', {
    placeholder: '学号 / 工号',
    value: existing?.username || '',
    autocomplete: 'username',
  });
  const passInput = h('input', {
    type: 'password',
    autocomplete: 'new-password',
    placeholder: isEdit ? '留空表示不修改' : '今日校园登录密码',
  });

  /* 只读设备信息 */
  let dev = {
    deviceId: existing?.deviceId || '',
    deviceModel: existing?.deviceModel || '',
    systemVersion: existing?.systemVersion || '',
    buildId: existing?.buildId || '',
    appVersion: existing?.appVersion || '',
    wisDeviceId: existing?.wisDeviceId || '',
    platform: existing?.platform || '',
    userAgent: existing?.userAgent || '',
  };

  /* 只读位置 */
  let loc = {
    lon: existing?.lon || '',
    lat: existing?.lat || '',
    address: existing?.address || '',
  };

  const devBox = h('div');
  const locBox = h('div');

  const paintDev = () => {
    if (!dev.deviceId) {
      devBox.replaceChildren(h('div', { class: 'box-empty' }, '尚未获取设备信息'));
      return;
    }
    devBox.replaceChildren(kvList([
      ['设备 ID', dev.deviceId, true],
      ['机型', dev.deviceModel || '—'],
      ['系统版本', dev.systemVersion || '—'],
      ['平台', dev.platform === 'ios' ? 'iOS' : 'Android'],
      dev.buildId ? ['构建号', dev.buildId] : null,
      dev.appVersion ? ['App 版本', dev.appVersion] : null,
    ].filter(Boolean)));
  };

  const paintLoc = () => {
    if (!loc.lon || !loc.lat) {
      locBox.replaceChildren(h('div', { class: 'box-empty' }, '尚未获取签到位置'));
      return;
    }
    locBox.replaceChildren(kvList([
      ['位置名称', loc.address || '—'],
      ['坐标', `${Number(loc.lon).toFixed(6)}, ${Number(loc.lat).toFixed(6)}`, true],
    ]));
  };

  paintDev();
  paintLoc();

  const scanBtn = h('button', {
    class: 'btn primary sm',
    disabled: !isEdit,
    onclick: async () => {
      if (!isEdit) { toast('请先保存账号，再扫码', 'err'); return; }
      await openDeviceScan(existing.id, (data) => {
        dev = {
          deviceId: data.deviceId || '',
          deviceModel: data.deviceModel || '',
          systemVersion: data.systemVersion || '',
          buildId: data.buildId || '',
          appVersion: data.appVersion || '',
          wisDeviceId: data.wisDeviceId || '',
          platform: data.platform || '',
          userAgent: data.userAgent || '',
        };
        paintDev();
        toast('设备信息已获取', 'ok');
      });
    },
  }, icon('qr'), '扫码获取');

  const locBtn = h('button', {
    class: 'btn primary sm',
    disabled: !isEdit,
    onclick: async (e) => {
      if (!isEdit) { toast('请先保存账号', 'err'); return; }
      const btn = e.currentTarget;
      const prev = btn.textContent;
      btn.disabled = true;
      btn.textContent = '获取中…';
      try {
        const res = await api.applySignRange(existing.id);
        const r = res.range || {};
        loc = { lon: r.lon, lat: r.lat, address: r.address || '' };
        paintLoc();
        toast('已获取：' + (r.address || '学校签到点'), 'ok');
      } catch (err) {
        toast(err.message, 'err');
      } finally {
        btn.disabled = false;
        btn.textContent = prev;
      }
    },
  }, icon('pin'), '自动获取');

  const photoInput = h('input', {
    type: 'file',
    accept: 'image/png,image/jpeg,image/webp',
  });
  const err = h('p', { class: 'form-error' });

  const body = h('div', null,
    h('section', { class: 'form-section' },
      h('h3', null, '学校账号'),
      formField({ label: '学校全称', required: true, input: schoolInput, hint: schoolHint }),
      datalist,
      formField({ label: '学号', required: true, input: userInput }),
      formField({
        label: isEdit ? '密码（留空不改）' : '密码',
        required: !isEdit,
        input: passInput,
        hint: 'AES-256-GCM 加密保存在你的服务器，仅用于自动登录。',
      }),
    ),

    h('section', { class: 'form-section' },
      h('div', { class: 'section-head' },
        h('h3', null, '设备信息'),
        scanBtn,
      ),
      devBox,
      h('p', { class: 'section-hint' },
        isEdit
          ? '用你平时打卡的那台手机打开今日校园 App 扫一扫，设备信息会自动填入。'
          : '保存账号后即可扫码获取。'),
    ),

    h('section', { class: 'form-section' },
      h('div', { class: 'section-head' },
        h('h3', null, '签到位置'),
        locBtn,
      ),
      locBox,
      h('p', { class: 'section-hint' },
        isEdit
          ? '直接读取学校设置的签到点与半径，不需要手动选点。'
          : '保存账号后即可自动获取。'),
    ),

    h('section', { class: 'form-section' },
      h('h3', null, '签到照片'),
      formField({
        input: photoInput,
        hint: '查寝任务需要照片时自动使用。支持 jpg / png / webp，不超过 8MB。',
      }),
    ),

    err,
  );

  openModal({
    title: isEdit ? '编辑账号' : '添加账号',
    sub: isEdit ? (existing.alias || existing.username) : '保存后可以扫码绑定设备',
    body,
    wide: true,
    footer: [
      h('button', { class: 'btn ghost', onclick: closeModal }, '取消'),
      h('button', {
        class: 'btn primary',
        onclick: async (e) => {
          const btn = e.currentTarget;
          const payload = {
            schoolName: schoolInput.value.trim(),
            username: userInput.value.trim(),
            password: passInput.value,
            deviceId: dev.deviceId,
            deviceModel: dev.deviceModel,
            systemVersion: dev.systemVersion,
            buildId: dev.buildId,
            appVersion: dev.appVersion,
            wisDeviceId: dev.wisDeviceId,
            platform: dev.platform,
            userAgent: dev.userAgent,
            lon: Number(loc.lon) || 0,
            lat: Number(loc.lat) || 0,
            address: loc.address,
          };
          if (!payload.schoolName) { err.textContent = '请填写学校全称'; return; }
          if (!payload.username) { err.textContent = '请填写学号'; return; }
          if (!isEdit && !payload.password) { err.textContent = '请填写密码'; return; }

          btn.disabled = true;
          btn.textContent = '保存中…';
          try {
            let account;
            if (isEdit) {
              ({ account } = await api.updateAccount(existing.id, payload));
            } else {
              ({ account } = await api.createAccount(payload));
            }
            const file = photoInput.files?.[0];
            if (file) {
              const fd = new FormData();
              fd.append('photo', file);
              await api.uploadPhoto(account.id, fd);
            }
            toast(isEdit ? '账号已更新' : '账号已添加', 'ok');
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

/* ---------------- 扫码（轮询） ---------------- */

async function openDeviceScan(accountId, onCollected) {
  let res;
  try {
    res = await api.deviceTicket(accountId);
  } catch (err) {
    toast(err.message, 'err');
    return;
  }

  let stopped = false;
  let timer = null;
  const stop = () => {
    stopped = true;
    if (timer) { clearTimeout(timer); timer = null; }
  };

  const status = h('div', { class: 'scan-status' },
    h('span', { class: 'scan-dot' }),
    h('span', null, '等待扫码…'),
  );
  const result = h('div');

  const poll = async () => {
    if (stopped) return;
    try {
      const data = await api.devicePoll(res.ticket);
      if (stopped) return;
      if (data && data.pending === false && data.collected) {
        stop();
        status.replaceChildren(
          h('span', { class: 'scan-dot ok' }),
          h('span', null, '已扫码，设备信息已获取'),
        );
        result.replaceChildren(kvList([
          ['设备 ID', data.deviceId, true],
          ['机型', data.deviceModel || '—'],
          ['系统版本', data.systemVersion || '—'],
          ['平台', data.platform === 'ios' ? 'iOS' : 'Android'],
        ]));
        onCollected?.(data);
        setTimeout(() => { if (!stopped) closeModal(); }, 1200);
        return;
      }
    } catch (err) {
      stop();
      status.replaceChildren(
        h('span', { class: 'scan-dot err' }),
        h('span', null, err.message || '二维码已失效'),
      );
      return;
    }
    timer = setTimeout(poll, 1500);
  };
  timer = setTimeout(poll, 1200);

  openModal({
    title: '扫码获取设备信息',
    sub: '用今日校园 App 扫一扫',
    body: h('div', null,
      h('div', { class: 'notice info', style: { marginBottom: '14px' } },
        '打开「今日校园 App」→ 右上角「＋」→ 扫一扫，扫描下方二维码。'
        + '扫码后设备信息会自动填回来，不需要手动输入。'),
      res.qrcode ? h('img', { class: 'qr-img', src: res.qrcode, alt: '设备绑定二维码' }) : null,
      status,
      result,
      h('p', { class: 'scan-note' }, '二维码 15 分钟内有效，仅可使用一次'),
      h('div', { class: 'notice warn', style: { marginTop: '12px' } },
        '必须用你平时打卡的那台手机扫码。换手机或重装 App 后设备 ID 会变化。'),
    ),
    footer: [
      h('button', {
        class: 'btn ghost',
        onclick: async () => {
          const ok = await copyText(res.url);
          toast(ok ? '链接已复制' : '复制失败');
        },
      }, '复制链接'),
      h('button', {
        class: 'btn ghost',
        onclick: () => { stop(); openDeviceScan(accountId, onCollected); },
      }, '刷新二维码'),
      h('button', { class: 'btn primary', onclick: () => { stop(); closeModal(); } }, '关闭'),
    ],
    onClose: stop,
  });
}

/* ---------------- 删除 ---------------- */

async function removeAccount(acc, refresh) {
  const ok = await confirmDialog({
    title: '删除账号',
    message: `确定删除「${acc.alias || acc.username}」吗？`
      + '该账号的计划与执行记录会一并删除，且无法恢复。',
    confirmText: '删除',
    danger: true,
  });
  if (!ok) return;
  try {
    await api.deleteAccount(acc.id);
    toast('账号已删除', 'ok');
    refresh();
  } catch (err) {
    toast(err.message, 'err');
  }
}

/* 学生账号页：列表、筛选、添加/编辑、登录、Cookie 导入、设备码、照片 */
import { api } from './api.js';
import {
  h, icon, toast, openModal, closeModal, confirmDialog, loginBadge,
  skeletonCard, emptyState, debounce, copyText, formatDateTime, initials,
} from './ui.js';

let cache = { accounts: [], keyword: '', filter: 'all' };

export async function renderAccounts(root, ctx) {
  root.replaceChildren(h('div', { class: 'loading-block' }, skeletonCard(false)));
  let accounts = [];
  try {
    ({ accounts } = await api.accounts());
  } catch (err) {
    root.replaceChildren(emptyState('加载账号失败', err.message,
      h('button', { class: 'btn primary', onclick: () => renderAccounts(root, ctx) }, '重试')));
    return;
  }
  cache.accounts = accounts;

  const head = h('div', { class: 'page-head' },
    h('div', null,
      h('h1', null, '学生账号'),
      h('p', { class: 'page-sub' }, `已绑定 ${accounts.length} 个账号`),
    ),
    h('div', { class: 'page-actions' },
      h('button', { class: 'btn primary', onclick: () => openAccountForm(ctx, null, () => renderAccounts(root, ctx)) },
        icon('plus'), '添加账号'),
    ),
  );

  const searchInput = h('input', {
    type: 'search', placeholder: '搜索账号、学号或学校', value: cache.keyword,
    oninput: debounce((e) => {
      cache.keyword = e.target.value.trim();
      redrawList();
    }, 200),
  });

  const filterTabs = h('div', { class: 'status-tabs', style: { marginBottom: '0', flex: '0 0 auto' } });
  const filters = [
    ['all', '全部账号'],
    ['authorized', '已登录'],
    ['attention', '需处理'],
  ];
  filters.forEach(([key, label]) => {
    filterTabs.append(h('button', {
      class: cache.filter === key ? 'active' : '',
      onclick: () => {
        cache.filter = key;
        filterTabs.querySelectorAll('button').forEach((b, i) => b.classList.toggle('active', filters[i][0] === key));
        redrawList();
      },
    }, label));
  });

  const toolbar = h('div', { class: 'accounts-toolbar' },
    h('div', { class: 'search-box' }, icon('users'), searchInput),
    filterTabs,
  );

  const list = h('div', { class: 'account-list' });

  function redrawList() {
    const kw = cache.keyword.toLowerCase();
    const filtered = accounts.filter((a) => {
      if (cache.filter === 'authorized' && a.loginStatus !== 'authorized') return false;
      if (cache.filter === 'attention' && a.loginStatus === 'authorized') return false;
      if (!kw) return true;
      return [a.alias, a.username, a.schoolName].some((v) => String(v || '').toLowerCase().includes(kw));
    });

    list.replaceChildren();
    if (filtered.length === 0) {
      list.append(emptyState(
        accounts.length === 0 ? '还没有绑定账号' : '没有匹配的账号',
        accounts.length === 0 ? '添加今日校园账号后开始管理签到与查寝' : '试试其他关键词',
        accounts.length === 0
          ? h('button', { class: 'btn primary sm', onclick: () => openAccountForm(ctx, null, () => renderAccounts(root, ctx)) }, '添加账号')
          : null,
      ));
      return;
    }
    filtered.forEach((acc) => list.append(buildRow(acc, ctx, () => renderAccounts(root, ctx))));
  }

  redrawList();
  root.replaceChildren(head, toolbar, list);

  // 支持 ?id= 直接打开某个账号
  const params = new URLSearchParams((location.hash.split('?')[1] || ''));
  const focusId = Number(params.get('id'));
  if (focusId) {
    const acc = accounts.find((a) => a.id === focusId);
    if (acc) openAccountDetail(acc, ctx, () => renderAccounts(root, ctx));
  }
}

function buildRow(acc, ctx, refresh) {
  return h('div', { class: 'account-row' },
    h('div', { class: 'avatar' }, initials(acc.alias || acc.username)),
    h('div', { class: 'account-info' },
      h('strong', null, acc.alias || acc.username),
      h('div', { class: 'school' }, `${acc.schoolName} · ${acc.username}`),
      h('div', { class: 'account-badges' },
        loginBadge(acc.loginStatus),
        acc.deviceId
          ? h('span', { class: 'badge info' }, '设备码已绑')
          : h('span', { class: 'badge plain' }, '未绑设备'),
        acc.lon && acc.lat
          ? h('span', { class: 'badge plain' }, '已设位置')
          : h('span', { class: 'badge warn' }, '未设位置'),
        h('span', { class: 'badge plain' }, acc.platform === 'ios' ? 'iOS' : 'Android'),
        acc.photoUrl ? h('span', { class: 'badge plain' }, '有照片') : null,
      ),
    ),
    h('div', { class: 'account-actions' },
      h('button', { class: 'btn ghost sm', onclick: () => ctx.go('/tasks?account=' + acc.id) }, '任务'),
      h('button', { class: 'btn soft sm', onclick: () => openAccountDetail(acc, ctx, refresh) }, '管理'),
    ),
  );
}

/* ---------------- 账号详情 ---------------- */

function openAccountDetail(acc, ctx, refresh) {
  const body = h('div', null);
  const foot = h('div', { style: { display: 'flex', gap: '10px', flexWrap: 'wrap', width: '100%' } });

  const render = () => {
    body.replaceChildren(
      h('dl', { class: 'kv-list', style: { marginBottom: '18px' } },
        kv('学校', acc.schoolName),
        kv('学号', acc.username),
        kv('登录状态', loginBadge(acc.loginStatus)),
        kv('最近登录', acc.lastLoginAt ? formatDateTime(acc.lastLoginAt) : '—'),
        kv('设备 ID', h('div', null,
          acc.deviceId
            ? h('span', { class: 'code-box', style: { display: 'inline-block', fontSize: '11.5px' } }, acc.deviceId)
            : h('span', { class: 'badge warn' }, '未绑定'),
          h('div', { class: 'hint-text', style: { marginTop: '5px' } },
            '签到提交时使用，由今日校园 App 生成')),),
        kv('设备型号', `${acc.deviceModel} · Android ${acc.systemVersion}`),
        kv('默认位置', acc.lon && acc.lat
          ? `${acc.address || '未填地址'} (${acc.lon.toFixed(5)}, ${acc.lat.toFixed(5)})`
          : '未设置'),
        kv('签到照片', acc.photoUrl
          ? h('a', { href: acc.photoUrl, target: '_blank', rel: 'noopener' }, '查看照片')
          : h('span', { class: 'badge plain' }, '未上传')),
      ),
    );

    foot.replaceChildren(
      h('button', { class: 'btn primary sm', onclick: () => doLogin(acc, ctx, refresh) }, '账密登录'),
      h('button', { class: 'btn ghost sm', onclick: () => openCookieForm(acc, refresh) }, '导入 Cookie'),
      h('button', { class: 'btn ghost sm', onclick: () => openDeviceCode(acc) }, icon('qr'), '设备码'),
      h('button', { class: 'btn ghost sm', onclick: () => openAccountForm(ctx, acc, refresh) }, '编辑'),
      h('button', { class: 'btn danger-ghost sm', onclick: () => removeAccount(acc, refresh) }, '删除'),
    );
  };

  render();
  openModal({ title: acc.alias || acc.username, body, footer: foot });
}

function kv(label, value) {
  return h('div', null, h('dt', null, label), h('dd', null, value));
}

/* ---------------- 登录 ---------------- */

async function doLogin(acc, ctx, refresh) {
  toast('正在登录，请稍候…');
  try {
    const res = await api.accountLogin(acc.id);
    if (res.done) {
      toast('登录成功', 'ok');
      closeModal();
      refresh();
      return;
    }
    // 需要验证码
    openCaptchaForm(acc, res, refresh);
  } catch (err) {
    toast(err.message, 'err');
  }
}

function openCaptchaForm(acc, res, refresh) {
  const input = h('input', {
    placeholder: '请输入图中字符', autocomplete: 'off', maxlength: '8',
    inputmode: 'text',
  });
  const img = h('img', {
    class: 'captcha-img',
    src: res.captcha,
    alt: '验证码，点击刷新',
    title: '看不清？点击后重新登录获取新验证码',
    onclick: () => { closeModal(); doLogin(acc, null, refresh); },
  });
  const error = h('p', { class: 'form-error' });
  const submitBtn = h('button', { class: 'btn primary' }, '提交验证码');

  const submit = async () => {
    const code = input.value.trim();
    if (!code) { error.textContent = '请输入验证码'; return; }
    submitBtn.disabled = true;
    submitBtn.textContent = '验证中…';
    try {
      await api.accountCaptcha(acc.id, res.ticket, code);
      toast('登录成功', 'ok');
      closeModal();
      refresh();
    } catch (err) {
      error.textContent = err.message;
      submitBtn.disabled = false;
      submitBtn.textContent = '提交验证码';
    }
  };

  submitBtn.addEventListener('click', submit);
  input.addEventListener('keydown', (e) => { if (e.key === 'Enter') submit(); });

  openModal({
    title: '输入验证码',
    body: h('div', null,
      h('p', { class: 'hint-text', style: { marginBottom: '10px' } },
        `正在为「${acc.alias || acc.username}」登录，学校要求图形验证码。`),
      img, input, error,
    ),
    footer: h('button', { class: 'btn primary', onclick: submit }, '提交验证码'),
  });
  setTimeout(() => input.focus(), 80);
}

function openCookieForm(acc, refresh) {
  const input = h('textarea', {
    placeholder: '从浏览器复制今日校园网页版 Cookie，形如：MOD_AUTH_CAS=xxx; sessionToken=yyy',
  });
  const error = h('p', { class: 'form-error' });

  openModal({
    title: '导入 Cookie',
    body: h('div', null,
      h('div', { class: 'info-box', style: { marginBottom: '14px' } },
        '在电脑浏览器登录今日校园网页版，打开开发者工具 → Network → 任选一个请求，复制请求头里的 Cookie 整段粘贴到下方。Cookie 会加密保存在你的服务器上。'),
      input, error,
    ),
    footer: [
      h('button', { class: 'btn ghost', onclick: closeModal }, '取消'),
      h('button', {
        class: 'btn primary',
        onclick: async () => {
          const raw = input.value.trim();
          if (!raw) { error.textContent = '请粘贴 Cookie'; return; }
          try {
            const res = await api.importCookies(acc.id, raw);
            toast(res.message || 'Cookie 已保存', 'ok');
            closeModal();
            refresh();
          } catch (err) {
            error.textContent = err.message;
          }
        },
      }, '保存并校验'),
    ],
  });
  setTimeout(() => input.focus(), 80);
}

/* ---------------- 设备码 ---------------- */

async function openDeviceCode(acc) {
  let res;
  try {
    res = await api.deviceTicket(acc.id);
  } catch (err) {
    toast(err.message, 'err');
    return;
  }

  const statusBox = h('div', { class: 'scan-status' },
    h('span', { class: 'scan-dot waiting' }),
    h('span', null, '等待扫码…'),
  );
  const resultBox = h('div', { class: 'scan-result', hidden: true });

  // 轮询：扫码页提交后，这里自动拿到设备信息
  let pollTimer = null;
  let stopped = false;
  const stopPolling = () => {
    stopped = true;
    if (pollTimer) { clearTimeout(pollTimer); pollTimer = null; }
  };

  const showCollected = (data) => {
    statusBox.replaceChildren(
      h('span', { class: 'scan-dot ok' }),
      h('span', null, '已扫码，设备信息已获取'),
    );
    resultBox.hidden = false;
    const rows = [
      ['设备 ID', data.deviceId],
      ['机型', data.deviceModel || '—'],
      ['系统版本', data.systemVersion || '—'],
      ['平台', data.platform === 'ios' ? 'iOS' : 'Android'],
    ];
    if (data.buildId) rows.push(['构建号', data.buildId]);
    if (data.uuid && data.uuid !== data.deviceId) rows.push(['UUID（不用于签到）', data.uuid]);
    resultBox.replaceChildren(
      h('div', { class: 'kv-list' },
        rows.map(([k, v]) => h('div', null, h('dt', null, k), h('dd', null, String(v))))),
    );
    if (acc.onCollected) acc.onCollected(data);
    // 自动关闭，回到表单
    setTimeout(() => {
      if (!stopped) {
        closeModal();
        toast('设备信息已自动填入表单，记得点保存', 'ok');
      }
    }, 1200);
  };

  const poll = async () => {
    if (stopped) return;
    try {
      const data = await api.devicePoll(res.ticket);
      if (stopped) return;
      if (data && data.pending === false && data.collected) {
        stopPolling();
        showCollected(data);
        return;
      }
    } catch (err) {
      // 票据过期或失效：停止轮询并提示
      stopPolling();
      statusBox.replaceChildren(
        h('span', { class: 'scan-dot err' }),
        h('span', null, err.message || '二维码已失效'),
      );
      return;
    }
    pollTimer = setTimeout(poll, 1500);
  };
  pollTimer = setTimeout(poll, 1200);

  const modalBody = h('div', null,
    h('div', { class: 'info-box', style: { marginBottom: '14px' } },
      '打开「今日校园 App」→ 右上角「＋」→ 扫一扫，扫描下方二维码。' +
      '扫码后会自动把设备信息填回来，不用手动输入。'),
    res.qrcode ? h('img', { class: 'qr-img', src: res.qrcode, alt: '设备绑定二维码' }) : null,
    statusBox,
    resultBox,
    h('p', { class: 'hint-text', style: { marginTop: '10px', textAlign: 'center' } },
      '二维码 15 分钟内有效，仅可使用一次'),
    h('div', { class: 'warn-box', style: { marginTop: '14px' } },
      '必须用你平时打卡的那台手机扫码。换手机或重装 App 后设备 ID 会变化。'),
  );

  openModal({
    title: '扫码绑定设备',
    wide: true,
    body: modalBody,
    footer: [
      h('button', {
        class: 'btn ghost',
        onclick: async () => {
          const ok = await copyText(res.url);
          toast(ok ? '链接已复制' : '复制失败，请手动选择', ok ? 'ok' : 'err');
        },
      }, '复制链接'),
      h('button', {
        class: 'btn ghost',
        onclick: () => { stopPolling(); openDeviceCode(acc); },
      }, '刷新二维码'),
      h('button', {
        class: 'btn primary',
        onclick: () => { stopPolling(); closeModal(); },
      }, '关闭'),
    ],
    onClose: stopPolling,
  });
}

/* ---------------- 新增 / 编辑 ---------------- */

function openAccountForm(ctx, existing, refresh) {
  const isEdit = Boolean(existing);

  /* ---------- 学校账号 ---------- */
  const schoolInput = h('input', {
    placeholder: '输入学校全称，如：三峡大学科技学院', value: existing?.schoolName || '',
    list: 'schoolOptions', autocomplete: 'off',
  });
  const datalist = h('datalist', { id: 'schoolOptions' });
  const schoolHint = h('p', { class: 'hint-text' }, '输入几个字会自动匹配，从下拉中选中即可。');

  schoolInput.addEventListener('input', debounce(async (e) => {
    const kw = e.target.value.trim();
    if (kw.length < 2) return;
    try {
      const { schools } = await api.schools(kw);
      datalist.replaceChildren();
      schools.slice(0, 20).forEach((s) => datalist.append(h('option', { value: s.name })));
      schoolHint.textContent = schools.length
        ? '匹配到 ' + schools.length + ' 所学校，请从下拉中选择'
        : '未匹配到学校，请检查名称是否为今日校园中的全称';
    } catch { schoolHint.textContent = '学校查询失败，可手动填写'; }
  }, 320));

  const usernameInput = h('input', {
    placeholder: '学号 / 工号', value: existing?.username || '', autocomplete: 'username',
  });
  const passwordInput = h('input', {
    type: 'password', autocomplete: 'new-password',
    placeholder: isEdit ? '留空表示不修改' : '今日校园登录密码',
  });

  /* ---------- 只读展示区（全部来自扫码） ---------- */
  const deviceInfoBox = h('div', { class: 'readonly-box' });
  const locInfoBox = h('div', { class: 'readonly-box' });

  const renderDeviceInfo = (info) => {
    const rows = [];
    if (info.deviceId) rows.push(['设备 ID', info.deviceId, true]);
    if (info.deviceModel) rows.push(['机型', info.deviceModel]);
    if (info.systemVersion) rows.push(['系统版本', info.systemVersion]);
    if (info.platform) rows.push(['平台', info.platform === 'ios' ? 'iOS' : 'Android']);
    if (info.buildId) rows.push(['构建号', info.buildId]);
    if (info.appVersion) rows.push(['App 版本', info.appVersion]);
    if (info.wisDeviceId) rows.push(['wisDeviceId', info.wisDeviceId, true]);

    if (rows.length === 0) {
      deviceInfoBox.replaceChildren(
        h('div', { class: 'readonly-empty' }, '尚未扫码获取设备信息'),
      );
      return;
    }
    deviceInfoBox.replaceChildren(
      h('div', { class: 'readonly-list' },
        rows.map(([k, v, mono]) => h('div', { class: 'readonly-row' },
          h('span', { class: 'readonly-key' }, k),
          h('b', { class: 'readonly-val' + (mono ? ' mono' : ''), title: v }, v),
        )),
      ),
    );
  };

  const renderLocInfo = (lon, lat, address) => {
    if (!lon || !lat) {
      locInfoBox.replaceChildren(
        h('div', { class: 'readonly-empty' }, '尚未获取签到位置'),
      );
      return;
    }
    const rows = [
      ['位置名称', address || '—'],
      ['坐标', Number(lon).toFixed(6) + ', ' + Number(lat).toFixed(6), true],
    ];
    locInfoBox.replaceChildren(
      h('div', { class: 'readonly-list' },
        rows.map(([k, v, mono]) => h('div', { class: 'readonly-row' },
          h('span', { class: 'readonly-key' }, k),
          h('b', { class: 'readonly-val' + (mono ? ' mono' : ''), title: v }, v),
        )),
      ),
    );
  };

  let curDevice = {
    deviceId: existing?.deviceId || '',
    deviceModel: existing?.deviceModel || '',
    systemVersion: existing?.systemVersion || '',
    buildId: existing?.buildId || '',
    appVersion: existing?.appVersion || '',
    wisDeviceId: existing?.wisDeviceId || '',
    platform: existing?.platform || '',
    userAgent: existing?.userAgent || '',
  };
  let curLon = existing?.lon || '';
  let curLat = existing?.lat || '';
  let curAddress = existing?.address || '';

  renderDeviceInfo(curDevice);
  renderLocInfo(curLon, curLat, curAddress);

  /* ---------- 扫码按钮 ---------- */
  const scanBtn = h('button', {
    type: 'button', class: 'btn primary sm',
    disabled: !isEdit,
    onclick: async () => {
      if (!isEdit) { toast('请先保存账号，再扫码绑定设备', 'err'); return; }
      await openDeviceCode({
        id: existing.id,
        alias: existing.alias,
        onCollected: (data) => {
          curDevice = {
            deviceId: data.deviceId || '',
            deviceModel: data.deviceModel || '',
            systemVersion: data.systemVersion || '',
            buildId: data.buildId || '',
            appVersion: data.appVersion || '',
            wisDeviceId: data.wisDeviceId || '',
            platform: data.platform || '',
            userAgent: data.userAgent || '',
          };
          renderDeviceInfo(curDevice);
          toast('设备信息已获取', 'ok');
        },
      });
    },
  }, icon('qr'), '扫码获取设备信息');

  const locationBtn = h('button', {
    type: 'button', class: 'btn primary sm',
    disabled: !isEdit,
    onclick: async (e) => {
      if (!isEdit) { toast('请先保存账号，再获取签到位置', 'err'); return; }
      const btn = e.currentTarget;
      const original = btn.textContent;
      btn.disabled = true;
      btn.textContent = '获取中…';
      try {
        const res = await api.applySignRange(existing.id);
        const r = res.range || {};
        curLon = r.lon;
        curLat = r.lat;
        curAddress = r.address || '';
        renderLocInfo(curLon, curLat, curAddress);
        toast('已获取签到位置：' + (r.address || '学校签到点'), 'ok');
      } catch (err) {
        toast(err.message, 'err');
      } finally {
        btn.disabled = false;
        btn.textContent = original;
      }
    },
  }, icon('pin'), '自动获取位置');

  /* ---------- 照片 ---------- */
  const photoInput = h('input', { type: 'file', accept: 'image/png,image/jpeg,image/webp' });
  const error = h('p', { class: 'form-error' });

  /* ---------- 组装 ---------- */
  const body = h('div', null,
    h('div', { class: 'form-section' },
      h('h3', null, '学校账号'),
      h('label', { class: 'field' }, h('span', null, '学校全称 *'), schoolInput, datalist, schoolHint),
      h('label', { class: 'field' }, h('span', null, '学号 *'), usernameInput),
      h('label', { class: 'field' },
        h('span', null, isEdit ? '密码（留空不改）' : '密码'),
        passwordInput,
        h('span', { class: 'hint-text' }, 'AES-256-GCM 加密保存在你自己的服务器，仅用于自动登录。')),
    ),

    h('div', { class: 'form-section' },
      h('div', { class: 'section-head' },
        h('h3', null, '设备信息'),
        scanBtn,
      ),
      deviceInfoBox,
      h('p', { class: 'hint-text' },
        isEdit
          ? '用你平时打卡的那台手机，打开今日校园 App 扫一扫。扫码后设备信息会自动填入，不需要手动填写。'
          : '保存账号后即可扫码获取设备信息。'),
    ),

    h('div', { class: 'form-section' },
      h('div', { class: 'section-head' },
        h('h3', null, '签到位置'),
        locationBtn,
      ),
      locInfoBox,
      h('p', { class: 'hint-text' },
        isEdit
          ? '读取学校设置好的签到点，自动填入位置名称与坐标，不需要手动选点。'
          : '保存账号后即可自动获取签到位置。'),
    ),

    h('div', { class: 'form-section' },
      h('h3', null, '签到照片'),
      h('label', { class: 'field' },
        photoInput,
        h('span', { class: 'hint-text' }, '查寝任务需要照片时自动使用；jpg / png / webp，不超过 8MB。')),
    ),

    error,
  );

  openModal({
    title: isEdit ? '编辑账号' : '添加账号',
    wide: true,
    body,
    footer: [
      h('button', { class: 'btn ghost', onclick: closeModal }, '取消'),
      h('button', {
        class: 'btn primary',
        onclick: async (e) => {
          const btn = e.currentTarget;
          const payload = {
            schoolName: schoolInput.value.trim(),
            username: usernameInput.value.trim(),
            password: passwordInput.value,
            // 设备与位置字段只读，全部来自扫码/自动获取
            deviceId: curDevice.deviceId,
            deviceModel: curDevice.deviceModel,
            systemVersion: curDevice.systemVersion,
            buildId: curDevice.buildId,
            appVersion: curDevice.appVersion,
            wisDeviceId: curDevice.wisDeviceId,
            platform: curDevice.platform,
            userAgent: curDevice.userAgent,
            lon: Number(curLon) || 0,
            lat: Number(curLat) || 0,
            address: curAddress,
          };
          if (!payload.schoolName) { error.textContent = '请填写学校全称'; return; }
          if (!payload.username) { error.textContent = '请填写学号'; return; }
          if (!isEdit && !payload.password) { error.textContent = '请填写密码'; return; }
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
              await api.upload('/accounts/' + account.id + '/photo', fd);
            }
            toast(isEdit ? '账号已更新' : '账号已添加', 'ok');
            closeModal();
            refresh();
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

/* ---------------- 删除账号 ---------------- */

async function removeAccount(acc, refresh) {
  const ok = await confirmDialog({
    title: '删除账号',
    message: '确定删除「' + (acc.alias || acc.username) + '」吗？该账号的计划与执行记录会一并删除，且不可恢复。',
    confirmText: '删除',
    danger: true,
  });
  if (!ok) return;
  try {
    await api.deleteAccount(acc.id);
    toast('账号已删除', 'ok');
    closeModal();
    refresh();
  } catch (err) {
    toast(err.message, 'err');
  }
}

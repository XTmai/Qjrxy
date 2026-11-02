/* 校园账号页 */
import { api } from './api.js';
import {
  setChildren,
  el,
  ico,
  toast,
  skeleton,
  emptyBox,
  accountTag,
  initial,
  debounce,
  fmtTime,
  openModal,
  closeModal,
  confirmBox,
  copyText,
  kvList,
} from './ui.js';

let cache = { list: [], keyword: '', filter: 'all' };

export async function pageAccounts(root, ctx) {
  setChildren(root, skeleton());
  let accounts = [];
  try {
    ({ accounts } = await api.accounts());
  } catch (err) {
    setChildren(root, emptyBox('加载账号失败', err.message,
      el('button', { class: 'btn main', onclick: () => pageAccounts(root, ctx) }, '重试')));
    return;
  }
  cache.list = accounts;

  const head = el('div', { class: 'head' },
    el('div', null,
      el('h1', null, '校园账号'),
      el('div', { class: 'sub' }, '已绑定 ' + accounts.length + ' 个账号'),
    ),
    el('div', { class: 'head-actions' },
      el('button', {
        class: 'btn main',
        onclick: () => accountForm(ctx, null, () => pageAccounts(root, ctx)),
      }, ico('plus'), '添加账号'),
    ),
  );

  const search = el('input', {
    placeholder: '搜索账号、学号或学校',
    value: cache.keyword,
    oninput: debounce((e) => {
      cache.keyword = e.target.value.trim();
      paint();
    }, 200),
  });

  const filters = [
    ['all', '全部'],
    ['authorized', '已登录'],
    ['attention', '需处理'],
  ];
  const seg = el('div', { class: 'seg' },
    filters.map(([key, label]) => el('button', {
      class: cache.filter === key ? 'on' : '',
      dataset: { filter: key },
      onclick: () => {
        cache.filter = key;
        seg.querySelectorAll('button').forEach((b) =>
          b.classList.toggle('on', b.dataset.filter === key));
        paint();
      },
    }, label)),
  );

  const toolbar = el('div', { class: 'toolbar' },
    el('div', { class: 'search-wrap' }, search),
    seg,
  );

  const list = el('div', { class: 'acct-list' });

  function paint() {
    const kw = cache.keyword.toLowerCase();
    const shown = accounts.filter((a) => {
      if (cache.filter === 'authorized' && a.loginStatus !== 'authorized') return false;
      if (cache.filter === 'attention' && a.loginStatus === 'authorized') return false;
      if (!kw) return true;
      return [a.alias, a.username, a.schoolName]
        .some((v) => String(v || '').toLowerCase().includes(kw));
    });

    list.replaceChildren();
    if (!shown.length) {
      list.append(emptyBox(
        accounts.length ? '没有匹配的账号' : '还没有绑定账号',
        accounts.length ? '换个关键词试试' : '添加今日校园账号后即可查询查寝任务',
        accounts.length ? null : el('button', {
          class: 'btn main sm',
          onclick: () => accountForm(ctx, null, () => pageAccounts(root, ctx)),
        }, '添加账号'),
      ));
      return;
    }
    shown.forEach((a) => list.append(accountRow(a, ctx, () => pageAccounts(root, ctx))));
  }

  paint();
  setChildren(root, head, toolbar, list);
}

function accountRow(acc, ctx, refresh) {
  const tags = el('div', { class: 'acct-tags' },
    accountTag(acc.loginStatus),
    acc.deviceId
      ? el('span', { class: 'tag info' }, (acc.deviceModel || '设备') + ' · ' + (acc.platform === 'ios' ? 'iOS' : 'Android'))
      : el('span', { class: 'tag warn' }, '未绑设备'),
    acc.lon && acc.lat
      ? el('span', { class: 'tag plain' }, '位置已设')
      : el('span', { class: 'tag warn' }, '未设位置'),
    acc.photoUrl ? el('span', { class: 'tag plain' }, '有照片') : null,
  );

  return el('div', { class: 'acct' },
    el('div', { class: 'avatar' }, initial(acc.alias || acc.username)),
    el('div', { class: 'acct-main' },
      el('div', { class: 'acct-name' }, acc.alias || acc.username),
      el('div', { class: 'acct-school' }, acc.schoolName + ' · ' + acc.username),
      tags,
    ),
    el('div', { class: 'acct-acts' },
      el('button', { class: 'btn plain sm', onclick: () => ctx.go('/tasks') }, '任务'),
      el('button', { class: 'btn soft sm', onclick: () => accountDetail(acc, ctx, refresh) }, '管理'),
    ),
  );
}

/* ---------------- 账号详情 ---------------- */

function accountDetail(acc, ctx, refresh) {
  const body = el('div', null,
    kvList([
      ['学校', acc.schoolName],
      ['学号', acc.username],
      ['登录状态', acc.loginStatus === 'authorized' ? '已登录'
        : acc.loginStatus === 'expired' ? '需重新登录' : '未登录'],
      ['最近登录', acc.lastLoginAt ? fmtTime(acc.lastLoginAt) : '—'],
      ['设备 ID', acc.deviceId || '未绑定', true],
      ['机型', acc.deviceId ? acc.deviceModel + ' · ' + acc.systemVersion : '—'],
      ['签到位置', acc.lon && acc.lat ? (acc.address || '已设置') : '未设置'],
      ['签到照片', acc.photoUrl ? '已上传' : '未上传'],
    ]),
  );

  openModal({
    title: acc.alias || acc.username,
    sub: acc.schoolName,
    body,
    wide: true,
    actions: [
      el('button', { class: 'btn plain sm', onclick: () => { closeModal(); doLogin(acc, refresh); } },
        ico('login'), '账密登录'),
      el('button', { class: 'btn plain sm', onclick: () => { closeModal(); cookieForm(acc, refresh); } },
        ico('link'), 'Cookie'),
      el('button', { class: 'btn plain sm', onclick: () => { closeModal(); accountForm(ctx, acc, refresh); } },
        ico('edit'), '编辑'),
      el('button', { class: 'btn danger-text sm', onclick: () => removeAccount(acc, refresh) },
        ico('trash'), '删除'),
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
    captchaForm(acc, res, refresh);
  } catch (err) {
    toast(err.message, 'err');
  }
}

function captchaForm(acc, res, refresh) {
  const input = el('input', { placeholder: '请输入图中字符', autocomplete: 'off' });
  const img = el('img', {
    class: 'captcha-img',
    src: res.captcha,
    alt: '验证码',
    title: '点击刷新',
    onclick: () => { closeModal(); doLogin(acc, refresh); },
  });
  const err = el('p', { class: 'form-err' });

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

  openModal({
    title: '输入验证码',
    sub: '学校要求图形验证码',
    body: el('div', null,
      img,
      el('div', { class: 'field' }, input),
      err,
    ),
    actions: [
      el('button', { class: 'btn plain', onclick: closeModal }, '取消'),
      el('button', { class: 'btn main', onclick: submit }, '提交'),
    ],
  });
}

function cookieForm(acc, refresh) {
  const area = el('textarea', {
    placeholder: '从浏览器复制今日校园网页版 Cookie，形如：MOD_AUTH_CAS=xxx',
  });
  const err = el('p', { class: 'form-err' });

  openModal({
    title: '导入 Cookie',
    sub: '账密登录失败时的备用方式',
    body: el('div', null,
      el('div', { class: 'note-box info', style: { marginBottom: '12px' } },
        '在浏览器登录今日校园网页版，打开开发者工具 → Network → 任选一个请求，复制请求头里的 Cookie 整段粘贴到下面。'),
      area,
      err,
    ),
    actions: [
      el('button', { class: 'btn plain', onclick: closeModal }, '取消'),
      el('button', {
        class: 'btn main',
        onclick: async () => {
          const raw = area.value.trim();
          if (!raw) { err.textContent = '请粘贴 Cookie'; return; }
          try {
            const r = await api.importCookies(acc.id, raw);
            toast(r.message || 'Cookie 已保存', 'ok');
            closeModal();
            refresh();
          } catch (e) {
            err.textContent = e.message;
          }
        },
      }, '保存并校验'),
    ],
  });
}

/* ---------------- 新增 / 编辑 ---------------- */

function accountForm(ctx, existing, refresh) {
  const isEdit = Boolean(existing);

  /* 学校账号 */
  const schoolInput = el('input', {
    placeholder: '输入学校全称，如：三峡大学科技学院',
    value: existing?.schoolName || '',
    list: 'schoolList',
    autocomplete: 'off',
  });
  const datalist = el('datalist', { id: 'schoolList' });
  const schoolHint = el('div', { class: 'hint' }, '输入几个字会自动匹配，从下拉中选中即可。');

  schoolInput.addEventListener('input', debounce(async (e) => {
    const kw = e.target.value.trim();
    if (kw.length < 2) return;
    try {
      const { schools } = await api.schools(kw);
      datalist.replaceChildren();
      schools.slice(0, 20).forEach((s) => datalist.append(el('option', { value: s.name })));
      schoolHint.textContent = schools.length
        ? '匹配到 ' + schools.length + ' 所学校'
        : '未匹配到，请检查名称';
    } catch { schoolHint.textContent = '查询失败，可手动填写'; }
  }, 320));

  const userInput = el('input', {
    placeholder: '学号 / 工号', value: existing?.username || '', autocomplete: 'username',
  });
  const passInput = el('input', {
    type: 'password', autocomplete: 'new-password',
    placeholder: isEdit ? '留空表示不修改' : '今日校园登录密码',
  });

  /* 只读：设备信息 */
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
  let loc = {
    lon: existing?.lon || '',
    lat: existing?.lat || '',
    address: existing?.address || '',
  };

  const devBox = el('div');
  const locBox = el('div');

  const paintDev = () => {
    if (!dev.deviceId) {
      devBox.replaceChildren(el('div', { class: 'box-empty' }, '尚未获取设备信息'));
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
      locBox.replaceChildren(el('div', { class: 'box-empty' }, '尚未获取签到位置'));
      return;
    }
    locBox.replaceChildren(kvList([
      ['位置名称', loc.address || '—'],
      ['坐标', Number(loc.lon).toFixed(6) + ', ' + Number(loc.lat).toFixed(6), true],
    ]));
  };

  paintDev();
  paintLoc();

  const scanBtn = el('button', {
    class: 'btn main sm',
    disabled: !isEdit,
    onclick: async () => {
      if (!isEdit) { toast('请先保存账号，再扫码', 'err'); return; }
      await deviceScan(existing.id, (data) => {
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
  }, ico('qr'), '扫码获取');

  const locBtn = el('button', {
    class: 'btn main sm',
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
  }, ico('pin'), '自动获取');

  const photoInput = el('input', { type: 'file', accept: 'image/png,image/jpeg,image/webp' });
  const err = el('p', { class: 'form-err' });

  const body = el('div', null,
    el('div', { class: 'block' },
      el('div', { class: 'block-head' }, el('h3', null, '学校账号')),
      el('div', { class: 'field' },
        el('label', null, '学校全称 *'), schoolInput, datalist, schoolHint),
      el('div', { class: 'field' },
        el('label', null, '学号 *'), userInput),
      el('div', { class: 'field' },
        el('label', null, isEdit ? '密码（留空不改）' : '密码'), passInput,
        el('div', { class: 'hint' }, 'AES-256-GCM 加密保存在你的服务器，仅用于自动登录。')),
    ),

    el('div', { class: 'block' },
      el('div', { class: 'block-head' },
        el('h3', null, '设备信息'), scanBtn,
      ),
      devBox,
      el('div', { class: 'hint', style: { marginTop: '8px' } },
        isEdit
          ? '用你打卡那台手机打开今日校园 App 扫一扫，设备信息自动填入。'
          : '保存账号后即可扫码获取。'),
    ),

    el('div', { class: 'block' },
      el('div', { class: 'block-head' },
        el('h3', null, '签到位置'), locBtn,
      ),
      locBox,
      el('div', { class: 'hint', style: { marginTop: '8px' } },
        isEdit ? '读取学校设置的签到点，自动填入位置与坐标。' : '保存账号后即可获取。'),
    ),

    el('div', { class: 'block' },
      el('div', { class: 'block-head' }, el('h3', null, '签到照片')),
      photoInput,
      el('div', { class: 'hint', style: { marginTop: '6px' } },
        '查寝任务需要照片时使用。jpg / png / webp，不超过 8MB。'),
    ),

    err,
  );

  openModal({
    title: isEdit ? '编辑账号' : '添加账号',
    sub: isEdit ? (existing.alias || existing.username) : '添加后可以扫码绑定设备',
    body,
    wide: true,
    actions: [
      el('button', { class: 'btn plain', onclick: closeModal }, '取消'),
      el('button', {
        class: 'btn main',
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
            let acc;
            if (isEdit) ({ account: acc } = await api.updateAccount(existing.id, payload));
            else ({ account: acc } = await api.createAccount(payload));

            const file = photoInput.files?.[0];
            if (file) {
              const fd = new FormData();
              fd.append('photo', file);
              await api.uploadPhoto(acc.id, fd);
            }
            toast(isEdit ? '已保存' : '账号已添加', 'ok');
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

async function deviceScan(accountId, onDone) {
  let ticket;
  try {
    const res = await api.deviceTicket(accountId);
    ticket = res.ticket;
    openScanModal(res, ticket, onDone);
  } catch (err) {
    toast(err.message, 'err');
  }
}

function openScanModal(res, ticket, onDone) {
  let stopped = false;
  let timer = null;

  const status = el('div', { class: 'scan-status' },
    el('span', { class: 'pulse' }),
    el('span', null, '等待扫码…'),
  );
  const result = el('div');

  const stop = () => {
    stopped = true;
    if (timer) { clearTimeout(timer); timer = null; }
  };

  const poll = async () => {
    if (stopped) return;
    try {
      const data = await api.devicePoll(ticket);
      if (stopped) return;
      if (data && data.pending === false && data.collected) {
        stop();
        status.replaceChildren(el('span', { class: 'pulse ok' }), el('span', null, '已获取设备信息'));
        result.replaceChildren(kvList([
          ['设备 ID', data.deviceId, true],
          ['机型', data.deviceModel || '—'],
          ['系统版本', data.systemVersion || '—'],
          ['平台', data.platform === 'ios' ? 'iOS' : 'Android'],
        ]));
        onDone?.(data);
        setTimeout(() => { if (!stopped) closeModal(); }, 1000);
        return;
      }
    } catch (e) {
      stop();
      status.replaceChildren(el('span', { class: 'pulse err' }), el('span', null, e.message));
      return;
    }
    timer = setTimeout(poll, 1500);
  };
  timer = setTimeout(poll, 1200);

  openModal({
    title: '扫码获取设备信息',
    sub: '用今日校园 App 扫一扫',
    body: el('div', null,
      el('div', { class: 'note-box info', style: { marginBottom: '14px' } },
        '打开今日校园 App → 右上角「＋」→ 扫一扫，扫描下方二维码。'),
      res.qrcode ? el('img', { class: 'qr-img', src: res.qrcode, alt: '设备二维码' }) : null,
      status,
      result,
      el('div', { class: 'hint', style: { marginTop: '10px', textAlign: 'center' } },
        '二维码 15 分钟内有效'),
      el('div', { class: 'note-box warn', style: { marginTop: '12px' } },
        '必须用你平时打卡的那台手机扫码。换手机或重装 App 后设备 ID 会变化。'),
    ),
    actions: [
      el('button', {
        class: 'btn plain', onclick: async () => {
          const ok = await copyText(res.url);
          toast(ok ? '链接已复制' : '复制失败');
        },
      }, '复制链接'),
      el('button', { class: 'btn main', onclick: () => { stop(); closeModal(); } }, '关闭'),
    ],
    onClose: stop,
  });
}

/* ---------------- 删除 ---------------- */

async function removeAccount(acc, refresh) {
  const ok = await confirmBox({
    title: '删除账号',
    message: '确定删除「' + (acc.alias || acc.username) +
      '」吗？该账号的计划与执行记录会一并删除，无法恢复。',
    okText: '删除',
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

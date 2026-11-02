/* =========================================================
   地图选点（站内弹层，不打开新标签页）
   - 高德底图（GCJ-02），与今日校园 App 定位一致
   - 返回 Promise<{lon, lat, address} | null>
   ========================================================= */
import { h, icon } from './ui.js';

let leafletPromise = null;

/** 按需加载 Leaflet（本地文件，不依赖外部 CDN） */
function loadLeaflet() {
  if (window.L) return Promise.resolve(window.L);
  if (leafletPromise) return leafletPromise;

  leafletPromise = new Promise((resolve, reject) => {
    const css = document.createElement('link');
    css.rel = 'stylesheet';
    css.href = '/vendor/leaflet.css';
    document.head.append(css);

    const script = document.createElement('script');
    script.src = '/vendor/leaflet.js';
    script.onload = () => (window.L ? resolve(window.L) : reject(new Error('Leaflet 加载失败')));
    script.onerror = () => reject(new Error('Leaflet 加载失败，请检查网络'));
    document.head.append(script);
  });
  return leafletPromise;
}

const DEFAULT_CENTER = [30.692, 111.286];
const DEFAULT_ZOOM = 16;

/**
 * 打开地图选点弹层。
 * @param {{lon?: number|string, lat?: number|string}} initial 初始坐标
 * @returns {Promise<{lon:number, lat:number, address:string}|null>}
 */
export async function pickLocation(initial = {}) {
  const lon0 = parseFloat(initial.lon);
  const lat0 = parseFloat(initial.lat);
  const hasInit = Number.isFinite(lon0) && Number.isFinite(lat0) && (lon0 !== 0 || lat0 !== 0);

  let L;
  try {
    L = await loadLeaflet();
  } catch (err) {
    throw err;
  }

  return new Promise((resolve) => {
    let leafletMap = null;
    let reverseName = '';
    let settled = false;

    const lngEl = h('div', { class: 'pk-coord-value' }, '--');
    const latEl = h('div', { class: 'pk-coord-value' }, '--');
    const msgEl = h('div', { class: 'pk-msg' }, '');

    const setMsg = (text, kind) => {
      msgEl.textContent = text || '';
      msgEl.className = 'pk-msg' + (kind ? ' ' + kind : '');
    };

    const mapHost = h('div', { class: 'pk-map' });

    const close = (value) => {
      if (settled) return;
      settled = true;
      document.removeEventListener('keydown', onKey);
      if (leafletMap) {
        leafletMap.remove();
        leafletMap = null;
      }
      overlay.classList.add('pk-closing');
      setTimeout(() => overlay.remove(), 160);
      document.body.style.removeProperty('overflow');
      resolve(value);
    };

    const onKey = (e) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        close(null);
      }
    };

    const current = () => {
      const c = leafletMap.getCenter();
      return { lon: c.lng, lat: c.lat };
    };

    const refresh = () => {
      const c = current();
      lngEl.textContent = c.lon.toFixed(6);
      latEl.textContent = c.lat.toFixed(6);
    };

    const jumpTo = (lon, lat, zoom) => {
      leafletMap.setView([lat, lon], zoom || Math.max(leafletMap.getZoom(), 17));
    };

    /* --- 控件 --- */
    const keywordInput = h('input', {
      placeholder: '搜索地点，如 XX 大学南门', autocomplete: 'off',
    });
    const jumpInput = h('input', {
      placeholder: '或粘贴坐标：经度,纬度', autocomplete: 'off',
    });

    const doSearch = async () => {
      const kw = keywordInput.value.trim();
      if (!kw) { setMsg('请输入地点关键字', 'warn'); return; }
      setMsg('正在搜索…');
      try {
        const res = await fetch('https://nominatim.openstreetmap.org/search?format=json&limit=5&q=' +
          encodeURIComponent(kw), { headers: { Accept: 'application/json' } });
        const list = await res.json();
        if (!Array.isArray(list) || list.length === 0) {
          setMsg('没有找到该地点，请拖动地图手动定位', 'warn');
          return;
        }
        const hit = list[0];
        const lat = parseFloat(hit.lat);
        const lon = parseFloat(hit.lon);
        if (!Number.isFinite(lat) || !Number.isFinite(lon)) {
          setMsg('搜索结果无效', 'warn');
          return;
        }
        jumpTo(lon, lat, 17);
        setMsg('已定位到「' + String(hit.display_name || kw).slice(0, 30) + '…」，请微调', 'ok');
      } catch {
        setMsg('搜索服务不可用，请手动拖动地图定位', 'warn');
      }
    };

    const doJump = () => {
      const raw = jumpInput.value.trim();
      if (!raw) { setMsg('请先粘贴坐标', 'warn'); return; }
      const parts = raw.split(/[,，\s]+/).filter(Boolean);
      if (parts.length < 2) { setMsg('坐标格式应为：经度,纬度', 'warn'); return; }
      const lon = parseFloat(parts[0]);
      const lat = parseFloat(parts[1]);
      if (!Number.isFinite(lon) || !Number.isFinite(lat)) { setMsg('坐标无法解析', 'warn'); return; }
      if (lon < -180 || lon > 180 || lat < -90 || lat > 90) { setMsg('坐标超出有效范围', 'warn'); return; }
      jumpTo(lon, lat);
      setMsg('已跳转到指定坐标');
    };

    const doLocate = () => {
      if (!navigator.geolocation) { setMsg('当前浏览器不支持定位', 'warn'); return; }
      setMsg('正在获取当前位置…');
      navigator.geolocation.getCurrentPosition(
        (pos) => {
          jumpTo(pos.coords.longitude, pos.coords.latitude, 17);
          setMsg('已定位到当前位置（浏览器为 WGS-84，与高德底图可能差几十米）', 'warn');
        },
        () => setMsg('定位失败或被拒绝，请手动选择', 'warn'),
        { enableHighAccuracy: true, timeout: 8000 },
      );
    };

    const doReverse = async () => {
      const c = current();
      setMsg('正在获取地点名…');
      try {
        const res = await fetch('https://nominatim.openstreetmap.org/reverse?format=json&zoom=18' +
          '&addressdetails=1&lat=' + c.lat + '&lon=' + c.lon,
          { headers: { Accept: 'application/json' } });
        const data = await res.json();
        const addr = data && data.address;
        const name = (data && data.display_name) ||
          (addr && (addr.amenity || addr.building || addr.road || addr.suburb || addr.city)) || '';
        if (name) {
          reverseName = String(name);
          setMsg('地点名：' + reverseName.slice(0, 40), 'ok');
        } else {
          setMsg('没有取到地点名，可手填地址', 'warn');
        }
      } catch {
        setMsg('地点名获取失败，可手填地址', 'warn');
      }
    };

    keywordInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); doSearch(); }
    });
    jumpInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); doJump(); }
    });

    const overlay = h('div', { class: 'pk-overlay', role: 'dialog', 'aria-modal': 'true',
      'aria-label': '选择签到位置' },
      h('div', { class: 'pk-card' },
        h('header', { class: 'pk-head' },
          h('div', null,
            h('h2', null, '选择签到位置'),
            h('p', null, '拖动地图，把准星对准你的位置'),
          ),
          h('button', {
            class: 'pk-close', type: 'button', 'aria-label': '关闭',
            onclick: () => close(null),
          }, icon('x')),
        ),
        h('div', { class: 'pk-body' },
          h('div', { class: 'pk-map-wrap' }, mapHost),
          h('aside', { class: 'pk-panel' },
            h('div', { class: 'pk-coord-row' },
              h('div', { class: 'pk-coord-box' },
                h('div', { class: 'pk-coord-label' }, '经度'), lngEl),
              h('div', { class: 'pk-coord-box' },
                h('div', { class: 'pk-coord-label' }, '纬度'), latEl),
            ),
            h('div', { class: 'pk-row' }, keywordInput,
              h('button', { class: 'pk-btn ghost', type: 'button', onclick: doSearch }, '搜索')),
            h('div', { class: 'pk-row' }, jumpInput,
              h('button', { class: 'pk-btn ghost', type: 'button', onclick: doJump }, '跳转')),
            h('div', { class: 'pk-row' },
              h('button', { class: 'pk-btn ghost grow', type: 'button', onclick: doLocate }, '定位到我'),
              h('button', { class: 'pk-btn ghost grow', type: 'button', onclick: doReverse }, '取地点名'),
            ),
            msgEl,
            h('p', { class: 'pk-tip' },
              '坐标需落在学校签到点半径内。这里是高德底图（GCJ-02），与今日校园定位一致。'),
          ),
        ),
        h('footer', { class: 'pk-foot' },
          h('button', { class: 'pk-btn ghost', type: 'button', onclick: () => close(null) }, '取消'),
          h('button', {
            class: 'pk-btn primary', type: 'button',
            onclick: () => {
              const c = current();
              if (!Number.isFinite(c.lon) || !Number.isFinite(c.lat)) {
                setMsg('坐标无效，请重新选择', 'warn');
                return;
              }
              close({
                lon: Number(c.lon.toFixed(6)),
                lat: Number(c.lat.toFixed(6)),
                address: reverseName,
              });
            },
          }, '使用此位置'),
        ),
      ),
    );

    document.body.append(overlay);
    document.body.style.setProperty('overflow', 'hidden');
    document.addEventListener('keydown', onKey);

    // 弹层动画结束后再初始化地图，避免容器尺寸为 0
    requestAnimationFrame(() => {
      leafletMap = L.map(mapHost, {
        zoomControl: true,
        attributionControl: true,
        center: hasInit ? [lat0, lon0] : DEFAULT_CENTER,
        zoom: hasInit ? 17 : DEFAULT_ZOOM,
      });
      L.tileLayer('https://webrd0{s}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}', {
        subdomains: '1234',
        minZoom: 3,
        maxZoom: 19,
        attribution: '底图 © 高德地图 · GCJ-02',
      }).addTo(leafletMap);

      leafletMap.on('move', refresh);
      leafletMap.on('moveend', refresh);
      refresh();

      // 准星用 Leaflet 的 marker 实现：DOM 覆盖层会落在地图容器下层，
      // 而 marker 位于 Leaflet 自己的 pane 内，始终压在地图之上。
      const crosshairIcon = L.divIcon({
        className: 'pk-pin',
        html: '<svg viewBox="0 0 34 44" aria-hidden="true">' +
              '<path d="M17 2c-7.2 0-13 5.8-13 13 0 9.4 13 27 13 27s13-17.6 13-27c0-7.2-5.8-13-13-13z"/>' +
              '<circle cx="17" cy="15" r="4.6"/></svg>',
        iconSize: [34, 44],
        iconAnchor: [17, 44],
      });
      const centerMarker = L.marker(leafletMap.getCenter(), {
        icon: crosshairIcon,
        interactive: false,
        keyboard: false,
        zIndexOffset: 1000,
      }).addTo(leafletMap);
      // 地图移动时让 marker 始终停在中心
      leafletMap.on('move', () => centerMarker.setLatLng(leafletMap.getCenter()));

      leafletMap.whenReady(() => {
        setMsg(hasInit ? '' : '拖动地图，把准星对准你的签到位置');
        // 弹层尺寸变化时需要重新计算，否则瓦片会缺角
        setTimeout(() => leafletMap && leafletMap.invalidateSize(), 120);
      });
    });
  });
}

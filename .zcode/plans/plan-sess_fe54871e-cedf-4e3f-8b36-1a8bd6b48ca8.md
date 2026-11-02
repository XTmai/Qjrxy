# 青衿 UI 深度重构方案

## Context

当前前端存在结构性问题：`ui.js` 只有「元素工厂 + Toast/Modal」三层原语，页面级组件（页头、卡片、表格容器、统计卡、表单字段、浏览器工具条）全部在 6 个页面里手工重复拼装。这是所有视觉不一致的根因——改一处卡片头样式要改 12 个调用点。

具体表现：统计卡 3 套实现、kv 行 3 份完全等价代码、标签页控件 5 套并存、状态徽标 3 套词汇表（同一状态在不同页显示为「已处理/已完成/成功」）、按钮层级 4 种表达（primary/soft/ghost/link-btn 混用）、错误态 5 页丢掉页头、空态 3 种挂载方式。CSS 1575 行里约 35% 是页面专属样式，尾部追加区块直接覆盖前文（`.task-time` 双定义），另有 188 行 `.pk-*` 地图弹层 + 296 行 `map-picker.js` + 165KB Leaflet 是确认的死代码（全库零引用）。

后端数据能力被浪费：`Log.detail` 里的签到点距离/半径/是否在范围内、设备 ID、提交坐标等结构化数据，目前只以原始 JSON dump 展示。账号的 `tenantId/host/joinType/lastLoginAt/createdAt` 等字段已序列化但从未展示。

目标：建立组件抽象层，统一视觉规范，清理死代码，把已具备但被浪费的数据用起来。视觉沿用现有蓝色（`#006ee6`），用户能认出还是同一个产品，但组件更规范、对齐更精确、移动端更可用。

## 约束

- 无构建步骤（`go:embed` 直接打包 `web/dist/`），改动后需 `go build` 才生效
- 保持现有后端 API 契约不变，本次不改后端逻辑
- 保持 `device.html` 独立入口（扫码页在 App WebView 内运行，不能依赖主站 JS）
- 深色模式必须与浅色同步
- 移动端底部导航现有 5 项保持不变

## 实施步骤

### 1. 扩充组件层 `web/dist/assets/ui.js`

在现有 `h/icon/toast/openModal/closeModal/confirmDialog/emptyState` 基础上补充页面级组件。所有组件统一签名，接受对象参数：

```js
pageHeader({ title, sub, actions })        // 统一页头，actions 为按钮数组
card({ title, note, action, body, pad })  // card + card-head + card-body 三件套
statGrid(items)                            // items: [{label, value, note, tone}]
kvList(rows)                               // rows: [[key, value, mono?]]
dataTable({ columns, rows, empty })        // columns: [{key, label, align, render}]
toolbar(left, right)                       // 工具栏容器
formField({ label, input, hint })          // 表单字段
emptyState / errorState / loadingState     // 三态统一
tabs({ items, active, onSelect })          // 替代 5 套标签页实现
badge(状态)                                 // 统一状态词汇表
```

同时修正现有缺陷：`modal.dataset.onClose` 只写不读（死代码）、`skeletonCard(title=true)` 分支从未使用、`formatClock/relativeTime` 零调用。

### 2. 统一状态词汇表

合并 3 套映射为单一来源，放在 `ui.js`：

- 执行状态：`success/failed/pending/executing` → 成功/失败/待核对/执行中
- 任务状态：`signStatus 0-1/2/3/4` → 可执行/未到时间/已过期/已签到
- 登录状态：`authorized/expired/unknown` → 已登录/需重新登录/未登录

所有页面统一调用 `badge(type, value)`，禁止页面内硬编码徽标文案。

### 3. 重排 `app.css`（按组件分层，非页面名）

新结构：

```
1. 令牌（:root + [data-theme=dark]，补全硬编码色：switch 开态、暗色按钮前景、backdrop）
2. Reset
3. 布局骨架（shell / sidebar / topbar / content / 移动底栏）
4. 基础组件（按钮 4 级、输入、标签页、徽标、工具栏、卡片、表格、统计卡、kv 行）
5. 三态（loading / empty / error）
6. 浮层（modal / toast / 下拉）
7. 登录屏
8. 响应式（900 / 720 / 480 三档，表格卡片化改为按 data-label 而非 nth-child）
```

消除重复：`.task-time` 双定义合并、`.hint` 补齐 `.card-body .hint` 作用域、`.col-center`/`.sr-only`/`.dot-status`/`.tab-line`/`.form-advanced`/`.device-scan-row` 等零引用规则删除。

### 4. 重写六个页面（保留全部功能逻辑）

| 页面 | 主要改动 |
|---|---|
| `page-dashboard.js` | 统计卡改用 `statGrid`；账号/最近执行改用 `dataTable`；右侧栏三个卡片统一用 `card`；去掉内联样式 |
| `page-accounts.js` | 列表行统一卡片式；详情弹窗改用 `kvList` 分组（账号/设备/位置/登录）；表单分区保留但改用 `formField` |
| `page-tasks.js` | 任务用卡片（信息密度低，卡片更清晰）；计划用卡片；筛选/分段控件改用 `tabs` |
| `page-logs.js` | 桌面用 `dataTable`，移动端整行卡片化；**详情弹窗把 `detail` 结构化展示**（签到点距离/半径/是否在范围、设备 ID、提交坐标），不再 raw JSON dump |
| `page-quota.js` | 统计卡改用 `statGrid`；规则列表改用统一样式 |
| `page-security.js` | 五个卡片统一 `card` 组件；表单字段统一 `formField` |

功能逻辑（API 调用、状态管理、扫码轮询、表单提交）不变，只重写渲染层。

### 5. 清理死代码

- 删除 `web/dist/assets/map-picker.js`（296 行，零引用）
- 删除 `web/dist/vendor/leaflet.js` + `leaflet.css`（165KB）
- 删除 `app.css` 里 `.pk-*` 区块（188 行）
- 删除 `ui.js` 里未使用的图标（27 个只用 9 个，可保留但标注）
- 删除各页面未使用的 import（`page-tasks.js` 的 `statusBadge/loginBadge/statusText`、`page-quota.js` 的 `icon`）
- 更新 `web/web.go` 的 `moduleNames` 去掉 map-picker

### 6. 修复已知缺陷

- `index.html` 里硬编码的 `?v=39085a1b89e3`（从线上抓回的残留）删除，改由 `web.go` 的 `serveHTML` 动态注入
- `GET /api/logs` 的 `scope=today` 用 `Truncate(24h)` 按 UTC 截断，与 `TodayStats` 的本地日期逻辑不一致——本次不改后端，但前端「今天」筛选结果可能与工作台统计不符，在 UI 上标注为「近 24 小时」

## 关键文件

- `web/dist/index.html` — 骨架微调（去掉硬编码版本号）
- `web/dist/assets/ui.js` — 扩充至 ~450 行，组件层核心
- `web/dist/assets/app.css` — 重排为 8 个分层区块
- `web/dist/assets/page-*.js` — 六页全部重写渲染层（共约 1600 行 → 预计 1300 行，去掉重复）
- `web/dist/assets/app.js` — 微调（路由不变）
- `web/web.go` — 更新 `moduleNames`
- `web/dist/device.html` — 令牌对齐主站（保留独立样式，但色值统一）

## 验证

1. **静态检查**：`node scripts/check-frontend.mjs` — 语法、未定义函数、前后端字段匹配
2. **后端测试**：`go vet ./... && go test ./...`（6 个包）
3. **本地起服务**：`go build -o bin/qingjin.exe ./cmd/qingjin && ./bin/qingjin.exe -addr :18080 -data ./data_test`
4. **浏览器遍历**：逐页访问 dashboard/accounts/tasks/logs/quota/security，确认无渲染错误、无 `null` 文本、无控制台报错
5. **交互回归**：
   - 账号：新增 → 列表出现 → 详情 → 编辑（改学号，确认列表更新）→ 删除（确认消失）
   - 任务：拉取 → 状态徽标正确（未到时间禁用按钮）→ 执行确认框
   - 记录：筛选切换 → 分页 → 详情弹窗结构化展示
   - 主题：浅色/深色切换，刷新后保持
   - 响应式：1280 / 768 / 390 三档宽度检查布局
6. **对比检查**：与重构前截图对比，确认视觉一致性和改进点

## 风险

- 六页全重写范围大，但每页功能逻辑独立且已有测试覆盖后端，风险主要在渲染层
- 无构建步骤意味着错误只能在浏览器运行时发现，因此每步都运行 `check-frontend.mjs`
- 建议分两次提交：先组件层+CSS+两页（dashboard/accounts）验证通过，再完成剩余四页
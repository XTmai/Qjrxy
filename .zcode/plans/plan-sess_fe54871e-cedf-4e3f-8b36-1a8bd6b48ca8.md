# Qjrxy 前端魔改：以 openilink-hub 前端为底座（路线 A）

## Context

用户确定走开源路线（仓库名 Qjrxy），喜欢 openilink-hub（MIT，1605 星）的界面风格。侦查确认其设计来源为 **shadcn/ui（radix-nova 风格）+ Radix UI + Tailwind v4 oklch 令牌 + Geist 字体 + lucide 图标 + 翠绿主色**。

关键结论：openilink 前端与我们暂停的 React 脚手架（`web/frontend/`，node_modules 已装）**架构完全同构**（Go + Vite + React 19 + TS + Tailwind v4 + react-router v7），但它的 `web/src` 是一套**现成可跑的 shadcn 骨架**（27 个 ui 组件含 sidebar 全家桶与 confirm-dialog、绿色主题令牌、侧边栏布局壳、登录卡片、TanStack Query + query-keys 分层、zustand stores）——以它为底座移植，比继续用 Appica 组件层省掉整个设计系统搭建。

后端契约已验证：39 条路由、`system/info` 含 `quotaEnabled`（额度开关已实现，默认关）、`devicePoll` 直接返回完整 account、`RunOutcome {logId,status,message,hint,taskName}`。

**可复用资产**（与框架无关，直接保留）：
- `web/frontend/src/lib/task.ts` — taskState / parseTaskWindow / 时间格式化（核心业务逻辑）
- `web/frontend/src/lib/types.ts` — 与 Go store 层一一对应的 15 个类型
- `web/frontend/src/api/client.ts` — Bearer token 客户端（openilink 用 cookie session，保留我们的 token 方案，改动面小）
- `web/frontend/src/api/index.ts` — 34 条端点方法
- `web/frontend/public/{device.html,favicon.svg}` — 原样保留

## 目录与依赖

```
web/frontend/
├── package.json        # 增: radix-ui/shadcn/tw-animate-css/@fontsource-variable/geist/lucide-react/
│                       #   @tanstack/react-query/zustand/class-variance-authority/clsx/tailwind-merge/qrcode
│                       # 删: @appica/ui-react
├── components.json     # shadcn 配置（radix-nova，从 openilink 复制）
├── vite.config.ts      # 标准 vite（不用 vite-plus）；别名 @→src；outDir ../dist；proxy /api→:8090
├── tsconfig.json       # 加 paths @/* → src/*
├── public/             # device.html + favicon.svg（已存在）
└── src/
    ├── index.css           # openilink 的 oklch 令牌照抄（绿色主题）+ Geist
    ├── components/
    │   ├── ui/             # ← openilink 的 27 个 shadcn 组件整体移植（含 sidebar/confirm-dialog）
    │   ├── layout.tsx      # ← 其布局壳改造：3 个导航项（首页/账号/设置）、Qjrxy 文字 logo、
    │   │                   #   用户菜单（退出登录）、主题切换、认证门禁改用我们的 token api
    │   ├── captcha-dialog.tsx / device-scan-dialog.tsx / plans.tsx / account-detail.tsx
    │   │                   # ← 已有逻辑用 shadcn Dialog/Tabs 重写
    │   └── login-form.tsx  # ← 其登录卡片模式，去微信扫码/Passkey/OAuth，保留账号密码+注册
    ├── hooks/
    │   ├── use-auth.ts     # ← 其模式（TanStack Query useUser），底层用我们的 token client
    │   ├── use-accounts.ts / use-tasks.ts / use-plans.ts / use-dashboard.ts  # 按其 hook 模式新写
    │   └── use-mobile.ts   # ← 移植
    ├── lib/
    │   ├── utils.ts (cn) / theme.tsx / query-client.ts / query-keys.ts  # ← 移植+改造
    │   ├── task.ts / types.ts  # 保留（TS 化业务逻辑）
    │   └── api/index.ts    # 保留（改造成 query hooks 的底层）
    ├── stores/             # ← auth-store（适配 token 登出+queryClient.clear）/ ui-store
    ├── pages/              # login / home(参考 dashboard-overview 模式) / accounts / settings(URL-tab 模式)
    └── main.tsx            # StrictMode > BrowserRouter > QueryClientProvider > ThemeProvider
                            #   > TooltipProvider > Toaster > Routes
```

## 实施步骤

### 阶段 0：底座移植（先打通构建）
1. `git clone --depth 1 openilink/openilink-hub` 到临时目录，复制其 `web/src/components/ui/`、`web/src/lib/{utils,theme,query-client}.tsx|.ts`、`web/src/stores/`、`web/src/components/{layout.tsx,login-form.tsx}`、`web/index.css`、`web/components.json` 进 `web/frontend/src/`（对应位置）
2. `package.json` 依赖合并（去 Appica、去 vite-plus 别名用标准 vite、去 posthog）；`npm install`
3. `vite.config.ts` 换标准 defineConfig + `@` 别名；`tsconfig.json` 加 paths；`index.css` 用其令牌 + Geist + 绿色主色
4. **`web/web.go` 重写为 Vite 适配版**（回退前写过一版：删 moduleNames/serveModule/serveHTML，hash 资源 immutable，index/device no-cache，/device 回退）——与上一次实现相同
5. 验证：占位页 `npm run build` + `go build` 通过

### 阶段 1：外壳与认证
6. `main.tsx` Provider 链 + 路由（`/login` 公开；`/`、`/accounts`、`/settings` 挂 Layout 门禁）
7. `layout.tsx` 适配：导航改 3 项、logo 改 Qjrxy 文字块、auth 门禁与用户菜单接我们的 token 登出（`api.logout` + `queryClient.clear()`）
8. `login-form.tsx` 精简为账号密码 + 注册切换（去微信/Passkey/OAuth/六边形背景保留与否视效果）
9. 认证适配：`request()` 401 由硬跳转改为「清 token + 跳 /login」；zustand auth-store 接 Bearer 登录

### 阶段 2：三页业务
10. **首页**（dashboard-overview 模式）：今日统计卡网格（执行/成功/失败/待核对）→ 每账号查寝状态行（`taskState` 徽章 + 窗口时间，可执行高亮）→ 最近动态
11. **账号页**：账号卡片（状态徽章 + 探测行 + 待扫码警告）+ 单卡片创建（学校 Combobox 自动匹配 + 学号 + 密码 → **保存即登录**[含验证码弹窗] → 自动 sign-range）+ 「检测登录态」批量探测
12. **账号详情 Dialog**（Tabs：任务/计划/记录/设备）：任务同步与执行（RunOutcome toast）、计划 CRUD + 启停 + 重置今日、记录列表 + 详情（坐标/在范围/设备 ID）、设备扫码 + 照片上传 + 自动获取位置 + 删除确认（confirm-dialog）
13. **设置页**（URL-tab 模式）：外观 / 安全（改密）/ 额度（`quotaEnabled` 门禁显隐）/ 关于+免责声明
14. 数据层：按其模式写 `use-dashboard/use-accounts/use-tasks/use-plans/use-info` hooks（query + mutation + invalidate query-keys）

### 阶段 3：收尾
15. 视觉走查修正（深浅色、移动端 390px、空态/加载骨架）
16. 清理：删 Appica 旧文件（AppShell/ui-bits/Toaster 等）、`README.md` 补界面说明（含 openilink 致谢、MIT 归属声明——复制了其 ui/ 组件）
17. 全量验证 + 构建交付

## 关键决策（已定）
- **TanStack Query 采纳**：基于 openilink 代码的 hook 模式天然依赖它，且提升加载/缓存体验；我们的 `api/index.ts` 作为其底层 fetch
- **认证保持 Bearer token**（localStorage）而非 openilink 的 cookie session——后端已按此实现，改动面最小
- **Geist 字体 + 翠绿主色照抄**；暗色默认跟随系统
- `vite-plus` 别名不引入，用标准 vite 7

## 验证
1. 每阶段：`npm --prefix web/frontend run build` + `go build` + `go test ./...` 全绿
2. 浏览器端到端（本地 :18080）：注册登录 → 首页状态板 → 创建账号（学校匹配/验证码路径）→ 任务执行（未到时间禁用）→ 计划 CRUD + 重置今日 → 记录详情 → 设置改密/主题 → 深浅色 + 390px 移动端
3. 额度开关：默认关 → 设置页无额度卡、执行不扣；`QINGJIN_QUOTA_ENABLED=true` → 显示余额与兑换
4. device.html：`/device.html` 可访问
5. 无 node 构建：`go build` 直接可用（dist 随仓库提交）
6. 用户部署：新二进制 + systemd 加 `Environment=QINGJIN_QUOTA_ENABLED=true`

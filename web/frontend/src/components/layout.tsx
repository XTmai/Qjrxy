// 应用布局壳：左侧固定侧栏（可收起为纯图标窄栏）+ 顶部状态条 + 主内容面板。
//
// 视觉语言：「灰画布 + 白卡浮层」。侧栏直接坐在画布上（同色、无边框），
// 内容区是浮在画布上的白色面板；颜色只用来表示状态，不做装饰。
import { Outlet, useNavigate, Link, useLocation } from "react-router-dom";
import { useEffect, useMemo, useState } from "react";
import { useUser, useInfo } from "@/hooks/use-auth";
import { useAccounts } from "@/hooks/use-accounts";
import { usePlans } from "@/hooks/use-plans";
import { useLiveUpdates } from "@/hooks/use-live-updates";
import { CaptchaChallengeDialog } from "@/components/captcha-challenge-dialog";
import { LogoGlyph } from "@/components/logo-mark";
import { useAuthStore } from "@/stores/auth-store";
import { useLiveStore } from "@/stores/live-store";
import {
  CalendarClock,
  ChevronDown,
  ChevronsUpDown,
  Github,
  House,
  LogOut,
  MessagesSquare,
  Moon,
  Plus,
  RefreshCw,
  ScrollText,
  Search,
  Settings2,
  ShieldCheck,
  Sun,
  Users,
} from "lucide-react";
import { api } from "@/api";
import { useTheme } from "@/lib/theme";
import { QQ_GROUP_URL } from "@/lib/links";
import { cn } from "@/lib/utils";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { Account } from "@/lib/types";

const NAV = [
  { key: "dashboard", to: "/", label: "监控总览", icon: House },
  { key: "accounts", to: "/accounts", label: "账号管理", icon: Users },
  { key: "audit", to: "/audit", label: "执行审计", icon: ScrollText },
  { key: "schedules", to: "/schedules", label: "调度计划", icon: CalendarClock },
];

/** 账号健康度排序：需重登在最前，缺配置其次，就绪最后 */
function healthRank(a: Account) {
  if (a.loginStatus !== "authorized") return 0;
  if (!a.deviceId || !a.lon || !a.lat) return 1;
  return 2;
}

function SidebarLogo() {
  const { open } = useSidebar();
  // 收起时 logo 是唯一可见的品牌元素，用主色块承载；
  // 展开时与文字并排，直接用前景色，跟随亮/暗主题。
  // 22px 下用单字版本：四字网格会糊成一团，见 logo-mark.tsx 的注释。
  if (open) {
    return (
      <div className="flex items-center gap-2.5">
        <span className="bg-primary text-primary-foreground flex size-9 shrink-0 items-center justify-center rounded-xl">
          <LogoGlyph className="size-[21px]" />
        </span>
        <div className="leading-tight">
          <p className="tracking-headline text-[15px] font-bold">Qjrxy</p>
          <p className="text-muted-foreground text-[11px] font-medium">查寝自动化</p>
        </div>
      </div>
    );
  }
  return (
    <span className="bg-primary text-primary-foreground mx-auto flex size-9 items-center justify-center rounded-xl">
      <LogoGlyph className="size-[21px]" />
    </span>
  );
}

/**
 * 侧栏里的学号快捷入口。
 *
 * 与账号页同源（useAccounts），异常账号置顶并在右侧挂短角标；
 * 学号很长，所以只给一个快搜框 + 限高滚动 + 底部渐隐，而不是全量铺开。
 * 收起入口放在标题行的 chevron 上（原型那套），列表里不再重复一个「收起」按钮。
 */
function AccountSublist({ collapsed }: { collapsed: boolean }) {
  const { data: accounts = [] } = useAccounts();
  const { open } = useSidebar();
  const [keyword, setKeyword] = useState("");
  const location = useLocation();

  const sorted = useMemo(
    () => [...accounts].sort((a, b) => healthRank(a) - healthRank(b)),
    [accounts],
  );
  const activeId = Number(new URLSearchParams(location.search).get("account") || 0);

  const list = keyword.trim()
    ? sorted.filter((a) => {
        const q = keyword.trim().toLowerCase();
        return (
          a.username.toLowerCase().includes(q) ||
          (a.alias ?? "").toLowerCase().includes(q) ||
          (a.deviceModel ?? "").toLowerCase().includes(q)
        );
      })
    : sorted;

  if (!open) return null;

  return (
    <div className={cn("mt-1 flex min-h-0 flex-col pl-3", collapsed && "hidden")}>
      <div className="relative mb-1 pr-1">
        <input
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          type="text"
          placeholder="快搜学号..."
          aria-label="快搜学号"
          className="focus:border-border-strong w-full rounded-lg border border-border bg-white/70 py-1 pr-2 pl-6 font-mono text-[11px] text-foreground placeholder:text-gray-400 focus:outline-none dark:bg-[color:var(--subtle)]/80"
        />
        <Search className="pointer-events-none absolute top-1/2 left-2 size-3 -translate-y-1/2 text-gray-400" />
      </div>

      <div className="sublist-fade-mask thin-scrollbar max-h-[210px] space-y-0.5 overflow-x-hidden overflow-y-auto pb-2">
        {list.length === 0 && (
          <div className="px-2.5 py-2 text-[11px] text-gray-400">
            {accounts.length === 0 ? "还没有绑定账号" : "未找到匹配学号"}
          </div>
        )}
        {list.map((a) => {
          const rank = healthRank(a);
          const dot = rank === 0 ? "bg-danger" : rank === 1 ? "bg-warning" : "bg-success";
          const badge = rank === 0 ? "重登" : rank === 1 ? "待补" : "";
          return (
            <Link
              key={a.id}
              to={`/accounts?account=${a.id}`}
              title={`${a.username} · ${a.schoolName}`}
              className={cn(
                "flex items-center justify-between gap-2 rounded-lg px-2.5 py-1.5 font-mono text-xs text-gray-600 transition-colors hover:bg-gray-200/60 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-800/70 dark:hover:text-white",
                activeId === a.id &&
                  "bg-white font-semibold text-foreground shadow-xs dark:bg-[color:var(--subtle)]",
              )}
            >
              <span className="flex min-w-0 items-center gap-2">
                <span className={cn("size-1.5 shrink-0 rounded-full", dot)} />
                <span className="truncate">{a.alias || a.username}</span>
              </span>
              {badge && (
                <span
                  className={cn(
                    "shrink-0 rounded px-1.5 text-[9px] font-medium",
                    rank === 0
                      ? "bg-danger-soft text-danger-text"
                      : "bg-warning-soft text-warning-text",
                  )}
                >
                  {badge}
                </span>
              )}
            </Link>
          );
        })}
      </div>
    </div>
  );
}

function TopBar() {
  const { resolvedTheme, setTheme } = useTheme();
  const navigate = useNavigate();
  const [version, setVersion] = useState("");
  const [syncing, setSyncing] = useState(false);
  const liveStatus = useLiveStore((s) => s.status);

  useEffect(() => {
    api
      .systemInfo()
      .then((d) => setVersion(d.appVersion || ""))
      .catch(() => {});
  }, []);

  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b border-[color:var(--border)] bg-background/80 px-4 backdrop-blur-md sm:px-6">
      <div className="flex min-w-0 items-center gap-3">
        <SidebarTrigger className="size-8" />

        <div className="flex items-center gap-2">
          <span className="inline-flex items-center gap-1.5 rounded-full border border-success-border/60 bg-success-soft px-2.5 py-1 text-[11px] font-medium whitespace-nowrap text-success-text">
            <span
              className={cn(
                "size-1.5 rounded-full bg-success",
                liveStatus === "live" && "pulse-dot",
                liveStatus === "connecting" && "animate-pulse",
              )}
            />
            <span className="hidden sm:inline">
              {liveStatus === "offline" ? "轮询兜底中" : "调度器活跃中"}
            </span>
            <span className="sm:hidden">{liveStatus === "offline" ? "轮询" : "活跃"}</span>
          </span>
          <span className="hidden items-center gap-1.5 rounded-lg bg-[color:var(--subtle)] px-2.5 py-1 font-mono text-[11px] whitespace-nowrap text-gray-600 sm:inline-flex dark:text-gray-300">
            <strong className="font-semibold text-gray-800 dark:text-gray-100">SQLite</strong>
            本地存储 · 单二进制
          </span>
        </div>
      </div>

      <TooltipProvider>
        <div className="flex items-center gap-1.5">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 px-3 text-xs"
                disabled={syncing}
                onClick={() => {
                  setSyncing(true);
                  navigate("/accounts");
                  setTimeout(() => setSyncing(false), 800);
                }}
              >
                <RefreshCw className={cn("size-3.5", syncing && "animate-spin")} />
                <span className="hidden sm:inline">同步</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>前往账号页同步任务</TooltipContent>
          </Tooltip>

          <Button
            size="sm"
            className="h-8 gap-1.5 px-3 text-xs"
            onClick={() => navigate("/accounts?create=1")}
          >
            <Plus className="size-3.5" />
            <span className="hidden sm:inline">添加账号</span>
          </Button>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-8"
                onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
              >
                {resolvedTheme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
              </Button>
            </TooltipTrigger>
            <TooltipContent>切换外观</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="size-8" asChild>
                <a href="https://github.com/XTmai/Qjrxy" target="_blank" rel="noopener noreferrer">
                  <Github className="size-4" />
                </a>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{version ? `v${version}` : "项目主页"}</TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>
    </header>
  );
}

/**
 * 侧栏底部的守护进程卡。
 *
 * 把「服务端变了页面会自己更新」做成可见状态：用户不用再怀疑数据是不是旧的，
 * 排障时也能一眼看出推送是否断开。收起成窄栏时退化为一个心跳圆点。
 */
function DaemonCard() {
  const { open } = useSidebar();
  const status = useLiveStore((s) => s.status);
  const meta = {
    live: { dot: "bg-success pulse-dot", label: "实时同步", hint: "数据变化会自动更新" },
    connecting: { dot: "bg-warning animate-pulse", label: "连接中", hint: "正在建立推送通道" },
    offline: { dot: "bg-muted-foreground/50", label: "轮询兜底", hint: "推送已断开，每 30 秒自动刷新" },
  }[status];

  if (!open) {
    // 窄栏心跳点：必须和下方头像按钮同宽同位置（36px 方块，左对齐到 x=4），
    // 否则圆点会落在整栏正中、头像却在左边，看起来「歪了」。
    return (
      <div
        className="flex h-9 w-9 items-center justify-center"
        title={`TASK DAEMON · ${meta.label}`}
      >
        <span className={cn("size-2.5 rounded-full", meta.dot)} />
      </div>
    );
  }

  return (
    <div className="mx-1 mb-1 rounded-xl border border-[color:var(--border)] bg-background p-3 shadow-xs">
      <div className="mb-1.5 flex items-center justify-between">
        <span className="tracking-label text-[10px] font-bold whitespace-nowrap text-gray-500 uppercase dark:text-gray-400">
          Task Daemon
        </span>
        <span className={cn("size-2 shrink-0 rounded-full", meta.dot)} title={meta.hint} />
      </div>
      <div className="mb-2 font-mono text-[11px] whitespace-nowrap text-gray-600 dark:text-gray-300">
        扫描 30s · 探测 1h
      </div>
      <div className="flex items-center justify-between border-t border-[color:var(--border)] pt-2 text-[11px]">
        <span className="text-gray-500 dark:text-gray-400">推送</span>
        <span className="font-medium text-gray-800 dark:text-gray-200" title={meta.hint}>
          {meta.label}
        </span>
      </div>
    </div>
  );
}

// 页脚：版本号 + 反馈群入口。用户排障时第一句往往是「你用的哪个版本」，
// 把版本号放在常驻位置，比藏在关于页里省一次来回。
function VersionFooter() {
  const { open } = useSidebar();
  const { data: info } = useInfo();
  if (!open) return null;
  return (
    <div className="flex items-center justify-between px-2 pb-1 font-mono text-[11px] text-gray-500 dark:text-gray-400">
      <span className="tabular-nums">{info?.appVersion ? `v${info.appVersion}` : "v—"}</span>
      <a
        href={QQ_GROUP_URL}
        target="_blank"
        rel="noopener noreferrer"
        className="hover:text-foreground flex items-center gap-1 transition-colors"
      >
        <MessagesSquare className="size-3.5" />
        反馈群
      </a>
    </div>
  );
}

export function Layout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data: user, isError } = useUser();
  const { data: info } = useInfo();
  const { data: accounts = [] } = useAccounts();
  const { data: plans = [] } = usePlans();
  const enabledPlanCount = plans.filter((p) => p.enabled).length;
  // 学号子列表折叠状态归壳层持有：折叠入口在「账号管理」行上
  const [sublistCollapsed, setSublistCollapsed] = useState(false);
  // 订阅服务端变更：任务执行、登录态探测、计划变更等都会让页面自动更新。
  useLiveUpdates();
  // 单用户模式没有账号体系：不显示退出/账号设置
  const single = info !== undefined && !info.multiUser;
  // 管理员：单用户本机自用，或多用户下的 admin 角色
  const isAdmin = single || user?.role === "admin";

  useEffect(() => {
    if (isError) navigate("/login", { replace: true });
  }, [isError, navigate]);

  if (!user) return null;

  const active = (to: string) =>
    to === "/" ? location.pathname === "/" : location.pathname === to || location.pathname.startsWith(to + "/");

  return (
    <SidebarProvider>
      <Sidebar variant="sidebar" collapsible="icon">
        {/* Header / Content / Footer 用同一套水平内边距：
            展开时 px-2，折叠成 68px 图标栏时 px-1，
            这样 Logo、菜单图标、底部头像的中心线一致。 */}
        <SidebarHeader className="h-16 justify-center px-2 pt-1 group-data-[collapsible=icon]:px-1">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" asChild className="h-12">
                <Link to="/" aria-label="返回监控总览">
                  <SidebarLogo />
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>

        <SidebarContent className="px-2 pt-2 group-data-[collapsible=icon]:px-1">
          <div className="tracking-label px-2 pb-1.5 text-[10px] font-semibold text-gray-400 uppercase group-data-[collapsible=icon]:hidden dark:text-gray-500">
            Navigation
          </div>

          <SidebarMenu className="gap-1">
            {NAV.map((item) => {
              const isAccounts = item.key === "accounts";
              return (
                <SidebarMenuItem key={item.key}>
                  {isAccounts ? (
                    // 账号管理行：整行点进账号页；右侧「数字徽标 + chevron」是
                    // 并排的一段，点它只折叠子列表。不做绝对定位叠放，
                    // 否则窄栏下徽标会被箭头压住。
                    <div className="relative">
                      <SidebarMenuButton
                        asChild
                        isActive={active(item.to)}
                        className="pr-9 data-[active=true]:border-[color:var(--border)] data-[active=true]:shadow-xs data-[active=true]:ring-1 data-[active=true]:ring-black/[0.02]"
                      >
                        <Link to={item.to}>
                          <item.icon />
                          <span className="flex-1 truncate group-data-[collapsible=icon]:hidden">{item.label}</span>
                          {accounts.length > 0 && (
                            <span className="shrink-0 rounded-md bg-gray-200/70 px-1.5 font-mono text-[10px] text-gray-600 group-data-[collapsible=icon]:hidden dark:bg-gray-800 dark:text-gray-300">
                              {accounts.length}
                            </span>
                          )}
                        </Link>
                      </SidebarMenuButton>
                      {accounts.length > 0 && (
                        <button
                          type="button"
                          onClick={(e) => {
                            e.preventDefault();
                            e.stopPropagation();
                            setSublistCollapsed((v) => !v);
                          }}
                          title={sublistCollapsed ? "展开学号列表" : "折叠学号列表"}
                          aria-label={sublistCollapsed ? "展开学号列表" : "折叠学号列表"}
                          aria-expanded={!sublistCollapsed}
                          className="text-muted-foreground hover:text-foreground absolute top-1/2 right-1.5 flex size-5 -translate-y-1/2 cursor-pointer items-center justify-center rounded transition-colors group-data-[collapsible=icon]:hidden"
                        >
                          <ChevronDown
                            className={cn(
                              "size-3.5 transition-transform",
                              sublistCollapsed && "-rotate-90",
                            )}
                          />
                        </button>
                      )}
                    </div>
                  ) : (
                    <SidebarMenuButton
                      asChild
                      isActive={active(item.to)}
                      tooltip={item.label}
                      className="data-[active=true]:border-[color:var(--border)] data-[active=true]:shadow-xs data-[active=true]:ring-1 data-[active=true]:ring-black/[0.02]"
                    >
                      <Link to={item.to}>
                        <item.icon />
                        <span className="flex-1 truncate group-data-[collapsible=icon]:hidden">{item.label}</span>
                        {item.key === "schedules" && (
                          <span className="shrink-0 rounded-md bg-success/10 px-1.5 font-mono text-[10px] text-success-text group-data-[collapsible=icon]:hidden">
                            {enabledPlanCount}
                          </span>
                        )}
                      </Link>
                    </SidebarMenuButton>
                  )}

                  {isAccounts && (
                    <AccountSublist collapsed={sublistCollapsed} />
                  )}
                </SidebarMenuItem>
              );
            })}
          </SidebarMenu>

          <div className="my-2 border-t border-[color:var(--border)]" />

          <SidebarMenu className="gap-1">
            <SidebarMenuItem>
              <SidebarMenuButton
                asChild
                isActive={active("/settings") && !active("/settings/admin")}
                tooltip="系统设置"
                className="data-[active=true]:border-[color:var(--border)] data-[active=true]:shadow-xs data-[active=true]:ring-1 data-[active=true]:ring-black/[0.02]"
              >
                <Link to="/settings">
                  <Settings2 />
                  <span className="group-data-[collapsible=icon]:hidden">系统设置</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
            {/* 管理员设置：仅管理员（role=admin 或单用户模式）可见 */}
            {isAdmin && (
              <SidebarMenuItem>
                <SidebarMenuButton
                  asChild
                  isActive={active("/settings/admin")}
                  tooltip="管理员设置"
                  className="data-[active=true]:border-[color:var(--border)] data-[active=true]:shadow-xs data-[active=true]:ring-1 data-[active=true]:ring-black/[0.02]"
                >
                  <Link to="/settings/admin">
                    <ShieldCheck />
                    <span className="group-data-[collapsible=icon]:hidden">管理员设置</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )}
          </SidebarMenu>
        </SidebarContent>

        <SidebarFooter className="gap-2 border-t border-[color:var(--border)] p-2 pt-2.5 group-data-[collapsible=icon]:items-center group-data-[collapsible=icon]:px-1">
          <DaemonCard />
          <VersionFooter />
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <SidebarMenuButton
                    size="lg"
                    className="hover:bg-gray-200/50 data-[state=open]:bg-gray-200/50 dark:hover:bg-gray-800/60"
                  >
                    <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-gray-200 text-xs font-semibold text-gray-700 dark:bg-gray-700 dark:text-gray-200">
                      {user.username.charAt(0).toUpperCase()}
                    </span>
                    <div className="grid flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
                      <span className="truncate text-xs font-semibold">
                        {user.nickname || user.username}
                      </span>
                      <span className="tracking-label truncate font-mono text-[10px] text-gray-400 uppercase">
                        {user.role}
                      </span>
                    </div>
                    <ChevronsUpDown className="ml-auto size-3.5 opacity-50 group-data-[collapsible=icon]:hidden" />
                  </SidebarMenuButton>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  className="w-(--radix-dropdown-menu-trigger-width) min-w-52 rounded-xl"
                  side="top"
                  align="end"
                  sideOffset={6}
                >
                  {!single && (
                    <>
                      <DropdownMenuItem
                        onClick={() => navigate("/settings/security")}
                        className="cursor-pointer text-xs"
                      >
                        <Settings2 className="mr-2 size-3.5" />
                        账号设置
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onClick={async () => {
                          await useAuthStore.getState().logout();
                          navigate("/login");
                        }}
                        className="focus:bg-danger-soft focus:text-danger-text text-danger cursor-pointer text-xs font-medium"
                      >
                        <LogOut className="mr-2 size-3.5" />
                        退出登录
                      </DropdownMenuItem>
                    </>
                  )}
                  {single && (
                    <DropdownMenuItem
                      onClick={() => navigate("/settings")}
                      className="cursor-pointer text-xs"
                    >
                      <Settings2 className="mr-2 size-3.5" />
                      系统设置
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>

      {/* 高度用 h-full（父级 body 已锁死 100vh）而不是 h-svh：
          h-svh 加上 lg:my-2 的上下外边距会变成 100vh+16px，
          于是文档本身也出现一条滚动条，和 main 的细滚动条并存。 */}
      <SidebarInset className="flex h-full min-h-0 flex-col bg-canvas lg:my-2 lg:mr-2 lg:rounded-2xl">
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden border-[color:var(--border)] bg-background shadow-xs lg:rounded-2xl lg:border">
          <TopBar />
          <main className="thin-scrollbar min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
            <div className="mx-auto w-full max-w-6xl p-4 sm:p-6 lg:p-8">
              <Outlet />
            </div>
          </main>
        </div>
      </SidebarInset>

      {/* 签到验证码待人工点选：挂在整个面板上，切到哪一页都能弹出来 */}
      <CaptchaChallengeDialog />
    </SidebarProvider>
  );
}

// 应用布局壳：Campus Precision Engine 风格
// 左侧固定侧边栏（可收起为图标）+ 顶部状态条 + 主内容区
import { Outlet, useNavigate, Link, useLocation } from "react-router-dom";
import { useEffect, useState } from "react";
import { useUser, useInfo } from "@/hooks/use-auth";
import { useAccounts } from "@/hooks/use-accounts";
import { useLiveUpdates } from "@/hooks/use-live-updates";
import { useAuthStore } from "@/stores/auth-store";
import { useLiveStore } from "@/stores/live-store";
import {
  Bot,
  CalendarClock,
  ChevronsUpDown,
  Github,
  House,
  LogOut,
  MessagesSquare,
  Moon,
  Plus,
  RefreshCw,
  ScrollText,
  Settings2,
  ShieldCheck,
  Sun,
} from "lucide-react";
import { api } from "@/api";
import { useTheme } from "@/lib/theme";
import { QQ_GROUP_URL } from "@/lib/links";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubItem,
  SidebarMenuSubButton,
  SidebarProvider,
  SidebarRail,
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
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

const NAV = [
  { key: "dashboard", to: "/", label: "监控总览", icon: House },
  { key: "accounts", to: "/accounts", label: "账号管理", icon: Bot },
  { key: "audit", to: "/audit", label: "执行审计", icon: ScrollText },
  { key: "schedules", to: "/schedules", label: "调度计划", icon: CalendarClock },
];

const statusColors: Record<string, string> = {
  authorized: "bg-success",
  expired: "bg-danger",
  unknown: "bg-muted-foreground/50",
};

function SidebarLogo() {
  const { open } = useSidebar();
  if (open) {
    return (
      <div className="flex items-center gap-2.5">
        <span className="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-md text-sm font-bold">
          Q
        </span>
        <div className="leading-tight">
          <p className="tracking-headline text-[13px] font-semibold">Qjrxy</p>
          <p className="text-muted-foreground tracking-label text-[10px]">查寝自动化</p>
        </div>
      </div>
    );
  }
  return (
    <span className="bg-primary text-primary-foreground mx-auto flex size-8 items-center justify-center rounded-md text-sm font-bold">
      Q
    </span>
  );
}

function TopBar() {
  const { resolvedTheme, setTheme } = useTheme();
  const navigate = useNavigate();
  const [version, setVersion] = useState("");
  const [syncing, setSyncing] = useState(false);

  useEffect(() => {
    api
      .systemInfo()
      .then((d) => setVersion(d.appVersion || ""))
      .catch(() => {});
  }, []);

  return (
    <header className="border-border/70 flex h-14 shrink-0 items-center justify-between gap-3 border-b px-4 lg:px-6">
      <div className="flex min-w-0 items-center gap-3">
        <SidebarTrigger className="-ml-1.5 size-8" />
        <Separator orientation="vertical" className="h-4 opacity-60" />

        <span className="hidden items-center gap-1.5 sm:flex">
          <span className="bg-success pulse-dot size-1.5 rounded-full" />
          <span className="text-muted-foreground text-[11px]">调度器活跃中</span>
        </span>

        <Separator orientation="vertical" className="hidden h-4 opacity-50 md:block" />

        <span className="text-muted-foreground hidden items-center gap-1.5 text-[11px] md:flex">
          <span className="bg-muted text-muted-foreground rounded px-1.5 py-0.5 font-mono text-[10px]">SQLite</span>
          本地存储 · 单二进制
        </span>
      </div>

      <TooltipProvider>
        <div className="flex items-center gap-1.5">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-8 gap-1.5 px-2.5 text-xs"
                disabled={syncing}
                onClick={() => {
                  setSyncing(true);
                  navigate("/accounts");
                  setTimeout(() => setSyncing(false), 800);
                }}
              >
                <RefreshCw className={`size-3.5 ${syncing ? "animate-spin" : ""}`} />
                <span className="hidden sm:inline">同步</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>前往账号页同步任务</TooltipContent>
          </Tooltip>

          <Button size="sm" className="h-8 gap-1.5 px-2.5 text-xs" onClick={() => navigate("/accounts?create=1")}>
            <Plus className="size-3.5" />
            <span className="hidden sm:inline">添加账号</span>
          </Button>

          <Separator orientation="vertical" className="mx-0.5 h-4 opacity-50" />

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

function DaemonCard() {
  const { open } = useSidebar();
  const status = useLiveStore((s) => s.status);
  if (!open) return null;
  // 把「服务端变了页面会自己更新」做成可见状态：
  // 用户不用再怀疑数据是不是旧的，排障时也能一眼看出推送是否断开。
  const meta = {
    live: { dot: "bg-success pulse-dot", label: "实时同步", hint: "数据变化会自动更新" },
    connecting: { dot: "bg-warning animate-pulse", label: "连接中", hint: "正在建立推送通道" },
    offline: { dot: "bg-muted-foreground/50", label: "轮询兜底", hint: "推送已断开，每 30 秒自动刷新" },
  }[status];
  return (
    <div className="border-border/70 bg-background shadow-l1 mx-2 mb-1 rounded-lg border p-2.5">
      <div className="flex items-center justify-between">
        <span className="text-muted-foreground tracking-label text-[10px] font-medium uppercase">Task Daemon</span>
        <span className={`size-1.5 rounded-full ${meta.dot}`} title={meta.hint} />
      </div>
      <p className="text-muted-foreground mt-1 font-mono text-[10px]">扫描 30s · 探测 1h</p>
      <div className="border-border/60 mt-1.5 flex items-center justify-between border-t pt-1.5">
        <span className="text-muted-foreground text-[10px]">推送</span>
        <span className="text-foreground text-[10px] font-medium" title={meta.hint}>
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
    <div className="text-muted-foreground flex items-center justify-between px-3 pb-1 text-[10px]">
      <span className="font-mono tabular-nums">
        {info?.appVersion ? `v${info.appVersion}` : "v—"}
      </span>
      <a
        href={QQ_GROUP_URL}
        target="_blank"
        rel="noopener noreferrer"
        className="hover:text-foreground flex items-center gap-1 transition-colors"
      >
        <MessagesSquare className="size-3" />
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
      <Sidebar variant="sidebar" collapsible="icon" className="border-r-0">
        <SidebarHeader className="h-16 justify-center px-2">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" asChild>
                <Link to="/">
                  <SidebarLogo />
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>

        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel className="tracking-label text-[10px] font-medium uppercase">
              Navigation
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {NAV.map((item) => (
                  <SidebarMenuItem key={item.key}>
                    <SidebarMenuButton asChild isActive={active(item.to)} tooltip={item.label}>
                      <Link to={item.to}>
                        <item.icon />
                        <span>{item.label}</span>
                      </Link>
                    </SidebarMenuButton>

                    {item.key === "accounts" && accounts.length > 0 && (
                      <SidebarMenuSub>
                        {accounts.map((a) => (
                          <SidebarMenuSubItem key={a.id}>
                            <SidebarMenuSubButton asChild size="sm">
                              <Link to={`/accounts?account=${a.id}`}>
                                <span
                                  className={`size-1.5 shrink-0 rounded-full ${statusColors[a.loginStatus] || statusColors.unknown}`}
                                />
                                <span className="truncate">{a.alias || a.username}</span>
                              </Link>
                            </SidebarMenuSubButton>
                          </SidebarMenuSubItem>
                        ))}
                      </SidebarMenuSub>
                    )}
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>

          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={active("/settings") && !active("/settings/admin")}
                    tooltip="系统设置"
                  >
                    <Link to="/settings">
                      <Settings2 />
                      <span>系统设置</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                {/* 管理员设置：仅管理员（role=admin 或单用户模式）可见 */}
                {isAdmin && (
                  <SidebarMenuItem>
                    <SidebarMenuButton asChild isActive={active("/settings/admin")} tooltip="管理员设置">
                      <Link to="/settings/admin">
                        <ShieldCheck />
                        <span>管理员设置</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter>
          <DaemonCard />
          <VersionFooter />
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <SidebarMenuButton
                    size="lg"
                    className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
                  >
                    <Avatar className="size-7 rounded-md border">
                      <AvatarFallback className="bg-primary/10 text-primary rounded-md text-[11px] font-bold">
                        {user.username.charAt(0).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <div className="ml-1 grid flex-1 text-left leading-tight">
                      <span className="truncate text-xs font-semibold">{user.nickname || user.username}</span>
                      <span className="text-muted-foreground tracking-label truncate text-[10px] uppercase">
                        {user.role}
                      </span>
                    </div>
                    <ChevronsUpDown className="ml-auto size-3.5 opacity-50" />
                  </SidebarMenuButton>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  className="w-(--radix-dropdown-menu-trigger-width) min-w-52 rounded-md"
                  side="top"
                  align="end"
                  sideOffset={6}
                >
                  {!single && (
                    <>
                      <DropdownMenuItem onClick={() => navigate("/settings/security")} className="cursor-pointer text-xs">
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
                    <DropdownMenuItem onClick={() => navigate("/settings")} className="cursor-pointer text-xs">
                      <Settings2 className="mr-2 size-3.5" />
                      系统设置
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

      <SidebarInset className="bg-canvas flex flex-col lg:p-2 lg:pl-0">
        <div className="bg-background border-border shadow-sheet flex min-h-0 flex-1 flex-col overflow-hidden border-0 lg:rounded-tl-2xl lg:border">
          <TopBar />
          <main className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
            <div className="mx-auto w-full max-w-[1400px] p-5 lg:p-7">
              <Outlet />
            </div>
          </main>
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}

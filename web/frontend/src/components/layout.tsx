// 应用布局壳：基于 openilink-hub 的 sidebar 结构改造（MIT）。
// 导航收敛为 首页 / 账号 / 设置；账号子项带登录状态圆点。
import { Outlet, useNavigate, Link, useLocation } from "react-router-dom";
import { useEffect, useState } from "react";
import { useUser } from "@/hooks/use-auth";
import { useAccounts } from "@/hooks/use-accounts";
import { useAuthStore } from "@/stores/auth-store";
import { LogOut, Github, Bot, House, ChevronsUpDown, Settings2, Circle, Sun, Moon } from "lucide-react";
import { api } from "@/api";
import { useTheme } from "@/lib/theme";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
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
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Separator } from "@/components/ui/separator";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

function SidebarLogo() {
  const { open } = useSidebar();
  if (open) {
    return (
      <div className="flex items-center gap-2.5">
        <span className="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-lg text-sm font-bold">
          Q
        </span>
        <div className="leading-tight">
          <p className="text-sm font-semibold">Qjrxy</p>
          <p className="text-muted-foreground text-[11px]">今日校园查寝助手</p>
        </div>
      </div>
    );
  }
  return (
    <span className="bg-primary text-primary-foreground mx-auto flex size-8 items-center justify-center rounded-lg text-sm font-bold">
      Q
    </span>
  );
}

// 账号登录状态 → 子菜单圆点颜色
const statusColors: Record<string, string> = {
  authorized: "text-green-500 fill-green-500",
  expired: "text-destructive fill-destructive",
  unknown: "text-muted-foreground fill-muted-foreground",
};

function LayoutHeader() {
  const { resolvedTheme, setTheme } = useTheme();

  return (
    <header className="bg-background/95 sticky top-0 z-40 flex h-16 shrink-0 items-center justify-between gap-2 border-b px-6 backdrop-blur">
      <div className="flex items-center gap-4">
        <SidebarTrigger className="-ml-2 h-9 w-9" />
        <Separator orientation="vertical" className="h-4 opacity-50" />
        <span className="text-muted-foreground text-sm">今日校园查寝助手 · 自部署</span>
      </div>

      <TooltipProvider>
        <div className="flex items-center gap-3">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-9 w-9"
                onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
              >
                {resolvedTheme === "dark" ? (
                  <Sun className="h-4 w-4" />
                ) : (
                  <Moon className="h-4 w-4" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>切换外观主题</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="h-9 w-9" asChild>
                <a
                  href="https://github.com/openilink/openilink-hub"
                  target="_blank"
                  rel="noopener noreferrer"
                  title="界面设计致谢：OpeniLink Hub"
                >
                  <Github className="h-4 w-4" />
                </a>
              </Button>
            </TooltipTrigger>
            <TooltipContent>界面设计致谢：OpeniLink Hub</TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>
    </header>
  );
}

export function Layout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data: user, isError } = useUser();
  const { data: accounts = [] } = useAccounts();
  const [version, setVersion] = useState("");

  useEffect(() => {
    if (isError) navigate("/login", { replace: true });
  }, [isError, navigate]);

  useEffect(() => {
    api
      .systemInfo()
      .then((d) => setVersion(d.appVersion || ""))
      .catch(() => {});
  }, []);

  if (!user) return null;

  const isActive = (path: string) => location.pathname.startsWith(path);

  return (
    <SidebarProvider>
      <Sidebar variant="inset" collapsible="icon" className="border-r-0 shadow-none">
        <SidebarHeader className="h-16 justify-center">
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
          {/* 首页 */}
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton asChild isActive={location.pathname === "/"} tooltip="首页">
                    <Link to="/">
                      <House />
                      <span>首页</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>

          {/* 账号：子项为每个账号，圆点即登录状态 */}
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton isActive={isActive("/accounts")} tooltip="账号">
                    <Bot />
                    <span>账号</span>
                  </SidebarMenuButton>
                  <SidebarMenuSub>
                    {accounts.map((a) => (
                      <SidebarMenuSubItem key={a.id}>
                        <SidebarMenuSubButton asChild isActive={isActive(`/accounts?account=${a.id}`)}>
                          <Link to={`/accounts?account=${a.id}`}>
                            <Circle
                              className={`size-2 ${statusColors[a.loginStatus] || "text-muted-foreground"}`}
                            />
                            <span className="truncate">{a.alias || a.username}</span>
                          </Link>
                        </SidebarMenuSubButton>
                      </SidebarMenuSubItem>
                    ))}
                    <SidebarMenuSubItem>
                      <SidebarMenuSubButton asChild>
                        <Link to="/accounts?create=1">
                          <Circle className="size-2 border border-dashed" />
                          <span>添加账号</span>
                        </Link>
                      </SidebarMenuSubButton>
                    </SidebarMenuSubItem>
                  </SidebarMenuSub>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild isActive={isActive("/settings")} tooltip="设置">
                <Link to="/settings">
                  <Settings2 />
                  <span>设置</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarSeparator className="mx-0" />
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <SidebarMenuButton
                    size="lg"
                    className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
                  >
                    <Avatar className="h-8 w-8 rounded-lg border border-border/50 shadow-sm">
                      <AvatarFallback className="bg-primary/10 text-primary rounded-lg text-xs font-bold">
                        {user.username.charAt(0).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <div className="ml-1 grid flex-1 text-left text-sm leading-tight">
                      <span className="truncate font-semibold">{user.nickname || user.username}</span>
                      <span className="text-muted-foreground truncate text-[10px] font-medium uppercase">
                        {user.role}
                      </span>
                    </div>
                    <ChevronsUpDown className="ml-auto size-4 opacity-50" />
                  </SidebarMenuButton>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-xl shadow-2xl"
                  side="top"
                  align="end"
                  sideOffset={8}
                >
                  <DropdownMenuItem
                    onClick={async () => {
                      await useAuthStore.getState().logout();
                      navigate("/login");
                    }}
                    className="focus:bg-destructive/10 focus:text-destructive cursor-pointer font-medium text-destructive"
                  >
                    <LogOut className="mr-2 h-4 w-4" />
                    退出登录
                  </DropdownMenuItem>
                  {version && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuLabel className="text-muted-foreground text-center text-[10px] font-normal">
                        {/^\d/.test(version) ? `v${version}` : version}
                      </DropdownMenuLabel>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

      <SidebarInset className="bg-background/50 flex flex-col overflow-hidden rounded-tl-2xl">
        <LayoutHeader />

        <main className="flex-1 overflow-x-hidden overflow-y-auto">
          <div className="mx-auto h-full w-full max-w-3xl p-6 animate-in fade-in slide-in-from-bottom-2 duration-300 lg:max-w-4xl lg:p-8">
            <Outlet />
          </div>
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}

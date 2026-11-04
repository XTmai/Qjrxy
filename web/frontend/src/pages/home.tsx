// 首页：今日查寝状态板。结构参考 openilink dashboard-overview（统计网格 + 分区卡片 + 快速开始）。
import { Link } from "react-router-dom";
import {
  ArrowRight,
  Bot,
  Calendar,
  CheckCircle2,
  Plus,
  QrCode,
  XCircle,
  Clock,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useDashboard } from "@/hooks/use-dashboard";
import { usePlans } from "@/hooks/use-plans";
import { useTasks } from "@/hooks/use-tasks";
import { describeDays, taskState, taskTime, todayLabel } from "@/lib/task";
import { useAuthStore } from "@/stores/auth-store";
import { StatCard } from "@/components/stat-card";
import { TrendChart } from "@/components/trend-chart";
import { EmptyBanner } from "@/components/empty-banner";
import type { Account, Task } from "@/lib/types";

interface Summary {
  text: string;
  tone: "ok" | "warn" | "err" | "info" | "plain";
}

const toneVariant: Record<Summary["tone"], "success" | "warning" | "error" | "info" | "secondary"> = {
  ok: "success",
  warn: "warning",
  err: "error",
  info: "info",
  plain: "secondary",
};

function summarize(list: Task[]): Summary {
  if (!list.length) return { text: "暂无任务", tone: "plain" };
  const states = list.map((t) => taskState(t));
  if (states.every((s) => s.text === "已处理" || s.text === "已请假" || s.text === "已签到")) {
    return { text: "已完成", tone: "ok" };
  }
  if (states.some((s) => s.runnable)) return { text: "可执行", tone: "info" };
  if (states.some((s) => s.text === "未到时间")) return { text: "未到时间", tone: "warn" };
  if (states.some((s) => s.text === "已过期")) return { text: "已过期", tone: "err" };
  return { text: "状态未知", tone: "plain" };
}

function logMeta(status: string): { text: string; tone: "ok" | "warn" | "err" | "info" | "plain" } {
  const map: Record<string, { text: string; tone: "ok" | "warn" | "err" | "info" | "plain" }> = {
    success: { text: "成功", tone: "ok" },
    failed: { text: "失败", tone: "err" },
    pending: { text: "待核对", tone: "warn" },
    executing: { text: "执行中", tone: "info" },
  };
  return map[status] ?? { text: status || "未知", tone: "plain" };
}

export function HomePage() {
  const user = useAuthStore((s) => s.user);
  const { data, isLoading } = useDashboard();
  const { data: tasksData } = useTasks();
  const { data: plans } = usePlans();

  if (isLoading || !data) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-10 w-64" />
        <div className="grid gap-4 md:grid-cols-4">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-24 rounded-xl" />
          ))}
        </div>
        <Skeleton className="h-64 rounded-xl" />
      </div>
    );
  }

  const { stats, accounts, recentLogs, activePlans } = data;
  const attendanceOf = (id: number) =>
    (tasksData?.tasks ?? []).filter((t) => t.accountId === id && t.kind === "attendance");

  const summaries = accounts.map((a: Account) => {
    const list = attendanceOf(a.id);
    return { account: a, list, summary: summarize(list) };
  });
  const allDone = summaries.length > 0 && summaries.every((s) => s.summary.text === "已完成");
  const needsLogin = accounts.filter((a) => a.loginStatus !== "authorized").length;
  const needsDevice = accounts.filter((a) => !a.deviceId).length;

  const headline = allDone
    ? "今天全部账号都已完成 ✔"
    : needsLogin > 0
      ? `${needsLogin} 个账号需要重新登录`
      : activePlans > 0
        ? `今晚自动查寝 ${activePlans} 个账号`
        : "暂无启用的自动计划";

  const statCards = [
    { label: "今日执行", value: stats.todayTotal, cls: "", icon: Bot, color: "blue" as const },
    {
      label: "成功",
      value: stats.todaySuccess,
      cls: "text-emerald-600 dark:text-emerald-400",
      icon: CheckCircle2,
      color: "emerald" as const,
    },
    { label: "失败", value: stats.todayFailed, cls: "text-destructive", icon: XCircle, color: "red" as const },
    { label: "待核对", value: stats.todayPending, cls: "text-amber-500", icon: Clock, color: "amber" as const },
  ];

  const enabledPlans = (plans ?? []).filter((p) => p.enabled);
  const recent = (plans ?? [])
    .filter((p) => p.lastRunAt)
    .sort((a, b) => String(b.lastRunAt).localeCompare(String(a.lastRunAt)))
    .slice(0, 3);

  const quickSteps = [
    {
      step: "01",
      title: "添加校园账号",
      desc: "学校自动匹配，保存即登录",
      link: "/accounts?create=1",
      icon: Bot,
      color: "text-blue-500 bg-blue-500/10",
    },
    {
      step: "02",
      title: "扫码绑定设备",
      desc: "真实设备指纹，防风控",
      link: "/accounts",
      icon: QrCode,
      color: "text-violet-500 bg-violet-500/10",
    },
    {
      step: "03",
      title: "创建自动计划",
      desc: "到点自动提交查寝",
      link: "/accounts",
      icon: Calendar,
      color: "text-orange-500 bg-orange-500/10",
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">首页</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            {todayLabel()} · {user?.nickname || user?.username}
          </p>
        </div>
      </div>

      {/* 一句话状态 + 快捷入口 */}
      <Card className="border-border/50">
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm font-medium">{headline}</p>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" asChild>
              <Link to="/accounts?create=1">
                <Plus className="size-3.5" />
                添加账号
              </Link>
            </Button>
            <Button variant="default" size="sm" asChild>
              <Link to="/accounts">
                <Bot className="size-3.5" />
                管理账号
              </Link>
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 需处理横幅 */}
      {needsLogin > 0 && (
        <EmptyBanner
          tone="destructive"
          title={`${needsLogin} 个账号登录态已失效`}
          description="服务端已尝试自动恢复；仍失效的账号请到账号页手动登录一次。"
          action={
            <Button variant="destructive" size="sm" asChild>
              <Link to="/accounts">去处理</Link>
            </Button>
          }
        />
      )}
      {needsDevice > 0 && needsLogin === 0 && (
        <EmptyBanner
          tone="warning"
          title={`${needsDevice} 个账号待扫码绑定设备`}
          description="设备信息用于提交查寝，与真实 App 特征保持一致。"
          action={
            <Button variant="outline" size="sm" asChild>
              <Link to="/accounts">去扫码</Link>
            </Button>
          }
        />
      )}

      {/* 统计网格 */}
      <div className="grid gap-4 md:grid-cols-4">
        {statCards.map((s) => (
          <StatCard key={s.label} icon={<s.icon />} label={s.label} value={s.value} color={s.color} />
        ))}
      </div>

      {/* 今日查寝状态板 + 趋势 */}
      <Card className="border-border/50">
          <CardHeader className="flex-row items-center justify-between">
            <div>
              <CardTitle className="text-base">今日查寝</CardTitle>
              <CardDescription>按账号查看签到窗口与结果</CardDescription>
            </div>
            <Badge variant="secondary">{accounts.length} 个账号</Badge>
          </CardHeader>
          <CardContent className="divide-y p-0">
            {summaries.length === 0 && (
              <div className="text-muted-foreground py-8 text-center text-sm">还没有绑定账号</div>
            )}
            {summaries.map(({ account, list, summary }) => {
              const first = list[0];
              return (
                <div key={account.id} className="flex items-center gap-3 px-5 py-3">
                  <div className="bg-primary/10 text-primary flex size-9 items-center justify-center rounded-lg text-xs font-bold">
                    {(account.alias || account.username).slice(0, 2)}
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{account.alias || account.username}</p>
                    <p className="text-muted-foreground mt-0.5 truncate text-xs">
                      {first ? `${first.taskName || "查寝"} · ${taskTime(first)}` : "今日暂无查寝任务"}
                    </p>
                  </div>
                  <Badge variant={toneVariant[summary.tone]}>{summary.text}</Badge>
                  {summary.text === "可执行" && (
                    <Button variant="default" size="sm" asChild>
                      <Link to={`/accounts?account=${account.id}`}>执行</Link>
                    </Button>
                  )}
                </div>
              );
            })}
          </CardContent>
        </Card>

      <TrendChart days={7} />

      {/* 自动计划 + 快速开始 */}
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="border-border/50">
          <CardHeader className="flex-row items-center justify-between">
            <div>
              <CardTitle className="text-base">自动计划</CardTitle>
              <CardDescription>到点自动提交，每计划每天最多成功一次</CardDescription>
            </div>
            <Badge variant={enabledPlans.length > 0 ? "success" : "secondary"}>
              {enabledPlans.length} 个启用
            </Badge>
          </CardHeader>
          <CardContent className="divide-y p-0">
            {(plans ?? []).length === 0 && (
              <div className="text-muted-foreground py-8 text-center text-sm">
                还没有自动计划
                <Link to="/accounts" className="text-primary ml-2 hover:underline">
                  在账号详情里创建
                </Link>
              </div>
            )}
            {recent.map((p) => (
              <div key={p.id} className="flex items-center gap-3 px-5 py-3">
                <Calendar className="text-muted-foreground size-4 shrink-0" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{p.name}</p>
                  <p className="text-muted-foreground mt-0.5 truncate text-xs">
                    {describeDays(p.days)}
                    {p.mode === "custom" ? ` · ${p.windowStart}–${p.windowEnd}` : " · 跟随学校时间"}
                    {p.lastResult ? ` · ${p.lastResult}` : ""}
                  </p>
                </div>
                {p.enabled ? (
                  <Badge variant="success">启用</Badge>
                ) : (
                  <Badge variant="secondary">暂停</Badge>
                )}
              </div>
            ))}
          </CardContent>
        </Card>

        <Card className="border-border/50">
          <CardHeader className="pb-3">
            <CardTitle className="text-base font-semibold">快速开始</CardTitle>
            <CardDescription className="mt-0.5 text-xs">三步完成自动化配置</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {quickSteps.map((s) => (
              <Link
                key={s.step}
                to={s.link}
                className="group hover:bg-muted/40 flex items-center gap-4 border-b border-border/50 px-6 py-4 transition-colors last:border-b-0"
              >
                <span className="text-muted-foreground/30 group-hover:text-muted-foreground/60 w-5 text-xs font-bold tabular-nums">
                  {s.step}
                </span>
                <span className={`flex size-9 items-center justify-center rounded-lg [&_svg]:size-4 ${s.color}`}>
                  <s.icon />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="group-hover:text-primary text-sm font-medium transition-colors">{s.title}</p>
                  <p className="text-muted-foreground mt-0.5 text-xs">{s.desc}</p>
                </div>
                <ArrowRight className="text-muted-foreground/50 group-hover:translate-x-0.5 size-4 transition-transform" />
              </Link>
            ))}
          </CardContent>
        </Card>
      </div>

      {/* 最近动态 */}
      <Card className="border-border/50">
        <CardHeader className="flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">最近动态</CardTitle>
            <CardDescription>最近几次执行的结果</CardDescription>
          </div>
          <Link to="/accounts" className="text-primary flex items-center gap-1 text-xs hover:underline">
            查看全部
            <ArrowRight className="size-3" />
          </Link>
        </CardHeader>
        <CardContent className="divide-y p-0">
          {recentLogs.length === 0 && (
            <div className="text-muted-foreground py-8 text-center text-sm">暂无执行记录</div>
          )}
          {recentLogs.map((log) => {
            const meta = logMeta(log.status);
            return (
              <div key={log.id} className="flex items-center gap-3 px-5 py-2.5">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm">
                    {log.taskName || "查寝"}
                    <span className="text-muted-foreground ml-2 text-xs">
                      {log.accountAlias || log.accountUsername}
                    </span>
                  </p>
                  <p className="text-muted-foreground truncate text-xs">{log.message}</p>
                </div>
                <span className="text-muted-foreground shrink-0 text-xs">
                  {log.createdAt
                    ? new Date(log.createdAt).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
                    : ""}
                </span>
                <Badge variant={toneVariant[meta.tone]}>{meta.text}</Badge>
              </div>
            );
          })}
        </CardContent>
      </Card>
    </div>
  );
}

// 首页：今日查寝状态板（dashboard-overview 模式）。
import { Link } from "react-router-dom";
import { Bot, Circle, Plus } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { useDashboard } from "@/hooks/use-dashboard";
import { useTasks } from "@/hooks/use-tasks";
import { taskState, taskTime, todayLabel } from "@/lib/task";
import { useAuthStore } from "@/stores/auth-store";
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

/** 汇总一个账号今日查寝任务的总体状态 */
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

/** 最近动态的状态徽章 */
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

  const headline = allDone
    ? "今天全部账号都已完成 ✔"
    : needsLogin > 0
      ? `${needsLogin} 个账号需要重新登录`
      : activePlans > 0
        ? `今晚自动查寝 ${activePlans} 个账号`
        : "暂无启用的自动计划";

  const statCards = [
    { label: "今日执行", value: stats.todayTotal, cls: "" },
    { label: "成功", value: stats.todaySuccess, cls: "text-green-600 dark:text-green-400" },
    { label: "失败", value: stats.todayFailed, cls: "text-destructive" },
    { label: "待核对", value: stats.todayPending, cls: "text-amber-500" },
  ];

  return (
    <div className="space-y-6">
      {/* 页头 */}
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">首页</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            {todayLabel()} · {user?.nickname || user?.username}
          </p>
        </div>
      </div>

      {/* 一句话状态 + 快捷入口 */}
      <Card>
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

      {/* 统计网格 */}
      <div className="grid gap-4 md:grid-cols-4">
        {statCards.map((s) => (
          <Card key={s.label}>
            <CardContent>
              <div className="flex items-center gap-2">
                <Circle
                  className={`size-2.5 fill-current ${
                    s.label === "成功"
                      ? "text-green-500 fill-green-500"
                      : s.label === "失败"
                        ? "text-destructive fill-destructive"
                        : s.label === "待核对"
                          ? "text-amber-500 fill-amber-500"
                          : "text-muted-foreground"
                  }`}
                />
                <p className="text-muted-foreground text-xs">{s.label}</p>
              </div>
              <p className={`mt-2 text-3xl font-semibold tabular-nums ${s.cls}`}>{s.value}</p>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* 今日查寝状态板 */}
      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">今日查寝</CardTitle>
            <CardDescription>按账号查看签到窗口与结果</CardDescription>
          </div>
          <span className="text-muted-foreground text-xs">{accounts.length} 个账号</span>
        </CardHeader>
        <CardContent className="divide-y p-0">
          {summaries.length === 0 && (
            <div className="text-muted-foreground flex flex-col items-center gap-3 py-10 text-center">
              <p className="text-sm">还没有绑定账号</p>
              <Button variant="default" size="sm" asChild>
                <Link to="/accounts?create=1">去添加</Link>
              </Button>
            </div>
          )}
          {summaries.map(({ account, list, summary }) => {
            const first = list[0];
            return (
              <div key={account.id} className="flex items-center gap-3 px-5 py-3">
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

      {/* 最近动态 */}
      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">最近动态</CardTitle>
            <CardDescription>最近几次执行的结果</CardDescription>
          </div>
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
                <Badge variant={toneVariant[meta.tone]}>{meta.text}</Badge>
              </div>
            );
          })}
        </CardContent>
      </Card>
    </div>
  );
}

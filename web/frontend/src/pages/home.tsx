// 监控总览：就绪率横幅 + 指标卡 + 执行流水 + 右栏监控
import { Link } from "react-router-dom";
import {
  Activity,
  ArrowRight,
  Bot,
  CalendarClock,
  Crosshair,
  Gauge,
  Play,
  RefreshCw,
  ShieldCheck,
  Timer,
  Zap,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useDashboard, useLogTrend } from "@/hooks/use-dashboard";
import { useStats7d } from "@/hooks/use-stats";
import { usePlans } from "@/hooks/use-plans";
import { useTasks } from "@/hooks/use-tasks";
import { describeDays, taskState, taskTime } from "@/lib/task";
import { EmptyBanner } from "@/components/empty-banner";
import type { Account, Task } from "@/lib/types";

interface Summary {
  text: string;
  tone: "ok" | "warn" | "err" | "info" | "plain";
}

const toneClass: Record<string, string> = {
  ok: "bg-success-soft text-success-text border-success-border",
  warn: "bg-warning-soft text-warning-text border-warning-border",
  err: "bg-danger-soft text-danger-text border-danger-border",
  info: "bg-brand-soft text-brand border-brand/20",
  plain: "bg-muted text-muted-foreground border-border",
};

function summarize(list: Task[]): Summary {
  if (!list.length) return { text: "暂无任务", tone: "plain" };
  const states = list.map((t) => taskState(t));
  if (states.every((s) => s.text === "已处理" || s.text === "已请假" || s.text === "已签到")) {
    return { text: "已完成", tone: "ok" };
  }
  if (states.some((s) => s.runnable)) return { text: "可执行", tone: "info" };
  if (states.some((s) => s.text === "未到时间")) return { text: "等待窗口", tone: "warn" };
  if (states.some((s) => s.text === "已过期")) return { text: "已过期", tone: "err" };
  return { text: "状态未知", tone: "plain" };
}

/** 指标卡：图标容器 + 大号数值 + 标签 + 可选的底部说明 */
function Metric({
  icon: Icon,
  label,
  value,
  unit,
  note,
  tone = "primary",
}: {
  icon: typeof Bot;
  label: string;
  value: string | number;
  unit?: string;
  note?: React.ReactNode;
  tone?: "primary" | "success" | "warning" | "danger";
}) {
  const toneCls = {
    primary: "bg-muted text-foreground",
    success: "bg-success-soft text-success",
    warning: "bg-muted text-muted-foreground",
    danger: "bg-danger-soft text-danger",
  }[tone];
  return (
    <Card className="border-border">
      <CardContent className="p-4">
        <div className="flex items-start justify-between">
          <p className="text-muted-foreground tracking-label text-[11px] font-medium uppercase">{label}</p>
          <span className={`flex size-7 items-center justify-center rounded-md ${toneCls}`}>
            <Icon className="size-3.5" />
          </span>
        </div>
        <p className="mt-2.5 flex items-baseline gap-1">
          <span className="tracking-headline text-2xl font-semibold tabular-nums">{value}</span>
          {unit && <span className="text-muted-foreground text-[11px]">{unit}</span>}
        </p>
        {note && <div className="text-muted-foreground mt-1.5 text-[11px]">{note}</div>}
      </CardContent>
    </Card>
  );
}

export function HomePage() {
  const { data, isLoading, refetch } = useDashboard();
  const { data: tasksData } = useTasks();
  const { data: plans } = usePlans();
  const { data: stats7d } = useStats7d();
  const { data: trend } = useLogTrend(7);

  if (isLoading || !data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-24 w-full rounded-lg" />
        <div className="grid gap-3 md:grid-cols-4">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-28 rounded-lg" />
          ))}
        </div>
        <Skeleton className="h-72 rounded-lg" />
      </div>
    );
  }

  const { stats, accounts, activePlans } = data;
  const attendanceOf = (id: number) =>
    (tasksData?.tasks ?? []).filter((t) => t.accountId === id && t.kind === "attendance");

  const summaries = accounts.map((a: Account) => {
    const list = attendanceOf(a.id);
    return { account: a, list, summary: summarize(list) };
  });
  const allDone = summaries.length > 0 && summaries.every((s) => s.summary.text === "已完成");
  const needsLogin = accounts.filter((a) => a.loginStatus !== "authorized").length;
  const needsDevice = accounts.filter((a) => !a.deviceId).length;
  const readyCount = accounts.filter(
    (a) => a.loginStatus === "authorized" && a.deviceId && a.lon && a.lat,
  ).length;

  const readiness = accounts.length ? Math.round((readyCount / accounts.length) * 100) : 0;
  const enabledPlans = (plans ?? []).filter((p) => p.enabled);
  const blockedPlans = enabledPlans.filter((p) => (p.lastResult ?? "").startsWith("需人工处理"));

  const headline = allDone
    ? "今日查寝全部完成"
    : needsLogin > 0
      ? `${needsLogin} 个账号需要重新登录`
      : enabledPlans.length > 0
        ? `${enabledPlans.length} 个计划待执行`
        : "暂无启用的自动计划";

  // 今日执行流水的账号行（取每个账号第一条查寝任务）
  const flow = summaries.map(({ account, list, summary }) => ({
    account,
    task: list[0],
    summary,
  }));

  return (
    <div className="space-y-4">
      {/* 就绪率横幅 */}
      <Card className="border-border overflow-hidden">
        <CardContent className="flex flex-wrap items-center gap-4 p-4">
          <span
            className={`flex size-11 shrink-0 items-center justify-center rounded-xl ${
              readiness === 100 ? "bg-success-soft text-success" : "bg-brand-soft text-brand"
            }`}
          >
            <ShieldCheck className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="tracking-headline whitespace-nowrap text-lg font-semibold">今日就绪率 {readiness}%</h1>
              <span className="text-muted-foreground flex items-center gap-1.5 whitespace-nowrap text-[11px]">
                <span className="bg-success pulse-dot size-1.5 rounded-full" />
                调度引擎在线
              </span>
            </div>
            <p className="text-muted-foreground mt-0.5 text-xs text-pretty">
              {accounts.length} 个账号中 {readyCount} 个已就绪 · {headline}
            </p>
          </div>
          <div className="border-border flex w-full shrink-0 items-center justify-end gap-2 border-t pt-2.5 sm:w-auto sm:border-t-0 sm:pt-0">
            <div className="text-left sm:text-right">
              <p className="text-muted-foreground tracking-label text-[10px] uppercase">今日执行</p>
              <p className="font-mono text-sm font-semibold tabular-nums">
                {stats.todaySuccess}
                <span className="text-muted-foreground font-normal"> / {stats.todayTotal}</span>
              </p>
            </div>
            <Button variant="ghost" size="icon" className="size-8" onClick={() => void refetch()}>
              <RefreshCw className="size-3.5" />
            </Button>
            <Button size="sm" className="h-8 gap-1.5 text-xs" asChild>
              <Link to="/schedules">
                <Play className="size-3.5" />
                执行计划
              </Link>
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 待处理提示 */}
      {needsLogin > 0 && (
        <EmptyBanner
          tone="destructive"
          title={`${needsLogin} 个账号登录态已失效`}
          description="服务端会尝试自动恢复；仍失效的账号请到账号管理页手动登录。"
          action={
            <Button variant="default" size="sm" className="h-7 text-xs" asChild>
              <Link to="/accounts">去处理</Link>
            </Button>
          }
        />
      )}
      {blockedPlans.length > 0 && (
        <EmptyBanner
          tone="warning"
          title={`${blockedPlans.length} 个计划需人工处理（设备需辅导员授权）`}
          description="学校判定「疑似更换手机频繁」，已停止当天自动重试。请联系辅导员在辅导猫「手机授权管理」中授权本机，之后在调度计划页对该计划点「重置今日」恢复自动执行。"
          action={
            <Button variant="outline" size="sm" className="h-7 text-xs" asChild>
              <Link to="/schedules">去处理</Link>
            </Button>
          }
        />
      )}
      {needsDevice > 0 && needsLogin === 0 && (
        <EmptyBanner
          tone="warning"
          title={`${needsDevice} 个账号待扫码绑定设备`}
          description="设备指纹用于提交查寝，需与真实 App 一致。"
          action={
            <Button variant="outline" size="sm" className="h-7 text-xs" asChild>
              <Link to="/accounts">去扫码</Link>
            </Button>
          }
        />
      )}

      {/* 指标卡组 */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Metric
          icon={Bot}
          label="受控账号"
          value={accounts.length}
          unit="个"
          tone="primary"
          note={
            <span className="flex items-center gap-1">
              <span className="bg-success size-1.5 rounded-full" />
              {readyCount} 个已就绪 · {accounts.length - readyCount} 待配置
            </span>
          }
        />
        <Metric
          icon={CalendarClock}
          label="今日任务"
          value={summaries.reduce((n, s) => n + s.list.length, 0)}
          unit="笔"
          tone="primary"
          note={`${stats.todaySuccess} 成功 · ${stats.todayFailed} 失败`}
        />
        <Metric
          icon={Crosshair}
          label="平均定位偏移"
          value={stats7d ? stats7d.avgGeoOffsetM.toFixed(1) : "—"}
          unit="米"
          tone="warning"
          note="安全阈值 ≤ 500 米"
        />
        <Metric
          icon={Gauge}
          label="7 日达标率"
          value={stats7d ? `${stats7d.successRate.toFixed(1)}%` : "—"}
          tone={stats7d && stats7d.successRate >= 90 ? "success" : "danger"}
          note={stats7d ? `${stats7d.success}/${stats7d.total} 次成功` : "暂无数据"}
        />
      </div>

      {/* 主体：执行流水 + 右栏 */}
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        {/* 左：执行流水 */}
        <div className="space-y-4">
          <Card className="border-border">
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <div>
                <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                  <Activity className="text-primary size-4" />
                  今日查寝执行流水
                </CardTitle>
                <CardDescription className="mt-0.5 text-xs">{accounts.length} 个账号节点</CardDescription>
              </div>
              <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs" asChild>
                <Link to="/audit">
                  全部
                  <ArrowRight className="size-3" />
                </Link>
              </Button>
            </CardHeader>
            <CardContent className="divide-y p-0">
              {flow.length === 0 && (
                <div className="text-muted-foreground py-10 text-center text-sm">
                  还没有绑定账号
                  <Link to="/accounts?create=1" className="text-primary ml-1.5 hover:underline">
                    去添加
                  </Link>
                </div>
              )}
              {flow.map(({ account, task, summary }) => {
                const st = task ? taskState(task) : null;
                return (
                  <div key={account.id} className="flex items-center gap-3 px-4 py-3">
                    <span className="bg-primary-soft text-primary flex size-8 shrink-0 items-center justify-center rounded-md text-[11px] font-bold">
                      {(account.alias || account.username).slice(0, 1)}
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <p className="truncate text-xs font-medium">{account.alias || account.username}</p>
                        <span className="text-muted-foreground shrink-0 font-mono text-[10px]">
                          {account.username.slice(0, 4)}****
                        </span>
                        <span className="text-muted-foreground shrink-0 text-[10px]">· {account.schoolName}</span>
                      </div>
                      <p className="text-muted-foreground mt-0.5 truncate text-[11px]">
                        {task ? (
                          <>
                            {task.taskName || "查寝"}
                            <span className="mx-1.5">·</span>
                            <span className="font-mono">{taskTime(task)}</span>
                          </>
                        ) : (
                          "今日暂无查寝任务"
                        )}
                      </p>
                    </div>
                    <Badge variant="outline" className={`shrink-0 border text-[10px] ${toneClass[summary.tone]}`}>
                      {summary.text}
                    </Badge>
                    {st?.runnable && (
                      <Button variant="outline" size="sm" className="h-6 px-2 text-[10px]" asChild>
                        <Link to={`/accounts?account=${account.id}`}>执行</Link>
                      </Button>
                    )}
                  </div>
                );
              })}
            </CardContent>
          </Card>

          {/* 近 7 日趋势 */}
          <Card className="border-border">
            <CardHeader className="pb-3">
              <CardTitle className="tracking-headline text-sm font-semibold">近 7 日打卡分布</CardTitle>
              <CardDescription className="mt-0.5 text-xs">
                {trend ? `${trend.trend.reduce((n, d) => n + d.success, 0)} 次成功 · 按天统计` : ""}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {trend && <BarChart data={trend.trend} />}
            </CardContent>
          </Card>
        </div>

        {/* 右栏 */}
        <div className="space-y-4">
          {/* 调度器双循环 */}
          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                <Zap className="text-primary size-4" />
                调度器双循环
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2.5">
              {[
                { label: "任务计算唤醒", value: "30 秒", note: "轻量循环 · 本地倒计时" },
                { label: "登录态探测", value: "仅 2 次 / 人", note: "活跃时段按需触发" },
                { label: "本地数据", value: "SQLite", note: "单二进制部署" },
              ].map((r) => (
                <div key={r.label} className="border-border flex items-center justify-between border-b pb-2 last:border-b-0 last:pb-0">
                  <div>
                    <p className="text-xs">{r.label}</p>
                    <p className="text-muted-foreground mt-0.5 text-[10px]">{r.note}</p>
                  </div>
                  <span className="text-primary shrink-0 font-mono text-[11px] font-medium">{r.value}</span>
                </div>
              ))}
            </CardContent>
          </Card>

          {/* 地理安全圈 */}
          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                <Crosshair className="text-primary size-4" />
                签到点地理安全圈
              </CardTitle>
            </CardHeader>
            <CardContent>
              <GeoRadar />
              <div className="text-muted-foreground mt-2.5 space-y-1 font-mono text-[10px]">
                <p className="flex justify-between">
                  <span>学校中心点</span>
                  <span className="text-foreground">{accounts[0]?.address ? "已配置" : "未配置"}</span>
                </p>
                <p className="flex justify-between">
                  <span>偏移范围</span>
                  <span className="text-foreground">± 50 米</span>
                </p>
              </div>
            </CardContent>
          </Card>

          {/* 自动计划 */}
          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                <Timer className="text-primary size-4" />
                自动计划
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {enabledPlans.length === 0 && (
                <p className="text-muted-foreground py-2 text-center text-[11px]">
                  暂无启用计划
                  <Link to="/schedules" className="text-primary ml-1 hover:underline">
                    去创建
                  </Link>
                </p>
              )}
              {enabledPlans.slice(0, 3).map((p) => (
                <div key={p.id} className="flex items-center justify-between text-[11px]">
                  <span className="truncate">{p.name}</span>
                  <span className="text-muted-foreground shrink-0 font-mono text-[10px]">
                    {p.mode === "custom" ? `${p.windowStart}-${p.windowEnd}` : describeDays(p.days)}
                  </span>
                </div>
              ))}
              {activePlans > 0 && (
                <div className="border-border mt-1 border-t pt-2">
                  <p className="text-muted-foreground text-[10px]">
                    下次唤醒 <span className="text-foreground font-mono">21:00</span>
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

/** 纯 CSS/SVG 柱状图（无图表库） */
function BarChart({ data }: { data: Array<{ date: string; success: number; failed: number; pending: number }> }) {
  const max = Math.max(1, ...data.map((d) => d.success + d.failed + d.pending));
  return (
    <div className="flex h-32 items-end gap-1.5">
      {data.map((d) => {
        const total = d.success + d.failed + d.pending;
        const h = (n: number) => `${Math.max(n > 0 ? 6 : 0, Math.round((n / max) * 100))}%`;
        return (
          <div key={d.date} className="flex min-w-0 flex-1 flex-col items-center gap-1">
            <div className="border-border bg-muted/40 flex h-full w-full flex-col justify-end overflow-hidden rounded-md border">
              {d.failed > 0 && <div className="bg-danger/80 w-full" style={{ height: h(d.failed) }} />}
              {d.pending > 0 && <div className="bg-warning/80 w-full" style={{ height: h(d.pending) }} />}
              {d.success > 0 && <div className="bg-success/85 w-full" style={{ height: h(d.success) }} />}
            </div>
            <span className="text-muted-foreground font-mono text-[9px]">{d.date.slice(8)}</span>
            <span className="text-foreground/70 font-mono text-[9px] tabular-nums">{total || ""}</span>
          </div>
        );
      })}
    </div>
  );
}

/** 纯 CSS 地理安全圈雷达图 */
function GeoRadar() {
  return (
    <div className="bg-muted/30 border-border relative flex aspect-square items-center justify-center overflow-hidden rounded-md border">
      {/* 同心圆 */}
      {[100, 74, 48, 22].map((s, i) => (
        <div
          key={s}
          className="border-border absolute rounded-full border"
          style={{ width: `${s}%`, height: `${s}%`, opacity: 0.7 - i * 0.1 }}
        />
      ))}
      {/* 十字准星 */}
      <div className="bg-border absolute h-px w-full" />
      <div className="bg-border absolute h-full w-px" />
      {/* 中心点 */}
      <div className="bg-primary relative z-10 flex size-3 items-center justify-center rounded-full">
        <span className="bg-primary/30 pulse-dot absolute size-6 rounded-full" />
      </div>
      {/* 标记点 */}
      <span className="bg-success absolute top-[26%] left-[62%] size-1.5 rounded-full" />
      <span className="bg-success absolute top-[58%] left-[24%] size-1.5 rounded-full" />
      <span className="bg-warning absolute top-[70%] left-[68%] size-1.5 rounded-full" />
      {/* 标签 */}
      <span className="text-muted-foreground absolute bottom-1.5 w-full text-center font-mono text-[9px]">
        GEOFENCE · READY
      </span>
    </div>
  );
}

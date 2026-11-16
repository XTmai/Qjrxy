// 监控总览：小蜜蜂花园就绪横幅 + 指标卡 + 执行流水 + 7/14 日分布 + 地理安全圈 + 自动计划。
//
// 花园的进度口径 = 今日已完成账号 / 总账号。花丛本身是 52 株（视觉密度），
// 不是每账号一朵——10 株花撑不出一条花园。
import { useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  Activity,
  ArrowRight,
  Bot,
  CalendarClock,
  Crosshair,
  Gauge,
  GripVertical,
  LogIn,
  Play,
  QrCode,
  RefreshCw,
  Timer,
  TriangleAlert,
  Zap,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useDashboard, useLogTrend } from "@/hooks/use-dashboard";
import { useStats7d } from "@/hooks/use-stats";
import { usePlans } from "@/hooks/use-plans";
import { useTasks } from "@/hooks/use-tasks";
import { useProbeAccounts } from "@/hooks/use-accounts";
import { toast } from "@/hooks/use-toast";
import { describeDays, taskState, taskTime } from "@/lib/task";
import { maskUser, cn } from "@/lib/utils";
import { BeeGarden, type BeeGardenHandle } from "@/components/bee-garden";
import type { Account, Task } from "@/lib/types";

/** 一个账号今天算不算「已完成」：有查寝任务且已处理完，或任务已消失（当天收工） */
function isAccountDone(tasks: Task[]): boolean {
  if (tasks.length === 0) return false;
  return tasks.every((t) => {
    const s = taskState(t);
    return s.text === "已处理" || s.text === "已请假" || s.text === "已签到";
  });
}

/** 一个账号今天算不算「已完成」：优先看当天成功执行记录，其次看任务缓存 */
function isAccountDoneToday(accountId: number, tasks: Task[], doneIDs: Set<number>): boolean {
  if (doneIDs.has(accountId)) return true;
  return isAccountDone(tasks);
}

/** 账号是否就绪：登录态 + 设备指纹 + 签到坐标三项齐备 */
function isReady(a: Account) {
  return a.loginStatus === "authorized" && Boolean(a.deviceId) && Boolean(a.lon) && Boolean(a.lat);
}

const toneClass: Record<string, string> = {
  ok: "bg-success-soft text-success-text border-success-border",
  warn: "bg-warning-soft text-warning-text border-warning-border",
  err: "bg-danger-soft text-danger-text border-danger-border",
  info: "bg-brand-soft text-success-text border-success-border/60",
  plain: "bg-[color:var(--subtle)] text-muted-foreground border-border",
};

function summarize(list: Task[]) {
  if (!list.length) return { text: "暂无任务", tone: "plain" as const };
  if (isAccountDone(list)) return { text: "已完成", tone: "ok" as const };
  const states = list.map((t) => taskState(t));
  if (states.some((s) => s.runnable)) return { text: "可执行", tone: "info" as const };
  if (states.some((s) => s.text === "未到时间")) return { text: "等待窗口", tone: "warn" as const };
  if (states.some((s) => s.text === "已过期")) return { text: "已过期", tone: "err" as const };
  return { text: "状态未知", tone: "plain" as const };
}

/** 指标卡：图标容器 + 大号数值 + 标签 + 可选说明 */
function Metric({
  icon: Icon,
  label,
  value,
  unit,
  note,
  tone = "primary",
  onClick,
}: {
  icon: typeof Bot;
  label: string;
  value: string | number;
  unit?: string;
  note?: React.ReactNode;
  tone?: "primary" | "success" | "warning" | "danger";
  onClick?: () => void;
}) {
  const iconTone = {
    primary: "bg-[color:var(--subtle)] text-foreground",
    success: "bg-success-soft text-success",
    warning: "bg-[color:var(--subtle)] text-muted-foreground",
    danger: "bg-danger-soft text-danger",
  }[tone];

  return (
    <div
      onClick={onClick}
      className={cn(
        "rounded-2xl border border-border bg-card p-4 transition-colors",
        onClick && "cursor-pointer hover:border-border-strong",
      )}
    >
      <div className="flex items-center justify-between">
        <p className="text-[11px] font-medium text-muted-foreground">{label}</p>
        <span className={cn("flex size-7 shrink-0 items-center justify-center rounded-xl", iconTone)}>
          <Icon className="size-3.5" />
        </span>
      </div>
      <p className="mt-2 flex items-baseline gap-1">
        <span className="tracking-headline text-2xl font-bold tabular-nums">{value}</span>
        {unit && <span className="text-[11px] font-normal text-muted-foreground">{unit}</span>}
      </p>
      {note && <div className="mt-1.5 text-[11px] font-medium text-muted-foreground">{note}</div>}
    </div>
  );
}

export function HomePage() {
  const { data, isLoading, refetch } = useDashboard();
  const { data: tasksData } = useTasks();
  const { data: plans } = usePlans();
  const { data: stats7d } = useStats7d();
  const probe = useProbeAccounts();
  const navigate = useNavigate();

  const gardenRef = useRef<BeeGardenHandle | null>(null);
  // 花园里平滑后的百分比：只用于屏幕阅读器播报，不进视觉层
  const [shownPct, setShownPct] = useState(0);
  const [confirmForce, setConfirmForce] = useState(false);

  if (isLoading || !data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-32 w-full rounded-2xl" />
        <div className="grid gap-3.5 md:grid-cols-4">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-24 rounded-2xl" />
          ))}
        </div>
        <Skeleton className="h-72 rounded-2xl" />
      </div>
    );
  }

  const { stats, accounts, activePlans, doneAccountIDs } = data;
  const tasks = tasksData?.tasks ?? [];

  // 「今日已完成」以当天成功执行记录为准（后端 doneAccountIDs）。
  // 不能用任务缓存：学校签完后任务会从待办列表消失，缓存一清空，
  // 刚签完的账号就会被算成「暂无任务 → 未完成」，进度条反而归零。
  const doneSet = new Set(doneAccountIDs ?? []);
  const rows = accounts.map((a: Account) => {
    const list = tasks.filter((t) => t.accountId === a.id && t.kind === "attendance");
    const done = isAccountDoneToday(a.id, list, doneSet);
    // 已签完的账号任务往往已从待办里消失，此时按执行记录显示「已完成」，
    // 而不是「暂无任务」——后者会让流水看起来像没跑过。
    const summary = done && list.length === 0 ? { text: "已完成", tone: "ok" as const } : summarize(list);
    return { account: a, list, summary, done };
  });

  const readyCount = accounts.filter(isReady).length;
  const doneCount = rows.filter((r) => r.done).length;
  const total = accounts.length;
  const progress = total ? doneCount / total : 0;
  const allDone = total > 0 && doneCount === total;

  const needsLogin = accounts.filter((a: Account) => a.loginStatus !== "authorized").length;
  const needsDevice = accounts.filter((a: Account) => !a.deviceId).length;
  const enabledPlans = (plans ?? []).filter((p) => p.enabled);
  const blockedPlans = enabledPlans.filter((p) => (p.lastResult ?? "").startsWith("需人工处理"));

  const headline = allDone
    ? "今日查寝全部完成"
    : needsLogin > 0
      ? `${needsLogin} 个账号需要重新登录`
      : enabledPlans.length > 0
        ? `${enabledPlans.length} 个计划待执行`
        : "暂无启用的自动计划";

  // 副标题里只挂一条最紧急的告警（重登 > 计划阻断 > 待绑设备）。
  // 多条同时存在时其余仍以 headline 文案呈现，不会丢信息。
  const alert = needsLogin > 0
    ? { text: `${needsLogin} 个需重登`, to: "/accounts", className: "text-danger" }
    : blockedPlans.length > 0
      ? { text: `${blockedPlans.length} 个计划待处理`, to: "/schedules", className: "text-warning-text" }
      : needsDevice > 0
        ? { text: `${needsDevice} 个待绑设备`, to: "/accounts", className: "text-warning-text" }
        : null;

  // 流水按「异常优先」排：需重登 → 待补 → 未完成 → 已完成，用户第一眼看到的就是要处理的
  const flow = [...rows].sort((a, b) => {
    const rank = (r: (typeof rows)[number]) =>
      r.account.loginStatus !== "authorized" ? 0 : !isReady(r.account) ? 1 : r.done ? 3 : 2;
    return rank(a) - rank(b);
  });

  const runProbe = () => {
    setConfirmForce(false);
    probe.mutate(undefined, {
      onSuccess: (res) => {
        const list = res.results ?? [];
        const ok = list.filter((r) => r.status === "authorized" || r.status === "relogged").length;
        const bad = list.length - ok;
        toast({
          title: `已探测 ${list.length} 个账号`,
          description: bad > 0 ? `${ok} 个正常，${bad} 个需重新登录` : `${ok} 个账号登录态正常`,
          variant: bad > 0 ? "default" : "default",
        });
        void refetch();
      },
      onError: (e) => {
        toast({ title: "探测失败", description: e.message, variant: "destructive" });
      },
    });
  };

  return (
    <div className="view-panel space-y-4">
      {/* ── 花园横幅 ──────────────────────────────────────────────
          桌面：标题在左、操作在右（绝对定位），花丛在下方的带子里。
          手机：标题 → 操作栏 → 花，全部竖排；操作栏在花上方单独一行，
          不和标题抢右侧空间，也不会压到花。
          pb 在手机上小一些（花带 96px），桌面留 112px 给更高的花。 */}
      <div className="relative overflow-hidden rounded-2xl border border-border bg-card px-4 pt-4 pb-[96px] shadow-xs select-none sm:px-6 md:pb-[112px]">
        {/* 标题区 */}
        <div className="relative z-20 min-w-0">
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
            {/* 标题数字用花园进度（今日已完成账号占比），与下方进度条同源。
                就绪率是「能不能跑」，不是「跑到哪」，两者混在一个数字里
                会出现「就绪率 100% + 进度条 0%」的自相矛盾画面。 */}
            <h1 className="tracking-headline text-base font-bold whitespace-nowrap sm:text-lg">
              今日进度 {shownPct}%
            </h1>
            <span className="inline-flex items-center gap-1 text-xs font-medium whitespace-nowrap text-success">
              <span className="size-1.5 rounded-full bg-success" />
              调度引擎在线
            </span>
          </div>
          {/* 副标题：就绪情况 + 一条告警链接。
              告警在手机端才降级到这里（桌面端右上角有独立徽章，重复显示反而啰嗦）。
              原来是三个彩色按钮堆在操作区，窄屏上把横幅撑得很满。 */}
          <p className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <span>
              {total} 个账号中 {readyCount} 个已就绪
            </span>
            <span className="text-faint">·</span>
            {alert ? (
              <>
                <Link
                  to={alert.to}
                  className={cn(
                    "inline-flex items-center gap-0.5 font-medium hover:underline md:hidden",
                    alert.className,
                  )}
                >
                  {alert.text}
                  <ArrowRight className="size-3" />
                </Link>
                <span className="hidden md:inline">{headline}</span>
              </>
            ) : (
              <span>{headline}</span>
            )}
          </p>
        </div>

        {/* 桌面端：操作栏绝对定位到右上，与标题同一行 */}
        <div className="absolute top-4 right-4 z-20 hidden items-center gap-2 md:flex">
          <BannerActions
            needsLogin={needsLogin}
            needsDevice={needsDevice}
            blockedCount={blockedPlans.length}
            onReplay={() => window.setTimeout(() => gardenRef.current?.replay(), 0)}
            onForceRun={() => setConfirmForce(true)}
          />
        </div>

        {/* bandPx 传 0：由引擎按画布高度自动估算花带，
            省掉「JS 里读 window.innerWidth」这种不随 resize 重算的写法 */}
        <BeeGarden progress={progress} onProgress={setShownPct} bandPx={0} handleRef={gardenRef} />

        {/* 手机端：只留「重播 + 补测」两个按钮，靠右一行。
            告警已经降级进副标题，这里不再堆彩色按钮。
            别用负边距往上提——border-t 会正好压在副标题那一行上。 */}
        <div className="relative z-20 flex items-center justify-end gap-2 border-t border-border/70 pt-3 md:hidden">
          <Button
            variant="outline"
            size="icon-sm"
            className="size-8 rounded-lg"
            title="重播今日开花进度动画"
            aria-label="重播开花动画"
            onClick={() => {
              window.setTimeout(() => gardenRef.current?.replay(), 0);
            }}
          >
            <RefreshCw className="size-3.5" />
          </Button>
          <Button
            size="sm"
            className="h-8 gap-1.5 rounded-lg px-3 text-[11px] font-semibold"
            onClick={() => setConfirmForce(true)}
          >
            <Play className="size-3.5" />
            补测 / 重跑
          </Button>
        </div>
      </div>

      {/* 二次确认：只在全部完成时才提示「要不要再跑一轮」 */}
      {confirmForce && (
        <div className="flex flex-col justify-between gap-3 rounded-2xl border border-warning-border bg-warning-soft/90 p-3.5 sm:flex-row sm:items-center">
          <div className="flex items-center gap-2.5 text-xs text-warning-text">
            <TriangleAlert className="size-4 shrink-0 text-warning" />
            <span>
              <strong>防误触提示：</strong>
              {allDone
                ? `今日 ${total} 个账号均已完成查寝提交。`
                : `今日还有 ${total - doneCount} 个账号未完成。`}
              将重新检测全部账号登录态并同步任务，不会自动提交查寝。
            </span>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="ghost" size="sm" className="h-8 text-xs" onClick={() => setConfirmForce(false)}>
              取消
            </Button>
            <Button
              size="sm"
              className="h-8 bg-warning text-xs text-white hover:bg-warning/90"
              disabled={probe.isPending}
              onClick={runProbe}
            >
              {probe.isPending ? "探测中..." : "确认探测"}
            </Button>
          </div>
        </div>
      )}

      {/* 指标卡组 */}
      <div className="grid grid-cols-2 gap-3.5 lg:grid-cols-4">
        <Metric
          icon={Bot}
          label="受控账号"
          value={total}
          unit="个"
          note={
            <span className="flex items-center gap-1">
              <span className="size-1.5 rounded-full bg-success" />
              {readyCount} 个已就绪 · {total - readyCount} 待配置
            </span>
          }
          onClick={() => navigate("/accounts")}
        />
        <Metric
          icon={CalendarClock}
          label="今日任务"
          value={rows.reduce((n, r) => n + r.list.length, 0)}
          unit="笔"
          note={`${stats.todaySuccess} 成功 · ${stats.todayFailed} 失败`}
        />
        <Metric
          icon={Crosshair}
          label="平均定位偏移"
          value={stats7d ? stats7d.avgGeoOffsetM.toFixed(1) : "—"}
          unit="米"
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

      {/* ── 主体：左侧（分布 + 流水） / 右侧（双循环 + 雷达 + 计划） ── */}
      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-12">
        <div className="space-y-4 lg:col-span-8">
          <DistributionCard />

          <Card className="border-border">
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <div>
                <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-bold">
                  <Activity className="size-4 text-muted-foreground" />
                  今日查寝执行流水
                </CardTitle>
                <CardDescription className="mt-0.5 text-[11px]">
                  {total} 个账号节点 · 点击任意行查看账号详情
                </CardDescription>
              </div>
              <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs" asChild>
                <Link to="/audit">
                  全部
                  <ArrowRight className="size-3" />
                </Link>
              </Button>
            </CardHeader>
            <CardContent className="p-0">
              {flow.length === 0 ? (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  还没有绑定账号
                  <Link to="/accounts?create=1" className="ml-1.5 text-success hover:underline">
                    去添加
                  </Link>
                </div>
              ) : (
                <div className="thin-scrollbar max-h-[250px] space-y-2 overflow-y-auto px-4 pb-4">
                  {flow.map(({ account, list, summary }) => {
                    const task = list[0];
                    const st = task ? taskState(task) : null;
                    const first = (account.alias || account.username).replace(/^2024级\s*/, "").slice(0, 1);
                    return (
                      <Link
                        key={account.id}
                        to={`/accounts?account=${account.id}`}
                        className="block rounded-xl border border-border bg-[color:var(--subtle)]/60 p-3 transition-colors hover:bg-[color:var(--subtle)]"
                      >
                        {/* 手机两行、桌面一行：状态徽章在窄屏挪到右上角，
                            不再和「学号 · 学校」抢那 106px 的宽度 */}
                        <div className="flex items-start gap-3">
                          <div className="flex size-8 shrink-0 items-center justify-center rounded-xl border border-border bg-card text-xs font-bold text-foreground">
                            {first.toUpperCase()}
                          </div>
                          <div className="min-w-0 flex-1">
                            <div className="flex items-start justify-between gap-2">
                              <div className="min-w-0">
                                <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                                  <span className="font-mono text-xs font-bold text-foreground">
                                    {account.alias || account.username}
                                  </span>
                                  <span className="font-mono text-[11px] text-muted-foreground">
                                    {maskUser(account.username)}
                                  </span>
                                </div>
                                <div className="mt-0.5 truncate text-[11px] text-muted-foreground">
                                  {account.schoolName}
                                </div>
                              </div>
                              <span
                                className={cn(
                                  "shrink-0 rounded-full border px-2 py-0.5 text-[10px] font-medium whitespace-nowrap",
                                  toneClass[summary.tone],
                                )}
                              >
                                {summary.text}
                              </span>
                            </div>
                            <div className="mt-1 flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px] text-muted-foreground">
                              <span className="truncate">{task ? task.taskName || "查寝" : "今日暂无查寝任务"}</span>
                              {task && (
                                <>
                                  <span className="text-faint">·</span>
                                  <span className="shrink-0 font-mono">{taskTime(task)}</span>
                                </>
                              )}
                              {st?.runnable && (
                                <span className="ml-auto shrink-0 rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] text-muted-foreground">
                                  可执行
                                </span>
                              )}
                            </div>
                          </div>
                        </div>
                      </Link>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <div className="space-y-4 lg:col-span-4">
          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-bold">
                <Zap className="size-4 text-muted-foreground" />
                调度器双循环
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-xs">
              {[
                { label: "任务计算唤醒", value: "30 秒", note: "轻量循环 · 本地倒计时" },
                { label: "登录态探测", value: "每小时", note: "仅活跃时段按需触发" },
                { label: "本地数据", value: "SQLite", note: "单二进制部署" },
              ].map((r, i) => (
                <div
                  key={r.label}
                  className={cn(
                    "flex items-center justify-between",
                    i > 0 && "border-t border-border pt-2.5",
                  )}
                >
                  <div>
                    <div className="font-semibold text-foreground">{r.label}</div>
                    <div className="mt-0.5 text-[11px] text-muted-foreground">{r.note}</div>
                  </div>
                  <span className="shrink-0 font-mono text-[11px] font-bold text-foreground">
                    {r.value}
                  </span>
                </div>
              ))}
            </CardContent>
          </Card>

          <GeoRadar accounts={accounts} rows={flow} />
          <AutoPlans plans={enabledPlans} activePlans={activePlans} />
        </div>
      </div>

      {/* 屏幕阅读器可读的进度摘要：canvas 本身是装饰，数字必须另有一份文本 */}
      <p className="sr-only" aria-live="polite">
        今日已完成 {doneCount} / {total} 个账号，花园绽放 {shownPct}%
      </p>
    </div>
  );
}

/** 横幅右上角的操作组（桌面端）：告警 → 分隔线 → 重播 / 补测。
 *  手机端由横幅底部那一行渲染，见 HomePage 里的 md:hidden 分支。
 *  「今日执行 N / M」不再重复渲染：标题的「今日进度 X%」就是同一个数。 */
function BannerActions({
  needsLogin,
  needsDevice,
  blockedCount,
  onReplay,
  onForceRun,
}: {
  needsLogin: number;
  needsDevice: number;
  blockedCount: number;
  onReplay: () => void;
  onForceRun: () => void;
}) {
  const hasAlert = needsLogin > 0 || blockedCount > 0 || needsDevice > 0;
  return (
    <>
      {needsLogin > 0 && (
        <Button
          size="sm"
          className="h-8 gap-1.5 rounded-lg bg-danger px-2.5 text-[11px] font-semibold text-white hover:bg-danger/90"
          asChild
        >
          <Link to="/accounts">
            <LogIn className="size-3.5" />
            {needsLogin} 个需重登
          </Link>
        </Button>
      )}
      {blockedCount > 0 && (
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5 rounded-lg border-warning-border bg-warning-soft px-2.5 text-[11px] font-semibold text-warning-text hover:bg-warning-soft/70"
          asChild
        >
          <Link to="/schedules">
            <TriangleAlert className="size-3.5" />
            {blockedCount} 个计划待处理
          </Link>
        </Button>
      )}
      {needsDevice > 0 && needsLogin === 0 && (
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5 rounded-lg border-warning-border bg-warning-soft px-2.5 text-[11px] font-semibold text-warning-text hover:bg-warning-soft/70"
          asChild
        >
          <Link to="/accounts">
            <QrCode className="size-3.5" />
            {needsDevice} 个待绑设备
          </Link>
        </Button>
      )}

      {hasAlert && <span className="h-5 w-px bg-border" />}

      <Button
        variant="outline"
        size="icon-sm"
        className="size-8 rounded-lg"
        title="重播今日开花进度动画"
        aria-label="重播开花动画"
        onClick={onReplay}
      >
        <RefreshCw className="size-3.5" />
      </Button>
      <Button size="sm" className="h-8 gap-1.5 rounded-lg px-3 text-[11px] font-semibold" onClick={onForceRun}>
        <Play className="size-3.5" />
        补测 / 重跑
      </Button>
    </>
  );
}

/** 7 日 / 14 日周期切换胶囊（原型右上角那组） */
function RangePills({
  range,
  onChange,
}: {
  range: "7d" | "14d";
  onChange: (r: "7d" | "14d") => void;
}) {
  return (
    <div className="inline-flex shrink-0 rounded-lg border border-border bg-[color:var(--subtle)] p-0.5 text-[11px]">
      {(
        [
          ["7d", "7 日"],
          ["14d", "14 日"],
        ] as const
      ).map(([key, label]) => (
        <button
          key={key}
          type="button"
          onClick={() => onChange(key)}
          aria-pressed={range === key}
          className={cn(
            "cursor-pointer rounded-md px-2.5 py-1 font-medium transition-colors",
            range === key
              ? "bg-card font-semibold text-foreground shadow-xs"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

/** 近 7 / 14 日打卡分布：周期切换 + 柱子点击切换当日明细 + 日均基准虚线 */
function DistributionCard() {
  const [range, setRange] = useState<"7d" | "14d">("7d");
  const [selected, setSelected] = useState<number | null>(null);
  const { data } = useLogTrend(range === "7d" ? 7 : 14);
  const trend = data?.trend ?? [];

  const switchRange = (next: "7d" | "14d") => {
    setRange(next);
    setSelected(null);
  };

  if (!trend.length) {
    return (
      <Card className="border-border">
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between">
            <CardTitle className="tracking-headline text-sm font-bold">近 7 日打卡分布</CardTitle>
            <RangePills range={range} onChange={switchRange} />
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex h-32 items-center justify-center text-xs text-muted-foreground">
            还没有执行记录
          </div>
        </CardContent>
      </Card>
    );
  }

  const max = Math.max(1, ...trend.map((d) => d.success + d.failed + d.pending));
  const sum = trend.reduce((n, d) => n + d.success, 0);
  const avg = sum / trend.length;
  const activeIdx = selected ?? trend.length - 1;
  const activeItem = trend[activeIdx];
  const avgPct = Math.min(100, (avg / max) * 100);
  const is14 = range === "14d";

  return (
    <Card className="border-border">
      <CardHeader className="pb-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle className="tracking-headline text-sm font-bold">
                近 {is14 ? 14 : 7} 日打卡分布
              </CardTitle>
              <span className="rounded-md bg-success-soft px-2 py-0.5 font-mono text-[11px] font-medium text-success-text">
                日均 {avg.toFixed(1)} 笔
              </span>
            </div>
            <CardDescription className="mt-0.5 text-[11px]">
              累计 {sum} 次成功 · 点击柱体查看当日执行详情
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <div className="hidden items-center gap-2.5 text-[11px] text-muted-foreground sm:flex">
              <span className="inline-flex items-center gap-1">
                <span className="size-2 rounded-[2px] bg-foreground" />
                成功
              </span>
              <span className="inline-flex items-center gap-1">
                <span className="size-2 rounded-[2px] bg-warning" />
                待核对
              </span>
              <span className="inline-flex items-center gap-1">
                <span className="size-2 rounded-[2px] bg-danger" />
                失败
              </span>
            </div>
            <RangePills range={range} onChange={switchRange} />
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="relative px-1 pt-5 pb-1">
          <div
            className="relative z-10 grid items-end gap-2"
            style={{
              gridTemplateColumns: `repeat(${trend.length}, minmax(0, 1fr))`,
              columnGap: is14 ? "4px" : "14px",
            }}
          >
            {/* 日均基准线：绝对定位在柱体区域内，按 84px 柱高的比例算，
                比挂在整张卡上更准（原先会压到最右侧柱子与标签上） */}
            <div
              className="pointer-events-none absolute inset-x-0 z-0 border-t border-dashed border-success/40"
              style={{ bottom: `${26 + (avgPct / 100) * 84}px` }}
            >
              <span className="absolute -top-4 right-0 rounded bg-success-soft px-1.5 py-0.5 font-mono text-[10px] text-success-text">
                AVG {avg.toFixed(1)}
              </span>
            </div>

            {trend.map((d, i) => {
              const total = d.success + d.failed + d.pending;
              const seg = (n: number) => `${(n / max) * 100}%`;
              return (
                <button
                  key={d.date}
                  type="button"
                  onClick={() => setSelected(i)}
                  className="group flex h-full cursor-pointer flex-col items-center justify-end focus:outline-none"
                  aria-label={`${d.date} 共 ${total} 次，成功 ${d.success}`}
                >
                  {/* 所有柱子同款：不把「选中/今天」做成绿色高亮，
                      颜色只表示成功/待核对/失败，不表示选中 */}
                  <div className="mb-1 font-mono text-[11px] font-bold text-muted-foreground group-hover:text-foreground">
                    {total}
                  </div>
                  <div
                    className={cn(
                      "flex h-[84px] w-full flex-col justify-end overflow-hidden bg-[color:var(--subtle)] transition-all",
                      is14 ? "max-w-[22px] rounded-lg p-0.5" : "max-w-[38px] rounded-xl p-1",
                    )}
                  >
                    {d.failed > 0 && (
                      <div className="w-full rounded-[2px] bg-danger" style={{ height: seg(d.failed) }} />
                    )}
                    {d.pending > 0 && (
                      <div className="w-full rounded-[2px] bg-warning" style={{ height: seg(d.pending) }} />
                    )}
                    {d.success > 0 && (
                      <div
                        className="w-full rounded-[2px] bg-foreground/85 dark:bg-foreground/80"
                        style={{ height: seg(d.success) }}
                      />
                    )}
                  </div>
                  <div className="mt-1.5 font-mono text-[11px] font-semibold text-muted-foreground">
                    {d.date.slice(8)}
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        <div className="mt-3 flex items-center justify-between gap-2 border-t border-border pt-2.5 text-[11px] text-muted-foreground">
          <span className="truncate">
            <strong className="text-foreground">{activeItem.date}</strong>：
            <strong className="ml-1 font-mono text-foreground">{activeItem.success}</strong> 成功
            {activeItem.failed > 0 && (
              <>
                {" · "}
                <strong className="font-mono text-danger">{activeItem.failed}</strong> 失败
              </>
            )}
            {activeItem.pending > 0 && (
              <>
                {" · "}
                <strong className="font-mono text-warning">{activeItem.pending}</strong> 待核对
              </>
            )}
          </span>
          <Link to="/audit" className="shrink-0 font-medium text-success hover:underline">
            查看审计 →
          </Link>
        </div>
      </CardContent>
    </Card>
  );
}

/**
 * 签到点地理安全圈。
 *
 * 圆心是学校配置的签到点，散点按账号排布（不是真实经纬度投影——那会让所有点
 * 挤在一个像素里）。悬停显示账号，点击跳到对应账号，靠位置感和「都在圈里」表达状态。
 */
function GeoRadar({
  accounts,
  rows,
}: {
  accounts: Account[];
  rows: Array<{ account: Account; done: boolean }>;
}) {
  const navigate = useNavigate();
  const [hover, setHover] = useState<string | null>(null);
  // 只取 3 个代表点：原型就是在同心圆里撒 3 个点，
  // 每账号一个点会把这圈填满、反而看不出「圈里圈外」。
  const placed = useMemo(
    () =>
      accounts.slice(0, 3).map((a, i) => ({
        account: a,
        left: [56, 32, 61][i] ?? 50,
        top: [29, 56, 67][i] ?? 50,
        ready: isReady(a),
      })),
    [accounts],
  );
  const readyAll = accounts.length > 0 && accounts.every(isReady);

  return (
    <Card className="border-border">
      <CardHeader className="flex flex-row items-center justify-between pb-2.5">
        <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-bold">
          <Crosshair className="size-4 text-muted-foreground" />
          签到点地理安全圈
        </CardTitle>
        <span className="font-mono text-[11px] text-muted-foreground">
          {hover ?? "悬停散点查看"}
        </span>
      </CardHeader>
      <CardContent>
        <div className="relative mx-auto flex aspect-square max-h-[215px] w-full items-center justify-center overflow-hidden rounded-2xl border border-border bg-[color:var(--subtle)]/60">
          {[82, 68, 52].map((s) => (
            <div
              key={s}
              className="absolute rounded-full border border-border"
              style={{ width: `${s}%`, height: `${s}%` }}
            />
          ))}
          <div className="absolute inset-x-0 h-px bg-border/70" />
          <div className="absolute inset-y-0 w-px bg-border/70" />
          {/* 中心点：黑实心 + 外圈脉冲，与原型一致 */}
          <div className="relative z-10 size-4 rounded-full bg-foreground ring-4 ring-foreground/15">
            <span className="absolute inset-0 rounded-full bg-foreground/25 pulse-dot" />
          </div>

          {placed.map((p) => (
            <button
              key={p.account.id}
              type="button"
              onMouseEnter={() => setHover(`${p.account.alias || p.account.username}`)}
              onFocus={() => setHover(`${p.account.alias || p.account.username}`)}
              onMouseLeave={() => setHover(null)}
              onBlur={() => setHover(null)}
              onClick={() => {
                navigate(`/accounts?account=${p.account.id}`);
              }}
              title={`${p.account.username} · ${p.ready ? "已配置坐标" : "缺坐标"}`}
              aria-label={`${p.account.username} ${p.ready ? "已配置坐标" : "缺坐标"}`}
              className="absolute size-2.5 -translate-x-1/2 -translate-y-1/2 cursor-pointer rounded-full transition-transform hover:scale-150 focus-visible:scale-150 focus-visible:outline-none"
              style={{
                left: `${p.left}%`,
                top: `${p.top}%`,
                background: p.ready ? "var(--success)" : "var(--warning)",
              }}
            />
          ))}

          <span className="absolute bottom-1.5 w-full text-center font-mono text-[9px] tracking-widest text-muted-foreground uppercase">
            GEOFENCE · {readyAll ? "READY" : "PARTIAL"}
          </span>
        </div>

        <div className="mt-2.5 space-y-1.5 text-xs">
          <div className="flex justify-between">
            <span className="text-muted-foreground">学校中心点</span>
            <span className="font-semibold text-foreground">
              {accounts.some((a) => a.address) ? "已配置" : "未配置"}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">坐标就绪账号</span>
            <span className="font-mono font-semibold text-foreground">
              {accounts.filter((a) => a.lon && a.lat).length} / {accounts.length}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">今日已完成</span>
            <span className="font-mono font-semibold text-foreground">
              {rows.filter((r) => r.done).length} / {accounts.length}
            </span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

/** 自动计划摘要：只显示启用中的，最多三条，避免侧栏被计划撑长 */
function AutoPlans({
  plans,
  activePlans,
}: {
  plans: Array<{ id: number; name: string; mode: string; days: number[]; windowStart: string; windowEnd: string; accountUsername?: string }>;
  activePlans: number;
}) {
  return (
    <Card className="border-border">
      <CardHeader className="flex flex-row items-center justify-between pb-2.5">
        <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-bold">
          <Timer className="size-4 text-muted-foreground" />
          自动计划
        </CardTitle>
        <Link
          to="/schedules"
          className="text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          管理 →
        </Link>
      </CardHeader>
      <CardContent className="space-y-2">
        {plans.length === 0 && (
          <p className="py-2 text-center text-[11px] text-muted-foreground">
            暂无启用计划
            <Link to="/schedules" className="ml-1 text-success hover:underline">
              去创建
            </Link>
          </p>
        )}
        {plans.slice(0, 3).map((p) => (
          <div key={p.id} className="flex items-center justify-between gap-2 text-xs">
            <span className="truncate font-medium text-foreground">
              <span className="font-mono text-muted-foreground">{p.accountUsername}</span> · {p.name}
            </span>
            <span className="shrink-0 text-[11px] text-muted-foreground">
              {p.mode === "custom" ? `${p.windowStart}-${p.windowEnd}` : describeDays(p.days)}
            </span>
          </div>
        ))}
        {activePlans > 3 && (
          <div className="pt-1 text-[11px] text-muted-foreground">另有 {activePlans - 3} 个启用中</div>
        )}
        {activePlans > 0 && (
          <div className="flex items-center gap-1.5 border-t border-border pt-2 text-[11px] text-muted-foreground">
            <GripVertical className="size-3" />
            守护进程每 30 秒扫描一次，命中时段即执行
          </div>
        )}
      </CardContent>
    </Card>
  );
}

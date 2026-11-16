// 调度计划：防呆编辑器（健康度预检 + 冲突拦截 + 日期互斥 + 规则预览）+ 防风控引擎展示 + 计划列表
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  CalendarDays,
  Clock4,
  Fingerprint,
  KeyRound,
  Lock,
  Save,
  School,
  ShieldAlert,
  ShieldCheck,
  Wand2,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { MonthCalendar } from "@/components/month-calendar";
import { PlanCard } from "@/components/plans";
import { useAccounts } from "@/hooks/use-accounts";
import { usePlans, useCreatePlan, useUpdatePlan } from "@/hooks/use-plans";
import { toast } from "@/hooks/use-toast";
import { AccountDetail, type DetailSubpage } from "@/components/account-detail";
import { describeDays } from "@/lib/task";
import { cn } from "@/lib/utils";
import type { Account, Plan, PlanInput } from "@/lib/types";

const DAY_NAMES = ["一", "二", "三", "四", "五", "六", "日"];

const WEEKDAY_PRESETS: Array<{ key: string; label: string; days: number[] }> = [
  { key: "all", label: "每天", days: [0, 1, 2, 3, 4, 5, 6] },
  { key: "schoolweek", label: "日~四晚", days: [6, 0, 1, 2, 3] },
  { key: "weekend", label: "仅周末", days: [4, 5] },
];

/** 账号是否还差前置条件：缺一项，计划就跑不起来 */
function healthIssues(a: Account) {
  const issues: Array<{
    key: string;
    title: string;
    desc: string;
    fixLabel: string;
    tab: "device" | "photos";
    sub: DetailSubpage | null;
  }> = [];
  if (a.loginStatus !== "authorized") {
    issues.push({
      key: "login",
      title: "校园会话已过期",
      desc: "执行前需刷新学校登录态，否则任务同步与提交都会失败",
      fixLabel: "去重新登录",
      tab: "device",
      sub: "login",
    });
  }
  if (!a.deviceId) {
    issues.push({
      key: "device",
      title: "未绑定设备指纹",
      desc: "缺少 CpUUID 无法构造提交头，查寝会被学校拒绝",
      fixLabel: "去扫码绑定",
      tab: "device",
      sub: "scan",
    });
  }
  if (!a.lon || !a.lat) {
    issues.push({
      key: "location",
      title: "未设置签到坐标",
      desc: "缺少 GCJ-02 经纬度，无法做围栏内的安全偏移",
      fixLabel: "去设置坐标",
      tab: "device",
      sub: "range",
    });
  }
  if (!a.photoUrl && (a.photoPoolCount ?? 0) === 0) {
    issues.push({
      key: "photo",
      title: "未上传查寝照片",
      desc: "要求照片的任务会直接失败",
      fixLabel: "去上传照片",
      tab: "photos",
      sub: null,
    });
  }
  return issues;
}

/** HH:MM → 分钟数 */
function toMinutes(v: string) {
  const [h, m] = v.split(":").map(Number);
  if (!Number.isFinite(h) || !Number.isFinite(m)) return NaN;
  return h * 60 + m;
}

export function SchedulesPage() {
  const { data: accounts } = useAccounts();
  const { data: plans, isLoading: plansLoading } = usePlans();
  const create = useCreatePlan();
  const update = useUpdatePlan();

  // 编辑器状态（editingId 为空 = 新建）
  const [editingId, setEditingId] = useState<number | null>(null);
  const [name, setName] = useState("");
  const [autoName, setAutoName] = useState(true);
  const [accountId, setAccountId] = useState("");
  const [mode, setMode] = useState<"school" | "custom">("school");
  const [windowStart, setWindowStart] = useState("21:00");
  const [windowEnd, setWindowEnd] = useState("23:00");
  const [days, setDays] = useState<number[]>([0, 1, 2, 3, 4, 5, 6]);
  const [dates, setDates] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState("");
  // 从健康度横幅跳进账号抽屉
  const [fixAccount, setFixAccount] = useState<{ account: Account; tab: "device" | "photos"; sub: DetailSubpage | null } | null>(null);

  const busy = create.isPending || update.isPending;

  const currentAccount = useMemo(
    () => (accounts ?? []).find((a) => String(a.id) === accountId) ?? null,
    [accounts, accountId],
  );
  const issues = currentAccount ? healthIssues(currentAccount) : [];
  const conflictPlan = useMemo(
    () =>
      (plans ?? []).find(
        (p) => String(p.accountId) === accountId && p.enabled && p.id !== editingId,
      ) ?? null,
    [plans, accountId, editingId],
  );

  // 时间窗跨度：结束必须晚于开始至少 5 分钟
  const windowMinutes = toMinutes(windowEnd) - toMinutes(windowStart);
  const windowInvalid = mode === "custom" && (!Number.isFinite(windowMinutes) || windowMinutes < 5);

  // 选中具体日期后星期设置挂起
  const datesOverride = dates.length > 0;
  const noCycle = !datesOverride && days.length === 0;

  const blockReason = !accountId
    ? "请选择要执行计划的账号"
    : windowInvalid
      ? "结束时间必须晚于开始时间至少 5 分钟"
      : noCycle
        ? "请至少选择一个执行星期，或在日历里指定具体日期"
        : "";

  const loadPlan = (p: Plan) => {
    setEditingId(p.id);
    setName(p.name);
    setAutoName(false);
    setAccountId(String(p.accountId));
    setMode(p.mode === "custom" ? "custom" : "school");
    setWindowStart(p.windowStart || "21:00");
    setWindowEnd(p.windowEnd || "23:00");
    setDays(p.days?.length ? p.days : [0, 1, 2, 3, 4, 5, 6]);
    setDates(p.dates ?? []);
    setEnabled(p.enabled);
    setError("");
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const resetEditor = () => {
    setEditingId(null);
    setName("");
    setAutoName(true);
    setAccountId("");
    setMode("school");
    setWindowStart("21:00");
    setWindowEnd("23:00");
    setDays([0, 1, 2, 3, 4, 5, 6]);
    setDates([]);
    setEnabled(true);
    setError("");
  };

  const toggleDay = (d: number) => {
    if (datesOverride) return;
    setDays((prev) => (prev.includes(d) ? prev.filter((x) => x !== d) : [...prev, d].sort((a, b) => a - b)));
  };

  const applyPreset = (preset: number[]) => {
    setDates([]);
    setDays(preset);
  };

  const save = () => {
    setError("");
    if (blockReason) return setError(blockReason);
    const payload: PlanInput = {
      accountId: Number(accountId),
      name: name.trim() || (currentAccount ? `${currentAccount.alias || currentAccount.username} · 查寝计划` : "查寝计划"),
      kind: "attendance",
      mode,
      windowStart,
      windowEnd,
      days: datesOverride ? [] : days,
      dates,
      enabled,
    };
    const mutation =
      editingId !== null
        ? update.mutateAsync({ id: editingId, input: payload })
        : create.mutateAsync(payload);
    mutation
      .then(() => {
        toast({
          title: editingId !== null ? "计划已更新" : "计划已创建",
          description: "到达执行窗口后由守护进程自动执行",
        });
        resetEditor();
      })
      .catch((e: Error) => setError(e.message));
  };

  return (
    <div className="view-panel space-y-5">
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
        <div>
          <h1 className="tracking-headline text-2xl font-bold">调度计划</h1>
          <p className="mt-1 text-xs text-muted-foreground">
            <span className="font-semibold text-foreground">{plans?.filter((p) => p.enabled).length ?? 0}</span> 个启用中 ·
            共 {plans?.length ?? 0} 个计划 · 内置前置健康度与互斥周期防呆校验
          </p>
        </div>
        <Button variant="outline" size="sm" className="h-8 self-start text-xs sm:self-auto" onClick={resetEditor}>
          重置表单
        </Button>
      </div>

      {accounts && accounts.length === 0 && (
        <div className="flex flex-col justify-between gap-3 rounded-2xl border border-border bg-card p-3.5 sm:flex-row sm:items-center">
          <div className="flex items-center gap-2.5 text-xs">
            <span className="h-8 w-[3px] shrink-0 rounded-full bg-warning" />
            <div>
              <div className="font-semibold">还没有可调度的账号</div>
              <div className="mt-0.5 text-[11px] text-muted-foreground">
                先在账号管理页添加账号并绑定设备，再回来创建计划。
              </div>
            </div>
          </div>
          <Button variant="outline" size="sm" className="h-8 shrink-0 text-xs" asChild>
            <Link to="/accounts?create=1">去添加账号</Link>
          </Button>
        </div>
      )}

      <div className="grid items-start gap-5 lg:grid-cols-12">
        {/* 左：防呆编辑器 (7 列) */}
        <div className="space-y-4 lg:col-span-7">
          <Card className="border-border">
            <CardHeader className="flex flex-col gap-2 border-b border-border pb-3.5 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <CardTitle className="tracking-headline text-sm font-bold">
                  {editingId !== null ? `编辑现有计划 (#${editingId})` : "新建计划"}
                </CardTitle>
                <CardDescription className="mt-0.5 text-[11px]">
                  {editingId !== null
                    ? "修改后立即生效，守护进程按新配置唤醒"
                    : "每计划每天最多成功一次，失败不占用当天机会"}
                </CardDescription>
              </div>
              <span className="shrink-0 self-start rounded-lg bg-[color:var(--subtle)] px-2.5 py-1 text-[11px] font-medium whitespace-nowrap text-muted-foreground">
                {autoName ? "名称自动生成中" : "自定义名称"}
              </span>
            </CardHeader>
            <CardContent className="space-y-5">
              {/* 执行账号 + 计划名称 */}
              <div className="grid gap-3.5 sm:grid-cols-2">
                <div>
                  <label className="mb-1.5 block text-xs font-semibold">执行账号</label>
                  <Select
                    value={accountId}
                    onValueChange={(v) => {
                      setAccountId(v);
                      const acc = (accounts ?? []).find((a) => String(a.id) === v);
                      if (acc && autoName) setName(`${acc.alias || acc.username} · 查寝计划`);
                    }}
                  >
                    <SelectTrigger className="w-full font-mono">
                      <SelectValue placeholder="选择账号" />
                    </SelectTrigger>
                    <SelectContent>
                      {(accounts ?? []).map((a) => {
                        const n = healthIssues(a).length;
                        return (
                          <SelectItem key={a.id} value={String(a.id)}>
                            {a.alias || a.username}（{maskName(a.username)}）{n > 0 ? ` · ⚠ ${n} 项待补` : " · ✓ 就绪"}
                          </SelectItem>
                        );
                      })}
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <div className="mb-1.5 flex items-center justify-between">
                    <label className="block text-xs font-semibold">计划名称</label>
                    {!autoName && (
                      <button
                        type="button"
                        onClick={() => {
                          setAutoName(true);
                          if (currentAccount) setName(`${currentAccount.alias || currentAccount.username} · 查寝计划`);
                        }}
                        className="cursor-pointer text-[11px] font-medium text-success hover:underline"
                      >
                        恢复自动命名
                      </button>
                    )}
                  </div>
                  <Input
                    value={name}
                    onChange={(e) => {
                      setName(e.target.value);
                      setAutoName(false);
                    }}
                    placeholder="如：晚间查寝计划"
                  />
                </div>
              </div>

              {/* 防呆横幅 1：账号前置健康度 */}
              {currentAccount && issues.length > 0 && (
                <div className="rounded-xl border border-warning-border bg-warning-soft p-3.5">
                  <div className="flex items-start gap-2.5">
                    <ShieldAlert className="mt-0.5 size-4 shrink-0 text-warning" />
                    <div className="min-w-0 flex-1">
                      <div className="text-xs font-bold text-warning-text">
                        前置条件不完整：{issues[0].title}
                      </div>
                      <div className="mt-0.5 text-[11px] text-warning-text opacity-90">
                        {issues[0].desc}
                        {issues.length > 1 && `（另有 ${issues.length - 1} 项待补）`}
                      </div>
                    </div>
                    <Button
                      size="sm"
                      className="h-7 shrink-0 bg-warning text-xs text-white hover:bg-warning/90"
                      onClick={() =>
                        setFixAccount({
                          account: currentAccount,
                          tab: issues[0].tab,
                          sub: issues[0].sub,
                        })
                      }
                    >
                      {issues[0].fixLabel}
                    </Button>
                  </div>
                </div>
              )}

              {/* 防呆横幅 2：同账号已启用计划的冲突 */}
              {conflictPlan && (
                <div className="flex flex-col justify-between gap-3 rounded-xl border border-border bg-[color:var(--subtle)] p-3.5 sm:flex-row sm:items-center">
                  <div className="text-xs">
                    <div className="font-bold">检测到已有启用中的计划</div>
                    <div className="mt-0.5 text-[11px] text-muted-foreground">
                      当前账号已存在「{conflictPlan.name}」（
                      {conflictPlan.mode === "custom"
                        ? `${conflictPlan.windowStart}-${conflictPlan.windowEnd}`
                        : describeDays(conflictPlan.days)}
                      ），保存会新建第二个计划。
                    </div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-7 shrink-0 text-xs"
                    onClick={() => loadPlan(conflictPlan)}
                  >
                    编辑现有计划
                  </Button>
                </div>
              )}

              {/* 执行模式单选卡 */}
              <div className="space-y-2.5">
                <label className="block text-xs font-semibold">执行模式</label>
                <div className="grid gap-3 sm:grid-cols-2">
                  <ModeCard
                    icon={School}
                    title="跟随学校时间"
                    desc="任务下发并进入可签到时段后立即执行，最贴近真实行为"
                    active={mode === "school"}
                    onClick={() => setMode("school")}
                  />
                  <ModeCard
                    icon={Clock4}
                    title="自定义时段"
                    desc="只在设定的时段内尝试执行，适合固定晚自习场景"
                    active={mode === "custom"}
                    onClick={() => setMode("custom")}
                  />
                </div>

                {mode === "custom" && (
                  <div className="space-y-2 rounded-xl border border-border bg-[color:var(--subtle)]/60 p-3.5">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2 text-xs">
                        <Input
                          type="time"
                          value={windowStart}
                          onChange={(e) => setWindowStart(e.target.value)}
                          className="w-auto bg-card px-2.5 py-1.5 font-mono"
                          aria-label="开始时间"
                        />
                        <span className="text-muted-foreground">至</span>
                        <Input
                          type="time"
                          value={windowEnd}
                          onChange={(e) => setWindowEnd(e.target.value)}
                          className="w-auto bg-card px-2.5 py-1.5 font-mono"
                          aria-label="结束时间"
                        />
                      </div>
                      <span className="font-mono text-[11px] text-muted-foreground">
                        {Number.isFinite(windowMinutes) && windowMinutes > 0
                          ? `窗口跨度 ${windowMinutes} 分钟`
                          : "时间区间异常"}
                      </span>
                    </div>
                    {windowInvalid && (
                      <div className="text-[11px] font-semibold text-danger">
                        ⚠ 结束时间必须晚于开始时间至少 5 分钟
                      </div>
                    )}
                  </div>
                )}
              </div>

              {/* 按周循环（与具体日期物理互斥挂起） */}
              <div
                className={cn(
                  "space-y-2.5 rounded-xl border border-border p-3.5 transition-opacity",
                  datesOverride && "opacity-40",
                )}
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-semibold">执行日期（按周循环）</span>
                    <span className="text-[11px] font-medium text-success">
                      {datesOverride ? "已由具体日期接管" : describeDays(days)}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[11px]">
                    {WEEKDAY_PRESETS.map((p) => (
                      <button
                        key={p.key}
                        type="button"
                        onClick={() => applyPreset(p.days)}
                        className="cursor-pointer rounded-md bg-[color:var(--subtle)] px-2 py-0.5 text-muted-foreground transition-colors hover:bg-border hover:text-foreground"
                      >
                        {p.label}
                      </button>
                    ))}
                  </div>
                </div>

                {datesOverride && (
                  <div className="flex items-center justify-between gap-2 rounded-lg border border-warning-border bg-warning-soft p-2.5 text-[11px] text-warning-text">
                    <span>当前已切换为「指定具体日期模式」，星期循环已自动挂起</span>
                    <button
                      type="button"
                      onClick={() => setDates([])}
                      className="shrink-0 cursor-pointer font-semibold underline"
                    >
                      清空日期并恢复按周
                    </button>
                  </div>
                )}

                <div className={cn("grid grid-cols-7 gap-2", datesOverride && "pointer-events-none")}>
                  {DAY_NAMES.map((label, idx) => {
                    const on = days.includes(idx);
                    return (
                      <button
                        key={label}
                        type="button"
                        onClick={() => toggleDay(idx)}
                        aria-pressed={on}
                        className={cn(
                          "cursor-pointer rounded-xl py-2 text-xs font-semibold transition-colors",
                          on
                            ? "bg-foreground text-background"
                            : "border border-border bg-[color:var(--subtle)] text-muted-foreground hover:text-foreground",
                        )}
                      >
                        {label}
                      </button>
                    );
                  })}
                </div>
                {noCycle && (
                  <div className="text-[11px] font-semibold text-danger">
                    ⚠ 请至少选择一个执行星期，或在下方日历指定具体日期
                  </div>
                )}
              </div>

              {/* 指定具体日期（过去日期由 MonthCalendar 锁死） */}
              <div className="space-y-3 rounded-xl border border-border p-3.5">
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-semibold">指定具体日期（可选）</span>
                      {dates.length > 0 && (
                        <span className="rounded-full bg-foreground px-2 py-0.5 text-[10px] font-semibold text-background">
                          已选 {dates.length} 天
                        </span>
                      )}
                    </div>
                    <p className="mt-0.5 text-[11px] text-muted-foreground">
                      适合临时补签或单次计划；过去日期已自动锁定不可选
                    </p>
                  </div>
                  <button
                    type="button"
                    onClick={() => setDates([])}
                    className="shrink-0 cursor-pointer text-[11px] text-muted-foreground hover:text-foreground"
                  >
                    留空按星期
                  </button>
                </div>
                <MonthCalendar value={dates} onChange={setDates} />
              </div>

              {/* 人类可读规则预览 */}
              <div className="rounded-xl border border-border bg-[color:var(--subtle)]/60 p-3.5 text-xs leading-relaxed text-muted-foreground">
                系统将在{" "}
                <strong className="text-foreground">
                  {datesOverride ? `指定的 ${dates.length} 个日期` : describeDays(days)}
                </strong>
                ，为账号{" "}
                <strong className="font-mono text-foreground">
                  {currentAccount ? currentAccount.username : "（未选择）"}
                </strong>{" "}
                {mode === "custom"
                  ? `在 ${windowStart} ~ ${windowEnd} 时间窗内自动探测并打卡`
                  : "检测到学校下发查寝任务后立即自动打卡"}
                。
              </div>

              {/* 启用开关 */}
              <label className="flex cursor-pointer items-center justify-between rounded-xl border border-border px-3 py-2.5 text-xs">
                <span className="flex items-center gap-2">
                  <CalendarDays className="size-3.5 text-muted-foreground" />
                  创建后立即启用
                </span>
                <Switch checked={enabled} onCheckedChange={setEnabled} />
              </label>

              {error && <p className="text-xs font-medium text-danger">{error}</p>}

              <div className="flex flex-col items-stretch justify-between gap-3 pt-1 sm:flex-row sm:items-center">
                <span className="text-[11px] font-medium text-danger">
                  {blockReason ? `⚠ ${blockReason}` : ""}
                </span>
                <Button
                  className="gap-1.5 px-5 sm:w-auto"
                  disabled={busy || (accounts?.length ?? 0) === 0 || Boolean(blockReason)}
                  onClick={save}
                >
                  <Save className="size-3.5" />
                  {busy ? "保存中…" : editingId !== null ? "保存修改" : "保存并启用计划"}
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* 防风控行为拟真引擎（参数固定，只读展示） */}
          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <div className="flex items-center justify-between">
                <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-bold">
                  <Wand2 className="size-4 text-muted-foreground" />
                  防风控行为拟真引擎
                </CardTitle>
                <Badge variant="outline" className="border-success-border bg-success-soft text-success-text">
                  <Lock className="mr-1 size-2.5" />
                  已锁定
                </Badge>
              </div>
              <CardDescription className="mt-0.5 text-[11px]">
                参数由引擎内置并锁定，模拟真实用户的提交节奏，无需手动调整
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <ReadOnlySlider percent={35} label="提交随机延迟" value="0 – 120 秒" hint="进入窗口后随机等待，避免整点准点提交" />
              <ReadOnlySlider percent={80} label="会话预热强度" value="标准" hint="执行前校验登录态与设备指纹一致性" />
              <ReadOnlySlider percent={60} label="失败重试节流" value="10 分钟" hint="失败后冷却再试，不占用当天签到机会" />
              <p className="mt-1 border-t border-border pt-2.5 text-[10px] leading-relaxed text-muted-foreground">
                引擎只读取任务窗口与设备指纹，不模拟点击轨迹；所有提交均走真实接口与真实参数。
              </p>
            </CardContent>
          </Card>
        </div>

        {/* 右：计划列表 + 凭证保密 (5 列) */}
        <div className="space-y-4 lg:col-span-5">
          <Card className="border-border">
            <CardHeader className="flex flex-row items-center justify-between pb-2.5">
              <CardTitle className="tracking-headline text-sm font-bold">计划列表</CardTitle>
              <span className="text-[11px] text-muted-foreground">实时同步至守护进程</span>
            </CardHeader>
            <CardContent className="space-y-3">
              {plansLoading ? (
                [0, 1].map((i) => <Skeleton key={i} className="h-28 rounded-xl" />)
              ) : !plans || plans.length === 0 ? (
                <p className="py-6 text-center text-xs text-muted-foreground">
                  暂无计划，在左侧创建第一个
                </p>
              ) : (
                plans.map((p) => <PlanCard key={p.id} plan={p} onEdit={loadPlan} />)
              )}
            </CardContent>
          </Card>

          {/* 凭证保密卡 */}
          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-bold">
                <KeyRound className="size-4 text-muted-foreground" />
                凭证与设备保密
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2.5 text-xs">
              <SecretRow icon={KeyRound} text="账号密码 AES-256-GCM 加密存储，仅用于自动登录" />
              <SecretRow icon={Fingerprint} text="设备指纹优先扫码授权获取，也支持手动粘贴设备 ID 与 UA" />
              <SecretRow icon={ShieldCheck} text="会话过期自动识别并静默恢复，无需人工介入" />
            </CardContent>
          </Card>
        </div>
      </div>

      {fixAccount && (
        <AccountDetail
          account={fixAccount.account}
          initialTab={fixAccount.tab}
          initialSubpage={fixAccount.sub}
          onClose={() => setFixAccount(null)}
        />
      )}
    </div>
  );
}

function maskName(v: string) {
  return v.length <= 4 ? `${v}****` : `${v.slice(0, 4)}****`;
}

/** 执行模式选择卡 */
function ModeCard({
  icon: Icon,
  title,
  desc,
  active,
  onClick,
}: {
  icon: typeof School;
  title: string;
  desc: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "cursor-pointer rounded-xl border p-3.5 text-left transition-colors",
        active
          ? "border-2 border-foreground bg-[color:var(--subtle)]/60"
          : "border border-border hover:border-border-strong",
      )}
    >
      <div className="mb-1 flex items-center justify-between">
        <span className="flex items-center gap-1.5 text-xs font-bold">
          <Icon className="size-3.5 text-muted-foreground" />
          {title}
        </span>
        <span
          className={cn(
            "size-2 rounded-full",
            active ? "bg-foreground" : "bg-border-strong",
          )}
        />
      </div>
      <p className="text-[11px] leading-relaxed text-muted-foreground">{desc}</p>
    </button>
  );
}

/** 只读滑块（参数固定展示） */
function ReadOnlySlider({
  percent,
  label,
  value,
  hint,
}: {
  percent: number;
  label: string;
  value: string;
  hint: string;
}) {
  return (
    <div>
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium">{label}</span>
        <span className="text-primary font-mono text-xs font-medium">{value}</span>
      </div>
      <div className="bg-muted relative mt-2 h-1.5 rounded-full">
        <div className="bg-primary/60 absolute inset-y-0 left-0 rounded-full" style={{ width: `${percent}%` }} />
        <span
          className="bg-primary ring-background absolute top-1/2 size-3 -translate-x-1/2 -translate-y-1/2 rounded-full ring-2"
          style={{ left: `${percent}%` }}
        />
      </div>
      <p className="text-muted-foreground mt-1.5 text-[10px]">{hint}</p>
    </div>
  );
}

function SecretRow({ icon: Icon, text }: { icon: typeof KeyRound; text: string }) {
  return (
    <div className="flex items-start gap-2">
      <Icon className="text-muted-foreground mt-0.5 size-3.5 shrink-0" />
      <span className="text-muted-foreground leading-relaxed">{text}</span>
    </div>
  );
}

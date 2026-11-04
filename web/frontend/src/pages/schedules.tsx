// 调度计划：计划编辑器（模式单选 + 7 日方块）+ 防风控引擎展示（参数固定）+ 计划列表
import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Bot,
  CalendarClock,
  CalendarDays,
  Clock4,
  Fingerprint,
  KeyRound,
  Lock,
  Plus,
  Save,
  School,
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
import { PageHeader } from "@/components/page-header";
import { EmptyBanner } from "@/components/empty-banner";
import { PlanCard } from "@/components/plans";
import { useAccounts } from "@/hooks/use-accounts";
import { usePlans, useCreatePlan, useUpdatePlan } from "@/hooks/use-plans";
import { toast } from "@/hooks/use-toast";
import { describeDays } from "@/lib/task";
import type { Plan, PlanInput } from "@/lib/types";

const DAY_NAMES = ["一", "二", "三", "四", "五", "六", "日"];

export function SchedulesPage() {
  const { data: accounts } = useAccounts();
  const { data: plans, isLoading: plansLoading } = usePlans();
  const create = useCreatePlan();
  const update = useUpdatePlan();

  // 编辑器状态（editingId 为空 = 新建）
  const [editingId, setEditingId] = useState<number | null>(null);
  const [name, setName] = useState("");
  const [accountId, setAccountId] = useState("");
  const [mode, setMode] = useState<"school" | "custom">("school");
  const [windowStart, setWindowStart] = useState("21:00");
  const [windowEnd, setWindowEnd] = useState("23:00");
  const [days, setDays] = useState<number[]>([0, 1, 2, 3, 4, 5, 6]);
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState("");

  const busy = create.isPending || update.isPending;

  const loadPlan = (p: Plan) => {
    setEditingId(p.id);
    setName(p.name);
    setAccountId(String(p.accountId));
    setMode(p.mode === "custom" ? "custom" : "school");
    setWindowStart(p.windowStart || "21:00");
    setWindowEnd(p.windowEnd || "23:00");
    setDays(p.days?.length ? p.days : [0, 1, 2, 3, 4, 5, 6]);
    setEnabled(p.enabled);
    setError("");
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const resetEditor = () => {
    setEditingId(null);
    setName("");
    setAccountId("");
    setMode("school");
    setWindowStart("21:00");
    setWindowEnd("23:00");
    setDays([0, 1, 2, 3, 4, 5, 6]);
    setEnabled(true);
    setError("");
  };

  const toggleDay = (d: number) =>
    setDays((prev) => (prev.includes(d) ? prev.filter((x) => x !== d) : [...prev, d].sort((a, b) => a - b)));

  const save = () => {
    setError("");
    if (!accountId) return setError("请选择要执行计划的账号");
    if (!days.length) return setError("请至少选择一个执行日");
    const payload: PlanInput = {
      accountId: Number(accountId),
      name: name.trim() || "查寝计划",
      kind: "attendance",
      mode,
      windowStart,
      windowEnd,
      days,
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
    <div className="space-y-5">
      <PageHeader
        title="调度计划"
        sub={plans ? `${plans.filter((p) => p.enabled).length} 个启用中 · 共 ${plans.length} 个计划` : "按学校窗口自动执行查寝"}
      />

      {accounts && accounts.length === 0 && (
        <EmptyBanner
          tone="warning"
          title="还没有可调度的账号"
          description="先在账号管理页添加账号并绑定设备，再回来创建计划。"
          action={
            <Button variant="outline" size="sm" asChild>
              <Link to="/accounts?create=1">去添加账号</Link>
            </Button>
          }
        />
      )}

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
        {/* 左：编辑器 + 防风控引擎 */}
        <div className="space-y-4">
          <Card className="border-border">
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <div>
                <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                  <CalendarClock className="text-primary size-4" />
                  {editingId !== null ? "编辑计划" : "新建计划"}
                </CardTitle>
                <CardDescription className="mt-0.5 text-xs">
                  {editingId !== null
                    ? "修改后立即生效，守护进程按新配置唤醒"
                    : "每计划每天最多成功一次，失败不占用当天机会"}
                </CardDescription>
              </div>
              {editingId !== null && (
                <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={resetEditor}>
                  取消编辑
                </Button>
              )}
            </CardHeader>
            <CardContent className="space-y-5">
              {/* 名称 + 账号 */}
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <p className="tracking-label text-muted-foreground text-[10px] font-medium uppercase">计划名称</p>
                  <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="如：晚间查寝计划" />
                </div>
                <div className="space-y-1.5">
                  <p className="tracking-label text-muted-foreground text-[10px] font-medium uppercase">执行账号</p>
                  <Select value={accountId} onValueChange={setAccountId}>
                    <SelectTrigger className="w-full">
                      <SelectValue placeholder="选择账号" />
                    </SelectTrigger>
                    <SelectContent>
                      {(accounts ?? []).map((a) => (
                        <SelectItem key={a.id} value={String(a.id)}>
                          {a.alias || a.username}（{maskName(a.username)}）
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>

              {/* 执行模式：卡片单选 */}
              <div className="space-y-1.5">
                <p className="tracking-label text-muted-foreground text-[10px] font-medium uppercase">执行模式</p>
                <div className="grid gap-2.5 sm:grid-cols-2">
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
                  <div className="grid grid-cols-2 gap-3 pt-1">
                    <div className="space-y-1.5">
                      <p className="text-muted-foreground text-[11px]">开始时间</p>
                      <Input type="time" value={windowStart} onChange={(e) => setWindowStart(e.target.value)} />
                    </div>
                    <div className="space-y-1.5">
                      <p className="text-muted-foreground text-[11px]">结束时间</p>
                      <Input type="time" value={windowEnd} onChange={(e) => setWindowEnd(e.target.value)} />
                    </div>
                  </div>
                )}
              </div>

              {/* 7 日方块 */}
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <p className="tracking-label text-muted-foreground text-[10px] font-medium uppercase">执行日期</p>
                  <span className="text-muted-foreground text-[11px]">{describeDays(days)}</span>
                </div>
                <div className="flex gap-1.5">
                  {DAY_NAMES.map((label, idx) => {
                    const on = days.includes(idx);
                    return (
                      <button
                        key={label}
                        type="button"
                        onClick={() => toggleDay(idx)}
                        className={`h-11 flex-1 rounded-md border text-xs font-medium transition-colors ${
                          on
                            ? "border-primary-border bg-primary-soft text-accent-foreground"
                            : "text-muted-foreground hover:border-border-strong border-border"
                        }`}
                      >
                        {label}
                      </button>
                    );
                  })}
                </div>
              </div>

              {/* 启用开关 */}
              <label className="border-border flex items-center justify-between rounded-md border px-3 py-2.5">
                <span className="flex items-center gap-2 text-xs">
                  <CalendarDays className="text-muted-foreground size-3.5" />
                  创建后立即启用
                </span>
                <Switch checked={enabled} onCheckedChange={setEnabled} />
              </label>

              {error && <p className="text-destructive text-xs">{error}</p>}

              <Button className="w-full gap-1.5" disabled={busy || (accounts?.length ?? 0) === 0} onClick={save}>
                <Save className="size-3.5" />
                {busy ? "保存中…" : editingId !== null ? "保存修改" : "创建计划"}
              </Button>
            </CardContent>
          </Card>

          {/* 防风控行为拟真引擎（参数固定，只读展示） */}
          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <div className="flex items-center justify-between">
                <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                  <Wand2 className="text-primary size-4" />
                  防风控行为拟真引擎
                </CardTitle>
                <Badge variant="outline" className="border-success-border bg-success-soft text-success-text">
                  <Lock className="mr-1 size-2.5" />
                  已锁定
                </Badge>
              </div>
              <CardDescription className="mt-0.5 text-xs">
                参数由引擎内置并锁定，模拟真实用户的提交节奏，无需手动调整
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <ReadOnlySlider percent={35} label="提交随机延迟" value="0 – 120 秒" hint="进入窗口后随机等待，避免整点准点提交" />
              <ReadOnlySlider percent={80} label="会话预热强度" value="标准" hint="执行前校验登录态与设备指纹一致性" />
              <ReadOnlySlider percent={60} label="失败重试节流" value="10 分钟" hint="失败后冷却再试，不占用当天签到机会" />
              <p className="text-muted-foreground border-border mt-1 border-t pt-2.5 text-[10px] leading-relaxed">
                引擎只读取任务窗口与设备指纹，不模拟点击轨迹；所有提交均走真实接口与真实参数。
              </p>
            </CardContent>
          </Card>
        </div>

        {/* 右：计划列表 + 凭证保密 */}
        <div className="space-y-4">
          <Card className="border-border">
            <CardHeader className="flex flex-row items-center justify-between pb-2.5">
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                <Bot className="text-primary size-4" />
                计划列表
              </CardTitle>
              <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs" onClick={resetEditor}>
                <Plus className="size-3" />
                新建
              </Button>
            </CardHeader>
            <CardContent className="space-y-3">
              {plansLoading ? (
                [0, 1].map((i) => <Skeleton key={i} className="h-28 rounded-lg" />)
              ) : !plans || plans.length === 0 ? (
                <p className="text-muted-foreground py-6 text-center text-xs">
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
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                <KeyRound className="text-primary size-4" />
                凭证与设备保密
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2.5 text-xs">
              <SecretRow icon={KeyRound} text="账号密码 AES-256-GCM 加密存储，仅用于自动登录" />
              <SecretRow icon={Fingerprint} text="设备指纹只经扫码授权获取，不做手动填写" />
              <SecretRow icon={ShieldCheck} text="会话过期自动识别并静默恢复，无需人工介入" />
            </CardContent>
          </Card>
        </div>
      </div>
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
      className={`rounded-lg border p-3 text-left transition-colors ${
        active
          ? "border-primary-border bg-primary-soft ring-primary/30 ring-1"
          : "hover:border-border-strong border-border"
      }`}
    >
      <div className="flex items-center gap-2">
        <span
          className={`flex size-6 items-center justify-center rounded-md ${
            active ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground"
          }`}
        >
          <Icon className="size-3.5" />
        </span>
        <span className="text-xs font-medium">{title}</span>
        {active && <span className="bg-primary ml-auto size-2 rounded-full" />}
      </div>
      <p className="text-muted-foreground mt-1.5 text-[11px] leading-relaxed">{desc}</p>
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

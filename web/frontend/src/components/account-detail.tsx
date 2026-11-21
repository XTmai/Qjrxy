// 账号详情抽屉：任务 / 计划 / 记录 / 照片 / 设置 五个 tab，设置内支持子页下钻。
//
// 形态是右侧滑出的 540px Sheet 而不是居中 Dialog：账号详情里全是要对照着看
// 的密集信息（任务窗口、指纹、坐标、照片池），横向空间比纵向更值钱；
// 抽屉也不会把背后的账号列表整页盖住，切账号时不用反复关开。
import { useEffect, useState } from "react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useConfirm } from "@/components/ui/confirm-dialog";

import {
  Bot,
  Check,
  ChevronLeft,
  ChevronRight,
  ClipboardPaste,
  Fingerprint,
  ImageIcon,
  Loader2,
  LogIn,
  MapPin,
  QrCode,
  TriangleAlert,
  X,
} from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { formatDateTime, relativeTime, taskState, taskTime } from "@/lib/task";
import { cn } from "@/lib/utils";
import { api } from "@/api";
import type { Account, AccountPhoto, RunLog, RunOutcome, Task } from "@/lib/types";
import { LoginBadge, RunStatusBadge, TaskStateBadge } from "@/components/status-badge";
import { PlansTab } from "./plans";
import { PhotoTab } from "./photo-pool";
import { DeviceScanDialog } from "./device-scan-dialog";
import { DeviceManualDialog } from "./device-manual-dialog";
import { LoginScanDialog } from "./login-scan-dialog";
import { SchoolLoginDialog } from "./school-login-dialog";
import { LocationDialog } from "./location-dialog";

export type DetailTab = "tasks" | "plans" | "records" | "photos" | "device";

/** 设置页里可以下钻的子页 */
export type DetailSubpage = "scan" | "manual" | "login" | "scan-login" | "range";

const SUBPAGE_TITLE: Record<DetailSubpage, string> = {
  scan: "重新扫码绑定设备",
  manual: "手动填写设备信息",
  login: "登录学校系统",
  "scan-login": "扫码获取登录态",
  range: "设置签到位置",
};

const TAB_LABEL: Record<DetailTab, string> = {
  tasks: "查寝任务",
  plans: "自动计划",
  records: "执行记录",
  photos: "照片",
  device: "设置",
};

interface Props {
  account: Account;
  initialTab?: DetailTab;
  /** 打开即下钻到某个设置子页（例如从调度页的健康度横幅跳过来） */
  initialSubpage?: DetailSubpage | null;
  onClose: (deleted: boolean) => void;
}

export function AccountDetail({ account, initialTab = "tasks", initialSubpage = null, onClose }: Props) {
  // 抽屉内维护账号快照：上传照片/登录/设置位置后立即刷新，无需重开或刷新页面
  const [current, setCurrent] = useState<Account>(account);
  const [tab, setTab] = useState<DetailTab>(initialTab);
  const [subpage, setSubpage] = useState<DetailSubpage | null>(initialSubpage);
  const [photos, setPhotos] = useState<AccountPhoto[]>([]);
  const [photosLoading, setPhotosLoading] = useState(true);

  const refreshAccount = async () => {
    try {
      const { account: fresh } = await api.account(account.id);
      if (fresh) setCurrent(fresh);
    } catch {
      /* 刷新失败保留旧数据 */
    }
  };

  const loadPhotos = async () => {
    try {
      const { photos: list } = await api.accountPhotos(account.id);
      setPhotos(list);
    } catch {
      /* 读取失败时保留已加载内容 */
    } finally {
      setPhotosLoading(false);
    }
  };

  useEffect(() => {
    setPhotosLoading(true);
    void loadPhotos();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [account.id]);

  const poolCount = photosLoading ? (current.photoPoolCount ?? 0) : photos.length;
  const hasPhoto = Boolean(current.photoUrl) || poolCount > 0;
  const needsLogin = current.loginStatus !== "authorized";
  const needsDevice = !current.deviceId;
  const needsPhoto = !photosLoading && !hasPhoto;

  return (
    <Sheet open onOpenChange={(o) => !o && onClose(false)}>
      <SheetContent
        side="right"
        showCloseButton={false}
        className="w-full gap-0 p-0 sm:max-w-[540px]"
      >
        {subpage ? (
          /* ── 子页头：下钻后主 Tab 让位，只留返回 + 标题 ── */
          <div className="flex shrink-0 items-center justify-between border-b border-border px-5 py-3.5">
            <Button
              variant="outline"
              size="sm"
              className="h-8 gap-1.5 rounded-xl px-3 text-xs font-semibold"
              onClick={() => setSubpage(null)}
            >
              <ChevronLeft className="size-3.5" />
              返回设置
            </Button>
            <span className="text-sm font-bold">{SUBPAGE_TITLE[subpage]}</span>
            <Button
              variant="ghost"
              size="icon"
              className="size-8 text-muted-foreground"
              aria-label="关闭抽屉"
              onClick={() => onClose(false)}
            >
              <X className="size-5" />
            </Button>
          </div>
        ) : (
          /* ── 一级头：账号身份 + 状态胶囊 + 5 段 Tab ── */
          <div className="shrink-0 border-b border-border p-5 pb-4">
            <div className="flex items-start justify-between gap-3">
              <div className="flex min-w-0 items-center gap-3.5">
                <Avatar size="lg" className="size-11 rounded-2xl">
                  <AvatarFallback className="rounded-2xl bg-[color:var(--subtle)] text-sm font-bold">
                    {(current.alias || current.username).replace(/^2024级\s*/, "").slice(0, 1).toUpperCase()}
                  </AvatarFallback>
                </Avatar>
                <div className="min-w-0">
                  <SheetTitle className="tracking-headline truncate font-mono text-lg font-bold">
                    {current.alias || current.username}
                  </SheetTitle>
                  <p className="truncate text-xs text-muted-foreground">
                    {current.schoolName} · {current.username}
                  </p>
                </div>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="size-8 shrink-0 text-muted-foreground"
                aria-label="关闭抽屉"
                onClick={() => onClose(false)}
              >
                <X className="size-5" />
              </Button>
            </div>

            {/* 关键状态：一眼看出还差什么 */}
            <div className="mt-3.5 flex flex-wrap items-center gap-1.5">
              <LoginBadge status={current.loginStatus} />
              <Badge variant={current.deviceId ? "secondary" : "warning"}>
                {current.deviceModel || (current.deviceId ? "设备已绑定" : "设备未绑定")}
              </Badge>
              <Badge variant={hasPhoto ? "secondary" : "warning"}>
                {poolCount > 0 ? `照片池 ${poolCount} 张` : current.photoUrl ? "已传照片" : "无照片"}
              </Badge>
              {current.address && <Badge variant="secondary">{current.address}</Badge>}
            </div>

            <Tabs value={tab} onValueChange={(v) => setTab(v as DetailTab)} className="mt-4">
              {/* 手机上 5 段横排每段只有 ~46px，四个汉字会折成竖排叠字；
                  窄屏改 3+2 两行，宽到 sm 再回到一行五段。 */}
              <TabsList className="grid !h-auto w-full auto-rows-fr grid-cols-3 gap-1 bg-[color:var(--subtle)] p-1 sm:grid-cols-5">
                <TabsTrigger value="tasks" className="h-auto px-1 py-1.5 text-[13px]">
                  {TAB_LABEL.tasks}
                </TabsTrigger>
                <TabsTrigger value="plans" className="h-auto px-1 py-1.5 text-[13px]">
                  {TAB_LABEL.plans}
                </TabsTrigger>
                <TabsTrigger value="records" className="h-auto px-1 py-1.5 text-[13px]">
                  {TAB_LABEL.records}
                </TabsTrigger>
                <TabsTrigger value="photos" className="h-auto px-1 py-1.5 text-[13px]">
                  {TAB_LABEL.photos}
                </TabsTrigger>
                <TabsTrigger value="device" className="h-auto px-1 py-1.5 text-[13px]">
                  {TAB_LABEL.device}
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
        )}

        {/* ── 抽屉主体：滚动区 ── */}
        <div className="thin-scrollbar min-h-0 flex-1 overflow-y-auto p-5">
          {subpage ? (
            <SubpageBody subpage={subpage} account={current} onChanged={refreshAccount} />
          ) : (
            <Tabs value={tab} onValueChange={(v) => setTab(v as DetailTab)}>
              {/* 待办清单：缺失项一行一条，点整行直达处理位置。
                  比一排小胶囊更容易扫读，也不会在窄抽屉里换行挤成一团。 */}
              {(needsLogin || needsDevice || needsPhoto) && (
                <div className="mb-4 overflow-hidden rounded-xl border border-warning-border/70">
                  <div className="flex items-center gap-1.5 border-b border-warning-border/50 bg-warning-soft/60 px-3.5 py-2">
                    <TriangleAlert className="size-3.5 shrink-0 text-warning" />
                    <span className="text-[11px] font-semibold text-warning-text">
                      还需要完成 {[needsLogin, needsDevice, needsPhoto].filter(Boolean).length} 项
                    </span>
                  </div>
                  <div className="divide-y divide-border bg-card">
                    {needsLogin && (
                      <TodoRow
                        icon={LogIn}
                        label="登录学校系统"
                        hint="登录态已失效，无法同步任务"
                        onClick={() => {
                          setTab("device");
                          setSubpage("login");
                        }}
                      />
                    )}
                    {needsDevice && (
                      <TodoRow
                        icon={Fingerprint}
                        label="扫码绑定设备"
                        hint="缺少设备指纹，提交会被拒绝"
                        onClick={() => {
                          setTab("device");
                          setSubpage("scan");
                        }}
                      />
                    )}
                    {needsPhoto && (
                      <TodoRow
                        icon={ImageIcon}
                        label="上传查寝照片"
                        hint="要求照片的任务需要至少一张"
                        onClick={() => setTab("photos")}
                      />
                    )}
                  </div>
                </div>
              )}

              <TabsContent value="tasks" className="m-0">
                <TasksTab account={current} onChanged={refreshAccount} />
              </TabsContent>
              <TabsContent value="plans" className="m-0">
                <PlansTab accountId={account.id} />
              </TabsContent>
              <TabsContent value="records" className="m-0">
                <LogsTab accountId={account.id} />
              </TabsContent>
              <TabsContent value="photos" className="m-0">
                <PhotoTab
                  account={current}
                  photos={photos}
                  loading={photosLoading}
                  onReload={loadPhotos}
                  onChanged={() => {
                    void refreshAccount();
                  }}
                />
              </TabsContent>
              <TabsContent value="device" className="m-0">
                <DeviceTab account={current} onDeleted={() => onClose(true)} onNavigate={setSubpage} />
              </TabsContent>
            </Tabs>
          )}
        </div>

        {/* ── 底部栏 ── */}
        <div className="flex shrink-0 items-center justify-between border-t border-border bg-[color:var(--subtle)]/40 p-4">
          <span className="font-mono text-[11px] text-muted-foreground">按 ESC 键关闭抽屉</span>
          <Button variant="outline" size="sm" className="h-8 px-4 text-xs" onClick={() => onClose(false)}>
            关闭
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

/* ---------------- 任务 tab ---------------- */

/** 待办清单里的一行：图标 + 标题 + 说明 + 右侧箭头，整行可点 */
function TodoRow({
  icon: Icon,
  label,
  hint,
  onClick,
}: {
  icon: typeof LogIn;
  label: string;
  hint: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full cursor-pointer items-center gap-3 px-3.5 py-2.5 text-left transition-colors hover:bg-[color:var(--subtle)]"
    >
      <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-warning-soft text-warning">
        <Icon className="size-3.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-xs font-semibold">{label}</span>
        <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{hint}</span>
      </span>
      <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
    </button>
  );
}

function TasksTab({ account, onChanged }: { account: Account; onChanged: () => void }) {
  const accountId = account.id;
  const { toast } = useToast();
  const [tasks, setTasks] = useState<Task[] | null>(null);
  const [loadError, setLoadError] = useState("");
  const [syncing, setSyncing] = useState(false);
  const [running, setRunning] = useState<string | null>(null);

  const load = async () => {
    try {
      const { tasks } = await api.tasks();
      setTasks(tasks.filter((t) => t.accountId === accountId && t.kind === "attendance"));
      setLoadError("");
    } catch (e) {
      // 吞掉异常会让界面永远停在「加载中」，必须落一个可重试的错误态
      setTasks([]);
      setLoadError(e instanceof Error ? e.message : "任务加载失败");
    }
  };

  useEffect(() => {
    // 弹窗进场动画 200ms；请求结果回来若撞上动画中段会因整树重渲染掉帧，
    // 推迟到动画结束后再拉数据（视觉上无感知差异）。
    const t = setTimeout(() => void load(), 260);
    return () => clearTimeout(t);
  }, [accountId]);

  const sync = async () => {
    setSyncing(true);
    try {
      await api.syncTasks(accountId);
      await load();
      toast({ title: "任务已更新" });
      onChanged();
    } catch (e) {
      toast({ title: "拉取失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    } finally {
      setSyncing(false);
    }
  };

  const run = async (task: Task) => {
    // 查寝任务需要照片时，缺照片提交必失败。提前拦截并引导上传。
    if (!account.photoUrl) {
      toast({
        title: "缺少签到照片",
        description: "如果该任务要求照片，提交会失败。请先到「设备」标签上传一张照片。",
        variant: "destructive",
      });
      return;
    }
    setRunning(task.signInstanceWid);
    try {
      const outcome: RunOutcome = await api.runTask(accountId, task.id, task.kind);
      if (outcome.status === "success") {
        toast({ title: "执行成功", description: outcome.message });
      } else if (outcome.status === "pending") {
        toast({ title: "待核对", description: outcome.hint || outcome.message });
      } else {
        toast({ title: "执行失败", description: outcome.hint || outcome.message, variant: "destructive" });
      }
      await load();
      onChanged();
    } catch (e) {
      // 等验证码时请求可能先断开（用户刷新/切网），但服务端仍在等他点完。
      // 这种情况不该报「执行失败」，否则用户以为白签了又去点一次。
      const pending = await api.captchaPending().catch(() => []);
      if (pending.length > 0) {
        toast({ title: "等待验证码", description: "请在弹窗中选出包含目标的图片，完成后会自动继续" });
      } else {
        toast({ title: "执行失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
      }
    } finally {
      setRunning(null);
    }
  };

  return (
    <div className="space-y-2.5 py-1">
      <Button variant="outline" size="sm" className="w-full" disabled={syncing} onClick={() => void sync()}>
        {syncing ? <Loader2 className="size-4 animate-spin" /> : "拉取今日任务"}
      </Button>
      {!tasks && (
        <p className="text-muted-foreground flex items-center justify-center gap-2 py-3 text-sm">
          <Loader2 className="size-4 animate-spin" /> 加载中…
        </p>
      )}
      {loadError && (
        <p className="rounded-lg bg-danger-soft px-3 py-2 text-xs text-danger-text">
          {loadError}
          <button type="button" className="ml-2 cursor-pointer underline" onClick={() => void load()}>
            重试
          </button>
        </p>
      )}
      {tasks && tasks.length === 0 && !loadError && (
        <p className="py-6 text-center text-sm text-muted-foreground">
          暂无查寝任务，点上方按钮从学校同步
        </p>
      )}
      {tasks?.map((t) => {
        const st = taskState(t);
        return (
          <div
            key={t.id}
            className="flex items-center gap-3 rounded-xl border border-border bg-[color:var(--subtle)]/60 p-3"
          >
            <div className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-card text-muted-foreground">
              <Bot className="size-4" />
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-semibold">{t.taskName || "查寝"}</p>
              <p className="mt-0.5 font-mono text-[11px] text-muted-foreground">{taskTime(t)}</p>
            </div>
            <TaskStateBadge state={st} />
            {/* 已完成的任务只留状态徽章，不再挂一个同样文字的禁用按钮 */}
            {st.runnable && (
              <Button size="sm" className="h-7 px-2.5 text-xs" disabled={running !== null} onClick={() => void run(t)}>
                {running === t.signInstanceWid ? <Loader2 className="size-3.5 animate-spin" /> : "执行"}
              </Button>
            )}
          </div>
        );
      })}
    </div>
  );
}

/* ---------------- 记录 tab ---------------- */

function LogsTab({ accountId }: { accountId: number }) {
  const [logs, setLogs] = useState<RunLog[] | null>(null);
  const [detail, setDetail] = useState<RunLog | null>(null);

  useEffect(() => {
    let alive = true;
    api
      .logs({ limit: 100 })
      .then((d) => {
        if (!alive) return;
        setLogs(d.logs.filter((l) => l.accountId === accountId).slice(0, 30));
      })
      .catch(() => alive && setLogs([]));
    return () => {
      alive = false;
    };
  }, [accountId]);

  return (
    <div className="space-y-2 py-1">
      {!logs && (
        <p className="text-muted-foreground flex items-center justify-center gap-2 py-3 text-sm">
          <Loader2 className="size-4 animate-spin" /> 加载中…
        </p>
      )}
      {logs && logs.length === 0 && <p className="text-muted-foreground py-6 text-center text-sm">暂无执行记录</p>}
      {logs?.map((l) => (
        <button
          key={l.id}
          type="button"
          onClick={() => setDetail(l)}
          className="flex w-full cursor-pointer items-center gap-3 rounded-xl border border-border bg-[color:var(--subtle)]/60 p-3 text-left transition-colors hover:bg-[color:var(--subtle)]"
        >
          <div className="min-w-0 flex-1">
            <p className="truncate text-xs font-semibold">{l.message || l.taskName}</p>
            <p className="mt-0.5 font-mono text-[11px] text-muted-foreground">
              {relativeTime(l.createdAt)} · {formatDateTime(l.createdAt)}
            </p>
          </div>
          <RunStatusBadge status={l.status} />
        </button>
      ))}

      <Dialog open={detail !== null} onOpenChange={(o) => !o && setDetail(null)}>
        <DialogContent className="max-w-sm">
          <DialogTitle>执行详情</DialogTitle>
          <DialogDescription>{detail?.taskName || "查寝"}</DialogDescription>
          <DialogBody className="space-y-1.5 text-sm">
            {detail && (
              <>
                <Row label="账号" value={detail.accountAlias || detail.accountUsername || "—"} />
                <Row label="状态" value={<RunStatusBadge status={detail.status} />} />
                <Row label="时间" value={formatDateTime(detail.createdAt)} />
                <Row label="结果" value={detail.message || "—"} />
                <LogDetailRows raw={detail.detail} />
              </>
            )}
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDetail(null)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="text-muted-foreground shrink-0 text-xs leading-6">{label}</span>
      <span className="min-w-0 break-all text-right">{value}</span>
    </div>
  );
}

// tgcStale 判断续期票据是否已超过主动轮换阈值（12h，与后端一致）。
// 只做展示分层用，真正的轮换决策在后端。
function tgcStale(iso: string): boolean {
  const t = Date.parse(iso);
  return Number.isFinite(t) && Date.now() - t > 12 * 3600_000;
}

// tgcAgeSuffix 拼在登录检测文案后面：一眼看出续期凭据的新鲜度。
// 有票据才显示；纯密码账号（无票据）不产生噪音。
function tgcAgeSuffix(iso?: string): string {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "";
  const desc = relativeTime(iso);
  return tgcStale(iso) ? ` · 票据已老化（${desc}轮换）` : ` · 票据新鲜（${desc}轮换）`;
}

function LogDetailRows({ raw }: { raw: string }) {
  try {
    const d = JSON.parse(raw) as Record<string, unknown>;
    const rows: React.ReactNode[] = [];
    if (d.address) rows.push(<Row key="addr" label="位置" value={String(d.address)} />);
    if (typeof d.lon === "number" && typeof d.lat === "number")
      rows.push(
        <Row
          key="coord"
          label="坐标"
          value={
            <span className="font-mono text-xs">
              {d.lon.toFixed(6)}, {d.lat.toFixed(6)}
            </span>
          }
        />,
      );
    if (typeof d.inRange === "boolean")
      rows.push(
        <Row
          key="range"
          label="在范围内"
          value={<Badge variant={d.inRange ? "success" : "error"}>{d.inRange ? "是" : "否"}</Badge>}
        />,
      );
    if (d.device)
      rows.push(
        <Row key="dev" label="设备 ID" value={<span className="font-mono text-xs">{String(d.device)}</span>} />,
      );
    if (d.error)
      rows.push(<Row key="err" label="错误" value={<span className="text-destructive">{String(d.error)}</span>} />);
    // 签到点列表：名称 + 距离/半径 + 是否在范围
    if (Array.isArray(d.places) && d.places.length > 0) {
      const places = d.places as Array<{ name: string; distance: number; radius: number; inRange: boolean }>;
      rows.push(
        <div key="places" className="border-t pt-2">
          <p className="text-muted-foreground mb-1.5 text-xs font-medium">签到点</p>
          <div className="space-y-1.5">
            {places.map((pl, i) => (
              <div key={i} className="flex items-center justify-between gap-2 text-xs">
                <span className="truncate">{pl.name || `签到点 ${i + 1}`}</span>
                <span className="text-muted-foreground shrink-0 tabular-nums">
                  {Math.round(pl.distance)}m / 半径 {Math.round(pl.radius)}m
                </span>
                <Badge variant={pl.inRange ? "success" : "error"}>{pl.inRange ? "在范围" : "范围外"}</Badge>
              </div>
            ))}
          </div>
        </div>,
      );
    }
    return rows.length ? <div className="border-t pt-2">{rows}</div> : <></>;
  } catch {
    return <></>;
  }
}

/* ---------------- 设备 tab ---------------- */

function DeviceTab({
  account,
  onDeleted,
  onNavigate,
}: {
  account: Account;
  onDeleted: () => void;
  onNavigate: (sub: DetailSubpage) => void;
}) {
  const { toast } = useToast();
  const { confirm, ConfirmDialog } = useConfirm();

  const remove = async () => {
    try {
      await api.deleteAccount(account.id);
      toast({ title: "账号已删除" });
      onDeleted();
    } catch (e) {
      toast({ title: "删除失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    }
  };

  const actions: Array<{
    key: DetailSubpage;
    title: string;
    desc: string;
    icon: typeof QrCode;
    done: boolean;
  }> = [
    {
      key: "scan",
      title: account.deviceId ? "重新扫码绑定" : "扫码绑定设备",
      desc: account.deviceId
        ? `${account.deviceModel || "已绑定"} · ${account.platform === "ios" ? "iOS" : "Android"}`
        : "用平时打卡的手机扫一扫，自动获取设备信息",
      icon: QrCode,
      done: Boolean(account.deviceId),
    },
    {
      key: "manual",
      title: "手动填写设备信息",
      desc: account.deviceId
        ? `当前 ${account.deviceId.slice(0, 8)}… · 可粘贴设备 ID 与 UA`
        : "扫码拿不到时，手动粘贴设备 ID / UA 等字段",
      icon: ClipboardPaste,
      done: false,
    },
    {
      key: "login",
      title: "登录学校系统",
      desc:
        account.loginStatus === "authorized"
          ? account.lastProbeAt
            ? `已登录 · ${relativeTime(account.lastProbeAt)}检测${tgcAgeSuffix(account.tgcRefreshedAt)}`
            : "已登录"
          : "选择账号密码或短信验证码登录",
      icon: LogIn,
      done: account.loginStatus === "authorized",
    },
    {
      key: "scan-login",
      title: "扫码获取登录态",
      desc: "用已登录今日校园的手机扫码，免密码换出学校会话",
      icon: QrCode,
      done: false,
    },
    {
      key: "range",
      title: "设置签到位置",
      desc: account.address
        ? account.address
        : account.lon && account.lat
          ? `${account.lon.toFixed(6)}, ${account.lat.toFixed(6)}`
          : "填写高德坐标，或从学校签到任务读取",
      icon: MapPin,
      done: Boolean(account.address || (account.lon && account.lat)),
    },
  ];

  return (
    <div className="space-y-2.5 py-1">
      {/* 操作列表：每项说明状态 + 点击下钻到子页 */}
      <div className="space-y-2.5">
        {actions.map((a) => (
          <button
            key={a.key}
            type="button"
            onClick={() => onNavigate(a.key)}
            className="flex w-full items-center justify-between gap-3 rounded-xl border border-border bg-[color:var(--subtle)]/60 p-3.5 text-left transition-colors hover:bg-[color:var(--subtle)]"
          >
            <span className="flex min-w-0 items-center gap-3">
              <span
                className={cn(
                  "flex size-8 shrink-0 items-center justify-center rounded-lg",
                  a.done
                    ? "bg-success-soft text-success"
                    : "bg-card text-muted-foreground",
                )}
              >
                <a.icon className="size-4" />
              </span>
              <span className="min-w-0">
                <span className="block text-sm font-bold">{a.title}</span>
                <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">
                  {a.desc}
                </span>
              </span>
            </span>
            {a.done ? (
              <Check className="size-4 shrink-0 text-success" />
            ) : (
              <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
            )}
          </button>
        ))}
      </div>

      <Button
        variant="ghost"
        size="sm"
        className="text-destructive mt-2 w-full"
        onClick={async () => {
          const ok = await confirm({
            title: "删除账号",
            description: `确定删除「${account.alias || account.username}」吗？该账号的计划与执行记录会一并删除，且无法恢复。`,
            confirmText: "删除",
            variant: "destructive",
          });
          if (ok) void remove();
        }}
      >
        删除账号
      </Button>

      {ConfirmDialog}
    </div>
  );
}

/**
 * 设置子页的落地内容。
 *
 * 每个子页只做一件事，进入后主 Tab 消失、只留返回按钮——比在抽屉里再叠一层
 * 弹窗干净得多，也避免「关掉弹窗发现连抽屉一起关了」的经典翻车。
 */
function SubpageBody({
  subpage,
  account,
  onChanged,
}: {
  subpage: DetailSubpage;
  account: Account;
  onChanged: () => void;
}) {
  const { toast } = useToast();

  if (subpage === "login") {
    return (
      <SchoolLoginDialog
        account={account}
        onClose={(loggedIn) => {
          if (loggedIn) {
            toast({ title: "登录成功" });
            onChanged();
          }
        }}
        embedded
      />
    );
  }

  if (subpage === "range") {
    return (
      <LocationDialog
        account={account}
        onClose={(saved) => {
          if (saved) {
            toast({ title: "位置已保存" });
            onChanged();
          }
        }}
        embedded
      />
    );
  }

  if (subpage === "manual") {
    return (
      <DeviceManualDialog
        account={account}
        onClose={(changed) => {
          if (changed) onChanged();
        }}
        embedded
      />
    );
  }

  if (subpage === "scan") {
    return (
      <DeviceScanDialog
        accountId={account.id}
        onClose={(collected) => {
          if (collected) {
            toast({ title: "设备已绑定" });
            onChanged();
          }
        }}
        embedded
      />
    );
  }

  return (
    <LoginScanDialog
      accountId={account.id}
      onClose={(collected) => {
        if (collected) {
          toast({ title: "登录态已更新" });
          onChanged();
        }
      }}
      embedded
    />
  );
}

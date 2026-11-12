// 账号详情：任务 / 计划 / 记录 / 照片 / 设置 五个 tab（shadcn 版）。
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
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useConfirm } from "@/components/ui/confirm-dialog";

import {
  Bot,
  Check,
  ChevronRight,
  ClipboardPaste,
  Loader2,
  LogIn,
  MapPin,
  QrCode,
} from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { formatDateTime, relativeTime, taskState, taskTime } from "@/lib/task";
import { api } from "@/api";
import type { Account, AccountPhoto, RunLog, RunOutcome, Task } from "@/lib/types";
import { LoginBadge, RunStatusBadge, TaskStateBadge } from "@/components/status-badge";
import { PlansTab } from "./plans";
import { PhotoTab } from "./photo-pool";
import { DeviceScanDialog } from "./device-scan-dialog";
import { DeviceManualDialog } from "./device-manual-dialog";
import { SchoolLoginDialog } from "./school-login-dialog";
import { LocationDialog } from "./location-dialog";

export type DetailTab = "tasks" | "plans" | "records" | "photos" | "device";

interface Props {
  account: Account;
  initialTab?: DetailTab;
  onClose: (deleted: boolean) => void;
}

export function AccountDetail({ account, initialTab = "tasks", onClose }: Props) {
  // 弹窗内维护账号快照：上传照片/登录/设置位置后立即刷新，无需重开弹窗或刷新页面
  const [current, setCurrent] = useState<Account>(account);
  const [tab, setTab] = useState<DetailTab>(initialTab);
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
    <Dialog open onOpenChange={(o) => !o && onClose(false)}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <Avatar size="lg">
              <AvatarFallback className="bg-primary/10 text-primary text-sm font-bold">
                {(current.alias || current.username).slice(0, 1)}
              </AvatarFallback>
            </Avatar>
            <div className="min-w-0 flex-1">
              <DialogTitle className="truncate">{current.alias || current.username}</DialogTitle>
              <DialogDescription className="truncate">
                {current.schoolName} · {current.username}
              </DialogDescription>
            </div>
          </div>
          {/* 关键状态：一眼看出还差什么 */}
          <div className="mt-3 flex flex-wrap gap-1.5">
            <LoginBadge status={current.loginStatus} />
            <Badge variant={current.deviceId ? "secondary" : "warning"}>
              {current.deviceId ? current.deviceModel || "设备已绑定" : "设备未绑定"}
            </Badge>
            <Badge variant={hasPhoto ? "secondary" : "warning"}>
              {poolCount > 0 ? `照片池 ${poolCount} 张` : current.photoUrl ? "已传照片" : "无照片"}
            </Badge>
            {current.address && <Badge variant="secondary">{current.address}</Badge>}
          </div>
        </DialogHeader>

        {/* 待办：只列缺失项，点一下即处理 */}
        {(needsLogin || needsDevice || needsPhoto) && (
          <div className="bg-warning/10 border-warning/25 -mt-1 rounded-lg border px-3 py-2.5">
            <p className="text-warning-strong text-[11px] font-medium">还需要完成</p>
            <div className="mt-1.5 flex flex-wrap gap-2">
              {needsLogin && (
                <Button size="sm" variant="outline" className="h-7 px-2.5 text-xs" onClick={() => setTab("device")}>
                  登录账号
                </Button>
              )}
              {needsDevice && (
                <Button size="sm" variant="outline" className="h-7 px-2.5 text-xs" onClick={() => setTab("device")}>
                  扫码绑定设备
                </Button>
              )}
              {needsPhoto && (
                <Button size="sm" variant="outline" className="h-7 px-2.5 text-xs" onClick={() => setTab("photos")}>
                  上传照片
                </Button>
              )}
            </div>
          </div>
        )}

        <Tabs value={tab} onValueChange={(v) => setTab(v as DetailTab)}>
          <TabsList className="bg-muted/50 w-full p-1">
            <TabsTrigger value="tasks" className="flex-1 px-2 text-[13px] sm:px-3 sm:text-sm">
              查寝任务
            </TabsTrigger>
            <TabsTrigger value="plans" className="flex-1 px-2 text-[13px] sm:px-3 sm:text-sm">
              自动计划
            </TabsTrigger>
            <TabsTrigger value="records" className="flex-1 px-2 text-[13px] sm:px-3 sm:text-sm">
              执行记录
            </TabsTrigger>
            <TabsTrigger value="photos" className="flex-1 px-2 text-[13px] sm:px-3 sm:text-sm">
              照片
            </TabsTrigger>
            <TabsTrigger value="device" className="flex-1 px-2 text-[13px] sm:px-3 sm:text-sm">
              设置
            </TabsTrigger>
          </TabsList>

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
            <DeviceTab account={current} onChanged={refreshAccount} onDeleted={() => onClose(true)} />
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onClose(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/* ---------------- 任务 tab ---------------- */

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
    <div className="space-y-3 py-3">
      <Button variant="outline" size="sm" className="w-full" disabled={syncing} onClick={() => void sync()}>
        {syncing ? <Loader2 className="size-4 animate-spin" /> : "拉取今日任务"}
      </Button>
      {!tasks && (
        <p className="text-muted-foreground flex items-center justify-center gap-2 py-3 text-sm">
          <Loader2 className="size-4 animate-spin" /> 加载中…
        </p>
      )}
      {loadError && (
        <p className="text-danger-text bg-danger-soft rounded-lg px-3 py-2 text-xs">
          {loadError}
          <button type="button" className="ml-2 underline" onClick={() => void load()}>
            重试
          </button>
        </p>
      )}
      {tasks && tasks.length === 0 && !loadError && (
        <p className="text-muted-foreground py-4 text-center text-sm">暂无查寝任务，点上方按钮从学校同步</p>
      )}
      {tasks?.map((t) => {
        const st = taskState(t);
        return (
          <div key={t.id} className="flex items-center gap-3 rounded-xl border p-3">
            <div className="bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-lg">
              <Bot className="size-4" />
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">{t.taskName || "查寝"}</p>
              <p className="text-muted-foreground mt-0.5 text-xs">{taskTime(t)}</p>
            </div>
            <TaskStateBadge state={st} />
            <Button
              variant={st.runnable ? "default" : "ghost"}
              size="sm"
              disabled={!st.runnable || running !== null}
              title={st.runnable ? "" : st.text}
              onClick={() => void run(t)}
            >
              {running === t.signInstanceWid ? (
                <Loader2 className="size-4 animate-spin" />
              ) : st.runnable ? (
                "执行"
              ) : (
                st.text
              )}
            </Button>
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
    <div className="space-y-2 py-3">
      {!logs && (
        <p className="text-muted-foreground flex items-center justify-center gap-2 py-3 text-sm">
          <Loader2 className="size-4 animate-spin" /> 加载中…
        </p>
      )}
      {logs && logs.length === 0 && <p className="text-muted-foreground py-4 text-center text-sm">暂无执行记录</p>}
      {logs?.map((l) => (
        <button
          key={l.id}
          type="button"
          onClick={() => setDetail(l)}
          className="hover:bg-muted flex w-full items-center gap-3 rounded-xl border p-3 text-left transition-colors"
        >
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm">{l.message || l.taskName}</p>
            <p className="text-muted-foreground mt-0.5 text-xs">
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
  onChanged,
  onDeleted,
}: {
  account: Account;
  onChanged: () => void;
  onDeleted: () => void;
}) {
  const { toast } = useToast();
  const { confirm, ConfirmDialog } = useConfirm();
  const [showScan, setShowScan] = useState(false);
  const [showManual, setShowManual] = useState(false);
  const [showLogin, setShowLogin] = useState(false);
  const [showLocation, setShowLocation] = useState(false);

  const remove = async () => {
    try {
      await api.deleteAccount(account.id);
      toast({ title: "账号已删除" });
      onDeleted();
    } catch (e) {
      toast({ title: "删除失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    }
  };

  const actions = [
    {
      key: "scan",
      title: account.deviceId ? "重新扫码绑定" : "扫码绑定设备",
      desc: account.deviceId
        ? `${account.deviceModel || "已绑定"} · ${account.platform === "ios" ? "iOS" : "Android"}`
        : "用平时打卡的手机扫一扫，自动获取设备信息",
      icon: QrCode,
      done: Boolean(account.deviceId),
      run: () => setShowScan(true),
    },
    {
      key: "manual",
      title: "手动填写设备信息",
      desc: account.deviceId
        ? `当前 ${account.deviceId.slice(0, 8)}… · 可粘贴设备 ID 与 UA`
        : "扫码拿不到时，手动粘贴设备 ID / UA 等字段",
      icon: ClipboardPaste,
      done: false,
      run: () => setShowManual(true),
    },
    {
      key: "login",
      title: "登录学校系统",
      desc:
        account.loginStatus === "authorized"
          ? account.lastProbeAt
            ? `已登录 · ${relativeTime(account.lastProbeAt)}检测`
            : "已登录"
          : "选择账号密码或短信验证码登录",
      icon: LogIn,
      done: account.loginStatus === "authorized",
      run: () => setShowLogin(true),
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
      run: () => setShowLocation(true),
    },
  ];

  return (
    <div className="space-y-3 py-3">
      {/* 操作列表：每项说明状态 + 点击即执行 */}
      <div className="divide-y divide-border overflow-hidden rounded-lg border border-border">
        {actions.map((a) => (
          <button
            key={a.key}
            type="button"
            onClick={a.run}
            className="hover:bg-muted/50 flex w-full items-center gap-3 px-3.5 py-3 text-left transition-colors"
          >
            <span
              className={`flex size-8 shrink-0 items-center justify-center rounded-lg ${
                a.done ? "bg-success/12 text-success" : "bg-muted text-muted-foreground"
              }`}
            >
              <a.icon className="size-4" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-sm font-medium">{a.title}</span>
              <span className="text-muted-foreground block truncate text-xs">{a.desc}</span>
            </span>
            {a.done ? (
              <Check className="size-4 shrink-0 text-success" />
            ) : (
              <ChevronRight className="text-muted-foreground/50 size-4 shrink-0" />
            )}
          </button>
        ))}
      </div>

      {/* 照片已迁到独立的「照片」tab：池 + 单张兜底 */}

      <Button
        variant="ghost"
        size="sm"
        className="text-destructive w-full"
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

      {showLogin && (
        <SchoolLoginDialog
          account={account}
          onClose={(loggedIn) => {
            setShowLogin(false);
            if (loggedIn) {
              toast({ title: "登录成功" });
              onChanged();
            }
          }}
        />
      )}

      {showLocation && (
        <LocationDialog
          account={account}
          onClose={(saved) => {
            setShowLocation(false);
            if (saved) {
              toast({ title: "位置已保存" });
              onChanged();
            }
          }}
        />
      )}

      {showManual && (
        <DeviceManualDialog
          account={account}
          onClose={(changed) => {
            setShowManual(false);
            if (changed) onChanged();
          }}
        />
      )}

      {showScan && (
        <DeviceScanDialog
          accountId={account.id}
          onClose={(collected) => {
            setShowScan(false);
            if (collected) {
              toast({ title: "设备已绑定" });
              onChanged();
            }
          }}
        />
      )}

      {ConfirmDialog}
    </div>
  );
}

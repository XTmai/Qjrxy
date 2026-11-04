// 账号详情：任务 / 计划 / 记录 / 设备 四个 tab（shadcn 版）。
import { useEffect, useRef, useState } from "react";
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

import { Bot, Check, ChevronRight, ImageIcon, Loader2, LogIn, MapPin, QrCode } from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { formatDateTime, relativeTime, taskState, taskTime } from "@/lib/task";
import { api } from "@/api";
import type { Account, RunLog, RunOutcome, Task } from "@/lib/types";
import { LoginBadge, RunStatusBadge, TaskStateBadge } from "@/components/status-badge";
import { PlansTab } from "./plans";
import { DeviceScanDialog } from "./device-scan-dialog";

interface Props {
  account: Account;
  onClose: (deleted: boolean) => void;
}

export function AccountDetail({ account, onClose }: Props) {
  const [deleted, setDeleted] = useState(false);
  // 弹窗内维护账号快照：上传照片/登录/设置位置后立即刷新，无需重开弹窗或刷新页面
  const [current, setCurrent] = useState<Account>(account);
  const [tab, setTab] = useState("tasks");

  const refreshAccount = async () => {
    try {
      const { account: fresh } = await api.account(account.id);
      if (fresh) setCurrent(fresh);
    } catch {
      /* 刷新失败保留旧数据 */
    }
    setDeleted(true);
  };

  const needsLogin = current.loginStatus !== "authorized";
  const needsDevice = !current.deviceId;
  const needsPhoto = !current.photoUrl;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose(deleted)}>
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
            <Badge variant={current.photoUrl ? "secondary" : "warning"}>
              {current.photoUrl ? "已传照片" : "无照片"}
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
                <Button size="sm" variant="outline" className="h-7 px-2.5 text-xs" onClick={() => setTab("device")}>
                  上传照片
                </Button>
              )}
            </div>
          </div>
        )}

        <Tabs value={tab} onValueChange={setTab}>
          <TabsList className="bg-muted/50 w-full p-1">
            <TabsTrigger value="tasks" className="flex-1">
              查寝任务
            </TabsTrigger>
            <TabsTrigger value="plans" className="flex-1">
              自动计划
            </TabsTrigger>
            <TabsTrigger value="records" className="flex-1">
              执行记录
            </TabsTrigger>
            <TabsTrigger value="device" className="flex-1">
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
          <TabsContent value="device" className="m-0">
            <DeviceTab account={current} onChanged={refreshAccount} onDeleted={() => onClose(true)} />
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onClose(deleted)}>
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
  const [syncing, setSyncing] = useState(false);
  const [running, setRunning] = useState<string | null>(null);

  const load = async () => {
    const { tasks } = await api.tasks();
    setTasks(tasks.filter((t) => t.accountId === accountId && t.kind === "attendance"));
  };

  useEffect(() => {
    void load();
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
      toast({ title: "执行失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
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
      {tasks && tasks.length === 0 && (
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
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [photoVersion, setPhotoVersion] = useState(Date.now());

  const relLogin = async () => {
    setBusy(true);
    try {
      const ch = await api.accountLogin(account.id);
      if (ch.done) {
        toast({ title: "登录成功" });
        onChanged();
      } else if (ch.needCaptcha) {
        toast({ title: "学校要求验证码", description: "请通过「任务」页操作，或稍后再试" });
      }
    } catch (e) {
      toast({ title: "登录失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const applyRange = async () => {
    setBusy(true);
    try {
      const res = await api.applySignRange(account.id);
      toast({ title: "位置已设置", description: res.range.address || undefined });
      onChanged();
    } catch (e) {
      toast({ title: "获取位置失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const uploadPhoto = async (file: File) => {
    setBusy(true);
    try {
      const fd = new FormData();
      fd.append("photo", file);
      await api.uploadPhoto(account.id, fd);
      setPhotoVersion(Date.now()); // 强制刷新 img 缓存
      toast({ title: "照片已上传", description: "已生效，可直接执行查寝" });
      onChanged();
    } catch (e) {
      toast({ title: "上传失败", description: e instanceof Error ? e.message : undefined, variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

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
      key: "login",
      title: "账密登录",
      desc: account.lastProbeAt
        ? `${relativeTime(account.lastProbeAt)}检测 · ${account.lastProbeMsg || ""}`
        : "用保存的密码登录学校系统",
      icon: LogIn,
      done: account.loginStatus === "authorized",
      run: () => void relLogin(),
    },
    {
      key: "range",
      title: "自动获取位置",
      desc: account.address || "从学校签到任务读取签到点",
      icon: MapPin,
      done: Boolean(account.address),
      run: () => void applyRange(),
    },
    {
      key: "photo",
      title: account.photoUrl ? "更换签到照片" : "上传签到照片",
      desc: account.photoUrl ? "已上传，查寝时自动使用" : "查寝要求照片时必须上传",
      icon: ImageIcon,
      done: Boolean(account.photoUrl),
      run: () => fileRef.current?.click(),
    },
  ];

  return (
    <div className="space-y-3 py-3">
      <input
        ref={fileRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void uploadPhoto(f);
          e.target.value = "";
        }}
      />

      {/* 操作列表：每项说明状态 + 点击即执行 */}
      <div className="divide-y divide-border overflow-hidden rounded-lg border border-border">
        {actions.map((a) => (
          <button
            key={a.key}
            type="button"
            disabled={busy}
            onClick={a.run}
            className="hover:bg-muted/50 flex w-full items-center gap-3 px-3.5 py-3 text-left transition-colors disabled:opacity-50"
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

      {/* 照片预览 */}
      {account.photoUrl && (
        <div className="border-border flex gap-3 rounded-lg border p-3">
          <img
            src={`${account.photoUrl}?t=${photoVersion}`}
            alt="签到照片"
            className="size-20 shrink-0 rounded-lg border object-cover"
          />
          <div className="min-w-0 flex-1">
            <p className="text-xs font-medium">签到照片</p>
            <p className="text-muted-foreground mt-0.5 text-[11px]">查寝提交时自动上传到学校</p>
          </div>
        </div>
      )}

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

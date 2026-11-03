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

import { Loader2 } from "lucide-react";
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

  return (
    <Dialog open onOpenChange={(o) => !o && onClose(deleted)}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <Avatar size="lg">
              <AvatarFallback className="bg-primary/10 text-primary text-sm font-bold">
                {(account.alias || account.username).slice(0, 1)}
              </AvatarFallback>
            </Avatar>
            <div>
              <DialogTitle>{account.alias || account.username}</DialogTitle>
              <DialogDescription>
                {account.schoolName} · {account.username}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <Tabs defaultValue="tasks">
          <TabsList className="w-full">
            <TabsTrigger value="tasks" className="flex-1">
              任务
            </TabsTrigger>
            <TabsTrigger value="plans" className="flex-1">
              计划
            </TabsTrigger>
            <TabsTrigger value="logs" className="flex-1">
              记录
            </TabsTrigger>
            <TabsTrigger value="device" className="flex-1">
              设备
            </TabsTrigger>
          </TabsList>

          <TabsContent value="tasks">
            <TasksTab accountId={account.id} onChanged={() => setDeleted(true)} />
          </TabsContent>
          <TabsContent value="plans">
            <PlansTab accountId={account.id} />
          </TabsContent>
          <TabsContent value="logs">
            <LogsTab accountId={account.id} />
          </TabsContent>
          <TabsContent value="device">
            <DeviceTab account={account} onChanged={() => setDeleted(true)} onDeleted={() => onClose(true)} />
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

function TasksTab({ accountId, onChanged }: { accountId: number; onChanged: () => void }) {
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
            <p className="text-muted-foreground mt-0.5 text-xs">{formatDateTime(l.createdAt)}</p>
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
      toast({ title: "照片已上传" });
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

  return (
    <div className="space-y-4 py-3">
      <div className="space-y-1.5 text-sm">
        <div className="flex items-center justify-between">
          <span className="text-muted-foreground text-xs">登录状态</span>
          <LoginBadge status={account.loginStatus} />
        </div>
        <div className="flex items-center justify-between">
          <span className="text-muted-foreground text-xs">最近探测</span>
          <span className="text-xs">
            {account.lastProbeAt ? relativeTime(account.lastProbeAt) : "—"} · {account.lastProbeMsg || ""}
          </span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-muted-foreground text-xs">设备</span>
          <span className="text-xs">
            {account.deviceId
              ? `${account.deviceModel || "已绑定"} · ${account.platform === "ios" ? "iOS" : "Android"}`
              : "未绑定"}
          </span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-muted-foreground text-xs">位置</span>
          <span className="text-xs">{account.address || "未设置"}</span>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-2">
        <Button variant="outline" size="sm" onClick={() => setShowScan(true)}>
          {account.deviceId ? "重新扫码" : "扫码绑定设备"}
        </Button>
        <Button variant="outline" size="sm" disabled={busy} onClick={() => void relLogin()}>
          账密登录
        </Button>
        <Button variant="outline" size="sm" disabled={busy} onClick={() => void applyRange()}>
          自动获取位置
        </Button>
        <Button variant="outline" size="sm" disabled={busy} onClick={() => fileRef.current?.click()}>
          上传照片
        </Button>
      </div>

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

      {account.photoUrl && (
        <div>
          <p className="text-muted-foreground mb-2 text-xs">签到照片</p>
          <img src={account.photoUrl} alt="签到照片" className="max-h-40 rounded-xl border object-cover" />
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

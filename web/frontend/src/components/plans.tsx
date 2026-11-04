// 计划：卡片列表 + 新建/编辑表单（shadcn 版）。
import { useState } from "react";

import { useToast } from "@/hooks/use-toast";
import { usePlans } from "@/hooks/use-plans";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useCreatePlan, useDeletePlan, useResetPlan, useTogglePlan, useUpdatePlan } from "@/hooks/use-plans";
import { describeDays, formatDateTime } from "@/lib/task";
import type { Plan, PlanInput } from "@/lib/types";

const DAY_NAMES = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];

export function PlanCard({ plan, onEdit }: { plan: Plan; onEdit?: (plan: Plan) => void }) {
  const { toast } = useToast();
  const toggle = useTogglePlan();
  const reset = useResetPlan();
  const del = useDeletePlan();

  const today = new Date();
  const todayStr = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")}`;
  const ranToday = Boolean(plan.lastRunAt) && plan.lastRunAt.startsWith(todayStr);

  return (
    <div className="rounded-xl border p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">{plan.name}</p>
          <p className="text-muted-foreground mt-0.5 text-xs">
            {plan.accountAlias || plan.accountUsername || `账号 ${plan.accountId}`}
          </p>
        </div>
        <Switch
          checked={plan.enabled}
          disabled={toggle.isPending || del.isPending}
          onCheckedChange={(checked: boolean) =>
            toggle.mutate(
              { id: plan.id, enabled: checked },
              {
                onSuccess: () => toast({ title: checked ? "计划已启用" : "计划已暂停" }),
                onError: (e) => toast({ title: "操作失败", description: e.message }),
              },
            )
          }
          aria-label={plan.enabled ? "暂停计划" : "启用计划"}
        />
      </div>

      <div className="mt-3 space-y-1.5 text-xs">
        <p>
          <span className="text-muted-foreground inline-block w-16">执行方式</span>
          {plan.mode === "school" ? "跟随学校下发时间" : `自定义时段 ${plan.windowStart} – ${plan.windowEnd}`}
        </p>
        <p>
          <span className="text-muted-foreground inline-block w-16">执行日期</span>
          {describeDays(plan.days)}
        </p>
        <p>
          <span className="text-muted-foreground inline-block w-16">最近执行</span>
          {plan.lastRunAt ? formatDateTime(plan.lastRunAt) : "尚未执行"}
        </p>
        {plan.lastResult && (
          <p>
            <span className="text-muted-foreground inline-block w-16">执行结果</span>
            {plan.lastResult}
          </p>
        )}
      </div>

      <div className="mt-3 flex gap-2">
        {onEdit && (
          <Button variant="ghost" size="sm" onClick={() => onEdit(plan)}>
            编辑
          </Button>
        )}
        {ranToday && (
          <Button
            variant="outline"
            size="sm"
            disabled={reset.isPending}
            title="清除今天已执行的状态，让计划当天重新尝试"
            onClick={() =>
              reset.mutate(plan.id, {
                onSuccess: () => toast({ title: "已重置", description: "计划会在下个周期重新检查" }),
                onError: (e) => toast({ title: "重置失败", description: e.message }),
              })
            }
          >
            重置今日
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          className="text-destructive"
          disabled={del.isPending}
          onClick={() =>
            del.mutate(plan.id, {
              onSuccess: () => toast({ title: "计划已删除" }),
              onError: (e) => toast({ title: "删除失败", description: e.message }),
            })
          }
        >
          删除
        </Button>
      </div>
    </div>
  );
}

export function PlanFormDialog({
  accountId,
  existing,
  onClose,
  onSaved,
}: {
  accountId: number;
  existing: Plan | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { toast } = useToast();
  const isEdit = existing !== null;
  const create = useCreatePlan();
  const update = useUpdatePlan();
  const [name, setName] = useState(existing?.name ?? "");
  const [mode, setMode] = useState<"school" | "custom">(existing?.mode === "custom" ? "custom" : "school");
  const [windowStart, setWindowStart] = useState(existing?.windowStart ?? "21:00");
  const [windowEnd, setWindowEnd] = useState(existing?.windowEnd ?? "23:00");
  const [days, setDays] = useState<number[]>(existing?.days ?? [0, 1, 2, 3, 4, 5, 6]);
  const [enabled, setEnabled] = useState(existing?.enabled ?? true);
  const [error, setError] = useState("");

  const toggleDay = (d: number) => {
    setDays((prev) => (prev.includes(d) ? prev.filter((x) => x !== d) : [...prev, d].sort((a, b) => a - b)));
  };

  const save = () => {
    setError("");
    if (!days.length) {
      setError("请至少选择一天");
      return;
    }
    const payload: PlanInput = {
      accountId,
      name: name.trim() || "查寝计划",
      kind: "attendance",
      mode,
      windowStart,
      windowEnd,
      days,
      enabled,
    };
    const mutation = isEdit ? update.mutateAsync({ id: existing.id, input: payload }) : create.mutateAsync(payload);
    mutation
      .then(() => {
        toast({ title: isEdit ? "计划已更新" : "计划已创建" });
        onSaved();
        onClose();
      })
      .catch((e: Error) => setError(e.message));
  };

  const busy = create.isPending || update.isPending;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogTitle>{isEdit ? "编辑计划" : "新建计划"}</DialogTitle>
        <DialogDescription>按设定时间自动执行查寝，每计划每天最多成功一次</DialogDescription>
        <DialogBody className="space-y-4">
          <Field>
            <FieldLabel>计划名称</FieldLabel>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="如：晚间查寝计划" />
          </Field>

          <Field>
            <FieldLabel>执行方式</FieldLabel>
            <div className="bg-muted grid grid-cols-2 gap-1 rounded-lg p-1">
              {(
                [
                  ["school", "跟随学校时间"],
                  ["custom", "自定义时段"],
                ] as const
              ).map(([key, label]) => (
                <button
                  key={key}
                  type="button"
                  onClick={() => setMode(key)}
                  className={`rounded-md py-1.5 text-sm transition-colors ${
                    mode === key ? "bg-background text-foreground font-medium shadow-sm" : "text-muted-foreground"
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
            <FieldDescription>{mode === "school" ? "任务下发并到可签到时段后自动执行" : "只在设定的时段内执行"}</FieldDescription>
          </Field>

          {mode === "custom" && (
            <div className="grid grid-cols-2 gap-3">
              <Field>
                <FieldLabel>开始时间</FieldLabel>
                <Input type="time" value={windowStart} onChange={(e) => setWindowStart(e.target.value)} />
              </Field>
              <Field>
                <FieldLabel>结束时间</FieldLabel>
                <Input type="time" value={windowEnd} onChange={(e) => setWindowEnd(e.target.value)} />
              </Field>
            </div>
          )}

          <Field>
            <FieldLabel>执行日期</FieldLabel>
            <div className="flex flex-wrap gap-1.5">
              {DAY_NAMES.map((label, idx) => (
                <button
                  key={label}
                  type="button"
                  onClick={() => toggleDay(idx)}
                  className={`rounded-lg border px-2.5 py-1 text-xs transition-colors ${
                    days.includes(idx)
                      ? "border-primary bg-primary/10 text-primary"
                      : "text-muted-foreground border-border"
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </Field>

          <label className="flex items-center justify-between rounded-lg border px-3 py-2.5">
            <span className="text-sm">启用计划</span>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </label>

          {error && <p className="text-destructive text-sm">{error}</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            取消
          </Button>
          <Button variant="default" disabled={busy} onClick={save}>
            {busy ? "保存中…" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}


/** 某账号的计划 tab：列表 + 表单调度（AccountDetail 用） */
export function PlansTab({ accountId }: { accountId: number }) {
  const { toast } = useToast();
  const { data: plansData } = usePlans();
  const [form, setForm] = useState<{ open: boolean; existing: Plan | null }>({ open: false, existing: null });
  const list = (plansData ?? []).filter((p) => p.accountId === accountId);

  return (
    <div className="space-y-3 py-3">
      {list.length === 0 && <p className="text-muted-foreground py-4 text-center text-sm">还没有自动计划</p>}
      {list.map((p) => (
        <PlanCard key={p.id} plan={p} />
      ))}
      <Button variant="outline" size="sm" className="w-full" onClick={() => setForm({ open: true, existing: null })}>
        新建计划
      </Button>
      {form.open && (
        <PlanFormDialog
          accountId={accountId}
          existing={form.existing}
          onClose={() => setForm({ open: false, existing: null })}
          onSaved={() => toast({ title: '计划列表已刷新' })}
        />
      )}
    </div>
  );
}

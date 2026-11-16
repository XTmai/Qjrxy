// 语义状态徽章：全部走设计令牌，深色模式自动跟随，不再硬编码调色板。
import { Badge } from "@/components/ui/badge";
import type { TaskState } from "@/lib/task";

const toneClasses: Record<string, string> = {
  ok: "border-success-border/60 bg-success-soft text-success-text",
  warn: "border-warning-border/60 bg-warning-soft text-warning-text",
  err: "border-danger-border/60 bg-danger-soft text-danger-text",
  info: "border-border bg-[color:var(--subtle)] text-muted-foreground",
  plain: "border-border bg-[color:var(--subtle)] text-muted-foreground",
};

export function StatusBadge({ tone, text }: { tone: string; text: string }) {
  return (
    <Badge variant="outline" className={`border ${toneClasses[tone] ?? toneClasses.plain}`}>
      {text}
    </Badge>
  );
}

export function TaskStateBadge({ state }: { state: TaskState }) {
  return <StatusBadge tone={state.tone} text={state.text} />;
}

export function LoginBadge({ status }: { status: string }) {
  const map: Record<string, { text: string; tone: string }> = {
    authorized: { text: "已登录", tone: "ok" },
    expired: { text: "需重新登录", tone: "err" },
    unknown: { text: "未登录", tone: "plain" },
  };
  const meta = map[status] ?? map.unknown;
  return <StatusBadge tone={meta.tone} text={meta.text} />;
}

export function RunStatusBadge({ status }: { status: string }) {
  const map: Record<string, { text: string; tone: string }> = {
    success: { text: "成功", tone: "ok" },
    failed: { text: "失败", tone: "err" },
    pending: { text: "待核对", tone: "warn" },
    executing: { text: "执行中", tone: "info" },
  };
  const meta = map[status] ?? { text: status || "未知", tone: "plain" };
  return <StatusBadge tone={meta.tone} text={meta.text} />;
}

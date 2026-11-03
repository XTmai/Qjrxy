// 语义状态徽章：openilink 的 Badge 只有 4 个 variant，状态色用类名映射补齐。
import { Badge } from "@/components/ui/badge";
import type { TaskState } from "@/lib/task";

const toneClasses: Record<string, string> = {
  ok: "border-transparent bg-green-500/15 text-green-700 dark:bg-green-500/20 dark:text-green-400",
  warn: "border-transparent bg-amber-500/15 text-amber-700 dark:bg-amber-500/20 dark:text-amber-400",
  err: "border-transparent bg-red-500/15 text-red-700 dark:bg-red-500/20 dark:text-red-400",
  info: "border-transparent bg-blue-500/15 text-blue-700 dark:bg-blue-500/20 dark:text-blue-400",
  plain: "border-transparent bg-muted text-muted-foreground",
};

export function StatusBadge({ tone, text }: { tone: string; text: string }) {
  return (
    <Badge variant="outline" className={toneClasses[tone] ?? toneClasses.plain}>
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

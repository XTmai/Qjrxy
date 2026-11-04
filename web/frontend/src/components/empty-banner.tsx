// 空态/提示横幅：openilink dashboard 空态模式（destructive 描边 + 图标 + 两行文字 + 动作钮）。
import type { ReactNode } from "react";
import { AlertTriangle, CircleCheck } from "lucide-react";

interface Props {
  tone?: "warning" | "destructive" | "primary";
  title: string;
  description?: string;
  action?: ReactNode;
}

const toneStyles = {
  warning: "border-amber-500/20 bg-amber-500/5",
  destructive: "border-destructive/20 bg-destructive/5",
  primary: "border-primary/20 bg-primary/5",
} as const;

const iconStyles = {
  warning: "text-amber-500",
  destructive: "text-destructive",
  primary: "text-primary",
} as const;

const titleStyles = {
  warning: "text-amber-700 dark:text-amber-400",
  destructive: "text-destructive",
  primary: "text-primary",
} as const;

export function EmptyBanner({ tone = "destructive", title, description, action }: Props) {
  const icon =
    tone === "primary" ? (
      <CircleCheck className={`h-4 w-4 shrink-0 ${iconStyles[tone]}`} />
    ) : (
      <AlertTriangle className={`h-4 w-4 shrink-0 ${iconStyles[tone]}`} />
    );
  return (
    <div className={`flex items-center gap-3 rounded-lg border p-4 ${toneStyles[tone]}`}>
      {icon}
      <div className="min-w-0 flex-1">
        <p className={`text-sm font-medium ${titleStyles[tone]}`}>{title}</p>
        {description && (
          <p className={`mt-0.5 text-xs ${tone === "primary" ? "text-primary/70" : "opacity-70"}`}>
            {description}
          </p>
        )}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}

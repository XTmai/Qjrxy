// 空态/提示横幅：安静的卡片 + 左侧一道状态色细条。
// 不做整块染色——操作台上大面积高饱和底色会压过正文，只剩「有东西需要注意」这个信息。
import type { ReactNode } from "react";
import { AlertTriangle, CircleCheck } from "lucide-react";

interface Props {
  tone?: "warning" | "destructive" | "primary";
  title: string;
  description?: string;
  action?: ReactNode;
}

const accentStyles = {
  warning: "bg-warning",
  destructive: "bg-danger",
  primary: "bg-brand",
} as const;

const iconStyles = {
  warning: "text-warning",
  destructive: "text-danger",
  primary: "text-brand",
} as const;

export function EmptyBanner({ tone = "destructive", title, description, action }: Props) {
  const icon =
    tone === "primary" ? (
      <CircleCheck className={`h-4 w-4 shrink-0 ${iconStyles[tone]}`} />
    ) : (
      <AlertTriangle className={`h-4 w-4 shrink-0 ${iconStyles[tone]}`} />
    );
  return (
    <div className="border-border bg-card shadow-l1 relative flex items-center gap-3 overflow-hidden rounded-xl border py-3.5 pr-4 pl-4">
      <span className={`absolute inset-y-0 left-0 w-[3px] ${accentStyles[tone]}`} />
      {icon}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{title}</p>
        {description && <p className="text-muted-foreground mt-0.5 text-xs text-pretty">{description}</p>}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}

import { cn } from "../../lib/utils";

const variants = {
  default: "bg-primary text-primary-foreground hover:bg-primary/85",
  secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
  destructive: "bg-destructive text-destructive-foreground hover:bg-destructive/85",
  outline: "border-border text-foreground",
  // 语义状态色（Qjrxy 扩展）：全部走令牌，深色模式自动跟随
  success: "border-transparent bg-success-soft text-success-text",
  warning: "border-transparent bg-warning-soft text-warning-text",
  error: "border-transparent bg-danger-soft text-danger-text",
  info: "border-transparent bg-[color:var(--subtle)] text-muted-foreground",
};

export function Badge({
  variant = "default",
  className,
  children,
}: {
  variant?: keyof typeof variants;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border border-transparent px-2.5 py-0.5 text-[11px] font-medium whitespace-nowrap transition-colors",
        variants[variant],
        className,
      )}
    >
      {children}
    </span>
  );
}

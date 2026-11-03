import { cn } from "../../lib/utils";

const variants = {
  default: "bg-primary text-primary-foreground shadow-sm hover:bg-primary/80",
  secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
  destructive: "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/80",
  outline: "border text-foreground",
  // 语义状态色（Qjrxy 扩展）
  success: "border-transparent bg-green-500/15 text-green-700 dark:bg-green-500/20 dark:text-green-400",
  warning: "border-transparent bg-amber-500/15 text-amber-700 dark:bg-amber-500/20 dark:text-amber-400",
  error: "border-transparent bg-red-500/15 text-red-700 dark:bg-red-500/20 dark:text-red-400",
  info: "border-transparent bg-blue-500/15 text-blue-700 dark:bg-blue-500/20 dark:text-blue-400",
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
        "inline-flex items-center rounded-md border border-transparent px-2.5 py-0.5 text-xs font-medium transition-colors",
        variants[variant],
        className,
      )}
    >
      {children}
    </span>
  );
}

// 统计卡：openilink dashboard-overview 的彩色图标容器模式。
import type { ReactNode } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

const colorMap = {
  blue: "bg-blue-500/10 text-blue-500",
  emerald: "bg-emerald-500/10 text-emerald-500",
  red: "bg-red-500/10 text-red-500",
  amber: "bg-amber-500/10 text-amber-500",
  violet: "bg-violet-500/10 text-violet-500",
} as const;

type StatColor = keyof typeof colorMap;

interface StatCardProps {
  icon: ReactNode;
  label: string;
  value: number | string;
  color: StatColor;
  /** 右上角可选徽标（如「已完成」） */
  badge?: { text: string; variant: "success" | "warning" | "error" | "info" | "secondary" } | null;
  /** 点击跳转目标（整卡可点时用） */
  to?: string;
}

export function StatCard({ icon, label, value, color, badge = null }: StatCardProps) {
  return (
    <Card className="hover:bg-card border-border/50 cursor-default bg-card/50 transition-colors">
      <CardContent className="p-5">
        <div className="flex items-start justify-between">
          <div
            className={cn(
              "flex h-8 w-8 items-center justify-center rounded-lg [&_svg]:size-4 [&_svg]:stroke-[2]",
              colorMap[color],
            )}
          >
            {icon}
          </div>
          {badge && <Badge variant={badge.variant}>{badge.text}</Badge>}
        </div>
        <p className="mt-3 text-2xl font-bold tabular-nums">{typeof value === "number" ? value.toLocaleString() : value}</p>
        <div className="mt-1 flex items-baseline gap-1.5">
          <span className="text-foreground/80 text-xs font-semibold">{label}</span>
        </div>
      </CardContent>
    </Card>
  );
}

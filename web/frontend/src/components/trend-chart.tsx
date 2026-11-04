// 近 7/14 天执行趋势：纯 div 堆叠迷你柱状图（无图表库依赖）。
// 数据来自 /api/logs/trend → DayStat[] { date, success, failed, pending }
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useLogTrend } from "@/hooks/use-dashboard";
import type { DayStat } from "@/lib/types";

function dayLabel(date: string): string {
  const [, , dd] = date.split("-");
  return dd; // 仅显示「日」，避免 3 列布局下 "10/28" 挤压重叠
}

function bars(d: DayStat, max: number) {
  const total = d.success + d.failed + d.pending;
  const pct = (n: number) => (max === 0 ? 0 : Math.round((n / max) * 100));
  return (
    <div className="bg-muted/40 group-hover:bg-muted flex h-full w-full flex-col justify-end rounded-md transition-colors" key="bars">
      {d.failed > 0 && (
        <div className="rounded-t-[2px] bg-red-500/80" style={{ height: `${pct(d.failed)}%` }} />
      )}
      {d.pending > 0 && (
        <div className="bg-amber-500/80" style={{ height: `${pct(d.pending)}%` }} />
      )}
      {d.success > 0 && (
        <div
          className={`bg-emerald-500/85 ${d.failed === 0 && d.pending === 0 ? "rounded-t-[2px]" : ""}`}
          style={{ height: `${pct(d.success)}%` }}
        />
      )}
      {total === 0 && <div className="h-px w-full bg-border/60" />}
    </div>
  );
}

export function TrendChart({ days = 7 }: { days?: number }) {
  const { data, isLoading } = useLogTrend(days);

  return (
    <Card className="border-border/50">
      <CardHeader>
        <CardTitle className="text-base">近 {days} 天趋势</CardTitle>
        <CardDescription>按天统计执行结果分布</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading && <Skeleton className="h-28 w-full" />}
        {data && (
          <>
            <div className="flex h-28 items-end gap-1.5">
              {data.trend.map((d) => {
                const max = Math.max(1, ...data.trend.map((x) => x.success + x.failed + x.pending));
                const total = d.success + d.failed + d.pending;
                return (
                  <div key={d.date} className="group flex h-full min-w-0 flex-1 flex-col items-center gap-1">
                    <span
                      className={`text-[10px] font-medium tabular-nums ${
                        total > 0 ? "text-foreground/70" : "text-muted-foreground/40"
                      }`}
                    >
                      {total > 0 ? total : ""}
                    </span>
                    {bars(d, max)}
                    <span className="text-muted-foreground whitespace-nowrap text-[10px]">{dayLabel(d.date)}</span>
                  </div>
                );
              })}
            </div>
            <div className="text-muted-foreground mt-3 flex items-center justify-center gap-4 text-[11px]">
              <span className="flex items-center gap-1">
                <i className="size-2 rounded-[2px] bg-emerald-500/85" /> 成功
              </span>
              <span className="flex items-center gap-1">
                <i className="size-2 rounded-[2px] bg-amber-500/80" /> 待核对
              </span>
              <span className="flex items-center gap-1">
                <i className="size-2 rounded-[2px] bg-red-500/80" /> 失败
              </span>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

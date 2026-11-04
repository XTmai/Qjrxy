// 执行审计：筛选栏 + 审计表格（流水号/耗时/围栏偏移）+ 右栏调用链路时间线
import { useEffect, useMemo, useState } from "react";
import {
  ArrowRight,
  CheckCircle2,
  Crosshair,
  Download,
  Fingerprint,
  MapPin,
  RefreshCw,
  Route,
  Timer,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { PageHeader } from "@/components/page-header";
import { RunStatusBadge } from "@/components/status-badge";
import { useAuditLogs } from "@/hooks/use-stats";
import { useStats7d } from "@/hooks/use-stats";
import { maskUser } from "@/lib/utils";
import type { AuditRow, LogDetail } from "@/lib/types";

const pad2 = (n: number) => String(n).padStart(2, "0");

/** 毫秒级时间：HH:MM:SS.d */
function clockOf(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}.${String(d.getMilliseconds()).padStart(3, "0")}`;
}

function dateOf(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

/** 掩码设备指纹：保留前 6 后 4 */
function maskDevice(v: string): string {
  if (!v) return "—";
  return v.length <= 12 ? v : `${v.slice(0, 6)}…${v.slice(-4)}`;
}

export function AuditPage() {
  const [status, setStatus] = useState("all");
  const [scope, setScope] = useState("week");
  const [auto, setAuto] = useState(false);
  const [selectedId, setSelectedId] = useState<number | null>(null);

  const { data, isLoading, refetch, isFetching } = useAuditLogs({ status, scope, limit: 100 });
  const { data: stats7d } = useStats7d();
  const rows = useMemo(() => data?.logs ?? [], [data]);

  // 实时刷新：每 15 秒拉一次
  useEffect(() => {
    if (!auto) return;
    const t = setInterval(() => void refetch(), 15000);
    return () => clearInterval(t);
  }, [auto, refetch]);

  // 默认选中第一条
  useEffect(() => {
    if (rows.length && (selectedId === null || !rows.some((r) => r.id === selectedId))) {
      setSelectedId(rows[0].id);
    }
    if (!rows.length && selectedId !== null) setSelectedId(null);
  }, [rows, selectedId]);

  const selected = rows.find((r) => r.id === selectedId) ?? null;

  const exportCsv = () => {
    const header = ["时间", "账号", "学号", "任务", "来源", "状态", "耗时(ms)", "围栏偏移(m)", "流水号", "结果"];
    const lines = rows.map((r) =>
      [
        r.createdAt,
        r.accountAlias || r.accountUsername || r.accountId,
        maskUser(r.accountUsername || ""),
        r.taskName,
        r.source === "auto" ? "自动" : "手动",
        r.status,
        r.latencyMs || "",
        r.geoOffsetM || "",
        r.traceId,
        (r.message || "").replace(/[\r\n,]+/g, " "),
      ]
        .map((v) => `"${String(v).replace(/"/g, '""')}"`)
        .join(","),
    );
    const blob = new Blob(["\uFEFF" + [header.join(","), ...lines].join("\n")], {
      type: "text/csv;charset=utf-8",
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `qjrxy-audit-${dateOf(new Date().toISOString())}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-5">
      <PageHeader
        title="执行审计"
        sub={stats7d ? `近 7 日 ${stats7d.total} 次执行 · 平均 ${stats7d.avgLatencyMs || "—"} ms` : "全量执行流水与调用链路"}
        actions={
          <>
            <label className="flex cursor-pointer items-center gap-2 text-xs">
              <Switch checked={auto} onCheckedChange={setAuto} aria-label="实时刷新" />
              <span className={auto ? "text-foreground" : "text-muted-foreground"}>实时刷新</span>
            </label>
            <Button variant="outline" size="sm" disabled={!rows.length} onClick={exportCsv}>
              <Download className="size-3.5" />
              导出 CSV
            </Button>
            <Button variant="outline" size="sm" disabled={isFetching} onClick={() => void refetch()}>
              <RefreshCw className={`size-3.5 ${isFetching ? "animate-spin" : ""}`} />
              刷新
            </Button>
          </>
        }
      />

      {/* 顶部指标 */}
      <div className="grid gap-3 sm:grid-cols-4">
        <HeadMetric label="执行总次数" value={stats7d ? stats7d.total : "—"} />
        <HeadMetric
          label="达标率"
          value={stats7d ? `${stats7d.successRate.toFixed(1)}%` : "—"}
          tone={stats7d && stats7d.successRate >= 90 ? "ok" : "warn"}
        />
        <HeadMetric label="平均耗时" value={stats7d?.avgLatencyMs ? `${stats7d.avgLatencyMs} ms` : "—"} />
        <HeadMetric
          label="平均围栏偏移"
          value={stats7d ? `${stats7d.avgGeoOffsetM.toFixed(0)} m` : "—"}
          tone={stats7d && stats7d.avgGeoOffsetM <= 200 ? "ok" : "warn"}
        />
      </div>

      {/* 筛选栏 */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs value={status} onValueChange={setStatus}>
          <TabsList>
            <TabsTrigger value="all">全部</TabsTrigger>
            <TabsTrigger value="success">成功</TabsTrigger>
            <TabsTrigger value="failed">失败</TabsTrigger>
            <TabsTrigger value="pending">待核对</TabsTrigger>
          </TabsList>
        </Tabs>
        <Select value={scope} onValueChange={setScope}>
          <SelectTrigger className="w-28" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="today">今天</SelectItem>
            <SelectItem value="week">本周</SelectItem>
            <SelectItem value="all">全部</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
        {/* 审计表格 */}
        <Card className="border-border py-0">
          <CardContent className="p-0">
            {isLoading ? (
              <div className="space-y-2 p-4">
                {[0, 1, 2, 3, 4].map((i) => (
                  <Skeleton key={i} className="h-10 rounded-md" />
                ))}
              </div>
            ) : rows.length === 0 ? (
              <p className="text-muted-foreground py-14 text-center text-sm">该筛选条件下暂无执行记录</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="pl-4">时间</TableHead>
                    <TableHead>账号</TableHead>
                    <TableHead>任务</TableHead>
                    <TableHead className="text-right">耗时</TableHead>
                    <TableHead className="text-right">偏移</TableHead>
                    <TableHead>状态</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((r) => (
                    <TableRow
                      key={r.id}
                      onClick={() => setSelectedId(r.id)}
                      className={
                        selectedId === r.id
                          ? "bg-primary-soft/60 cursor-pointer"
                          : "cursor-pointer"
                      }
                    >
                      <TableCell className="pl-4">
                        <p className="font-mono text-xs tabular-nums">{clockOf(r.createdAt)}</p>
                        <p className="text-muted-foreground font-mono text-[10px]">{dateOf(r.createdAt)}</p>
                      </TableCell>
                      <TableCell>
                        <p className="truncate text-xs font-medium">
                          {r.accountAlias || `账号 ${r.accountId}`}
                        </p>
                        <p className="text-muted-foreground font-mono text-[10px]">
                          {maskUser(r.accountUsername || "")}
                        </p>
                      </TableCell>
                      <TableCell>
                        <p className="max-w-40 truncate text-xs">{r.taskName || "查寝"}</p>
                        <p className="text-muted-foreground text-[10px]">
                          {r.source === "auto" ? "自动计划" : "手动执行"}
                        </p>
                      </TableCell>
                      <TableCell className="text-right font-mono text-xs tabular-nums">
                        {r.latencyMs ? `${r.latencyMs}ms` : "—"}
                      </TableCell>
                      <TableCell className="text-right font-mono text-xs tabular-nums">
                        {r.geoOffsetM ? `${r.geoOffsetM.toFixed(0)}m` : "—"}
                      </TableCell>
                      <TableCell>
                        <RunStatusBadge status={r.status} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* 右栏：调用链路时间线 + 坐标验证 */}
        <div className="space-y-4">
          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                <Route className="text-primary size-4" />
                调用链路
              </CardTitle>
              <CardDescription className="mt-0.5 font-mono text-[10px]">
                {selected?.traceId || "—"}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {selected ? (
                <TraceTimeline row={selected} />
              ) : (
                <p className="text-muted-foreground py-6 text-center text-xs">选择左侧一条记录查看链路</p>
              )}
            </CardContent>
          </Card>

          <Card className="border-border">
            <CardHeader className="pb-2.5">
              <CardTitle className="tracking-headline flex items-center gap-2 text-sm font-semibold">
                <Crosshair className="text-primary size-4" />
                坐标偏移验证
              </CardTitle>
            </CardHeader>
            <CardContent>
              <GeoCheck row={selected} />
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

function HeadMetric({
  label,
  value,
  tone,
}: {
  label: string;
  value: string | number;
  tone?: "ok" | "warn";
}) {
  const toneCls =
    tone === "ok" ? "text-success" : tone === "warn" ? "text-warning" : "text-foreground";
  return (
    <Card className="border-border">
      <CardContent className="p-3.5">
        <p className="text-muted-foreground tracking-label text-[10px] font-medium uppercase">{label}</p>
        <p className={`tracking-headline mt-1.5 text-xl font-semibold tabular-nums ${toneCls}`}>{value}</p>
      </CardContent>
    </Card>
  );
}

/** 链路时间线：从 detail JSON 还原关键节点（展示用） */
function TraceTimeline({ row }: { row: AuditRow }) {
  let detail: LogDetail & { traceId?: string; latencyMs?: number; signDate?: string; beginClock?: string; endClock?: string } = {};
  try {
    if (row.detail) detail = JSON.parse(row.detail);
  } catch {
    /* detail 可能不是 JSON */
  }

  const steps: Array<{ icon: React.ReactNode; title: string; body: React.ReactNode }> = [
    {
      icon: <Timer className="size-3" />,
      title: "请求发起",
      body: <span className="font-mono">{clockOf(row.createdAt)}</span>,
    },
    {
      icon: <Fingerprint className="size-3" />,
      title: "设备指纹",
      body: <span className="font-mono">{maskDevice(detail.device || "")}</span>,
    },
    {
      icon: <MapPin className="size-3" />,
      title: "提交坐标",
      body:
        detail.lon && detail.lat ? (
          <span className="font-mono">
            {detail.lon.toFixed(4)}, {detail.lat.toFixed(4)}
          </span>
        ) : (
          <span>{detail.address || "—"}</span>
        ),
    },
    {
      icon: <RefreshCw className="size-3" />,
      title: "服务端响应",
      body: (
        <span className="font-mono">
          {detail.latencyMs ? `${detail.latencyMs} ms` : "—"}
          {detail.code ? ` · code=${detail.code}` : ""}
        </span>
      ),
    },
    {
      icon:
        row.status === "success" ? (
          <CheckCircle2 className="text-success size-3" />
        ) : (
          <ArrowRight className="size-3" />
        ),
      title: row.status === "success" ? "执行成功" : row.status === "failed" ? "执行失败" : "待核对",
      body: <span>{row.message || detail.message || "—"}</span>,
    },
  ];

  return (
    <ol className="relative ml-1.5 space-y-3.5 border-l border-border pl-4">
      {steps.map((s) => (
        <li key={s.title} className="relative">
          <span className="bg-primary-soft text-primary border-border absolute -left-[25px] flex size-4 items-center justify-center rounded-full border">
            {s.icon}
          </span>
          <p className="tracking-label text-[10px] font-medium uppercase">{s.title}</p>
          <p className="mt-0.5 text-xs break-all">{s.body}</p>
        </li>
      ))}
    </ol>
  );
}

/** 坐标偏移可视化：横向刻度条 + 当前偏移位置 */
function GeoCheck({ row }: { row: AuditRow | null }) {
  if (!row || !row.geoOffsetM) {
    return (
      <div className="flex aspect-[2/1] items-center justify-center rounded-md border border-dashed">
        <p className="text-muted-foreground text-xs">暂无偏移数据</p>
      </div>
    );
  }
  // 刻度上限 600m（常见安全半径量级）
  const max = 600;
  const pct = Math.min(100, (row.geoOffsetM / max) * 100);
  const inRange = row.inRange !== false;
  return (
    <div className="space-y-3">
      <div className="flex items-baseline justify-between">
        <p className="flex items-baseline gap-1">
          <span className="tracking-headline text-xl font-semibold tabular-nums">
            {row.geoOffsetM.toFixed(0)}
          </span>
          <span className="text-muted-foreground text-[10px]">米</span>
        </p>
        <span
          className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${
            inRange ? "bg-success-soft text-success-text" : "bg-warning-soft text-warning-text"
          }`}
        >
          {inRange ? "安全圈内" : "超出安全圈"}
        </span>
      </div>
      <div className="bg-muted relative h-2 overflow-hidden rounded-full">
        <div
          className={`h-full rounded-full ${inRange ? "bg-success" : "bg-warning"}`}
          style={{ width: `${pct}%` }}
        />
        {/* 500m 阈值刻度 */}
        <span className="absolute top-[-3px] bottom-[-3px] w-px bg-foreground/40" style={{ left: `${(500 / max) * 100}%` }} />
      </div>
      <div className="text-muted-foreground flex justify-between font-mono text-[9px]">
        <span>0m</span>
        <span>500m 阈值</span>
        <span>600m</span>
      </div>
    </div>
  );
}

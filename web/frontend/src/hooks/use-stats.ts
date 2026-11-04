// 7 日聚合指标 + 审计流水（监控总览 / 执行审计页共用）。
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { queryKeys } from "@/lib/query-keys";

/** 近 7 天聚合：成功数 / 达标率 / 平均延迟 / 平均地理偏移 */
export function useStats7d() {
  return useQuery({
    queryKey: queryKeys.stats.sevenDays(),
    queryFn: () => api.stats7d(),
  });
}

export interface AuditParams {
  status?: string;
  scope?: string;
  kind?: string;
  limit?: number;
  offset?: number;
  [key: string]: unknown;
}

/** 审计流水（扁平化 detail：traceId / latencyMs / geoOffsetM / inRange） */
export function useAuditLogs(params: AuditParams) {
  return useQuery({
    queryKey: queryKeys.stats.audit(params),
    queryFn: () => api.auditLogs(params),
  });
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api";
import { queryKeys } from "@/lib/query-keys";
import type { RunOutcome, Task } from "@/lib/types";

/** 全部缓存任务（三类都拉，前端按 kind 过滤查寝） */
export function useTasks() {
  return useQuery({
    queryKey: queryKeys.tasks.all(),
    queryFn: () => api.tasks(),
  });
}

/** 从学校同步今日任务 */
export function useSyncTasks(accountId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.syncTasks(accountId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.tasks.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
    },
  });
}

/** 执行查寝任务（RunOutcome 由页面按 status 分支提示） */
export function useRunTask(accountId: number, taskId: number, kind: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (): Promise<RunOutcome> => api.runTask(accountId, taskId, kind),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.tasks.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
      void qc.invalidateQueries({ queryKey: queryKeys.logs.list({}) });
    },
  });
}

/** 过滤某账号的查寝任务 */
export function attendanceTasksOf(tasks: Task[] | undefined, accountId: number): Task[] {
  return (tasks ?? []).filter((t) => t.accountId === accountId && t.kind === "attendance");
}

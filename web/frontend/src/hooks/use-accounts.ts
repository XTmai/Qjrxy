import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api";
import { queryKeys } from "@/lib/query-keys";
import type { AccountInput } from "@/lib/types";

/** 账号列表 */
export function useAccounts() {
  return useQuery({
    queryKey: queryKeys.accounts.all(),
    queryFn: () => api.listAccounts(),
    select: (d) => d.accounts,
  });
}

/**
 * 创建账号。只负责入库 + 刷新列表，登录由调用方单独发起——
 * 之前把「创建」和「登录」串在同一个 mutation 里，登录一失败整个 mutation 就 reject：
 * 账号其实已经写入数据库，弹窗却退回表单报错，用户重试又撞上「已绑定」，
 * 列表还得手动刷新才看得到新账号。
 *
 * 已存在（同学校 + 学号）时后端复用并更新密码/设备信息，返回 existed=true，不再报错。
 */
export function useCreateAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AccountInput) => api.createAccount(input),
    onSuccess: () => {
      // 账号已入库就先把列表刷新出来，后续登录成败都不影响可见性。
      void qc.invalidateQueries({ queryKey: queryKeys.accounts.all() });
    },
  });
}

/** 删除账号 */
export function useDeleteAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteAccount(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.accounts.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
    },
  });
}

/** 批量登录态探测（返回逐账号结果） */
export function useProbeAccounts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.probeAccounts(0),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.accounts.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
    },
  });
}

/** 上传签到照片 */
export function useUploadPhoto(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (formData: FormData) => api.uploadPhoto(id, formData),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.accounts.detail(id) });
    },
  });
}

/** 自动获取签到点 */
export function useApplySignRange(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.applySignRange(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.accounts.detail(id) });
    },
  });
}

/** 账密登录（手动触发，返回验证码挑战或完成） */
export function useAccountLogin(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.accountLogin(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.accounts.all() });
    },
  });
}

/** 提交图形验证码 */
export function useSubmitCaptcha(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { ticket: string; code: string }) => api.submitCaptcha(id, input.ticket, input.code),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.accounts.all() });
    },
  });
}

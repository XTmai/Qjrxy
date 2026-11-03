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

/** 创建账号 → 自动登录 → 自动获取签到点。任一环节失败都会抛出，由调用方提示。 */
export function useCreateAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: AccountInput) => {
      const { account } = await api.createAccount(input);
      // 保存即登录
      const ch = await api.accountLogin(account.id);
      return { account, challenge: ch };
    },
    onSuccess: () => {
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

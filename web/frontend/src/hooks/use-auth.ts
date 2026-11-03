import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { getToken, setToken } from "@/api/client";
import { queryClient } from "@/lib/query-client";
import { queryKeys } from "@/lib/query-keys";
import { useAuthStore } from "@/stores/auth-store";
import type { User } from "@/lib/types";

/** 当前登录用户；无 token 时不发起请求。 */
export function useUser() {
  const setUser = useAuthStore((s) => s.setUser);
  return useQuery({
    queryKey: queryKeys.user(),
    queryFn: async () => {
      const { user } = await api.me();
      setUser(user);
      return user;
    },
    enabled: Boolean(getToken()),
    staleTime: 5 * 60_000,
    retry: false,
  });
}

/** 登录 / 注册共用：拿到 token 与用户后写入本地与缓存 */
function onAuthSuccess(data: { user: User; token?: string }) {
  if (data.token) setToken(data.token);
  useAuthStore.getState().setUser(data.user);
  void queryClient.invalidateQueries({ queryKey: queryKeys.user() });
}

export function useLogin() {
  return useMutation({
    mutationFn: (input: { username: string; password: string }) =>
      api.login(input.username, input.password),
    onSuccess: onAuthSuccess,
  });
}

export function useRegister() {
  return useMutation({
    mutationFn: (input: { username: string; password: string; nickname?: string }) =>
      api.register(input.username, input.password, input.nickname),
    onSuccess: onAuthSuccess,
  });
}

/** 服务端信息（quotaEnabled 等） */
export function useInfo() {
  return useQuery({
    queryKey: queryKeys.info(),
    queryFn: () => api.systemInfo(),
    staleTime: 10 * 60_000,
  });
}

/** 当前服务端信息（quotaEnabled 等） */

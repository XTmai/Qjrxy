import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      retry: 1,
      // 关闭聚焦重取：每次 alt-tab 回来都会触发 5+ 个查询重取 → 整页重渲染，
      // 体感就是「切回来顿一下」。staleTime 30s 已保证数据新鲜度。
      refetchOnWindowFocus: false,
    },
  },
});

// 实时数据更新：订阅服务端变更流，按主题让对应查询失效并重取。
//
// 设计取舍：
//  - 事件只说明「哪类数据变了」，具体数据仍由各自的接口拉取，权限与
//    字段过滤不会被绕过。
//  - 250ms 合并：一轮登录态探测会连续写多个账号，逐条刷新会让页面抖动。
//  - 页面隐藏时只标脏不请求，切回来仅重取真正变化过的查询，避免
//    「切回窗口整页重渲染」的顿挫。
//  - 推送断开时退化为 30s 轮询，保证反向代理吞流等环境下仍然可用。
import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { subscribeLiveEvents } from "@/api/events";
import { useLiveStore } from "@/stores/live-store";

/** 服务端主题 → 需要失效的 queryKeys 根节点 */
const TOPIC_ROOTS: Record<string, readonly string[]> = {
  accounts: ["accounts"],
  tasks: ["tasks"],
  plans: ["plans"],
  // 执行流水变化会同时影响日志列表、趋势与 7 日聚合
  logs: ["logs", "stats"],
};

const ALL_LIVE_ROOTS = ["accounts", "tasks", "plans", "logs", "stats", "dashboard"] as const;

/** 合并窗口：一轮探测/一次执行产生的多条事件合成一次刷新 */
const FLUSH_DELAY_MS = 250;
/** 推送不可用时的兜底轮询间隔 */
const FALLBACK_POLL_MS = 30_000;

type RefetchType = "active" | "none";

export function useLiveUpdates() {
  const queryClient = useQueryClient();
  const setStatus = useLiveStore((s) => s.setStatus);

  useEffect(() => {
    let pendingTopics = new Set<string>();
    let flushTimer: number | undefined;
    let hiddenRoots = new Set<string>();
    /** 曾经连上过推送：只有这种情况下的断开才需要重连后全量对齐 */
    let wasLive = false;
    /** 断开过：重连成功后需要全量对齐一次，补齐漏掉的事件 */
    let missedWhileDown = false;

    const invalidate = (roots: Iterable<string>, refetchType: RefetchType) => {
      for (const root of roots) {
        void queryClient.invalidateQueries({ queryKey: [root], refetchType });
      }
    };

    const flush = () => {
      flushTimer = undefined;
      if (pendingTopics.size === 0) return;

      const roots = new Set<string>(["dashboard"]);
      for (const topic of pendingTopics) {
        if (topic === "all") {
          for (const root of ALL_LIVE_ROOTS) roots.add(root);
          continue;
        }
        for (const root of TOPIC_ROOTS[topic] ?? []) roots.add(root);
      }
      pendingTopics = new Set();

      if (document.hidden) {
        // 隐藏页：只标脏，等用户回来看时再拉，避免后台持续发请求。
        for (const root of roots) hiddenRoots.add(root);
        invalidate(roots, "none");
        return;
      }
      invalidate(roots, "active");
    };

    const queue = (topics: string[]) => {
      for (const topic of topics) pendingTopics.add(topic);
      if (flushTimer === undefined) flushTimer = window.setTimeout(flush, FLUSH_DELAY_MS);
    };

    const onVisible = () => {
      if (document.hidden || hiddenRoots.size === 0) return;
      const roots = hiddenRoots;
      hiddenRoots = new Set();
      invalidate(roots, "active");
    };
    document.addEventListener("visibilitychange", onVisible);

    const stop = subscribeLiveEvents({
      onStatus: (status) => {
        setStatus(status);
        if (status === "live") {
          wasLive = true;
          return;
        }
        // 首次建连就失败时页面数据是刚拉过的，不必再全量重取一次。
        if (status === "offline" && wasLive) missedWhileDown = true;
      },
      onReady: () => {
        if (!missedWhileDown) return;
        missedWhileDown = false;
        queue(["all"]);
      },
      onInvalidate: (topics) => {
        useLiveStore.getState().markEvent();
        queue(topics);
      },
    });

    // 兜底：推送没连上时按固定间隔刷新当前活跃查询。
    const pollTimer = window.setInterval(() => {
      if (useLiveStore.getState().status === "live") return;
      if (document.hidden) {
        for (const root of ALL_LIVE_ROOTS) hiddenRoots.add(root);
        invalidate(ALL_LIVE_ROOTS, "none");
        return;
      }
      invalidate(ALL_LIVE_ROOTS, "active");
    }, FALLBACK_POLL_MS);

    return () => {
      stop();
      window.clearInterval(pollTimer);
      if (flushTimer !== undefined) window.clearTimeout(flushTimer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [queryClient, setStatus]);
}

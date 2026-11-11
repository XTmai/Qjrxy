import { create } from "zustand";

/** 实时通道状态：live=推送已连接；connecting=正在建连/重连；offline=断开，走轮询兜底 */
export type LiveStatus = "connecting" | "live" | "offline";

interface LiveState {
  status: LiveStatus;
  /** 最近一次收到变更事件的时间（毫秒），用于侧栏提示与排障 */
  lastEventAt: number;
  setStatus: (status: LiveStatus) => void;
  markEvent: () => void;
}

export const useLiveStore = create<LiveState>((set) => ({
  status: "connecting",
  lastEventAt: 0,
  setStatus: (status) => set({ status }),
  markEvent: () => set({ lastEventAt: Date.now() }),
}));

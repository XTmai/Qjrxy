import { create } from "zustand";

/**
 * 验证码待处理信号。
 *
 * SSE 只发主题不带数据（题目可能是一百多 KB 的图），所以这里只记「变了」，
 * 真正的题目由弹窗组件调 /api/captcha/pending 拉取。这样刷新页面、推送断开
 * 都能靠组件挂载时的那次拉取自愈。
 */
interface CaptchaState {
  /** 收到「有验证码待处理」的次数 */
  signal: number;
  bump: () => void;
}

export const useCaptchaStore = create<CaptchaState>((set) => ({
  signal: 0,
  bump: () => set((s) => ({ signal: s.signal + 1 })),
}));

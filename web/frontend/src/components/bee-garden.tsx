// 小蜜蜂花园横幅的 React 包装。
//
// 只做三件事：把 canvas 挂到容器上、把容器尺寸/指针/进度喂给引擎、
// 在卸载和页面不可见时停帧。所有绘制逻辑都在 lib/bee-garden.ts。
//
// 结构约定：本组件渲染一层 absolute inset-0 的装饰层（pointer-events-none），
// 指针事件绑在「最近定位祖先」上——也就是调用方的卡片。卡片里的按钮需要
// 自己 stopPropagation，否则按下按钮会被当成拖拽进度。
import { useEffect, useImperativeHandle, useRef, type RefObject } from "react";
import { GardenEngine } from "@/lib/bee-garden";

export interface BeeGardenHandle {
  replay: () => void;
}

interface BeeGardenProps {
  /** 进度 0..1：今日已完成账号 / 总账号 */
  progress: number;
  /** 平滑后的百分比，用来同步标题里的数字 */
  onProgress?: (pct: number) => void;
  /**
   * 花园带高度（px）：卡片底部为花丛预留的空间，应等于卡片的 padding-bottom。
   * 必须与实际 pb 一致，株高才会刚好填满这条带子而头顶不越界。
   */
  bandPx: number;
  handleRef?: RefObject<BeeGardenHandle | null>;
}

export function BeeGarden({ progress, onProgress, bandPx, handleRef }: BeeGardenProps) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const engineRef = useRef<GardenEngine | null>(null);
  const progressRef = useRef(progress);
  const startedRef = useRef(false);
  const cbRef = useRef(onProgress);
  const bandRef = useRef(bandPx);

  progressRef.current = progress;
  cbRef.current = onProgress;
  bandRef.current = bandPx;

  // 引擎生命周期：挂载一次。props 变化只喂新值，不重建动画。
  useEffect(() => {
    const root = rootRef.current;
    const canvas = canvasRef.current;
    const surface = root?.parentElement;
    if (!root || !canvas || !surface) return;

    let engine: GardenEngine;
    try {
      engine = new GardenEngine(canvas, root, {
        bandPx: bandRef.current,
        onProgress: (pct) => cbRef.current?.(pct),
      });
    } catch {
      // 拿不到 2d context（极少数被禁用的环境）不该让整页白屏
      return;
    }
    engineRef.current = engine;
    // 首帧先落到真实进度：这样「减少动效」环境下直接呈现终态
    engine.setProgress(progressRef.current);
    engine.start();
    startedRef.current = true;

    const onDown = (e: PointerEvent) => engine.pointerDownAt(e.clientX);
    const onMove = (e: PointerEvent) => engine.pointerMoveAt(e.clientX);
    const onLeave = () => engine.pointerLeaveAll();
    const onUp = () => engine.pointerUpAll();

    surface.addEventListener("pointerdown", onDown);
    surface.addEventListener("pointermove", onMove);
    surface.addEventListener("pointerenter", onMove);
    surface.addEventListener("pointerleave", onLeave);
    window.addEventListener("pointerup", onUp);

    // 切到后台就别再跑 rAF：后台标签页的 rAF 被节流，回前台时
    // lastFrame 是一个很旧的时刻，不重置会跳一大帧。
    const onVisibility = () => {
      if (document.hidden) engine.stop();
      else engine.start();
    };
    document.addEventListener("visibilitychange", onVisibility);

    return () => {
      surface.removeEventListener("pointerdown", onDown);
      surface.removeEventListener("pointermove", onMove);
      surface.removeEventListener("pointerenter", onMove);
      surface.removeEventListener("pointerleave", onLeave);
      window.removeEventListener("pointerup", onUp);
      document.removeEventListener("visibilitychange", onVisibility);
      engine.destroy();
      engineRef.current = null;
      startedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 进度变化：数据晚于引擎挂载回来时，把真实值补上。
  // 只有在没在播 intro 的情况下才对齐，避免一次 refetch 把开花动画腰斩。
  useEffect(() => {
    const engine = engineRef.current;
    if (!engine || !startedRef.current) return;
    const reduced =
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduced) engine.setProgress(progress);
  }, [progress]);

  useImperativeHandle(handleRef, () => ({ replay: () => engineRef.current?.replay() }), []);

  return (
    <div ref={rootRef} className="pointer-events-none absolute inset-0 z-10 overflow-visible">
      <canvas ref={canvasRef} className="block h-full w-full" aria-hidden="true" />
    </div>
  );
}

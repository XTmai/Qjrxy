// 弹窗展开起点：记录最近一次指针按下坐标，让弹窗从触发元素的位置展开。
// 无指针场景（键盘 Enter 打开等）回退居中。
//
// 关键性能约束：这里绝不能读 DOM（getBoundingClientRect 会强制同步布局，
// 把「遮罩插入 + body 锁滚动」的布局计算全部压进同一帧，实测造成 125ms 长帧）。
// 弹窗恒定居中，所以「元素中心」等价于「视口中心」：
//   origin = 50% + (指针 - 视口中心)
// 不需要知道弹窗自身尺寸，直接用 calc() 表达，全程零布局读取。
let last = { x: 0, y: 0, t: 0 };

if (typeof window !== "undefined") {
  window.addEventListener(
    "pointerdown",
    (e) => {
      last = { x: e.clientX, y: e.clientY, t: Date.now() };
    },
    { capture: true, passive: true },
  );
}

/**
 * 返回可直接赋给 transform-origin 的字符串，不触发布局。
 *
 * 用百分比 + 像素偏移的 calc 表达：弹窗居中定位，所以视口中心 = 元素中心，
 * 指针相对视口中心的位移就是相对元素中心的位移。transform-origin 允许
 * 超出元素范围的值，因此不需要 clamp。
 */
export function dialogOriginCss(): string {
  if (typeof window === "undefined") return "50% 50%";
  // 1.5s 内没有指针交互，说明是键盘/程序触发的，居中展开
  if (Date.now() - last.t > 1500) return "50% 50%";
  const dx = Math.round(last.x - window.innerWidth / 2);
  const dy = Math.round(last.y - window.innerHeight / 2);
  // 偏移为 0 时直接用百分比，避免生成 "calc(50% + 0px)" 这种无意义值
  const ox = dx === 0 ? "50%" : `calc(50% + ${dx}px)`;
  const oy = dy === 0 ? "50%" : `calc(50% + ${dy}px)`;
  return `${ox} ${oy}`;
}

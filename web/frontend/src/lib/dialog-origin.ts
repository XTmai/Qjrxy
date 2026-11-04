// 弹窗展开起点：记录最近一次指针按下坐标，让弹窗从触发元素的位置展开。
// 无指针场景（键盘 Enter 打开等）回退居中。
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

/** 根据弹窗自身矩形计算 transform-origin（点击点相对弹窗的偏移，越界收敛到边缘）。 */
export function dialogOrigin(rect: DOMRect): string {
  if (Date.now() - last.t > 1500) return "50% 50%";
  const x = Math.min(Math.max(last.x - rect.left, 0), rect.width);
  const y = Math.min(Math.max(last.y - rect.top, 0), rect.height);
  return `${Math.round(x)}px ${Math.round(y)}px`;
}

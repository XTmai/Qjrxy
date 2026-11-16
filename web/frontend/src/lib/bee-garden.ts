// 小蜜蜂花园进度引擎 —— 从设计原型逐函数移植的 Canvas 实现。
//
// 形态：一条草茎轨道 + 52 株花 + 一只黄黑小蜜蜂。
// 进度 v ∈ [0,1] 之前的花全部绽放，之后的花退场（飘瓣）；小蜜蜂用临界阻尼
// 弹簧追随进度头部，悬停时附近的花会朝光标弯腰。
//
// 这里是纯 TS、不依赖 React：一帧内的所有状态都在实例上，React 只负责
// 挂 canvas、喂尺寸、喂指针、喂进度，避免每次渲染重建动画。

export interface GardenOptions {
  /** 平滑后的百分比（取整）变化时回调，用于同步标题数字 */
  onProgress?: (pct: number) => void;
  /**
   * 花园带高度（px）：卡片底部为花丛预留的空间。
   *
   * 传 0 或不传时，引擎按「画布高度 × 0.42」自己估一个——卡片 pb 通常
   * 就是这个量级，且 resize 时会重算，不需要调用方同步窗口宽度。
   */
  bandPx?: number;
  /**
   * 画布顶部需要让出的高度（px）：标题文案区，花冠不许侵入。
   */
  topInsetPx?: number;
  /**
   * 是否允许按住横幅拖动进度。
   *
   * 原型里这是个可拖动的进度条；但在真实控制台里「进度」是今日签完的账号占比，
   * 让用户拖到 100% 会显示一个假的「全部完成」，这比动画好看重要得多。
   * 因此默认关闭，只保留悬停弯腰的装饰性交互。
   */
  scrub?: boolean;
}

type Kind =
  | "tulip"
  | "poppy"
  | "cosmos"
  | "daisy"
  | "aster"
  | "blue"
  | "lavender"
  | "pinkTulip"
  | "bud";

interface Leaf {
  f: number;
  side: number;
}

interface Slot {
  t: number;
  kind: Kind;
  len: number;
  lean: number;
  curve: number;
  phase: number;
  period: number;
  leaves: Leaf[];
  bud: number;
  alive: boolean;
  age: number;
  delay: number;
  s: number;
  h: number;
  bend: number;
}

interface Petal {
  x: number;
  y: number;
  vx: number;
  vy: number;
  a: number;
  spin: number;
  life: number;
  max: number;
  color: string;
  len: number;
  round: boolean;
}

const GARDEN_CFG = {
  flowerCount: 52,
  seed: 7,
  stemMin: 60,
  stemMax: 135,
  headScale: 100,
  stemWidth: 2,
  growMs: 820,
  popMs: 520,
  staggerMs: 42,
  wiltMs: 550,
  petalCount: 6,
  petalMs: 1600,
  petalRise: 70,
  beeSpring: 3.4,
  beeOffset: 2,
  beeBob: 3.5,
  beeSize: 105,
  swayDeg: 1.8,
  trackThick: 5,
  tuftColor: "#5A6E54",
  tulipColor: "#E3B65C",
  poppyColor: "#F08A78",
  pinkColor: "#F7A2A7",
  daisyColor: "#F0E3DA",
  centreColor: "#E9B44C",
  blueColor: "#8FB6F0",
  lilacColor: "#A99BE0",
  asterColor: "#BBA6EA",
  beeColor: "#F2C14E",
  hover: 55,
} as const;

/** 左侧手工摆位的 11 株：起手位置、花种与株高都刻意错开，避免排成队 */
const FIXED_SLOTS: Array<[number, Kind, number]> = [
  [0.014, "tulip", 104],
  [0.036, "poppy", 115],
  [0.066, "daisy", 124],
  [0.101, "lavender", 83],
  [0.123, "daisy", 101],
  [0.151, "blue", 50],
  [0.173, "blue", 87],
  [0.221, "poppy", 116],
  [0.252, "cosmos", 59],
  [0.273, "pinkTulip", 99],
  [0.297, "poppy", 71],
];

const KIND_W: Array<[Kind, number]> = [
  ["daisy", 3],
  ["cosmos", 2],
  ["poppy", 2],
  ["aster", 1.3],
  ["blue", 1.5],
  ["lavender", 0.6],
  ["pinkTulip", 1],
  ["bud", 1.4],
];

const TAIL_W: Array<[Kind, number]> = [
  ["bud", 4],
  ["daisy", 1.5],
  ["aster", 1],
  ["blue", 1],
  ["cosmos", 1],
  ["lavender", 0.6],
];

/** 确定性伪随机：同一个种子永远铺出同一片花，切页面回来不会「换一片花」 */
function gardenRng(seed: number) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const clamp01 = (x: number) => (x < 0 ? 0 : x > 1 ? 1 : x);
const easeOut = (t: number) => 1 - Math.pow(1 - t, 3);
function backOut(t: number) {
  const c1 = 1.70158;
  const c3 = c1 + 1;
  return 1 + c3 * Math.pow(t - 1, 3) + c1 * Math.pow(t - 1, 2);
}

function pickKind(r: number, tail: boolean): Kind {
  const w = tail ? TAIL_W : KIND_W;
  let sum = 0;
  for (const [, x] of w) sum += x;
  let q = r * sum;
  for (const [k, x] of w) {
    q -= x;
    if (q <= 0) return k;
  }
  return w[0][0];
}

function makeSlot(r: () => number, t: number, kind: Kind, len: number): Slot {
  const leafN = kind === "bud" ? (r() < 0.5 ? 0 : 1) : r() < 0.55 ? 1 : 2;
  const leaves: Leaf[] = [];
  let side = r() < 0.5 ? -1 : 1;
  for (let i = 0; i < leafN; i++) {
    leaves.push({ f: 0.2 + r() * 0.35, side });
    side = -side;
  }
  return {
    t,
    kind,
    len,
    lean: (r() - 0.5) * 0.12,
    curve: (r() - 0.5) * 0.14,
    phase: r() * Math.PI * 2,
    period: 2.5 + r() * 1.5,
    leaves,
    bud: Math.floor(r() * 3),
    alive: false,
    age: 0,
    delay: 0,
    s: 0,
    h: 0,
    bend: 0,
  };
}

function buildSlots(): Slot[] {
  const r = gardenRng(GARDEN_CFG.seed * 9973 + 1);
  const out = FIXED_SLOTS.map(([t, k, l]) => makeSlot(r, t, k, l));
  const rest = GARDEN_CFG.flowerCount - FIXED_SLOTS.length;
  const a = 0.322;
  const b = 0.992;
  let prev: Kind = "poppy";
  for (let i = 0; i < rest; i++) {
    const t = a + ((b - a) * (i + 0.5 + (r() - 0.5) * 0.5)) / rest;
    const tail = t > 0.86;
    let kind = pickKind(r(), tail);
    // 相邻同种会连成一条色带，重抽一次
    if (kind === prev) kind = pickKind(r(), tail);
    prev = kind;
    // 尾部收窄：右端花丛自然矮下去，不给画布右缘留一条硬边
    const taper = tail ? 1 - ((t - 0.86) / 0.14) * 0.45 : 1;
    let len = (GARDEN_CFG.stemMin + (GARDEN_CFG.stemMax - GARDEN_CFG.stemMin) * r()) * taper;
    if (kind === "bud") len *= 0.9;
    const sl = makeSlot(r, t, kind, len);
    // 最后一株固定成小花苞：收尾更轻，也不会在右边界被截断成半朵花
    if (i === rest - 1) {
      sl.kind = "bud";
      sl.lean = 0.16;
      sl.leaves = [];
    }
    out.push(sl);
  }
  return out;
}

function buildTufts() {
  const r = gardenRng(4242);
  const out: Array<{ d: number; h: number; lean: number }> = [];
  for (let d = 2; d < 946; d += 5 + r() * 4) {
    out.push({ d, h: 6 + r() * 8, lean: (r() - 0.5) * 0.52 });
  }
  return out;
}

function kindColor(k: Kind, bud: number, isDark: boolean): string {
  switch (k) {
    case "tulip":
      return GARDEN_CFG.tulipColor;
    case "poppy":
      return GARDEN_CFG.poppyColor;
    case "pinkTulip":
    case "cosmos":
      return GARDEN_CFG.pinkColor;
    case "daisy":
      return isDark ? "#F0E3DA" : "#E6D5C8";
    case "blue":
      return GARDEN_CFG.blueColor;
    case "lavender":
      return GARDEN_CFG.lilacColor;
    case "aster":
      return GARDEN_CFG.asterColor;
    default:
      return bud === 0
        ? isDark
          ? "#F0E3DA"
          : "#DECBC0"
        : bud === 1
          ? GARDEN_CFG.pinkColor
          : GARDEN_CFG.asterColor;
  }
}

function ringOfCircles(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  n: number,
  rr: number,
  pr: number,
  rot: number,
) {
  ctx.beginPath();
  for (let i = 0; i < n; i++) {
    const a = rot + (i / n) * Math.PI * 2;
    const x = cx + Math.cos(a) * rr;
    const y = cy + Math.sin(a) * rr;
    ctx.moveTo(x + pr, y);
    ctx.arc(x, y, pr, 0, Math.PI * 2);
  }
  ctx.fill();
}

function disc(ctx: CanvasRenderingContext2D, x: number, y: number, r: number) {
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fill();
}

function petalRing(
  ctx: CanvasRenderingContext2D,
  cy: number,
  n: number,
  rr: number,
  rx: number,
  ry: number,
) {
  ctx.beginPath();
  for (let i = 0; i < n; i++) {
    const a = (i / n) * Math.PI * 2;
    const x = Math.cos(a) * rr;
    const y = cy + Math.sin(a) * rr;
    ctx.moveTo(x + Math.cos(a) * rx, y + Math.sin(a) * rx);
    ctx.ellipse(x, y, rx, ry, a, 0, Math.PI * 2);
  }
  ctx.fill();
}

/** 九种花冠。画在原点上方（茎的顶端在 y=0），调用方负责平移/旋转/缩放。 */
function drawHead(ctx: CanvasRenderingContext2D, sl: Slot, isDark: boolean) {
  ctx.fillStyle = kindColor(sl.kind, sl.bud, isDark);
  switch (sl.kind) {
    case "tulip":
    case "pinkTulip": {
      const k = sl.kind === "pinkTulip" ? 0.9 : 1;
      ctx.beginPath();
      ctx.moveTo(-10 * k, -16 * k);
      ctx.lineTo(-5 * k, -11 * k);
      ctx.lineTo(0, -18 * k);
      ctx.lineTo(5 * k, -11 * k);
      ctx.lineTo(10 * k, -16 * k);
      ctx.quadraticCurveTo(11 * k, 0, 0, k);
      ctx.quadraticCurveTo(-11 * k, 0, -10 * k, -16 * k);
      ctx.closePath();
      ctx.fill();
      break;
    }
    case "poppy": {
      ringOfCircles(ctx, 0, -12, 8, 9, 6, sl.phase);
      disc(ctx, 0, -12, 10);
      ctx.fillStyle = "#7A3A34";
      disc(ctx, 0, -12, 3.5);
      break;
    }
    case "cosmos": {
      ringOfCircles(ctx, 0, -11, 6, 7.5, 6.5, sl.phase);
      disc(ctx, 0, -11, 10);
      ctx.fillStyle = "#E0707A";
      disc(ctx, 0, -11, 4.5);
      break;
    }
    case "daisy": {
      petalRing(ctx, -10, 11, 6.5, 5, 2.4);
      ctx.fillStyle = GARDEN_CFG.centreColor;
      disc(ctx, 0, -10, 4);
      break;
    }
    case "aster": {
      petalRing(ctx, -10, 12, 7, 5.5, 1.9);
      ctx.fillStyle = "#EFE6F2";
      disc(ctx, 0, -10, 3.5);
      break;
    }
    case "blue": {
      ringOfCircles(ctx, 0, -9, 5, 5, 4.3, -Math.PI / 2);
      ctx.fillStyle = GARDEN_CFG.centreColor;
      disc(ctx, 0, -9, 2);
      break;
    }
    case "lavender": {
      ctx.beginPath();
      for (let i = 0; i < 6; i++) {
        const s = 0.7 + (0.3 * i) / 5;
        const x = i % 2 ? 1.5 : -1.5;
        const y = -6 + i * 6.5;
        ctx.moveTo(x + 4.2 * s, y);
        ctx.ellipse(x, y, 4.2 * s, 4 * s, 0, 0, Math.PI * 2);
      }
      ctx.fill();
      break;
    }
    default:
      disc(ctx, 0, -2, 2.8);
  }
}

/** 小蜜蜂：黄身 + 两道黑条纹 + 半透明翅膀 + 一根触角 */
function drawBee(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  k: number,
  tilt: number,
  flap: number,
) {
  ctx.save();
  ctx.translate(x, y);
  ctx.rotate(tilt);
  ctx.scale(k, k);

  // 深色底上黄身会「陷」进背景：先描一圈亮边把它提出来
  const isDark =
    typeof document !== "undefined" && document.documentElement.classList.contains("dark");
  if (isDark) {
    ctx.strokeStyle = "rgba(255, 255, 255, 0.35)";
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.ellipse(0, 0, 17, 13, 0, 0, Math.PI * 2);
    ctx.stroke();
  }

  ctx.fillStyle = GARDEN_CFG.beeColor;
  ctx.beginPath();
  ctx.ellipse(0, 0, 17, 13, 0, 0, Math.PI * 2);
  ctx.fill();

  // 条纹裁剪进身体椭圆，边缘不会溢出
  ctx.save();
  ctx.clip();
  ctx.fillStyle = "#3A3530";
  ctx.fillRect(-8, -14, 3.5, 28);
  ctx.fillRect(0, -14, 3.5, 28);
  ctx.restore();

  ctx.fillStyle = "#3A3530";
  disc(ctx, 11, -3, 1.8);

  ctx.strokeStyle = "#3A3530";
  ctx.lineWidth = 1.2;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(12, -9);
  ctx.quadraticCurveTo(15, -13, 18, -14);
  ctx.stroke();

  ctx.fillStyle = "rgba(186, 200, 214, 0.78)";
  ctx.beginPath();
  ctx.ellipse(-2, -15, 5.5, 8 * flap, -0.15, 0, Math.PI * 2);
  ctx.fill();

  ctx.restore();
}

export class GardenEngine {
  private canvas: HTMLCanvasElement;
  private container: HTMLElement;
  private ctx: CanvasRenderingContext2D;
  private opts: GardenOptions;

  private slots: Slot[] = buildSlots();
  private tufts = buildTufts();
  private petals: Petal[] = [];

  private W = 0;
  private H = 0;
  private dpr = 1;

  private targetV = 0;
  private prevV = 0;
  private pct = 0;
  private lastShownPct = -1;
  /** 真实进度：开花动画从 0 长到这个值，而不是固定长满 */
  private realV = 0;

  private beeX = 0;
  private beeV = 0;

  private first = true;
  private autoIntroDone = false;
  private introAnimating = false;
  private introElapsed = 0;
  private introDuration = 4.8;

  // 只有横向位置参与计算：花是朝光标方向弯腰，纵向距离不需要
  private pointerX = -999;
  private pointerOver = 0;
  private pointerInside = false;
  private pointerDown = false;

  private clock = 0;
  private lastFrame = -1;
  private rafId = 0;
  private running = false;
  private reducedMotion = false;
  private ro: ResizeObserver | null = null;
  /** 花园带高度：调用方给就给，没给由画布高估算 */
  private bandPx = 0;

  constructor(canvas: HTMLCanvasElement, container: HTMLElement, opts: GardenOptions = {}) {
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("canvas 2d context unavailable");
    this.canvas = canvas;
    this.container = container;
    this.ctx = ctx;
    this.opts = opts;
    this.bandPx = opts.bandPx ?? 0;
    this.reducedMotion =
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    this.resize();
    if (typeof ResizeObserver !== "undefined") {
      this.ro = new ResizeObserver(() => this.resize());
      this.ro.observe(container);
    }
  }

  /** 开始跑帧循环（重复调用无副作用） */
  start() {
    if (this.running) return;
    this.running = true;
    this.lastFrame = -1;
    this.rafId = requestAnimationFrame(this.frame);

    // 首帧画完之后自动播一次开花，等同原型里「打开页面花园自己长出来」
    if (!this.autoIntroDone) {
      this.autoIntroDone = true;
      if (!this.reducedMotion) {
        window.setTimeout(() => this.replay(), 120);
      }
    }
  }

  stop() {
    this.running = false;
    cancelAnimationFrame(this.rafId);
    this.rafId = 0;
  }

  destroy() {
    this.stop();
    this.ro?.disconnect();
    this.ro = null;
  }

  /** 目标进度 0..1。会打断正在播的开花动画，交给用户/新数据接管。 */
  setProgress(v: number) {
    this.introAnimating = false;
    this.realV = clamp01(v);
    this.targetV = this.realV;
  }

  /** 重播一次「从 0 长到当前真实进度」的开花动画 */
  replay() {
    this.introAnimating = false;
    this.targetV = 0;
    this.prevV = 0;
    this.beeX = 24;
    this.beeV = 0;
    this.pct = 0;
    this.petals = [];
    for (const sl of this.slots) {
      sl.alive = false;
      sl.age = 0;
      sl.delay = 0;
      sl.s = 0;
      sl.h = 0;
    }
    window.setTimeout(() => {
      this.introElapsed = 0;
      this.introAnimating = true;
    }, 60);
  }

  /** 容器尺寸变化：重设 backing store，DPR 上限 2 避免 4K 屏上过度绘制 */
  resize() {
    const w = this.container.clientWidth || 800;
    const h = this.container.clientHeight || 150;
    if (w === this.W && h === this.H) return;
    this.W = w;
    this.H = h;
    this.dpr = Math.min(window.devicePixelRatio || 1, 2);
    this.canvas.width = Math.max(1, Math.round(w * this.dpr));
    this.canvas.height = Math.max(1, Math.round(h * this.dpr));
  }

  pointerDownAt(clientX: number) {
    const rect = this.container.getBoundingClientRect();
    this.pointerDown = true;
    this.pointerX = clientX - rect.left;
    if (!this.opts.scrub) return;
    this.introAnimating = false;
    this.targetFromClientX(clientX, rect);
  }

  pointerMoveAt(clientX: number) {
    const rect = this.container.getBoundingClientRect();
    this.pointerX = clientX - rect.left;
    this.pointerInside = true;
    if (this.pointerDown && this.opts.scrub) {
      this.introAnimating = false;
      this.targetFromClientX(clientX, rect);
    }
  }

  pointerLeaveAll() {
    this.pointerInside = false;
    this.pointerDown = false;
  }

  pointerUpAll() {
    this.pointerDown = false;
  }

  private targetFromClientX(clientX: number, rect: DOMRect) {
    const pad = 24;
    const L = Math.max(1, rect.width - pad * 2);
    this.realV = clamp01((clientX - rect.left - pad) / L);
    this.targetV = this.realV;
  }

  private frame = (now: number) => {
    if (!this.running) return;
    this.rafId = requestAnimationFrame(this.frame);

    const dt = this.lastFrame < 0 ? 0.016 : Math.min((now - this.lastFrame) / 1000, 0.05);
    this.lastFrame = now;
    this.clock += dt;

    this.pointerOver += ((this.pointerInside ? 1 : 0) - this.pointerOver) * (1 - Math.exp(-8 * dt));

    if (this.introAnimating) {
      this.introElapsed += dt;
      const p = clamp01(this.introElapsed / this.introDuration);
      // easeOutQuart：前段从容铺展，末段柔和减速。
      // 长到「真实进度」就停，不是固定长满——只有全部签完才会开满。
      this.targetV = this.realV * (1 - Math.pow(1 - p, 4));
      if (p >= 1) {
        this.targetV = this.realV;
        this.introAnimating = false;
      }
    }

    this.draw(dt);

    // 数字平滑跟随，避免每帧跳整数
    if (this.first) this.pct = this.targetV * 100;
    else this.pct += (this.targetV * 100 - this.pct) * (1 - Math.exp(-dt / 0.2));
    const shown = Math.round(this.pct);
    if (shown !== this.lastShownPct) {
      this.lastShownPct = shown;
      this.opts.onProgress?.(shown);
    }

    this.prevV = this.targetV;
    this.first = false;
  };

  private draw(dt: number) {
    const ctx = this.ctx;
    const isDark =
      typeof document !== "undefined" && document.documentElement.classList.contains("dark");
    const { W, H } = this;
    const pad = 24;
    const L = Math.max(100, W - pad * 2);
    const u = Math.max(0.5, L / 946);
    const x0 = pad;
    const trackY = H - 10;
    const v = this.targetV;
    const headX = x0 + v * L;

    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);

    /* 1. 轨道底线与草绿已填充进度 */
    const thick = GARDEN_CFG.trackThick * u;
    ctx.lineCap = "round";
    ctx.lineWidth = thick;
    ctx.strokeStyle = isDark ? "rgba(255, 255, 255, 0.14)" : "rgba(56, 57, 54, 0.14)";
    ctx.beginPath();
    ctx.moveTo(x0, trackY);
    ctx.lineTo(x0 + L, trackY);
    ctx.stroke();

    const green = isDark ? "#7C9A70" : "#6C8462";
    if (v > 0.001) {
      ctx.strokeStyle = green;
      ctx.beginPath();
      ctx.moveTo(x0, trackY);
      ctx.lineTo(headX, trackY);
      ctx.stroke();

      // 草茎上的细碎草簇
      ctx.strokeStyle = isDark ? "#66825B" : GARDEN_CFG.tuftColor;
      ctx.lineWidth = 1.3 * u;
      ctx.beginPath();
      for (const tf of this.tufts) {
        const x = x0 + tf.d * u;
        if (x > headX) break;
        const y = trackY - thick / 2;
        ctx.moveTo(x, y + 0.5 * u);
        ctx.lineTo(x + Math.sin(tf.lean) * tf.h * u, y - Math.cos(tf.lean) * tf.h * u);
      }
      ctx.stroke();
    }

    /* 2. 52 株花：生长、微风摇曳、鼠标靠近弯腰 */
    const n = this.slots.length;
    const sig = 70 * u;
    const headK = (GARDEN_CFG.headScale / 100) * u * 0.85;
    const stemW = GARDEN_CFG.stemWidth * u;
    const growS = GARDEN_CFG.growMs / 1000;
    const popS = GARDEN_CFG.popMs / 1000;
    const wiltS = GARDEN_CFG.wiltMs / 1000;
    const staggerS = GARDEN_CFG.staggerMs / 1000;
    const swayRad = (GARDEN_CFG.swayDeg * Math.PI) / 180;
    // 株高缩放：花园带（bandPx）是卡片底部为花丛预留的那段，
    // 减去花冠自身高度就是茎能用的净高度。
    // 上限 0.60 与原型一致——再高就变成「细长豆芽」，花园的层次感就没了。
    const band = this.bandPx > 0 ? this.bandPx : Math.max(88, Math.min(124, H * 0.42));
    const maxStem = GARDEN_CFG.stemMax * u;
    const headRoom = 26 * u; // 花冠自原点向上的视觉半径
    const heightScale = Math.min(0.6, Math.max(0.32, (band - headRoom) / maxStem));

    for (const sl of this.slots) {
      const bx = x0 + sl.t * L;
      const dx = bx - this.pointerX;
      const near =
        this.pointerOver > 0.001
          ? Math.sign(dx || 1) *
            Math.exp(-(dx * dx) / (sig * sig)) *
            this.pointerOver *
            (GARDEN_CFG.hover / 50) *
            ((6 * Math.PI) / 180)
          : 0;
      sl.bend += (near - sl.bend) * (1 - Math.exp(-10 * dt));

      let justDied = false;
      if (!sl.alive && sl.t <= v) {
        sl.alive = true;
        if (this.first) {
          // 首帧直接到位：页面刚打开时不该看到 52 株花从零开始各长一遍
          sl.age = 99;
          sl.delay = 0;
        } else {
          sl.age = (1 - Math.cbrt(1 - Math.min(1, sl.s))) * growS;
          sl.delay = sl.s > 0 ? 0 : Math.max(0, sl.t - this.prevV) * n * staggerS;
        }
      } else if (sl.alive && sl.t > v + 0.002) {
        sl.alive = false;
        justDied = true;
      }

      if (sl.alive) {
        if (sl.delay > 0) sl.delay -= dt;
        else sl.age += dt;
        sl.s = easeOut(clamp01(sl.age / growS));
        sl.h = sl.age > 0.7 * growS ? backOut(clamp01((sl.age - 0.7 * growS) / popS)) : 0;
      }

      const th =
        sl.lean +
        Math.sin((this.clock * Math.PI * 2) / sl.period + sl.phase) * swayRad +
        sl.bend;
      const len = sl.len * heightScale * u * sl.s;
      const tx = bx + Math.sin(th) * len;
      const ty = trackY - Math.cos(th) * len;
      const head = sl.h;

      if (justDied && sl.h > 0.3) this.spawnPetals(sl, tx, ty, u, isDark);
      if (!sl.alive) {
        sl.s = Math.max(0, sl.s - dt / wiltS);
        sl.h = 0;
      }
      if (len < 0.5) continue;

      const cpx = (bx + tx) / 2 + Math.cos(th) * sl.curve * len;
      const cpy = (trackY + ty) / 2 + Math.sin(th) * sl.curve * len;
      ctx.strokeStyle = green;
      ctx.fillStyle = green;
      ctx.lineWidth = sl.kind === "bud" ? stemW * 0.65 : stemW;
      ctx.beginPath();
      ctx.moveTo(bx, trackY);
      ctx.quadraticCurveTo(cpx, cpy, tx, ty);
      ctx.stroke();

      const g = clamp01((sl.s - 0.4) / 0.35);
      if (g > 0) {
        for (const lf of sl.leaves) {
          const f = lf.f;
          const q = 1 - f;
          const px = q * q * bx + 2 * q * f * cpx + f * f * tx;
          const py = q * q * trackY + 2 * q * f * cpy + f * f * ty;
          const la = th + lf.side * 0.75;
          const lx = Math.sin(la);
          const ly = -Math.cos(la);
          ctx.beginPath();
          ctx.ellipse(
            px + lx * 5 * u * g,
            py + ly * 5 * u * g,
            5 * u * g,
            2.3 * u * g,
            la - Math.PI / 2,
            0,
            Math.PI * 2,
          );
          ctx.fill();
        }
      }

      if (head > 0.01 && !justDied) {
        ctx.save();
        ctx.translate(tx, ty);
        ctx.rotate(th * 0.7);
        const k = headK * head;
        ctx.scale(k, k);
        drawHead(ctx, sl, isDark);
        ctx.restore();
      }
    }

    /* 3. 退场飘瓣：重力 + 阻尼的抛物线 */
    const keep: Petal[] = [];
    for (const pt of this.petals) {
      pt.life -= dt;
      if (pt.life <= 0) continue;
      pt.vy += 18 * u * dt;
      const drag = Math.exp(-1.0 * dt);
      pt.vx *= drag;
      pt.vy *= drag;
      pt.x += pt.vx * dt;
      pt.y += pt.vy * dt;
      pt.a += pt.spin * dt;
      ctx.globalAlpha = clamp01(pt.life / (0.35 * pt.max));
      ctx.fillStyle = pt.color;
      ctx.beginPath();
      ctx.ellipse(pt.x, pt.y, pt.len, pt.round ? pt.len : pt.len * 0.45, pt.a, 0, Math.PI * 2);
      ctx.fill();
      keep.push(pt);
    }
    ctx.globalAlpha = 1;
    this.petals = keep;

    /* 4. 临界阻尼弹簧追随进度头部的小蜜蜂 */
    const targetBee = Math.min(x0 + L, Math.max(x0, headX + GARDEN_CFG.beeOffset * u));
    if (this.first) {
      this.beeX = targetBee;
      this.beeV = 0;
    } else {
      const w = GARDEN_CFG.beeSpring;
      this.beeV += (w * w * (targetBee - this.beeX) - 2 * w * this.beeV) * dt;
      this.beeX += this.beeV * dt;
    }
    const bob = Math.sin(this.clock * Math.PI * 2 * 3) * GARDEN_CFG.beeBob * u;
    const tilt = Math.max(-0.3, Math.min(0.3, (this.beeV / Math.max(1, 1500 * u)) * 0.25));
    const flap = 0.7 + 0.3 * Math.abs(Math.sin(this.clock * 55));
    drawBee(
      ctx,
      this.beeX,
      trackY - 22 * u + bob,
      u * (GARDEN_CFG.beeSize / 100),
      tilt,
      flap,
    );
  }

  private spawnPetals(sl: Slot, x: number, y: number, u: number, isDark: boolean) {
    const n = 3 + Math.floor(Math.random() * Math.max(1, GARDEN_CFG.petalCount - 2));
    const color = kindColor(sl.kind, sl.bud, isDark);
    const round = sl.kind === "blue" || sl.kind === "bud" || sl.kind === "lavender";
    const rise = GARDEN_CFG.petalRise * u;
    const life = GARDEN_CFG.petalMs / 1000;
    for (let i = 0; i < n; i++) {
      if (this.petals.length > 400) break;
      const l = life * (0.8 + Math.random() * 0.4);
      this.petals.push({
        x: x + (Math.random() - 0.5) * 16 * u,
        y: y - 10 * u + (Math.random() - 0.5) * 12 * u,
        vx: (Math.random() * 1.6 - 0.5) * rise * 0.7,
        vy: -(0.4 + Math.random() * 0.8) * rise,
        a: Math.random() * Math.PI * 2,
        spin: (Math.random() - 0.5) * 6,
        life: l,
        max: l,
        color,
        len: round ? 2.4 * u : (3 + Math.random() * 3) * u,
        round,
      });
    }
  }
}

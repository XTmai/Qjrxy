// 任务状态与时间窗口判断（从旧版 ui.js 移植，TS 化）。
import type { Task } from './types';

export interface TaskState {
  text: string;
  tone: 'ok' | 'warn' | 'err' | 'info' | 'plain';
  runnable: boolean;
}

export interface TaskWindow {
  start: number;
  end: number;
}

/**
 * 解析任务由 signDate + beginClock~endClock 定义的签到时段。
 * signDate 形如 "2026-11-03" 或 "2026-11-03 (周二)"。
 * 返回毫秒时间戳；跨天时段（如 23:30-00:30）自动把结束推到次日。
 */
export function parseTaskWindow(task: Task | null | undefined): TaskWindow | null {
  if (!task) return null;
  let dateRaw = String(task.signDate || '').trim();
  if (!dateRaw) return null;
  const cut = dateRaw.search(/[\s(（]/);
  if (cut > 0) dateRaw = dateRaw.slice(0, cut);
  const begin = String(task.beginClock || '').trim();
  const end = String(task.endClock || '').trim();
  if (!dateRaw || !begin || !end) return null;

  const toTs = (clock: string) => {
    const d = new Date(`${dateRaw}T${clock}:00`);
    return Number.isNaN(d.getTime()) ? null : d.getTime();
  };
  const start = toTs(begin);
  let endTs = toTs(end);
  if (start === null || endTs === null) return null;
  if (endTs <= start) endTs += 24 * 60 * 60 * 1000;
  return { start, end: endTs };
}

/**
 * 任务状态。
 * signStatus: 0/1 可签到 | 2 未到时间 | 3 已过期 | 4 已签到 | 空 → 按时间字段判断
 * 部分学校不下发 signStatus，一律当作可执行会把「21:00 才开始」的任务
 * 在 16:00 显示成可执行，点了才被服务端拒绝。
 */
export function taskState(task: Task | null | undefined): TaskState {
  if (!task) return { text: '未知', tone: 'plain', runnable: false };
  if (task.status === 'done') return { text: '已处理', tone: 'ok', runnable: false };
  if (task.status === 'leave') return { text: '已请假', tone: 'plain', runnable: false };
  const st = String(task.signStatus || '').trim();
  if (st === '2') return { text: '未到时间', tone: 'warn', runnable: false };
  if (st === '3') return { text: '已过期', tone: 'plain', runnable: false };
  if (st === '4') return { text: '已签到', tone: 'ok', runnable: false };

  const win = parseTaskWindow(task);
  if (win) {
    const now = Date.now();
    if (now < win.start) return { text: '未到时间', tone: 'warn', runnable: false };
    if (now > win.end) return { text: '已过期', tone: 'plain', runnable: false };
  }
  return { text: '可执行', tone: 'info', runnable: true };
}

/** 任务时间展示：优先「日期 + 时段」，否则起止时间。 */
export function taskTime(task: Task): string {
  if (!task) return '—';
  if (task.signDate || task.beginClock) {
    const date = String(task.signDate || '').split(' ')[0];
    const clock =
      task.beginClock && task.endClock
        ? `${task.beginClock} – ${task.endClock}`
        : task.beginClock || '';
    return [date, clock].filter(Boolean).join('  ') || '时间待定';
  }
  const norm = (v?: string) =>
    v ? String(v).replace('T', ' ').replace(/\.\d+/, '').replace(/(Z|[+-]\d{2}:?\d{2})$/, '').slice(0, 16) : '';
  const s = norm(task.startTime);
  const e = norm(task.endTime);
  if (!s && !e) return '时间待定';
  return e ? `${s} → ${e}` : s;
}

const pad2 = (n: number) => String(n).padStart(2, '0');

export function formatDateTime(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

export function relativeTime(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const diff = Date.now() - d.getTime();
  const min = Math.floor(diff / 60000);
  if (min < 1) return '刚刚';
  if (min < 60) return `${min} 分钟前`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour} 小时前`;
  const day = Math.floor(hour / 24);
  if (day < 30) return `${day} 天前`;
  return formatDateTime(iso);
}

const WEEK = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];

export function todayLabel(date = new Date()): string {
  return `${date.getMonth() + 1}月${date.getDate()}日${WEEK[date.getDay()]}`;
}

/** 计划执行日期的可读描述 */
export function describeDays(days: number[] | undefined): string {
  const names = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];
  const set = new Set(days || []);
  if (set.size === 7) return '每天';
  if (set.size === 5 && [0, 1, 2, 3, 4].every((d) => set.has(d))) return '工作日';
  if (set.size === 2 && set.has(5) && set.has(6)) return '周末';
  return [...set].sort((a, b) => a - b).map((d) => names[d]).join('、') || '每天';
}

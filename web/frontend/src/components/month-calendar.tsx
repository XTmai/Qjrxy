// 手写月历：计划「指定日期」多选。
// 不引入 react-day-picker/date-fns——只需要一个月份网格，多带两个依赖不值当。
import { useState } from "react";
import { ChevronLeft, ChevronRight, X } from "lucide-react";

import { Button } from "@/components/ui/button";

const WEEKDAYS = ["一", "二", "三", "四", "五", "六", "日"];

function pad(n: number) {
  return String(n).padStart(2, "0");
}

function ymd(y: number, m: number, d: number) {
  return `${y}-${pad(m)}-${pad(d)}`;
}

interface Props {
  /** 已选日期（YYYY-MM-DD，升序） */
  value: string[];
  onChange: (dates: string[]) => void;
}

export function MonthCalendar({ value, onChange }: Props) {
  const today = new Date();
  const todayKey = ymd(today.getFullYear(), today.getMonth() + 1, today.getDate());
  const [cursor, setCursor] = useState(() => new Date(today.getFullYear(), today.getMonth(), 1));

  const selected = new Set(value);
  const year = cursor.getFullYear();
  const month = cursor.getMonth() + 1;
  // 周一作为一周起点：JS 的 getDay() 里周日是 0
  const firstWeekday = (new Date(year, month - 1, 1).getDay() + 6) % 7;
  const daysInMonth = new Date(year, month, 0).getDate();

  const cells: (string | null)[] = [];
  for (let i = 0; i < firstWeekday; i++) cells.push(null);
  for (let d = 1; d <= daysInMonth; d++) cells.push(ymd(year, month, d));

  const toggle = (key: string) => {
    const next = new Set(selected);
    if (next.has(key)) {
      next.delete(key);
    } else {
      next.add(key);
    }
    onChange([...next].sort());
  };

  const shiftMonth = (delta: number) => {
    setCursor(new Date(year, month - 1 + delta, 1));
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="icon" className="size-7" onClick={() => shiftMonth(-1)} aria-label="上个月">
          <ChevronLeft className="size-4" />
        </Button>
        <span className="text-sm font-medium tabular-nums">
          {year} 年 {month} 月
        </span>
        <Button variant="ghost" size="icon" className="size-7" onClick={() => shiftMonth(1)} aria-label="下个月">
          <ChevronRight className="size-4" />
        </Button>
      </div>

      <div className="grid grid-cols-7 gap-1 text-center">
        {WEEKDAYS.map((w) => (
          <span key={w} className="text-muted-foreground py-1 text-[10px]">
            {w}
          </span>
        ))}
        {cells.map((key, idx) =>
          key === null ? (
            <span key={`blank-${idx}`} />
          ) : (
            <button
              key={key}
              type="button"
              onClick={() => toggle(key)}
              className={`h-8 rounded-md text-xs transition-colors ${
                selected.has(key)
                  ? "bg-primary text-primary-foreground font-medium"
                  : key === todayKey
                    ? "border-border text-foreground border"
                    : "text-muted-foreground hover:bg-muted"
              }`}
            >
              {Number(key.slice(-2))}
            </button>
          ),
        )}
      </div>

      {value.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {value.map((d) => (
            <span
              key={d}
              className="bg-muted text-foreground inline-flex items-center gap-1 rounded-md py-0.5 pr-1 pl-1.5 font-mono text-[10px]"
            >
              {d.slice(5)}
              <button
                type="button"
                onClick={() => toggle(d)}
                className="text-muted-foreground hover:text-foreground"
                aria-label={`移除 ${d}`}
              >
                <X className="size-3" />
              </button>
            </span>
          ))}
          <button
            type="button"
            onClick={() => onChange([])}
            className="text-muted-foreground hover:text-foreground text-[10px] underline underline-offset-2"
          >
            清空
          </button>
        </div>
      )}
    </div>
  );
}

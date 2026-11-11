// 签到验证码人工弹窗：自动识别没认出来时，把九宫格交给用户点选。
//
// 由服务端挂起任务、这里轮询 /api/captcha/pending 取题目。之所以走轮询
// 而不是依赖 SSE：用户可能在手机上点、也可能刷新页面，轮询能自愈。
import { useCallback, useEffect, useRef, useState } from "react";

import { Check, Clock, Loader2, RefreshCw, ScanEye, X } from "lucide-react";
import { api } from "@/api";
import { useAccounts } from "@/hooks/use-accounts";
import type { ManualCaptcha } from "@/lib/types";
import { useCaptchaStore } from "@/stores/captcha-store";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

/** 轮询间隔：换一张、选错重出、超时都会让 token 变化，2s 足够跟手 */
const POLL_MS = 2000;
/** 换一张后服务端要等 15s 才能重新出题，这里按最坏情况提示 */
const REFRESH_HINT_SEC = 15;

export function CaptchaChallengeDialog() {
  const { data: accounts = [] } = useAccounts();
  const signal = useCaptchaStore((s) => s.signal);

  const [items, setItems] = useState<ManualCaptcha[]>([]);
  const [selected, setSelected] = useState<number[]>([]);
  const [submitted, setSubmitted] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [skipping, setSkipping] = useState(false);
  const [left, setLeft] = useState(0);
  const prevToken = useRef("");

  const current = items[0];
  const busy = !!current && submitted === current.token;

  const load = useCallback(async () => {
    try {
      const list = await api.captchaPending();
      setItems(Array.isArray(list) ? list : []);
      if (list.length > 0) setSkipping(false);
    } catch {
      // 网络抖动忽略：下一轮继续，用户还能看到上次的题目
    }
  }, []);

  // 挂载即拉一次，之后跟随服务端通知
  useEffect(() => {
    void load();
  }, [load, signal]);

  // 有题目（或正在换题）时持续轮询
  useEffect(() => {
    if (items.length === 0 && !skipping) return;
    const timer = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [items.length, skipping, load]);

  // 题目换了一张：清空选择，并说明上一张为什么没了
  useEffect(() => {
    if (!current) return;
    if (prevToken.current && prevToken.current !== current.token && submitted) {
      setNote("上一次没通过，已换一张，请再试一次");
    } else {
      setNote("");
    }
    prevToken.current = current.token;
    setSelected([]);
    setError("");
  }, [current?.token]); // eslint-disable-line react-hooks/exhaustive-deps

  // 倒计时：让用户知道还有多久，而不是干等
  useEffect(() => {
    if (!current) {
      setLeft(0);
      return;
    }
    const deadline = new Date(current.expiresAt).getTime();
    const tick = () => setLeft(Math.max(0, Math.ceil((deadline - Date.now()) / 1000)));
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [current?.token, current?.expiresAt]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggle = (cell: number) => {
    if (busy) return;
    setSelected((prev) => (prev.includes(cell) ? prev.filter((c) => c !== cell) : [...prev, cell].sort((a, b) => a - b)));
    setError("");
  };

  const submit = async () => {
    if (!current || busy || selected.length === 0) return;
    const token = current.token;
    setSubmitted(token);
    setError("");
    setNote("已提交，正在校验…");
    try {
      await api.captchaSubmit(token, { cells: selected });
      void load();
    } catch (e) {
      setSubmitted("");
      setNote("");
      setError(e instanceof Error ? e.message : "提交失败，请重试");
    }
  };

  const skip = async () => {
    if (!current || busy) return;
    const token = current.token;
    setSubmitted(token);
    setSkipping(true);
    setError("");
    setNote(`正在换一张，约 ${REFRESH_HINT_SEC} 秒…`);
    setItems([]);
    try {
      await api.captchaSubmit(token, { action: "skip" });
    } catch (e) {
      setSkipping(false);
      setNote("");
      setError(e instanceof Error ? e.message : "换一张失败，请重试");
    }
  };

  const cancel = async () => {
    if (!current) return;
    const token = current.token;
    setItems((prev) => prev.filter((x) => x.token !== token));
    setSkipping(false);
    try {
      await api.captchaSubmit(token, { action: "cancel" });
    } catch {
      // 服务端已经结束了这段等待，忽略即可
    }
  };

  if (!current && !skipping) return null;

  const account = accounts.find((a) => a.id === current?.accountId);
  const who = account ? account.alias || account.schoolName : "";

  return (
    <Dialog open onOpenChange={() => {}}>
      <DialogContent
        // [&>button.absolute] 是弹窗自带的右上角关闭按钮：它和「取消本次」
        // 语义重复，而误点一下就作废整次签到，直接藏掉更安全。
        className="max-w-[min(440px,calc(100vw-2rem))] gap-0 overflow-hidden p-0 [&>button.absolute]:hidden"
        // 遮罩点击与 ESC 都不关闭：误触会让这次签到直接作废，只留明确的取消按钮
        onPointerDownOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
        // 默认焦点会落在第一格上，看起来像「已选中第 1 张」。
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <div className="flex items-start gap-3 px-5 pt-5">
          <div className="bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-lg">
            <ScanEye className="size-5" />
          </div>
          <div className="min-w-0 flex-1">
            <DialogTitle className="text-base">签到需要验证码</DialogTitle>
            <DialogDescription className="mt-1 text-xs">
              {who ? `${who} · ` : ""}
              {"请选出"}
              {current?.name ? <span className="text-foreground font-medium">包含「{current.name}」</span> : "符合条件的"}
              {"的图片，可以多选"}
            </DialogDescription>
          </div>
        </div>

        <DialogBody className="gap-3 px-5 pb-5 pt-4">
          {current ? (
            <>
              <div className="relative aspect-square w-full overflow-hidden rounded-xl border">
                <img src={current.grid} alt="验证码九宫格" className="absolute inset-0 size-full select-none" draggable={false} />
                <div className="absolute inset-0 grid grid-cols-3 grid-rows-3">
                  {Array.from({ length: current.cells || 9 }, (_, i) => i + 1).map((cell) => {
                    const on = selected.includes(cell);
                    return (
                      <button
                        key={cell}
                        type="button"
                        onClick={() => toggle(cell)}
                        disabled={busy}
                        aria-pressed={on}
                        aria-label={`第 ${cell} 张`}
                        className={cn(
                          "relative flex items-start justify-start p-1.5 outline-none transition-colors",
                          "focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary",
                          on ? "bg-primary/25 ring-2 ring-inset ring-primary" : "hover:bg-white/20",
                        )}
                      >
                        {/* 编号放左上角：九宫格的价值就在于看清每张图，
                            标签压在正中间会挡住主体。 */}
                        <span
                          className={cn(
                            "flex size-5 items-center justify-center rounded-md text-[11px] font-semibold transition",
                            // 用任意值而不是 bg-black/45：浅色照片上要压得住，
                            // 透明度写死比跟随主题变量更可控。
                            on ? "bg-primary text-primary-foreground" : "bg-[rgba(15,23,42,0.58)] text-white",
                          )}
                        >
                          {on ? <Check className="size-3" /> : cell}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>

              <div className="flex items-center justify-between text-xs">
                <span className="text-muted-foreground inline-flex items-center gap-1">
                  <Clock className="size-3.5" />
                  {left > 0 ? `${left} 秒后失效` : "已失效，正在刷新"}
                </span>
                <span className="text-muted-foreground">
                  已选 <span className="text-foreground font-medium">{selected.length}</span> 张
                </span>
              </div>

              {note && <p className="text-muted-foreground text-xs">{note}</p>}
              {error && <p className="text-destructive text-xs">{error}</p>}

              <div className="flex gap-2 pt-1">
                <Button variant="ghost" size="lg" className="flex-1" onClick={() => void cancel()}>
                  <X className="size-4" />
                  取消本次
                </Button>
                <Button variant="outline" size="lg" className="flex-1" onClick={() => void skip()} disabled={busy}>
                  <RefreshCw className={cn("size-4", busy && "animate-spin")} />
                  换一张
                </Button>
                <Button size="lg" className="flex-[1.4]" onClick={() => void submit()} disabled={busy || selected.length === 0}>
                  {busy ? <Loader2 className="size-4 animate-spin" /> : null}
                  {busy ? "校验中" : "确认"}
                </Button>
              </div>
            </>
          ) : (
            <div className="flex flex-col items-center gap-3 py-10">
              <Loader2 className="text-muted-foreground size-6 animate-spin" />
              <p className="text-muted-foreground text-sm">正在重新出题，约 {REFRESH_HINT_SEC} 秒</p>
              <p className="text-muted-foreground/80 text-xs">服务端限制验证码刷新间隔，请稍候</p>
              <Button variant="ghost" size="sm" onClick={() => setSkipping(false)}>
                返回
              </Button>
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

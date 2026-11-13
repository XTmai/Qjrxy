// 扫码登录：用今日校园 App 扫 QR 获取登录态（无需密码/验证码）。
// 基于官方 connect/qrcode/jsLogin 流程，所有学校通用。
//
// 注意：campushoy 的二维码实际 TTL 约 60 秒。弹窗内每 50 秒自动换一张新码，
// 否则用户从电脑切到手机 App 的时间里码就过期了。
import { useEffect, useRef, useState } from "react";
import { Loader2, QrCode } from "lucide-react";

import { api } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";

interface Props {
  schoolName?: string;
  onClose: (loggedIn: boolean) => void;
}

type QRState = "loading" | "waiting" | "scanned" | "success" | "expired" | "error";

// campushoy 对同一个 qrId 的 validation 有频率限制：约 10 次后会直接返回
// status=5（二维码作废）。实测 1.5-2s 轮询 20 秒就失效，3-5s 可撑到 60s。
// 因此未扫码时 6 秒查一次，45 秒主动换码，把单个 qrId 的请求数压在 8 次内。
const REFRESH_SECONDS = 45;
const POLL_INTERVAL_MS = 6000;
const SCANNED_POLL_INTERVAL_MS = 2000;

export function QRLoginDialog({ schoolName, onClose }: Props) {
  const [state, setState] = useState<QRState>("loading");
  const [qrImage, setQrImage] = useState("");
  const [error, setError] = useState("");
  const [seconds, setSeconds] = useState(REFRESH_SECONDS);
  const ticketRef = useRef("");
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const stopRef = useRef(false);
  const startingRef = useRef(false);
  const stateRef = useRef<QRState>("loading");

  const updateState = (next: QRState) => {
    stateRef.current = next;
    setState(next);
  };

  const clearTimers = () => {
    if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
    if (countdownRef.current) clearInterval(countdownRef.current);
    pollTimerRef.current = null;
    countdownRef.current = null;
  };

  const beginCountdown = () => {
    if (countdownRef.current) clearInterval(countdownRef.current);
    setSeconds(REFRESH_SECONDS);
    countdownRef.current = setInterval(() => {
      if (stopRef.current) return;
      setSeconds((s) => {
        if (s > 1) return s - 1;
        // 只在「还没扫码」时自动换码；已扫码就等用户确认
        if (stateRef.current === "waiting") void start(true);
        return REFRESH_SECONDS;
      });
    }, 1000);
  };

  // 启动 / 刷新二维码。refresh=true 时保留旧图，避免闪烁。
  const start = async (refresh = false) => {
    if (stopRef.current || startingRef.current) return;
    startingRef.current = true;
    clearTimers();
    if (!refresh) updateState("loading");
    setError("");
    try {
      const res = await api.qrLoginStart({ schoolName });
      if (stopRef.current) return;
      ticketRef.current = res.ticket;
      setQrImage(res.qrImage);
      updateState("waiting");
      beginCountdown();
      poll();
    } catch (e) {
      if (stopRef.current) return;
      setError(e instanceof Error ? e.message : "生成二维码失败");
      updateState("error");
    } finally {
      startingRef.current = false;
    }
  };

  // 轮询扫码状态
  const poll = async () => {
    if (stopRef.current || !ticketRef.current) return;
    const ticket = ticketRef.current;
    try {
      const res = await api.qrLoginPoll({ ticket });
      // 等待期间可能已经自动换码，旧 ticket 的响应直接丢弃
      if (stopRef.current || ticket !== ticketRef.current) return;
      switch (res.status) {
        case "success":
          clearTimers();
          updateState("success");
          setTimeout(() => onClose(true), 800);
          return;
        case "failed":
          // 服务端明确失败（如 NOTCLOUD 跳转链拿不到门户会话）：
          // 如实展示原因并停住，绝不能落到「二维码已过期」误导用户。
          clearTimers();
          setError(res.message || "扫码登录失败");
          updateState("error");
          return;
        case "scanned":
          if (countdownRef.current) clearInterval(countdownRef.current);
          countdownRef.current = null;
          updateState("scanned");
          break;
        case "expired":
          clearTimers();
          updateState("expired");
          return;
        case "waiting":
          break;
      }
    } catch {
      // 网络错误，继续轮询
    }
    if (!stopRef.current) {
      const interval = stateRef.current === "scanned" ? SCANNED_POLL_INTERVAL_MS : POLL_INTERVAL_MS;
      pollTimerRef.current = setTimeout(poll, interval);
    }
  };

  useEffect(() => {
    void start();
    return () => {
      stopRef.current = true;
      clearTimers();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <Dialog open onOpenChange={(o) => !o && !stopRef.current && onClose(false)}>
      <DialogContent className="max-w-sm">
        <DialogTitle className="flex items-center gap-2">
          <QrCode className="size-4" />
          扫码登录
        </DialogTitle>
        <DialogDescription>
          用已登录今日校园的手机 App 扫描下方二维码，无需输入密码。二维码会自动刷新。
        </DialogDescription>
        <DialogBody className="flex flex-col items-center gap-3">
          {state === "loading" && (
            <div className="flex size-52 items-center justify-center rounded-lg border border-border">
              <Loader2 className="size-8 animate-spin text-muted-foreground" />
            </div>
          )}
          {qrImage && state !== "loading" && state !== "error" && (
            <img
              src={qrImage}
              alt="扫码登录二维码"
              className={`size-52 rounded-lg border border-border transition-opacity ${
                state === "expired" ? "opacity-30" : ""
              }`}
            />
          )}
          {state === "waiting" && (
            <p className="text-muted-foreground text-xs">
              等待扫码… <span className="tabular-nums">{seconds}s</span> 后自动刷新
            </p>
          )}
          {state === "scanned" && (
            <p className="text-success-text text-xs font-medium">已扫码，请在手机上确认授权…</p>
          )}
          {state === "success" && (
            <p className="text-success-text text-xs font-medium">登录成功！</p>
          )}
          {state === "expired" && (
            <div className="flex flex-col items-center gap-2">
              <p className="text-destructive text-xs">二维码已过期</p>
              <Button variant="outline" size="sm" onClick={() => void start()}>
                重新获取
              </Button>
            </div>
          )}
          {error && <p className="text-destructive text-xs">{error}</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onClose(false)}>
            取消
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

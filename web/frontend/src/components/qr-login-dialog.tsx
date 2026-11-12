// 扫码登录：用今日校园 App 扫 QR 获取登录态（无需密码/验证码）。
// 基于官方 connect/qrcode/jsLogin 流程，所有学校通用。
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
  onClose: (loggedIn: boolean) => void;
}

type QRState = "loading" | "waiting" | "scanned" | "success" | "expired" | "error";

export function QRLoginDialog({ onClose }: Props) {
  const [state, setState] = useState<QRState>("loading");
  const [qrImage, setQrImage] = useState("");
  const [error, setError] = useState("");
  const ticketRef = useRef("");
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stopRef = useRef(false);

  // 启动扫码登录
  const start = async () => {
    setState("loading");
    setError("");
    try {
      const res = await api.qrLoginStart({});
      ticketRef.current = res.ticket;
      setQrImage(res.qrImage);
      setState("waiting");
      poll();
    } catch (e) {
      setError(e instanceof Error ? e.message : "生成二维码失败");
      setState("error");
    }
  };

  // 轮询扫码状态
  const poll = async () => {
    if (stopRef.current || !ticketRef.current) return;
    try {
      const res = await api.qrLoginPoll({ ticket: ticketRef.current });
      if (stopRef.current) return;
      switch (res.status) {
        case "success":
          setState("success");
          setTimeout(() => onClose(true), 800);
          return;
        case "scanned":
          setState("scanned");
          break;
        case "expired":
          setState("expired");
          return;
        case "waiting":
          break;
      }
    } catch {
      // 网络错误，继续轮询
    }
    timerRef.current = setTimeout(poll, 2000);
  };

  useEffect(() => {
    void start();
    return () => {
      stopRef.current = true;
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  return (
    <Dialog open onOpenChange={(o) => !o && !stopRef.current && onClose(false)}>
      <DialogContent className="max-w-sm">
        <DialogTitle className="flex items-center gap-2">
          <QrCode className="size-4" />
          扫码登录
        </DialogTitle>
        <DialogDescription>
          用已登录今日校园的手机 App 扫描下方二维码，无需输入密码。
        </DialogDescription>
        <DialogBody className="flex flex-col items-center gap-3">
          {state === "loading" && (
            <div className="flex size-52 items-center justify-center rounded-lg border border-border">
              <Loader2 className="size-8 animate-spin text-muted-foreground" />
            </div>
          )}
          {qrImage && state !== "error" && state !== "loading" && (
            <img
              src={qrImage}
              alt="扫码登录二维码"
              className={`size-52 rounded-lg border border-border transition-opacity ${
                state === "expired" ? "opacity-30" : ""
              }`}
            />
          )}
          {state === "waiting" && (
            <p className="text-muted-foreground text-xs">等待扫码…</p>
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

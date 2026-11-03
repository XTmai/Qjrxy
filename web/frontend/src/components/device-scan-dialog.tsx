// 设备扫码弹窗：生成二维码 → 手机 App 扫码 → 轮询回填。
// 服务端在轮询命中时已把设备信息写入账号，这里只做展示与刷新。
import { useEffect, useRef, useState } from "react";

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
  accountId: number;
  onClose: (collected: boolean) => void;
}

export function DeviceScanDialog({ accountId, onClose }: Props) {
  const [qr, setQr] = useState("");
  const [status, setStatus] = useState<"loading" | "waiting" | "collected" | "error">("loading");
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const stopped = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => {
    let alive = true;

    const poll = async (ticket: string) => {
      while (!stopped.current && alive) {
        try {
          const data = await api.devicePoll(ticket);
          if (stopped.current || !alive) return;
          if (!data.pending && data.collected) {
            setStatus("collected");
            setInfo(`${data.deviceModel || "设备"} · ${(data.platform || "android") === "ios" ? "iOS" : "Android"}`);
            return;
          }
        } catch (e) {
          if (stopped.current || !alive) return;
          setStatus("error");
          setError(e instanceof Error ? e.message : "二维码已失效");
          return;
        }
        await new Promise((r) => setTimeout(r, 1500));
      }
    };

    (async () => {
      try {
        const res = await api.deviceTicket(accountId);
        if (stopped.current || !alive) return;
        setQr(res.qrcode);
        setStatus("waiting");
        void poll(res.ticket);
      } catch (e) {
        setStatus("error");
        setError(e instanceof Error ? e.message : "生成二维码失败");
      }
    })();

    return () => {
      alive = false;
      stopped.current = true;
      if (timer.current) clearTimeout(timer.current);
    };
  }, [accountId]);

  return (
    <Dialog open onOpenChange={(o) => !o && onClose(status === "collected")}>
      <DialogContent className="max-w-sm">
        <DialogTitle>扫码绑定设备</DialogTitle>
        <DialogDescription>用你平时打卡的那台手机扫码，设备信息自动写入</DialogDescription>
        <DialogBody className="space-y-4 text-center">
          {qr && <img src={qr} alt="设备绑定二维码" className="mx-auto size-52 rounded-xl border p-2" />}
          {status === "loading" && <p className="text-muted-foreground text-sm">正在生成二维码…</p>}
          {status === "waiting" && (
            <p className="text-muted-foreground animate-pulse text-sm">
              打开今日校园 App → 右上角「＋」→ 扫一扫
            </p>
          )}
          {status === "collected" && (
            <p className="text-sm font-medium text-green-600 dark:text-green-400">已绑定：{info}</p>
          )}
          {status === "error" && <p className="text-destructive text-sm">{error}</p>}
          <p className="text-muted-foreground text-xs leading-relaxed">
            必须用平时打卡的手机扫码；换手机或重装 App 后设备 ID 会变化。
            二维码 15 分钟内有效，仅可使用一次。
          </p>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onClose(status === "collected")}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

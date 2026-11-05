// 设备扫码弹窗：生成二维码 → 手机 App 扫码 → 轮询回填。
// 服务端在轮询命中时已把设备信息写入账号，这里只做展示与刷新。
import { useEffect, useRef, useState } from "react";

import { api } from "@/api";
import { useInfo } from "@/hooks/use-auth";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { Globe, ShieldAlert, Wifi } from "lucide-react";

/** 当前页面是否通过本机回环地址打开（手机无法访问这种地址）。 */
function onLoopback(): boolean {
  const h = location.hostname.toLowerCase();
  return h === "localhost" || h === "127.0.0.1" || h === "[::1]" || h === "::1";
}

interface Props {
  accountId: number;
  onClose: (collected: boolean) => void;
}

export function DeviceScanDialog({ accountId, onClose }: Props) {
  const { data: sysInfo } = useInfo();
  const loopback = onLoopback();
  const lanUrls = sysInfo?.lanUrls ?? [];
  const [qr, setQr] = useState("");
  const [status, setStatus] = useState<"loading" | "waiting" | "collected" | "error">("loading");
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const stopped = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => {
    let alive = true;
    // StrictMode 下 effect 会执行两次：上一轮 cleanup 把 stopped 置了 true，
    // 不重置的话这一轮会立刻退出，二维码永远停在 loading。
    stopped.current = false;

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
          {/* 局域网引导：手机必须能访问到这台电脑，二维码才能被打开 */}
          {loopback && lanUrls.length > 0 && (
            <div className="border-warning-border bg-warning-soft rounded-lg border p-3 text-left">
              <p className="text-warning-text flex items-center gap-1.5 text-xs font-medium">
                <Wifi className="size-3.5" />
                手机与电脑需在同一 Wi-Fi（或手机开热点给电脑）
              </p>
              <p className="text-warning-text mt-1 text-[11px] opacity-80">
                当前页面是本机地址，手机扫不开。请点击下方地址改用局域网打开，再重新扫码：
              </p>
              <div className="mt-2 space-y-1.5">
                {lanUrls.map((u) => (
                  <button
                    key={u}
                    type="button"
                    onClick={() => {
                      location.host = new URL(u).host; // 同路径切换到局域网地址
                    }}
                    className="text-warning-text bg-background/60 flex w-full items-center justify-between rounded-md px-2.5 py-1.5 font-mono text-[11px] transition-colors hover:bg-background"
                  >
                    <span>{u}</span>
                    <Globe className="size-3 shrink-0" />
                  </button>
                ))}
              </div>
            </div>
          )}
          {loopback && lanUrls.length === 0 && (
            <div className="border-danger-border bg-danger-soft rounded-lg border p-3 text-left">
              <p className="text-danger-text flex items-center gap-1.5 text-xs font-medium">
                <ShieldAlert className="size-3.5" />
                服务只监听了本机（127.0.0.1），手机无法访问
              </p>
              <p className="text-danger-text mt-1 text-[11px] opacity-80">
                请改用默认方式启动（监听全部网卡），或设置 QINGJIN_ADDR=:8090 后重启，再扫码。
              </p>
            </div>
          )}
          {!loopback && (
            <p className="text-muted-foreground flex items-center justify-center gap-1.5 text-[11px]">
              <Wifi className="size-3" />
              手机与电脑保持同一网络即可；扫描后手机会打开本页地址提交设备信息
            </p>
          )}
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

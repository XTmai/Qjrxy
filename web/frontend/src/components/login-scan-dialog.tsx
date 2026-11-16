// 扫码获取登录态弹窗：生成二维码 → 手机 App 扫码换票 → 轮询回填。
// 与 device-scan-dialog 是两个独立入口：那个绑设备指纹，这个换学校登录态。
// 服务端在轮询命中时已把登录态写入账号，这里只做展示与刷新。
import { useEffect, useRef, useState } from "react";

import { api } from "@/api";
import { useInfo } from "@/hooks/use-auth";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
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
  /** 嵌在账号抽屉的子页里：只渲染内容，不套 Dialog 壳 */
  embedded?: boolean;
}

export function LoginScanDialog({ accountId, onClose, embedded = false }: Props) {
  const { data: sysInfo } = useInfo();
  const loopback = onLoopback();
  const lanUrls = sysInfo?.lanUrls ?? [];
  const [qr, setQr] = useState("");
  const [status, setStatus] = useState<"loading" | "waiting" | "collected" | "error">("loading");
  const [error, setError] = useState("");
  const [target, setTarget] = useState("");

  // StrictMode 下 effect 会跑两次：上一轮 cleanup 把 stopped 置了 true，
  // 不重置的话这一轮会立刻退出，二维码永远停在 loading。
  const stopped = useRef(false);

  useEffect(() => {
    let alive = true;
    stopped.current = false;

    const poll = async (ticket: string) => {
      while (!stopped.current && alive) {
        try {
          const data = await api.loginScanPoll(accountId, ticket);
          if (stopped.current || !alive) return;
          if (!data.pending && data.collected) {
            setStatus("collected");
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
        const res = await api.loginScanTicket(accountId);
        if (stopped.current || !alive) return;
        setQr(res.qrcode);
        const t = res.target;
        setTarget(t.alias ? `${t.schoolName} · ${t.username}（${t.alias}）` : `${t.schoolName} · ${t.username}`);
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
    };
  }, [accountId]);

  const body = (
    <div className="space-y-4 text-center">
      {/* 局域网引导：手机必须能访问到这台电脑，二维码才能被打开 */}
      {loopback && lanUrls.length > 0 && (
        <div className="rounded-xl border border-warning-border bg-warning-soft p-3 text-left">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-warning-text">
            <Wifi className="size-3.5" />
            手机与电脑需在同一 Wi-Fi（或手机开热点给电脑）
          </p>
          <p className="mt-1 text-[11px] text-warning-text opacity-90">
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
                className="flex w-full items-center justify-between rounded-lg bg-background/60 px-2.5 py-1.5 font-mono text-[11px] text-warning-text transition-colors hover:bg-background"
              >
                <span>{u}</span>
                <Globe className="size-3 shrink-0" />
              </button>
            ))}
          </div>
        </div>
      )}
      {loopback && lanUrls.length === 0 && (
        <div className="rounded-xl border border-danger-border bg-danger-soft p-3 text-left">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-danger-text">
            <ShieldAlert className="size-3.5" />
            服务只监听了本机（127.0.0.1），手机无法访问
          </p>
          <p className="mt-1 text-[11px] text-danger-text opacity-90">
            请改用默认方式启动（监听全部网卡），或设置 QINGJIN_ADDR=:8090 后重启，再扫码。
          </p>
        </div>
      )}
      {target && (
        <div className="rounded-xl border border-border bg-[color:var(--subtle)]/70 p-3 text-left">
          <p className="text-[11px] text-muted-foreground">即将绑定到这条记录</p>
          <p className="mt-0.5 font-mono text-sm font-semibold">{target}</p>
        </div>
      )}
      {qr && (
        <img
          src={qr}
          alt="扫码获取登录态"
          className="mx-auto size-52 rounded-xl border border-border bg-white p-2"
        />
      )}
      {status === "loading" && <p className="text-sm text-muted-foreground">正在生成二维码…</p>}
      {status === "waiting" && (
        <p className="animate-pulse text-sm text-muted-foreground">
          打开今日校园 App → 右上角「＋」→ 扫一扫
        </p>
      )}
      {status === "collected" && <p className="text-sm font-semibold text-success">登录态已更新</p>}
      {status === "error" && <p className="text-sm font-medium text-danger">{error}</p>}
      <p className="text-xs leading-relaxed text-muted-foreground">
        必须用已登录<strong>本人</strong>今日校园的手机扫码；手机页会显示目标学号，请核对后再确认。
        二维码 15 分钟内有效，仅可使用一次。
      </p>
      <div className="flex items-center justify-center gap-2 pt-1">
        <Button variant="ghost" onClick={() => onClose(status === "collected")}>
          关闭
        </Button>
      </div>
    </div>
  );

  if (embedded) return body;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose(status === "collected")}>
      <DialogContent className="max-w-sm">
        <DialogTitle>扫码获取登录态</DialogTitle>
        <DialogDescription>
          用那台已登录今日校园的手机扫码，自动换出学校会话，无需输入密码
        </DialogDescription>
        <DialogBody>{body}</DialogBody>
      </DialogContent>
    </Dialog>
  );
}

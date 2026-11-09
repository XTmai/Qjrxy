// 短信验证码登录弹窗：面向「学校自建统一身份认证」（如 my.nut.edu.cn/cas）。
// 手机号 + 验证码作为 CAS 的认证因子，绕开各校自定义的密码加密。
import { useEffect, useRef, useState } from "react";
import { Loader2, MessageSquare } from "lucide-react";

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
import { Input } from "@/components/ui/input";

interface Props {
  accountId: number;
  onClose: (loggedIn: boolean) => void;
}

export function SmsLoginDialog({ accountId, onClose }: Props) {
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [ticket, setTicket] = useState("");
  const [countdown, setCountdown] = useState(0);
  const [busy, setBusy] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [hint, setHint] = useState("");
  const timer = useRef<ReturnType<typeof setInterval> | undefined>(undefined);

  useEffect(() => {
    return () => {
      if (timer.current) clearInterval(timer.current);
    };
  }, []);

  const startCountdown = () => {
    setCountdown(60);
    if (timer.current) clearInterval(timer.current);
    timer.current = setInterval(() => {
      setCountdown((v) => {
        if (v <= 1) {
          if (timer.current) clearInterval(timer.current);
          return 0;
        }
        return v - 1;
      });
    }, 1000);
  };

  const send = async () => {
    setError("");
    setHint("");
    if (!phone.trim()) return setError("请填写手机号");
    setSending(true);
    try {
      const res = await api.smsSend(accountId, phone.trim());
      setTicket(res.ticket);
      startCountdown();
      setHint(`验证码已发送至 ${res.phone}，5 分钟内有效`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "发送验证码失败");
    } finally {
      setSending(false);
    }
  };

  const submit = async () => {
    setError("");
    if (!ticket) return setError("请先获取验证码");
    if (!code.trim()) return setError("请输入验证码");
    setBusy(true);
    try {
      await api.smsLogin(accountId, ticket, code.trim());
      onClose(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "登录失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose(false)}>
      <DialogContent className="max-w-sm">
        <DialogTitle className="flex items-center gap-2">
          <MessageSquare className="size-4" />
          短信验证码登录
        </DialogTitle>
        <DialogDescription>
          使用今日校园账号绑定的手机号登录，所有学校通用。密码错误的备用方案。
        </DialogDescription>
        <DialogBody className="space-y-4">
          <div className="space-y-1.5">
            <p className="text-muted-foreground text-[11px]">手机号</p>
            <div className="flex gap-2">
              <Input
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                placeholder="今日校园绑定的手机号"
                inputMode="numeric"
                autoComplete="tel"
              />
              <Button
                variant="outline"
                className="shrink-0"
                disabled={sending || countdown > 0}
                onClick={() => void send()}
              >
                {sending ? <Loader2 className="size-4 animate-spin" /> : countdown > 0 ? `${countdown}s` : "发送验证码"}
              </Button>
            </div>
          </div>
          <div className="space-y-1.5">
            <p className="text-muted-foreground text-[11px]">短信验证码</p>
            <Input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder={ticket ? "请输入收到的验证码" : "请先获取验证码"}
              inputMode="numeric"
              autoComplete="one-time-code"
              disabled={!ticket}
              onKeyDown={(e) => {
                if (e.key === "Enter") void submit();
              }}
            />
          </div>
          {hint && <p className="text-muted-foreground text-xs">{hint}</p>}
          {error && <p className="text-destructive text-xs">{error}</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={() => onClose(false)}>
            取消
          </Button>
          <Button disabled={busy || !ticket} onClick={() => void submit()}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "登录"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

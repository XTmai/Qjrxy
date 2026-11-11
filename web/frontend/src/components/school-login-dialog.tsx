// 学校系统登录弹窗：账号密码 / 短信验证码二选一。
// 两种方式只是认证因子不同，目标一致（让该账号的学校会话有效），因此合并在同一入口。
// 图形验证码挑战就地展开，不再另开弹窗或把用户支走去别的页面。
import { useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";

import { api } from "@/api";
import type { Account } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

type Method = "password" | "sms";

interface Props {
  account: Account;
  onClose: (loggedIn: boolean) => void;
}

export function SchoolLoginDialog({ account, onClose }: Props) {
  const [method, setMethod] = useState<Method>("password");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  // 账号密码
  const [password, setPassword] = useState("");
  const [captchaTicket, setCaptchaTicket] = useState("");
  const [captchaImage, setCaptchaImage] = useState("");
  const [captchaCode, setCaptchaCode] = useState("");

  // 短信验证码
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [ticket, setTicket] = useState("");
  const [countdown, setCountdown] = useState(0);
  const [sending, setSending] = useState(false);
  const [hint, setHint] = useState("");
  const timer = useRef<ReturnType<typeof setInterval> | undefined>(undefined);

  const inCaptcha = captchaTicket !== "";

  useEffect(() => {
    return () => {
      if (timer.current) clearInterval(timer.current);
    };
  }, []);

  const fail = (e: unknown, fallback: string) => setError(e instanceof Error ? e.message : fallback);

  const switchMethod = (m: Method) => {
    if (busy) return;
    setMethod(m);
    setError("");
  };

  /* ---------------- 账号密码 ---------------- */

  const startPasswordLogin = async () => {
    setError("");
    setBusy(true);
    try {
      // 填了新密码就先落库；留空则沿用服务端保存的密码。
      if (password) await api.updateAccount(account.id, { password });
      const ch = await api.accountLogin(account.id);
      if (ch.done) {
        onClose(true);
        return;
      }
      if (ch.needCaptcha) {
        setCaptchaTicket(ch.ticket ?? "");
        setCaptchaImage(ch.captcha ?? "");
        setCaptchaCode("");
        return;
      }
      setError(ch.message || "登录未完成，请重试");
    } catch (e) {
      fail(e, "登录失败");
    } finally {
      setBusy(false);
    }
  };

  const refreshCaptcha = async () => {
    setBusy(true);
    setError("");
    try {
      const ch = await api.accountLogin(account.id);
      if (ch.done) {
        onClose(true);
        return;
      }
      setCaptchaTicket(ch.ticket ?? "");
      setCaptchaImage(ch.captcha ?? "");
      setCaptchaCode("");
    } catch (e) {
      fail(e, "获取验证码失败");
    } finally {
      setBusy(false);
    }
  };

  const submitCaptcha = async () => {
    if (!captchaCode.trim()) return setError("请输入验证码");
    setBusy(true);
    setError("");
    try {
      await api.submitCaptcha(account.id, captchaTicket, captchaCode.trim());
      onClose(true);
    } catch (e) {
      fail(e, "验证失败");
      setCaptchaCode("");
    } finally {
      setBusy(false);
    }
  };

  /* ---------------- 短信验证码 ---------------- */

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

  const sendSMS = async () => {
    setError("");
    setHint("");
    if (!phone.trim()) return setError("请填写手机号");
    setSending(true);
    try {
      const res = await api.smsSend(account.id, phone.trim());
      setTicket(res.ticket);
      startCountdown();
      setHint(`验证码已发送至 ${res.phone}，5 分钟内有效`);
    } catch (e) {
      fail(e, "发送验证码失败");
    } finally {
      setSending(false);
    }
  };

  const submitSMS = async () => {
    setError("");
    if (!ticket) return setError("请先获取验证码");
    if (!code.trim()) return setError("请输入验证码");
    setBusy(true);
    try {
      await api.smsLogin(account.id, ticket, code.trim());
      onClose(true);
    } catch (e) {
      fail(e, "登录失败");
    } finally {
      setBusy(false);
    }
  };

  /* ---------------- 渲染 ---------------- */

  const primary = () => {
    if (method === "sms") return { label: "登录", disabled: busy || !ticket || !code.trim(), run: submitSMS };
    if (inCaptcha) return { label: "确认验证码", disabled: busy || !captchaCode.trim(), run: submitCaptcha };
    return { label: "登录", disabled: busy, run: startPasswordLogin };
  };
  const action = primary();

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose(false)}>
      <DialogContent className="max-w-sm">
        <DialogTitle>登录学校系统</DialogTitle>
        <DialogDescription className="truncate">
          {account.schoolName} · {account.username}
        </DialogDescription>
        <DialogBody className="space-y-4">
          {/* 二选一：两种认证因子并列，避免看起来像两个独立功能 */}
          <div className="bg-muted grid grid-cols-2 gap-1 rounded-lg p-1">
            {(
              [
                ["password", "账号密码"],
                ["sms", "短信验证码"],
              ] as const
            ).map(([m, label]) => (
              <button
                key={m}
                type="button"
                onClick={() => switchMethod(m)}
                className={`rounded-md py-1.5 text-sm transition-colors ${
                  method === m ? "bg-background text-foreground font-medium shadow-sm" : "text-muted-foreground"
                }`}
              >
                {label}
              </button>
            ))}
          </div>

          {method === "password" ? (
            inCaptcha ? (
              <div className="space-y-3">
                {captchaImage && (
                  <img
                    src={captchaImage}
                    alt="验证码"
                    title="点击刷新"
                    className="mx-auto max-h-20 cursor-pointer rounded-lg border"
                    onClick={() => void refreshCaptcha()}
                  />
                )}
                <Field>
                  <FieldLabel>图形验证码</FieldLabel>
                  <Input
                    value={captchaCode}
                    onChange={(e) => setCaptchaCode(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void submitCaptcha();
                    }}
                    placeholder="不区分大小写"
                    autoFocus
                  />
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">看不清？点击图片刷新。</span>
                    <button
                      type="button"
                      className="text-muted-foreground hover:text-foreground underline underline-offset-2"
                      onClick={() => {
                        setCaptchaTicket("");
                        setCaptchaImage("");
                        setCaptchaCode("");
                        setError("");
                      }}
                    >
                      重新填写密码
                    </button>
                  </div>
                </Field>
              </div>
            ) : (
              <>
                <Field>
                  <FieldLabel>密码</FieldLabel>
                  <Input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="留空则使用已保存的密码"
                    autoComplete="current-password"
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void startPasswordLogin();
                    }}
                  />
                  <FieldDescription>填写后会用新密码覆盖已保存的密码。</FieldDescription>
                </Field>
                <p className="text-muted-foreground text-xs">
                  密码错误的学校可改用「短信验证码」，支持所有学校。
                </p>
              </>
            )
          ) : (
            <div className="space-y-4">
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
                    onClick={() => void sendSMS()}
                  >
                    {sending ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : countdown > 0 ? (
                      `${countdown}s`
                    ) : (
                      "发送验证码"
                    )}
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
                    if (e.key === "Enter") void submitSMS();
                  }}
                />
              </div>
              <p className="text-muted-foreground text-xs">
                账密登录失败（自建统一身份认证）时的备用方式，登录后可能让手机 App 下线。
              </p>
              {hint && <p className="text-muted-foreground text-xs">{hint}</p>}
            </div>
          )}

          {error && <p className="text-destructive text-xs">{error}</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={() => onClose(false)}>
            取消
          </Button>
          <Button disabled={action.disabled} onClick={() => void action.run()}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : action.label}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// 图形验证码弹窗：学校要求验证码时在登录流程内直接输入。
import { useRef, useState } from "react";

import { Loader2 } from "lucide-react";
import { api } from "@/api";
import type { LoginChallenge } from "@/lib/types";
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

interface Props {
  accountId: number;
  accountName: string;
  challenge: LoginChallenge;
  onResolved: (ok: boolean) => void;
}

export function CaptchaDialog({ accountId, accountName, challenge, onResolved }: Props) {
  const [code, setCode] = useState("");
  const [captcha, setCaptcha] = useState(challenge.captcha ?? "");
  const [ticket, setTicket] = useState(challenge.ticket ?? "");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const restart = async () => {
    // 看不清就重新发起登录，拿新的验证码
    setBusy(true);
    setError("");
    try {
      const fresh = await api.accountLogin(accountId);
      if (fresh.done) {
        onResolved(true);
        return;
      }
      setCaptcha(fresh.captcha ?? "");
      setTicket(fresh.ticket ?? "");
    } catch (e) {
      setError(e instanceof Error ? e.message : "获取验证码失败");
    } finally {
      setBusy(false);
    }
  };

  const submit = async () => {
    if (!code.trim()) {
      setError("请输入验证码");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.submitCaptcha(accountId, ticket, code.trim());
      onResolved(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "验证失败");
      setCode("");
    } finally {
      setBusy(false);
      inputRef.current?.focus();
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onResolved(false)}>
      <DialogContent className="max-w-sm">
        <DialogTitle>输入验证码</DialogTitle>
        <DialogDescription>「{accountName}」登录需要图形验证码</DialogDescription>
        <DialogBody className="space-y-4">
          {captcha && (
            <img
              src={captcha}
              alt="验证码"
              title="点击刷新"
              className="mx-auto max-h-20 cursor-pointer rounded-lg border"
              onClick={() => void restart()}
            />
          )}
          <Field>
            <FieldLabel>图中字符</FieldLabel>
            <Input
              ref={inputRef}
              value={code}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setCode(e.target.value)}
              onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => e.key === "Enter" && void submit()}
              placeholder="不区分大小写"
              autoFocus
            />
            <FieldDescription>看不清？点击图片刷新。</FieldDescription>
          </Field>
          {error && <p className="text-destructive text-sm">{error}</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onResolved(false)}>
            取消
          </Button>
          <Button variant="default" disabled={busy} onClick={() => void submit()}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "确认"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

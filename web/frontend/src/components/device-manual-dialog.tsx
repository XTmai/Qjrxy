// 手动填写设备信息：扫码拿不到时的兜底。
// 字段与扫码采集完全一致（deviceId 是签到用的 CpUUID），后端 PATCH 直接接收。
import { useState } from "react";
import { ClipboardPaste, Loader2 } from "lucide-react";

import { api } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useToast } from "@/hooks/use-toast";
import type { Account } from "@/lib/types";

interface Props {
  account: Account;
  onClose: (changed: boolean) => void;
  /** 嵌在账号抽屉的子页里：只渲染表单，不套 Dialog 壳 */
  embedded?: boolean;
}

export function DeviceManualDialog({ account, onClose, embedded = false }: Props) {
  const { toast } = useToast();
  const [deviceId, setDeviceId] = useState(account.deviceId || "");
  const [deviceModel, setDeviceModel] = useState(account.deviceModel || "");
  const [systemVersion, setSystemVersion] = useState(account.systemVersion || "");
  const [platform, setPlatform] = useState(account.platform || "android");
  const [userAgent, setUserAgent] = useState(account.userAgent || "");
  const [appVersion, setAppVersion] = useState(account.appVersion || "");
  const [buildId, setBuildId] = useState(account.buildId || "");
  const [wisDeviceId, setWisDeviceId] = useState(account.wisDeviceId || "");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setError("");
    const id = deviceId.trim();
    if (!id) return setError("请填写设备 ID");
    if (id.length < 8 || id.length > 128) return setError("设备 ID 长度需在 8-128 之间");
    setBusy(true);
    try {
      await api.updateAccount(account.id, {
        deviceId: id,
        deviceModel: deviceModel.trim(),
        systemVersion: systemVersion.trim(),
        platform,
        userAgent: userAgent.trim(),
        appVersion: appVersion.trim(),
        buildId: buildId.trim(),
        wisDeviceId: wisDeviceId.trim(),
      });
      toast({ title: "设备信息已保存" });
      onClose(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setBusy(false);
    }
  };

  const body = (
    <div className="space-y-3.5">
      <Field>
        <FieldLabel>设备 ID（必填）</FieldLabel>
        <Input
          value={deviceId}
          onChange={(e) => setDeviceId(e.target.value)}
          placeholder="如 86792b16-9a0e-4dd6-ba7f-22a9c4a558ea"
          className="font-mono text-xs"
        />
        <FieldDescription>签到提交时 CpdailyInfo 里携带的设备标识。</FieldDescription>
      </Field>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel>机型</FieldLabel>
          <Input
            value={deviceModel}
            onChange={(e) => setDeviceModel(e.target.value)}
            placeholder="如 23127PN0CC / iPhone17,1"
          />
        </Field>
        <Field>
          <FieldLabel>系统版本</FieldLabel>
          <Input
            value={systemVersion}
            onChange={(e) => setSystemVersion(e.target.value)}
            placeholder="如 14 / 18.2"
          />
        </Field>
        <Field>
          <FieldLabel>平台</FieldLabel>
          <select
            value={platform}
            onChange={(e) => setPlatform(e.target.value)}
            className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            <option value="android">Android</option>
            <option value="ios">iOS</option>
          </select>
        </Field>
        <Field>
          <FieldLabel>构建号</FieldLabel>
          <Input
            value={buildId}
            onChange={(e) => setBuildId(e.target.value)}
            placeholder="如 AP2A.240705.005"
          />
        </Field>
        <Field>
          <FieldLabel>App 版本</FieldLabel>
          <Input
            value={appVersion}
            onChange={(e) => setAppVersion(e.target.value)}
            placeholder="如 9.9.24"
          />
        </Field>
        <Field>
          <FieldLabel>wisDeviceId（iOS 可选）</FieldLabel>
          <Input
            value={wisDeviceId}
            onChange={(e) => setWisDeviceId(e.target.value)}
            placeholder="iOS 原生客户端的设备头"
            className="font-mono text-xs"
          />
        </Field>
      </div>
      <Field>
        <FieldLabel>User-Agent（可选）</FieldLabel>
        <textarea
          value={userAgent}
          onChange={(e) => setUserAgent(e.target.value)}
          placeholder="留空则按机型/系统版本自动拼装"
          className="h-20 w-full resize-none rounded-lg border border-input bg-background px-3 py-2 font-mono text-[11px] outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
        />
        <FieldDescription>原样透传到请求头，需与该设备真实 App 的 UA 一致。</FieldDescription>
      </Field>
      {error && <p className="text-xs font-medium text-danger">{error}</p>}
      <div className="flex items-center justify-end gap-2 pt-1">
        <Button variant="ghost" disabled={busy} onClick={() => onClose(false)}>
          取消
        </Button>
        <Button disabled={busy} onClick={() => void save()}>
          {busy ? <Loader2 className="size-4 animate-spin" /> : "保存"}
        </Button>
      </div>
    </div>
  );

  if (embedded) return body;

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose(false)}>
      <DialogContent className="max-w-lg">
        <DialogTitle className="flex items-center gap-2">
          <ClipboardPaste className="size-4" />
          手动填写设备信息
        </DialogTitle>
        <DialogDescription>
          扫码拿不到时的兜底。设备 ID 是提交签到时使用的 CpUUID（不是手机系统 UUID），
          可从原手机的扫码页复制；留空的字段会沿用当前值。
        </DialogDescription>
        <DialogBody>{body}</DialogBody>
      </DialogContent>
    </Dialog>
  );
}

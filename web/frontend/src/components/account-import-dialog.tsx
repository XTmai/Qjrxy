// 账号导入：选文件或粘贴文本，先预览再导入。
// 支持后端导出的 JSON（{accounts:[...]} 或数组）与 CSV；CSV 里的 cookies
// 列是 JSON 字符串，解析失败时按无登录态处理（不让整行失败）。
import { useMemo, useRef, useState } from "react";
import { Loader2, Upload } from "lucide-react";

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
import { useToast } from "@/hooks/use-toast";
import { parseCSV } from "@/lib/csv";
import type { ExportAccount, ImportResult } from "@/lib/types";

interface Props {
  onClose: (imported: boolean) => void;
}

type Row = {
  schoolName: string;
  username: string;
  alias: string;
  hasSession: boolean;
  hasPassword: boolean;
};

function toExportAccount(raw: Record<string, unknown>): ExportAccount {
  const str = (k: string) => String(raw[k] ?? "").trim();
  const num = (k: string) => {
    const n = Number(raw[k]);
    return Number.isFinite(n) ? n : 0;
  };
  let cookies: Record<string, string> | undefined;
  const rawCookies = raw.cookies;
  if (rawCookies && typeof rawCookies === "object") {
    cookies = rawCookies as Record<string, string>;
  } else if (typeof rawCookies === "string" && rawCookies.trim()) {
    try {
      const parsed = JSON.parse(rawCookies);
      if (parsed && typeof parsed === "object") cookies = parsed as Record<string, string>;
    } catch {
      // 文件被手工改坏时忽略登录态，账号本身仍可导入
    }
  }
  return {
    alias: str("alias"),
    schoolName: str("schoolName"),
    username: str("username"),
    deviceId: str("deviceId"),
    deviceModel: str("deviceModel"),
    systemVersion: str("systemVersion"),
    platform: str("platform"),
    wisDeviceId: str("wisDeviceId"),
    userAgent: str("userAgent"),
    appVersion: str("appVersion"),
    buildId: str("buildId"),
    address: str("address"),
    lon: num("lon"),
    lat: num("lat"),
    abnormalReason: str("abnormalReason"),
    loginStatus: str("loginStatus"),
    lastLoginAt: str("lastLoginAt"),
    cookies,
    tgc: str("tgc") || undefined,
    password: str("password") || undefined,
  };
}

function parseImport(text: string): ExportAccount[] {
  const trimmed = text.trim();
  if (!trimmed) return [];
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    const data: unknown = JSON.parse(trimmed);
    const obj = data && typeof data === "object" ? (data as { accounts?: unknown }) : {};
    const list: unknown[] = Array.isArray(data) ? data : Array.isArray(obj.accounts) ? obj.accounts : [];
    return list
      .filter((x): x is Record<string, unknown> => Boolean(x) && typeof x === "object")
      .map((x) => toExportAccount(x));
  }
  return parseCSV(trimmed).map((row) => toExportAccount(row));
}

export function AccountImportDialog({ onClose }: Props) {
  const { toast } = useToast();
  const fileRef = useRef<HTMLInputElement>(null);
  const [accounts, setAccounts] = useState<ExportAccount[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [fileName, setFileName] = useState("");

  const preview = useMemo<Row[]>(
    () =>
      accounts.map((a) => ({
        schoolName: a.schoolName,
        username: a.username,
        alias: a.alias,
        hasSession: Boolean(a.cookies && Object.keys(a.cookies).length > 0),
        hasPassword: Boolean(a.password),
      })),
    [accounts],
  );
  const sessionCount = preview.filter((r) => r.hasSession).length;
  const passwordCount = preview.filter((r) => r.hasPassword).length;

  const loadText = (text: string, name: string) => {
    setError("");
    setResult(null);
    try {
      const list = parseImport(text);
      if (list.length === 0) {
        setError("没有解析到账号，请检查文件格式");
        setAccounts([]);
        return;
      }
      setAccounts(list);
      setFileName(name);
    } catch (e) {
      setAccounts([]);
      setError(e instanceof Error ? `解析失败：${e.message}` : "解析失败");
    }
  };

  const pickFile = async (file: File) => {
    const text = await file.text();
    loadText(text, file.name);
  };

  const submit = async () => {
    setError("");
    setBusy(true);
    try {
      const res = await api.importAccounts(accounts);
      setResult(res as ImportResult);
      if (res.failed === 0) {
        toast({ title: `导入完成：新增 ${res.created}，更新 ${res.updated}` });
      } else {
        toast({
          title: `导入完成，${res.failed} 条失败`,
          description: "失败原因见列表，修正后可重新导入",
          variant: "destructive",
        });
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "导入失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose(Boolean(result))}>
      <DialogContent className="max-w-lg">
        <DialogTitle className="flex items-center gap-2">
          <Upload className="size-4" />
          导入账号
        </DialogTitle>
        <DialogDescription>
          支持导出的 CSV / JSON 文件。导入会按「学校 + 学号」合并：已存在则更新，不重复创建。
        </DialogDescription>
        <DialogBody className="space-y-4">
          <div className="flex items-center gap-2">
            <Input
              value={fileName}
              readOnly
              placeholder="选择导出的 .csv / .json 文件"
              className="flex-1"
            />
            <input
              ref={fileRef}
              type="file"
              accept=".csv,.json,text/csv,application/json"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) void pickFile(f);
                e.target.value = "";
              }}
            />
            <Button variant="outline" className="shrink-0" disabled={busy} onClick={() => fileRef.current?.click()}>
              选择文件
            </Button>
          </div>

          <div className="space-y-1.5">
            <p className="text-muted-foreground text-[11px]">或粘贴文件内容</p>
            <textarea
              className="border-input bg-background focus-visible:border-ring focus-visible:ring-ring/50 h-24 w-full resize-none rounded-lg border px-3 py-2 font-mono text-[11px] outline-none focus-visible:ring-3"
              placeholder="把 CSV 或 JSON 内容粘贴到这里"
              onChange={(e) => loadText(e.target.value, "粘贴的内容")}
            />
          </div>

          {accounts.length > 0 && (
            <div className="space-y-2">
              <p className="text-xs">
                解析到 <span className="font-semibold tabular-nums">{accounts.length}</span> 个账号
                {sessionCount > 0 && <span className="text-success-text"> · {sessionCount} 条含登录态</span>}
                {passwordCount > 0 && <span className="text-warning-text"> · {passwordCount} 条含密码</span>}
              </p>
              <div className="border-border max-h-48 overflow-y-auto rounded-lg border">
                <table className="w-full text-xs">
                  <thead className="bg-muted/50 text-muted-foreground">
                    <tr>
                      <th className="px-2 py-1.5 text-left font-medium">学号</th>
                      <th className="px-2 py-1.5 text-left font-medium">学校</th>
                      <th className="px-2 py-1.5 text-left font-medium">登录态</th>
                    </tr>
                  </thead>
                  <tbody className="divide-border divide-y">
                    {preview.slice(0, 50).map((r, i) => (
                      <tr key={`${r.username}-${i}`}>
                        <td className="px-2 py-1.5 font-mono">{r.alias || r.username}</td>
                        <td className="text-muted-foreground max-w-40 truncate px-2 py-1.5">{r.schoolName}</td>
                        <td className="px-2 py-1.5">
                          {r.hasSession ? (
                            <span className="text-success-text">含</span>
                          ) : (
                            <span className="text-muted-foreground">无</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {preview.length > 50 && (
                <p className="text-muted-foreground text-[11px]">仅预览前 50 条</p>
              )}
            </div>
          )}

          {result && (
            <div className="space-y-1.5 rounded-lg border border-border p-2.5 text-xs">
              <p>
                新增 <span className="font-semibold tabular-nums">{result.created}</span> · 更新{" "}
                <span className="font-semibold tabular-nums">{result.updated}</span> · 失败{" "}
                <span className="font-semibold tabular-nums">{result.failed}</span>
              </p>
              {result.items
                .filter((it) => it.status === "failed")
                .slice(0, 10)
                .map((it, i) => (
                  <p key={`${it.username}-${i}`} className="text-danger-text">
                    {it.username}：{it.error}
                  </p>
                ))}
            </div>
          )}

          {error && <p className="text-destructive text-xs">{error}</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={() => onClose(Boolean(result))}>
            {result ? "关闭" : "取消"}
          </Button>
          {!result && (
            <Button disabled={busy || accounts.length === 0} onClick={() => void submit()}>
              {busy ? <Loader2 className="size-4 animate-spin" /> : `确认导入 ${accounts.length || ""}`}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// 账号页：搜索/过滤 + 大卡片列表（指纹/照片/位置/计划四块）+ 单卡片创建（保存即登录）+ 批量登录态检测 + 详情弹窗。
import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  CalendarClock,
  Fingerprint,
  ImageIcon,
  Loader2,
  MapPin,
  Plus,
  QrCode,
  Search,
  Settings2,
} from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
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
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/api";
import { useAccounts, useCreateAccount, useProbeAccounts } from "@/hooks/use-accounts";
import { usePlans } from "@/hooks/use-plans";
import { toast } from "@/hooks/use-toast";
import { AccountDetail } from "@/components/account-detail";
import { CaptchaDialog } from "@/components/captcha-dialog";
import { DeviceScanDialog } from "@/components/device-scan-dialog";
import { LoginBadge } from "@/components/status-badge";
import { EmptyBanner } from "@/components/empty-banner";
import { PageHeader } from "@/components/page-header";
import { maskUser } from "@/lib/utils";
import type { Account, LoginChallenge, School } from "@/lib/types";

type StatusFilter = "all" | "needs-login" | "needs-device";

export function AccountsPage() {
  const { data: accounts } = useAccounts();
  const { data: plans } = usePlans();
  const [search, setSearch] = useSearchParams();
  const [showCreate, setShowCreate] = useState(false);
  const [detailId, setDetailId] = useState<number | null>(null);
  const [keyword, setKeyword] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [scanAccountId, setScanAccountId] = useState<number | null>(null);
  const probe = useProbeAccounts();

  // 支持 /accounts?account=N 直达详情、?create=1 直达创建
  useEffect(() => {
    const id = Number(search.get("account"));
    if (id && accounts?.some((a) => a.id === id)) {
      setDetailId(id);
      search.delete("account");
      setSearch(search, { replace: true });
    }
  }, [accounts, search, setSearch]);

  useEffect(() => {
    if (search.get("create") === "1") {
      setShowCreate(true);
      search.delete("create");
      setSearch(search, { replace: true });
    }
  }, [search, setSearch]);

  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    return (accounts ?? []).filter((a) => {
      if (statusFilter === "needs-login" && a.loginStatus === "authorized") return false;
      if (statusFilter === "needs-device" && a.deviceId) return false;
      if (!kw) return true;
      return [a.alias, a.username, a.schoolName].some((v) =>
        String(v ?? "").toLowerCase().includes(kw),
      );
    });
  }, [accounts, keyword, statusFilter]);

  const probeAll = () => {
    probe.mutate(undefined, {
      onSuccess: ({ results }) => {
        const relogged = results.filter((r) => r.status === "relogged").length;
        const expired = results.filter((r) => r.status === "expired").length;
        if (expired) {
          toast({ title: `${relogged} 个已自动恢复，${expired} 个需要重新登录` });
        } else if (relogged) {
          toast({ title: `全部正常，${relogged} 个会话已自动恢复` });
        } else {
          toast({ title: "全部正常，已同步最新任务" });
        }
      },
      onError: (e) => toast({ title: "检测失败", description: e.message, variant: "destructive" }),
    });
  };

  const detail = accounts?.find((a) => a.id === detailId) ?? null;

  return (
    <div className="space-y-5">
      <PageHeader
        title="账号"
        sub={accounts ? `已绑定 ${accounts.length} 个账号` : undefined}
        actions={
          <>
            {accounts && accounts.length > 0 && (
              <Button variant="outline" size="sm" disabled={probe.isPending} onClick={probeAll}>
                {probe.isPending ? <Loader2 className="size-4 animate-spin" /> : "检测登录态"}
              </Button>
            )}
            <Button variant="default" size="sm" onClick={() => setShowCreate(true)}>
              <Plus className="size-3.5" />
              添加账号
            </Button>
          </>
        }
      />

      {/* 搜索 + 过滤 chips */}
      {accounts && accounts.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative flex-1 min-w-48">
            <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2" />
            <Input
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              placeholder="搜索别名、学号或学校"
              className="pl-9"
            />
          </div>
          {(
            [
              ["all", "全部"],
              ["needs-login", "需登录"],
              ["needs-device", "待扫码"],
            ] as const
          ).map(([key, label]) => (
            <Button
              key={key}
              variant={statusFilter === key ? "secondary" : "ghost"}
              size="sm"
              className={statusFilter === key ? "" : "text-muted-foreground"}
              onClick={() => setStatusFilter(key)}
            >
              {label}
            </Button>
          ))}
        </div>
      )}

      {!accounts && (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-24 rounded-lg" />
          ))}
        </div>
      )}

      {accounts && accounts.length === 0 && (
        <EmptyBanner
          tone="destructive"
          title="还没有绑定账号"
          description="添加今日校园账号后即可自动查寝。学校 + 学号 + 密码，保存后自动登录。"
          action={
            <Button variant="destructive" size="sm" onClick={() => setShowCreate(true)}>
              去添加
            </Button>
          }
        />
      )}

      {accounts && accounts.length > 0 && filtered.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center text-sm">
            没有匹配的账号
          </CardContent>
        </Card>
      )}

      {/* 大卡片两列网格 */}
      <div className="grid gap-4 lg:grid-cols-2">
        {filtered.map((a) => (
          <AccountCard
            key={a.id}
            account={a}
            planCount={(plans ?? []).filter((p) => p.accountId === a.id && p.enabled).length}
            onOpen={() => setDetailId(a.id)}
            onScan={() => setScanAccountId(a.id)}
            onLogin={async () => {
              try {
                const ch = await api.accountLogin(a.id);
                if (ch.done) {
                  toast({ title: "登录成功" });
                  probe.mutate(undefined, { onSuccess: () => {}, onError: () => {} });
                } else if (ch.needCaptcha) {
                  toast({
                    title: "学校要求验证码",
                    description: "请点「管理」进入账号详情完成登录",
                    variant: "destructive",
                  });
                }
              } catch (e) {
                toast({
                  title: "登录失败",
                  description: e instanceof Error ? e.message : undefined,
                  variant: "destructive",
                });
              }
            }}
          />
        ))}
      </div>

      {showCreate && (
        <CreateAccountDialog onClose={(created) => { setShowCreate(false); if (created) void 0; }} />
      )}

      {detail && (
        <AccountDetail account={detail} onClose={(deleted) => { setDetailId(null); if (deleted) void 0; }} />
      )}

      {scanAccountId !== null && (
        <DeviceScanDialog
          accountId={scanAccountId}
          onClose={(collected) => {
            const id = scanAccountId;
            setScanAccountId(null);
            if (collected && id) {
              toast({ title: "设备已绑定" });
              probe.mutate(undefined, { onSuccess: () => {}, onError: () => {} });
            }
          }}
        />
      )}
    </div>
  );
}

/* ---------------- 账号大卡片（指纹 / 照片 / 位置 / 计划 四块） ---------------- */

function AccountCard({
  account,
  planCount,
  onOpen,
  onScan,
  onLogin,
}: {
  account: Account;
  planCount: number;
  onOpen: () => void;
  onScan: () => void;
  onLogin: () => void;
}) {
  const needsLogin = account.loginStatus !== "authorized";
  const needsDevice = !account.deviceId;
  const ok = !needsLogin && !needsDevice;

  return (
    <div
      className={`bg-card rounded-lg border p-4 transition-colors ${
        ok ? "border-border hover:border-primary/40" : "border-warning-border"
      }`}
    >
      {/* 头部：头像 + 身份 + 状态 */}
      <div className="flex items-start gap-3">
        <span className="relative shrink-0">
          <Avatar size="lg">
            <AvatarFallback className="bg-primary-soft text-primary text-sm font-bold">
              {(account.alias || account.username).slice(0, 1)}
            </AvatarFallback>
          </Avatar>
          {/* 登录状态脉冲点 */}
          <span
            className={`absolute -top-0.5 -right-0.5 size-2.5 rounded-full ring-2 ring-card ${
              account.loginStatus === "authorized"
                ? "bg-success pulse-dot"
                : account.loginStatus === "expired"
                  ? "bg-danger pulse-dot"
                  : "bg-muted-foreground"
            }`}
          />
        </span>
        <button type="button" onClick={onOpen} className="min-w-0 flex-1 text-left">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate text-sm font-semibold">{account.alias || account.username}</p>
            <LoginBadge status={account.loginStatus} />
          </div>
          <p className="text-muted-foreground mt-0.5 truncate font-mono text-[11px]">
            {maskUser(account.username)} · {account.schoolName}
          </p>
        </button>
      </div>

      {/* 四块网格：缺失项琥珀高亮 */}
      <div className="mt-3.5 grid grid-cols-2 gap-2">
        <InfoBlock
          icon={Fingerprint}
          label="设备指纹"
          ok={!!account.deviceId}
          value={
            account.deviceId
              ? `${account.deviceModel || "已绑定"} · ${account.platform === "ios" ? "iOS" : "Android"}`
              : "待扫码绑定"
          }
          action={needsDevice ? { text: "扫码授权", onClick: onScan } : undefined}
        />
        <InfoBlock
          icon={ImageIcon}
          label="查寝照片"
          ok={!!account.photoUrl}
          value={account.photoUrl ? "已上传" : "未上传"}
          action={!account.photoUrl ? { text: "去上传", onClick: onOpen } : undefined}
          thumb={account.photoUrl ? `${account.photoUrl}?t=${account.updatedAt}` : undefined}
        />
        <InfoBlock
          icon={MapPin}
          label="签到位置"
          ok={!!account.address}
          value={account.address ? account.address : "未设置"}
          action={!account.address ? { text: "自动获取", onClick: onOpen } : undefined}
        />
        <InfoBlock
          icon={CalendarClock}
          label="自动计划"
          ok={planCount > 0}
          value={planCount > 0 ? `${planCount} 个启用中` : "未创建"}
          action={planCount === 0 ? { text: "去创建", onClick: onOpen } : undefined}
        />
      </div>

      {/* 操作区：登录/扫码 优先，管理常驻 */}
      <div className="border-border mt-3.5 flex gap-2 border-t pt-3">
        {needsLogin && (
          <Button variant="default" size="sm" className="h-8 flex-1 gap-1 text-xs" onClick={onLogin}>
            {needsDevice ? "登录账号" : "重新登录"}
          </Button>
        )}
        {needsDevice && (
          <Button variant={needsLogin ? "outline" : "default"} size="sm" className="h-8 flex-1 gap-1 text-xs" onClick={onScan}>
            <QrCode className="size-3.5" />
            扫码绑定设备
          </Button>
        )}
        {ok && (
          <Button variant="outline" size="sm" className="h-8 flex-1 gap-1 text-xs" onClick={onOpen}>
            <Settings2 className="size-3.5" />
            管理账号
          </Button>
        )}
        <Button variant="ghost" size="sm" className="h-8 text-xs" onClick={onOpen}>
          详情
        </Button>
      </div>
    </div>
  );
}

/** 四块信息格：ok 灰调、缺失琥珀调，可带缩略图与快捷动作 */
function InfoBlock({
  icon: Icon,
  label,
  value,
  ok,
  action,
  thumb,
}: {
  icon: typeof Fingerprint;
  label: string;
  value: string;
  ok: boolean;
  action?: { text: string; onClick: () => void };
  thumb?: string;
}) {
  return (
    <div
      className={`rounded-md border p-2.5 ${
        ok ? "border-border bg-muted/30" : "border-warning-border bg-warning-soft/60"
      }`}
    >
      <div className="flex items-center gap-1.5">
        <Icon className={`size-3 ${ok ? "text-muted-foreground" : "text-warning"}`} />
        <span className={`tracking-label text-[10px] font-medium uppercase ${ok ? "text-muted-foreground" : "text-warning-text"}`}>
          {label}
        </span>
        {thumb && (
          <img
            src={thumb}
            alt={label}
            className="ml-auto size-5 rounded object-cover ring-1 ring-border"
            loading="lazy"
          />
        )}
      </div>
      <p className={`mt-1.5 truncate text-xs font-medium ${ok ? "" : "text-warning-text"}`} title={value}>
        {value}
      </p>
      {action && (
        <button
          type="button"
          onClick={action.onClick}
          className="text-warning mt-1 text-[10px] font-medium hover:underline"
        >
          {action.text} →
        </button>
      )}
    </div>
  );
}

/* ---------------- 单卡片创建 ---------------- */

function CreateAccountDialog({ onClose }: { onClose: (created: boolean) => void }) {
  const create = useCreateAccount();
  const [school, setSchool] = useState("");
  const [schools, setSchools] = useState<School[]>([]);
  const [showSchools, setShowSchools] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [step, setStep] = useState<"form" | "logging">("form");
  const [logLabel, setLogLabel] = useState("");
  const [createdId, setCreatedId] = useState<number | null>(null);
  const [challenge, setChallenge] = useState<LoginChallenge | null>(null);
  const captchaResolve = useRef<((ok: boolean) => void) | null>(null);
  const searchTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  // 学校自动匹配（防抖 320ms）
  useEffect(() => {
    if (searchTimer.current) clearTimeout(searchTimer.current);
    const kw = school.trim();
    if (kw.length < 2) {
      setSchools([]);
      setShowSchools(false);
      return;
    }
    searchTimer.current = setTimeout(async () => {
      try {
        const d = await api.schools(kw);
        setSchools(d.schools.slice(0, 20));
        setShowSchools(d.schools.length > 0);
      } catch {
        /* 查询失败可手动填写 */
      }
    }, 320);
    return () => {
      if (searchTimer.current) clearTimeout(searchTimer.current);
    };
  }, [school]);

  const save = async () => {
    setError("");
    if (!school.trim()) return setError("请填写学校全称");
    if (!username.trim()) return setError("请填写学号");
    if (!password) return setError("请填写密码");
    setBusy(true);
    try {
      setStep("logging");
      setLogLabel("正在创建账号…");
      const { account } = await create.mutateAsync({
        schoolName: school.trim(),
        username: username.trim(),
        password,
      });

      // 保存即登录；学校要验证码时挂起流程，等 CaptchaDialog 回传
      setLogLabel("正在登录…");
      const loginOk = await new Promise<boolean>((resolve) => {
        const ch = create.data?.challenge;
        if (ch?.done) {
          resolve(true);
          return;
        }
        captchaResolve.current = resolve;
        setCreatedId(account.id);
        setChallenge(ch ?? null);
      });
      if (loginOk) toast({ title: "登录成功" });

      // 自动按学校签到点设置位置（需要登录态；失败不阻塞）
      try {
        const res = await api.applySignRange(account.id);
        toast({ title: "位置已设置", description: res.range.address || undefined });
      } catch {
        toast({ title: "位置未设置", description: "可在账号详情里点「自动获取位置」重试" });
      }

      toast({ title: "账号已添加" });
      onClose(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "创建失败");
      setStep("form");
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Dialog open onOpenChange={(o) => !o && !busy && onClose(false)}>
        <DialogContent className="max-w-sm">
          <DialogTitle>添加账号</DialogTitle>
          <DialogDescription>保存后自动登录并获取学校签到点</DialogDescription>
          <DialogBody className="space-y-4">
            {step === "form" ? (
              <>
                <Field>
                  <FieldLabel>学校全称</FieldLabel>
                  <Input
                    value={school}
                    onChange={(e) => setSchool(e.target.value)}
                    onFocus={() => schools.length && setShowSchools(true)}
                    placeholder="输入几个字自动匹配"
                    autoComplete="off"
                  />
                  {showSchools && (
                    <div className="bg-popover rounded-lg border border-border shadow-sm">
                      {schools.map((s) => (
                        <button
                          key={s.id}
                          type="button"
                          onClick={() => {
                            setSchool(s.name);
                            setShowSchools(false);
                          }}
                          className="hover:bg-muted block w-full px-3 py-2 text-left text-sm"
                        >
                          {s.name}
                        </button>
                      ))}
                    </div>
                  )}
                  <FieldDescription>输入几个字会自动匹配，从下拉中选择即可。</FieldDescription>
                </Field>
                <Field>
                  <FieldLabel>学号</FieldLabel>
                  <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
                </Field>
                <Field>
                  <FieldLabel>密码</FieldLabel>
                  <Input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="new-password"
                  />
                  <FieldDescription>AES-256-GCM 加密保存在你的服务器，仅用于自动登录。</FieldDescription>
                </Field>
                {error && <p className="text-destructive text-sm">{error}</p>}
              </>
            ) : (
              <div className="text-muted-foreground flex items-center gap-3 py-6 text-sm">
                <Loader2 className="size-4 animate-spin" />
                {logLabel}
              </div>
            )}
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" disabled={busy} onClick={() => onClose(false)}>
              取消
            </Button>
            {step === "form" && (
              <Button variant="default" onClick={() => void save()}>
                保存并登录
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {challenge && createdId !== null && (
        <CaptchaDialog
          accountId={createdId}
          accountName={username || "新账号"}
          challenge={challenge}
          onResolved={(ok) => {
            captchaResolve.current?.(ok);
            captchaResolve.current = null;
            setChallenge(null);
          }}
        />
      )}
    </>
  );
}

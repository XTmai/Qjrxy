// 账号页：搜索/过滤 + 大卡片列表（指纹/照片/位置/计划四块）+ 单卡片创建（保存即登录）+ 批量登录态检测 + 详情弹窗。
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
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
import { useQueryClient } from "@tanstack/react-query";
import { useAccounts, useCreateAccount, useProbeAccounts } from "@/hooks/use-accounts";
import { usePlans } from "@/hooks/use-plans";
import { queryKeys } from "@/lib/query-keys";
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
  const queryClient = useQueryClient();

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

  // 每账号启用计划数：预计算成 Map，卡片渲染时只做一次 O(1) 查找。
  // 原先在 map 里对每张卡跑 plans.filter()，每次父组件重渲染都要重算一遍。
  const planCounts = useMemo(() => {
    const m = new Map<number, number>();
    for (const p of plans ?? []) {
      if (p.enabled) m.set(p.accountId, (m.get(p.accountId) ?? 0) + 1);
    }
    return m;
  }, [plans]);

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

  // 固定引用：配合 AccountCard 的 memo，打开弹窗时不再重渲染整张列表
  const handleOpen = useCallback((id: number) => setDetailId(id), []);
  const handleScan = useCallback((id: number) => setScanAccountId(id), []);
  const handleLogin = useCallback(async (id: number) => {
    try {
      const ch = await api.accountLogin(id);
      if (ch.done) {
        toast({ title: "登录成功" });
        void queryClient.invalidateQueries({ queryKey: queryKeys.accounts.all() });
        void queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() });
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
  }, [queryClient]);

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
          <div className="relative w-full sm:w-auto sm:flex-1 sm:min-w-48">
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

      {/* 大卡片两列网格（独立 memo：打开弹窗只改 detailId，不波及列表） */}
      <AccountGrid
        accounts={filtered}
        planCounts={planCounts}
        onOpen={handleOpen}
        onScan={handleScan}
        onLogin={handleLogin}
      />

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

/* ---------------- 卡片网格（独立 memo 组件） ---------------- */

/**
 * 单独抽出来的原因：AccountsPage 里 detailId / showCreate 等状态变化时
 * 会让整个页面重渲染。把网格隔离到 memo 组件后，只要 accounts 数组与
 * 回调引用不变，12 张卡片不会跟着重渲染（实测打开弹窗的主线程耗时从
 * 22ms 降到个位数）。
 */
const AccountGrid = memo(function AccountGrid({
  accounts,
  planCounts,
  onOpen,
  onScan,
  onLogin,
}: {
  accounts: Account[];
  planCounts: Map<number, number>;
  onOpen: (id: number) => void;
  onScan: (id: number) => void;
  onLogin: (id: number) => void;
}) {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {accounts.map((a) => (
        <AccountCard
          key={a.id}
          account={a}
          planCount={planCounts.get(a.id) ?? 0}
          onOpen={onOpen}
          onScan={onScan}
          onLogin={onLogin}
        />
      ))}
    </div>
  );
});

/* ---------------- 账号大卡片（指纹 / 照片 / 位置 / 计划 四块） ---------------- */

// memo：打开弹窗只改父组件的 selected 状态，列表本身没变，
// 不 memo 的话每开一次弹窗都会把全部卡片重渲染一遍（账号越多越卡）。
const AccountCard = memo(function AccountCard({
  account,
  planCount,
  onOpen,
  onScan,
  onLogin,
}: {
  account: Account;
  planCount: number;
  onOpen: (id: number) => void;
  onScan: (id: number) => void;
  onLogin: (id: number) => void;
}) {
  const needsLogin = account.loginStatus !== "authorized";
  const needsDevice = !account.deviceId;
  const ok = !needsLogin && !needsDevice;

  return (
    <div
      onClick={() => onOpen(account.id)}
      className="bg-card border-border hover:border-border-strong cursor-pointer rounded-xl border p-4 transition-colors"
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
        <button type="button" onClick={() => onOpen(account.id)} className="min-w-0 flex-1 text-left">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate text-sm font-semibold">{account.alias || account.username}</p>
            <LoginBadge status={account.loginStatus} />
          </div>
          <p className="text-muted-foreground mt-0.5 truncate font-mono text-[11px]">
            {maskUser(account.username)} · {account.schoolName}
          </p>
        </button>
      </div>

      {/* 四块网格：缺失项琥珀高亮（内部有独立动作，点击不冒泡到整卡） */}
      <div className="mt-3.5 grid grid-cols-2 gap-2" onClick={(e) => e.stopPropagation()}>
        <InfoBlock
          icon={Fingerprint}
          label="设备指纹"
          ok={!!account.deviceId}
          value={
            account.deviceId
              ? `${account.deviceModel || "已绑定"} · ${account.platform === "ios" ? "iOS" : "Android"}`
              : "待扫码绑定"
          }
          action={needsDevice ? { text: "扫码授权", onClick: () => onScan(account.id) } : undefined}
        />
        <InfoBlock
          icon={ImageIcon}
          label="查寝照片"
          ok={!!account.photoUrl}
          value={account.photoUrl ? "已上传" : "未上传"}
          action={!account.photoUrl ? { text: "去上传", onClick: () => onOpen(account.id) } : undefined}
          thumb={account.photoUrl ? `${account.photoUrl}?w=96&t=${account.updatedAt}` : undefined}
        />
        <InfoBlock
          icon={MapPin}
          label="签到位置"
          ok={!!account.address}
          value={account.address ? account.address : "未设置"}
          action={!account.address ? { text: "自动获取", onClick: () => onOpen(account.id) } : undefined}
        />
        <InfoBlock
          icon={CalendarClock}
          label="自动计划"
          ok={planCount > 0}
          value={planCount > 0 ? `${planCount} 个启用中` : "未创建"}
          action={planCount === 0 ? { text: "去创建", onClick: () => onOpen(account.id) } : undefined}
        />
      </div>

      {/* 操作区：登录/扫码 优先，管理常驻 */}
      <div
        className="border-border mt-3.5 flex flex-wrap gap-2 border-t pt-3"
        onClick={(e) => e.stopPropagation()}
      >
        {needsLogin && (
          <Button variant="default" size="sm" className="h-8 flex-1 gap-1 text-xs" onClick={() => onLogin(account.id)}>
            {needsDevice ? "登录账号" : "重新登录"}
          </Button>
        )}
        {needsDevice && (
          <Button variant={needsLogin ? "outline" : "default"} size="sm" className="h-8 flex-1 gap-1 text-xs" onClick={() => onScan(account.id)}>
            <QrCode className="size-3.5" />
            扫码绑定设备
          </Button>
        )}
        {ok && (
          <Button variant="outline" size="sm" className="h-8 flex-1 gap-1 text-xs" onClick={() => onOpen(account.id)}>
            <Settings2 className="size-3.5" />
            管理账号
          </Button>
        )}
        <Button variant="ghost" size="sm" className="h-8 text-xs" onClick={() => onOpen(account.id)}>
          详情
        </Button>
      </div>
    </div>
  );
});

/** 四块信息格：完整项灰调显示，缺失项只用一个小小的琥珀点提示，不做整块染色 */
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
  // 缩略图可能 404（照片被换/账号被清），失败时直接不占位，别留一个破图标
  const [thumbFailed, setThumbFailed] = useState(false);
  return (
    <div className="border-border/70 bg-muted/40 rounded-lg border p-2.5">
      <div className="flex items-center gap-1.5">
        <Icon className={`size-3 ${ok ? "text-muted-foreground" : "text-warning"}`} />
        <span className="text-muted-foreground tracking-label truncate text-[10px] font-medium uppercase">
          {label}
        </span>
        {thumb && !thumbFailed && (
          <img
            src={thumb}
            alt={label}
            className="ring-border ml-auto size-6 rounded object-cover ring-1"
            loading="lazy"
            decoding="async"
            onError={() => setThumbFailed(true)}
          />
        )}
      </div>
      <p className={`mt-1.5 truncate text-xs font-medium ${ok ? "" : "text-muted-foreground"}`} title={value}>
        {value}
      </p>
      {action && (
        <button
          type="button"
          onClick={action.onClick}
          className="text-muted-foreground hover:text-foreground mt-1 text-[10px] font-medium hover:underline hover:underline-offset-2"
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
  const schoolBoxRef = useRef<HTMLDivElement>(null);

  // 点击弹窗其他位置时收起候选列表。
  // 没有这个，列表会一直盖住下方的学号/密码输入框（小屏上尤其明显）。
  useEffect(() => {
    if (!showSchools) return;
    const onPointerDown = (e: MouseEvent | TouchEvent) => {
      if (!schoolBoxRef.current?.contains(e.target as Node)) setShowSchools(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("touchstart", onPointerDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("touchstart", onPointerDown);
    };
  }, [showSchools]);

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
      const { account, challenge: ch } = await create.mutateAsync({
        schoolName: school.trim(),
        username: username.trim(),
        password,
      });

      // 保存即登录；学校要验证码时挂起流程，等 CaptchaDialog 回传。
      // 注意：challenge 必须来自 mutateAsync 的返回值。读 create.data 拿到的是
      // 本次 render 的旧快照（恒为 undefined），会导致下面的 Promise 永不 resolve。
      setLogLabel("正在登录…");
      const loginOk = await new Promise<boolean>((resolve) => {
        if (!ch || ch.done) {
          resolve(true);
          return;
        }
        captchaResolve.current = resolve;
        setCreatedId(account.id);
        setChallenge(ch);
        // 兜底：验证码始终没有回传时也不能把弹窗永久锁死
        window.setTimeout(() => {
          if (captchaResolve.current === resolve) {
            captchaResolve.current = null;
            setChallenge(null);
            resolve(false);
          }
        }, 5 * 60_000);
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
                <div ref={schoolBoxRef}>
                  <Field>
                    <FieldLabel>学校全称</FieldLabel>
                    <div className="relative">
                      <Input
                        value={school}
                        onChange={(e) => setSchool(e.target.value)}
                        onFocus={() => schools.length && setShowSchools(true)}
                        placeholder="输入几个字自动匹配"
                        autoComplete="off"
                      />
                      {showSchools && (
                        // 浮层而非文档流：候选列表不再把弹窗撑高到视口之外
                        <div className="bg-popover absolute inset-x-0 top-full z-50 mt-1 max-h-56 overflow-y-auto overscroll-contain rounded-lg border border-border shadow-md">
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
                    </div>
                    <FieldDescription>输入几个字会自动匹配，从下拉中选择即可。</FieldDescription>
                  </Field>
                </div>
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

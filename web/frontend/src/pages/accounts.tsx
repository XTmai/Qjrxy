// 账号页：搜索/过滤 + 双列卡片（指纹/照片/位置/计划四块）+ 单卡片创建（保存即登录）+ 批量登录态检测 + 右侧详情抽屉。
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  CalendarClock,
  Download,
  Fingerprint,
  ImageIcon,
  Loader2,
  MapPin,
  Plus,
  QrCode,
  Search,
  Settings2,
  ShieldCheck,
  Upload,
  Users,
} from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
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
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/api";
import { QQ_GROUP_URL } from "@/lib/links";
import { AccountImportDialog } from "@/components/account-import-dialog";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { downloadBlob, todayStamp } from "@/lib/download";
import { useQueryClient } from "@tanstack/react-query";
import { useAccounts, useCreateAccount, useProbeAccounts } from "@/hooks/use-accounts";
import { usePlans } from "@/hooks/use-plans";
import { queryKeys } from "@/lib/query-keys";
import { toast } from "@/hooks/use-toast";
import { AccountDetail, type DetailTab } from "@/components/account-detail";
import { CaptchaDialog } from "@/components/captcha-dialog";
import { DeviceScanDialog } from "@/components/device-scan-dialog";
import { LoginScanDialog } from "@/components/login-scan-dialog";
import { LoginBadge } from "@/components/status-badge";
import { cn, maskUser } from "@/lib/utils";
import type { Account, LoginChallenge, School } from "@/lib/types";

type StatusFilter = "all" | "needs-login" | "needs-device";

export function AccountsPage() {
  const { data: accounts } = useAccounts();
  const { data: plans } = usePlans();
  const [search, setSearch] = useSearchParams();
  const [showCreate, setShowCreate] = useState(false);
  const [detailId, setDetailId] = useState<number | null>(null);
  const [detailTab, setDetailTab] = useState<DetailTab>("tasks");
  const [showImport, setShowImport] = useState(false);
  const { confirm, ConfirmDialog } = useConfirm();
  const [keyword, setKeyword] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [scanAccountId, setScanAccountId] = useState<number | null>(null);
  const [loginScanAccountId, setLoginScanAccountId] = useState<number | null>(null);
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

  const doExport = async (
    format: "json" | "csv",
    withSession: boolean,
    withPassword: boolean,
  ) => {
    if (withSession || withPassword) {
      const ok = await confirm({
        title: "导出包含敏感凭证",
        description: withPassword
          ? "文件包含登录态（cookies / 续期票据）和明文密码。任何人拿到文件都能直接登录这些账号，请只在受控设备上保存。"
          : "文件包含登录态（cookies / 续期票据）。拿到文件的人可以在其有效期内使用这些账号。",
        confirmText: "继续导出",
        variant: "destructive",
      });
      if (!ok) return;
    }
    try {
      const blob = await api.exportAccounts({ format, withSession, withPassword });
      const suffix = `${withSession ? "-with-session" : ""}${withPassword ? "-with-password" : ""}`;
      downloadBlob(blob, `qjrxy-accounts-${todayStamp()}${suffix}.${format}`);
    } catch (e) {
      toast({
        title: "导出失败",
        description: e instanceof Error ? e.message : undefined,
        variant: "destructive",
      });
    }
  };

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
  const handleOpen = useCallback((id: number, tab: DetailTab = "tasks") => {
    setDetailTab(tab);
    setDetailId(id);
  }, []);
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
    <div className="view-panel space-y-5">
      {/* 页面标题 + 次级操作 */}
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
        <div>
          <h1 className="tracking-headline text-2xl font-bold">账号</h1>
          <p className="mt-1 text-xs text-muted-foreground">
            已绑定 <span className="font-semibold text-foreground">{accounts?.length ?? 0}</span> 个账号 ·
            点击任意卡片打开管理抽屉
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {accounts && accounts.length > 0 && (
            <Button variant="outline" size="sm" className="h-8 gap-1.5 text-xs" disabled={probe.isPending} onClick={probeAll}>
              {probe.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <ShieldCheck className="size-3.5 text-success" />
              )}
              检测登录态
            </Button>
          )}
          <Button variant="outline" size="sm" className="h-8 gap-1.5 text-xs" onClick={() => setShowImport(true)}>
            <Upload className="size-3.5" />
            导入
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm" className="h-8 gap-1.5 text-xs" disabled={!accounts || accounts.length === 0}>
                <Download className="size-3.5" />
                导出
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56 rounded-xl">
              <DropdownMenuLabel className="text-[10px] text-muted-foreground uppercase">
                不含密码 · 含登录态
              </DropdownMenuLabel>
              <DropdownMenuItem className="cursor-pointer text-xs" onClick={() => void doExport("csv", true, false)}>
                CSV
              </DropdownMenuItem>
              <DropdownMenuItem className="cursor-pointer text-xs" onClick={() => void doExport("json", true, false)}>
                JSON
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuLabel className="text-[10px] text-muted-foreground uppercase">
                含密码（明文，谨慎）
              </DropdownMenuLabel>
              <DropdownMenuItem className="cursor-pointer text-xs" onClick={() => void doExport("csv", true, true)}>
                CSV + 密码
              </DropdownMenuItem>
              <DropdownMenuItem className="cursor-pointer text-xs" onClick={() => void doExport("json", true, true)}>
                JSON + 密码
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <Button size="sm" className="h-8 gap-1.5 text-xs" onClick={() => setShowCreate(true)}>
            <Plus className="size-3.5" />
            添加账号
          </Button>
        </div>
      </div>

      {/* 搜索 + 状态筛选胶囊 */}
      {accounts && accounts.length > 0 && (
        <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              placeholder="搜索别名、学号或学校..."
              className="h-9 rounded-xl bg-[color:var(--subtle)] pl-10 text-xs focus:bg-card"
            />
          </div>
          <div className="inline-flex self-start rounded-xl border border-border bg-[color:var(--subtle)] p-1">
            {(
              [
                ["all", "全部"],
                ["needs-login", "需登录"],
                ["needs-device", "待扫码"],
              ] as const
            ).map(([key, label]) => (
              <button
                key={key}
                type="button"
                onClick={() => setStatusFilter(key)}
                className={cn(
                  "cursor-pointer rounded-lg px-3 py-1 text-xs font-medium transition-colors",
                  statusFilter === key
                    ? "bg-card font-semibold text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {label}
              </button>
            ))}
          </div>
        </div>
      )}

      {!accounts && (
        <div className="grid gap-4 lg:grid-cols-2">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-52 rounded-2xl" />
          ))}
        </div>
      )}

      {accounts && accounts.length === 0 && (
        <div className="rounded-2xl border border-border bg-card p-8 text-center">
          <div className="mx-auto flex size-12 items-center justify-center rounded-2xl bg-[color:var(--subtle)]">
            <Users className="size-5 text-muted-foreground" />
          </div>
          <h2 className="mt-3 text-sm font-bold">还没有绑定账号</h2>
          <p className="mx-auto mt-1 max-w-sm text-xs text-muted-foreground">
            添加今日校园账号后即可自动查寝。学校 + 学号 + 密码，保存后自动登录。
          </p>
          <Button size="sm" className="mt-4 h-8 gap-1.5 text-xs" onClick={() => setShowCreate(true)}>
            <Plus className="size-3.5" />
            去添加
          </Button>
        </div>
      )}

      {accounts && accounts.length > 0 && filtered.length === 0 && (
        <div className="rounded-2xl border border-border bg-card py-12 text-center text-sm text-muted-foreground">
          没有匹配的账号
        </div>
      )}

      {/* 双列 Bento 卡片网格（独立 memo：打开抽屉只改 detailId，不波及列表） */}
      <AccountGrid
        accounts={filtered}
        planCounts={planCounts}
        onOpen={handleOpen}
        onScan={handleScan}
        onScanLogin={setLoginScanAccountId}
        onLogin={handleLogin}
      />

      {showCreate && (
        <CreateAccountDialog onClose={(created) => { setShowCreate(false); if (created) void 0; }} />
      )}

      {detail && (
        <AccountDetail
          account={detail}
          initialTab={detailTab}
          onClose={(deleted) => {
            setDetailId(null);
            if (deleted) void 0;
          }}
        />
      )}

      {showImport && (
        <AccountImportDialog
          onClose={(imported) => {
            setShowImport(false);
            if (imported) {
              void queryClient.invalidateQueries({ queryKey: queryKeys.accounts.all() });
              void queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() });
            }
          }}
        />
      )}
      {ConfirmDialog}

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

      {loginScanAccountId !== null && (
        <LoginScanDialog
          accountId={loginScanAccountId}
          onClose={(collected) => {
            setLoginScanAccountId(null);
            if (collected) {
              toast({ title: "登录态已更新" });
              void queryClient.invalidateQueries({ queryKey: queryKeys.accounts.all() });
              void queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() });
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
  onScanLogin,
  onLogin,
}: {
  accounts: Account[];
  planCounts: Map<number, number>;
  onOpen: (id: number, tab?: DetailTab) => void;
  onScan: (id: number) => void;
  onScanLogin: (id: number) => void;
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
          onScanLogin={onScanLogin}
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
  onScanLogin,
  onLogin,
}: {
  account: Account;
  planCount: number;
  onOpen: (id: number, tab?: DetailTab) => void;
  onScan: (id: number) => void;
  onScanLogin: (id: number) => void;
  onLogin: (id: number) => void;
}) {
  const needsLogin = account.loginStatus !== "authorized";
  const needsDevice = !account.deviceId;
  const ok = !needsLogin && !needsDevice;

  return (
    <div
      onClick={() => onOpen(account.id)}
      className="cursor-pointer space-y-3.5 rounded-2xl border border-border bg-card p-4 shadow-xs transition-colors hover:border-border-strong sm:p-5"
    >
      {/* 头部：头像 + 身份 + 状态 */}
      <div className="flex items-start gap-3">
        <span className="relative shrink-0">
          <Avatar size="lg" className="size-10 rounded-full">
            <AvatarFallback className="rounded-full border border-border bg-[color:var(--subtle)] text-[11px] font-bold text-foreground">
              {(account.alias || account.username).replace(/^2024级\s*/, "").slice(0, 1)}
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
            <p className="truncate font-mono text-sm font-bold">{account.alias || account.username}</p>
            <LoginBadge status={account.loginStatus} />
          </div>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {maskUser(account.username)} · {account.schoolName}
          </p>
        </button>
      </div>

      {/* 四块网格：缺失项只用少量语义色提示（内部有独立动作，点击不冒泡到整卡） */}
      <div className="grid grid-cols-2 gap-2.5" onClick={(e) => e.stopPropagation()}>
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
          value={
            (account.photoPoolCount ?? 0) > 0
              ? `照片池 ${account.photoPoolCount} 张`
              : account.photoUrl
                ? "已上传"
                : "未上传"
          }
          action={!account.photoUrl ? { text: "去上传", onClick: () => onOpen(account.id, "photos") } : undefined}
          thumb={account.photoUrl ? `${account.photoUrl}?w=96&t=${account.updatedAt}` : undefined}
        />
        <InfoBlock
          icon={MapPin}
          label="签到位置"
          ok={!!account.address}
          value={account.address ? account.address : "未设置"}
          action={!account.address ? { text: "自动获取", onClick: () => onOpen(account.id, "device") } : undefined}
        />
        <InfoBlock
          icon={CalendarClock}
          label="自动计划"
          ok={planCount > 0}
          value={planCount > 0 ? `${planCount} 个启用中` : "未创建"}
          action={planCount === 0 ? { text: "去创建", onClick: () => onOpen(account.id, "plans") } : undefined}
        />
      </div>

      {/* 操作区：登录/扫码 优先，管理常驻 */}
      <div className="flex flex-wrap gap-2" onClick={(e) => e.stopPropagation()}>
        {needsLogin && (
          <Button size="sm" className="h-8 flex-1 gap-1 text-xs" onClick={() => onLogin(account.id)}>
            {needsDevice ? "登录账号" : "重新登录"}
          </Button>
        )}
        {needsLogin && (
          <Button variant="outline" size="sm" className="h-8 flex-1 gap-1 text-xs" onClick={() => onScanLogin(account.id)}>
            <QrCode className="size-3.5" />
            扫码登录
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
        <Button variant="ghost" size="sm" className="h-8 px-3 text-xs" onClick={() => onOpen(account.id)}>
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
    <div className="rounded-xl border border-border bg-[color:var(--subtle)]/60 p-3">
      <div className="flex items-center gap-1.5">
        <Icon className={cn("size-3 shrink-0", ok ? "text-muted-foreground" : "text-warning")} />
        <span className="truncate text-[11px] font-medium text-muted-foreground">
          {label}
        </span>
        {thumb && !thumbFailed && (
          <img
            src={thumb}
            alt={label}
            className="ml-auto size-6 rounded border border-border object-cover"
            loading="lazy"
            decoding="async"
            onError={() => setThumbFailed(true)}
          />
        )}
      </div>
      {/* 设备指纹这类长值在手机上允许折两行，别截成「24031PN0DC · A…」 */}
      <p
        className={cn("mt-1 line-clamp-2 text-xs font-semibold break-words", !ok && "text-muted-foreground")}
        title={value}
      >
        {value}
      </p>
      {action && (
        <button
          type="button"
          onClick={action.onClick}
          className="mt-1 cursor-pointer text-[10px] font-medium text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
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
  // 选中自建认证的学校时记录校名，用于展示「密码留空 + 扫码登录」的说明
  const [schoolUnsupported, setSchoolUnsupported] = useState("");
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

  // 已从下拉里拿到、但当前未适配的学校名集合。
  // 手输的校名若不在候选里就放行——后端 CreateAccount 会再校验一次。
  const unsupportedNames = new Set(
    schools.filter((s) => !s.supported).map((s) => s.name),
  );
  const isUnsupportedSchool = (name: string) => unsupportedNames.has(name);

  const save = async () => {
    if (busy) return;
    setError("");
    if (!school.trim()) return setError("请填写学校全称");
    // NOTCLOUD 学校走扫码登录，不需要密码：只建一条学校+学号的记录，
    // 登录态在卡片上点「扫码登录」获取。
    const unsupported = isUnsupportedSchool(school.trim());
    if (unsupported && password) {
      setSchoolUnsupported(school.trim());
      return setError(
        `「${school.trim()}」使用学校自建统一身份认证，不支持账密登录。请把密码留空后保存，再用「扫码登录」获取登录态`,
      );
    }
    if (!username.trim()) return setError("请填写学号");
    if (!unsupported && !password) return setError("请填写密码");
    setBusy(true);
    try {
      setStep("logging");
      setLogLabel("正在创建账号…");
      const created = await create.mutateAsync({
        schoolName: school.trim(),
        username: username.trim(),
        password,
      });
      const accountId = created.account.id;
      const existed = !!created.existed;
      setCreatedId(accountId);

      // NOTCLOUD 学校没有密码，账密登录必然失败——直接收尾，让用户去扫码。
      if (unsupported) {
        toast({
          title: existed ? "账号已存在" : "账号已添加",
          description: "请点卡片上的「扫码登录」获取登录态",
        });
        onClose(true);
        return;
      }

      // 账号已经入库，后面登录失败只能提示「登录未完成」，不能再报成「创建失败」：
      // 那样用户以为没保存，重试又撞上「已绑定」，列表也没刷新，看起来就像白添加了一次。
      setLogLabel("正在登录…");
      let loginErr = "";
      try {
        const ch = await api.accountLogin(accountId);
        // 学校要验证码时挂起流程，等 CaptchaDialog 回传。
        // 注意：challenge 必须来自本次请求的返回值，不能读 create.data（旧快照，恒为 undefined），
        // 否则下面的 Promise 永不 resolve。
        const loginOk = await new Promise<boolean>((resolve) => {
          if (!ch || ch.done) {
            resolve(true);
            return;
          }
          captchaResolve.current = resolve;
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
        if (!loginOk) loginErr = "验证码未完成";
      } catch (e) {
        loginErr = e instanceof Error ? e.message : "登录失败";
      }

      const okTitle = existed ? "账号已存在，信息已更新" : "账号已添加";
      if (loginErr) {
        // 账号已经入库，不能报「创建失败」让用户以为没保存。
        // 留在表单里说明清楚：直接改密码/验证码后重试即可，
        // 重试会复用同一个账号（后端不再报「已绑定」），列表也已经刷新。
        setError(`${okTitle}，但自动登录未完成：${loginErr}。修改后可再次点「保存并登录」重试。`);
        setStep("form");
        return;
      }

      // 自动按学校签到点设置位置（需要登录态；失败不阻塞）
      try {
        const res = await api.applySignRange(accountId);
        toast({ title: "位置已设置", description: res.range.address || undefined });
      } catch {
        toast({ title: "位置未设置", description: "可在账号详情里点「自动获取位置」重试" });
      }

      toast({ title: existed ? "账号已存在，已重新登录" : "登录成功" });
      onClose(true);
    } catch (e) {
      // 只有创建本身失败（学校解析不到、网络断了）才留在表单让用户改。
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
                                // 未适配的学校：直接给出明确说明 + 反馈群入口，
                                // 不让用户填完学号密码才在登录环节失败。
                                if (!s.supported) {
                                  setSchoolUnsupported(s.name);
                                } else {
                                  setSchoolUnsupported("");
                                }
                              }}
                              className={`block w-full px-3 py-2 text-left text-sm ${
                                s.supported
                                  ? "hover:bg-muted"
                                  : "text-muted-foreground hover:bg-muted/60"
                              }`}
                            >
                              {s.name}
                              {!s.supported && (
                                <span className="ml-2 text-[10px] text-brand">需扫码登录</span>
                              )}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                    <FieldDescription>输入几个字会自动匹配，从下拉中选择即可。</FieldDescription>
                    {schoolUnsupported && (
                      <div className="bg-brand-soft text-brand mt-2 space-y-1.5 rounded-lg px-3 py-2 text-[11px] leading-relaxed">
                        <p>
                          「{schoolUnsupported}」使用学校自建统一身份认证，不支持账密或短信登录。
                        </p>
                        <p>
                          请把<strong>密码留空</strong>保存账号，然后在账号卡片上点「
                          <strong>扫码登录</strong>
                          」——用已登录今日校园的手机扫一次码即可获取登录态，不需要密码。
                          该校未适配请加
                          <a
                            href={QQ_GROUP_URL}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="mx-1 font-medium underline"
                          >
                            反馈群
                          </a>
                          登记。
                        </p>
                      </div>
                    )}
                  </Field>
                </div>
                <Field>
                  <FieldLabel>学号</FieldLabel>
                  <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
                </Field>
                <Field>
                  <FieldLabel>{schoolUnsupported ? "密码（留空即可）" : "密码"}</FieldLabel>
                  <Input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="new-password"
                    placeholder={schoolUnsupported ? "该校不支持账密登录，留空" : undefined}
                  />
                  <FieldDescription>
                    {schoolUnsupported
                      ? "该校走扫码登录，密码无需填写。"
                      : "AES-256-GCM 加密保存在你的服务器，仅用于自动登录。"}
                  </FieldDescription>
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
              <Button variant="default" disabled={busy} onClick={() => void save()}>
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

// 设置：URL 驱动的 tab（沿用 openilink settings 模式）。
// 外观 / 安全（改密）/ 额度（QINGJIN_QUOTA_ENABLED=true 时）/ 关于。
import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useInfo, useUser } from "@/hooks/use-auth";
import { useTheme } from "@/lib/theme";
import { QQ_GROUP_URL } from "@/lib/links";
import { api } from "@/api";
import { useToast } from "@/hooks/use-toast";
import { PageHeader } from "@/components/page-header";
import { setToken } from "@/api/client";
import type { AdminConfig } from "@/lib/types";

const TABS = ["appearance", "security", "quota", "admin", "about"] as const;
type Tab = (typeof TABS)[number];

export function SettingsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const { toast } = useToast();
  const { data: user } = useUser();
  const { theme, setTheme } = useTheme();
  const { data: info } = useInfo();

  const quotaOn = info?.quotaEnabled ?? false;
  const singleUser = info !== undefined && !info.multiUser;
  // 管理员：多用户模式下 role=admin；单用户模式本机自用即管理员。
  const isAdmin = singleUser || user?.role === "admin";

  const rawTab = location.pathname.split("/").pop() as Tab;
  // 深链接防护：非管理员访问 /settings/admin 时落回外观页
  const tab: Tab = TABS.includes(rawTab) && !(rawTab === "admin" && !isAdmin) ? rawTab : "appearance";
  const switchTab = (t: Tab) => navigate(`/settings/${t}`, { replace: true });

  const remaining = user ? Math.max(0, user.quota - user.quotaUsed) : 0;
  const usedPct = user && user.quota > 0 ? Math.min(100, Math.round((user.quotaUsed / user.quota) * 100)) : 0;

  const [oldPass, setOldPass] = useState("");
  const [newPass, setNewPass] = useState("");
  const [pwError, setPwError] = useState("");
  const [code, setCode] = useState("");
  const [redeemError, setRedeemError] = useState("");
  const [busy, setBusy] = useState(false);

  const changePassword = async () => {
    setPwError("");
    if (!oldPass) return setPwError("请输入当前密码");
    if (newPass.length < 6) return setPwError("新密码至少 6 位");
    setBusy(true);
    try {
      const res = await api.changePassword(oldPass, newPass);
      toast({ title: "密码已更新", description: res.message });
      setOldPass("");
      setNewPass("");
    } catch (e) {
      setPwError(e instanceof Error ? e.message : "修改失败");
    } finally {
      setBusy(false);
    }
  };

  const redeem = async () => {
    setRedeemError("");
    if (!code.trim()) return setRedeemError("请输入卡密");
    setBusy(true);
    try {
      const res = await api.redeem(code.trim());
      toast({ title: `兑换成功，增加 ${res.granted} 次` });
      setCode("");
    } catch (e) {
      setRedeemError(e instanceof Error ? e.message : "兑换失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-5">
      <PageHeader title="设置" sub={user ? `当前登录：${user.nickname || user.username}` : undefined} />

      <Tabs value={tab} onValueChange={(v) => switchTab(v as Tab)}>
        <TabsList className="bg-muted/50 w-full p-1">
          <TabsTrigger value="appearance" className="flex-1 px-6">
            外观
          </TabsTrigger>
          <TabsTrigger value="security" className="flex-1 px-6">
            安全
          </TabsTrigger>
          {quotaOn && (
            <TabsTrigger value="quota" className="flex-1 px-6">
              额度
            </TabsTrigger>
          )}
          {isAdmin && (
            <TabsTrigger value="admin" className="flex-1 px-6">
              管理员
            </TabsTrigger>
          )}
          <TabsTrigger value="about" className="flex-1 px-6">
            关于
          </TabsTrigger>
        </TabsList>

        {/* 外观 */}
        <TabsContent value="appearance" className="m-0 space-y-5">
          <Card className="border-border/50">
            <CardHeader>
              <CardTitle className="text-base">外观主题</CardTitle>
              <CardDescription>选择界面的配色方案</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                {(
                  [
                    { key: "light", label: "浅色", bg: "bg-white", fg: "bg-neutral-200", accent: "bg-neutral-900" },
                    { key: "dark", label: "深色", bg: "bg-neutral-900", fg: "bg-neutral-700", accent: "bg-neutral-100" },
                  ] as const
                ).map((t) => (
                  <button
                    key={t.key}
                    type="button"
                    onClick={() => setTheme(t.key)}
                    className={`flex flex-col items-center gap-3 rounded-xl border p-4 transition-all ${
                      theme === t.key
                        ? "border-foreground/25 bg-card shadow-l1"
                        : "border-border/60 hover:border-border-strong"
                    }`}
                  >
                    {/* mini 预览 */}
                    <div className={`h-16 w-full overflow-hidden rounded-lg border border-border/50 ${t.bg}`}>
                      <div className="flex h-full flex-col gap-1 p-2">
                        <div className={`h-2 w-1/3 rounded-full ${t.accent}`} />
                        <div className={`h-1.5 w-2/3 rounded-full ${t.fg}`} />
                        <div className={`h-1.5 w-1/2 rounded-full ${t.fg} opacity-60`} />
                        <div className={`mt-auto h-3 w-full rounded ${t.fg} opacity-40`} />
                      </div>
                    </div>
                    <span className={`text-sm font-medium ${theme === t.key ? "text-primary" : "text-muted-foreground"}`}>
                      {t.label}
                    </span>
                    {theme === t.key && <Badge variant="default">当前</Badge>}
                  </button>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* 安全 */}
        <TabsContent value="security" className="m-0 space-y-5">
          {singleUser ? (
            <Card className="border-border/50">
              <CardHeader>
                <CardTitle className="text-base">单用户模式</CardTitle>
                <CardDescription>
                  当前以单用户模式运行：打开即用，无需账号密码，数据只保存在本机。
                  如需多人使用并开启登录，请在服务端设置环境变量 QINGJIN_MULTI_USER=true 后重启。
                </CardDescription>
              </CardHeader>
            </Card>
          ) : (
            <Card className="border-border/50">
              <CardHeader>
                <CardTitle className="text-base">修改登录密码</CardTitle>
                <CardDescription>用于登录本系统的密码，与今日校园的密码无关</CardDescription>
              </CardHeader>
              <CardContent className="max-w-2xl space-y-4">
                <Field>
                  <FieldLabel>当前密码</FieldLabel>
                  <Input
                    type="password"
                    value={oldPass}
                    onChange={(e) => setOldPass(e.target.value)}
                    autoComplete="current-password"
                  />
                </Field>
                <Field>
                  <FieldLabel>新密码</FieldLabel>
                  <Input
                    type="password"
                    value={newPass}
                    onChange={(e) => setNewPass(e.target.value)}
                    placeholder="至少 6 位"
                    autoComplete="new-password"
                  />
                </Field>
                {pwError && <p className="text-destructive text-sm">{pwError}</p>}
                <Button variant="default" size="sm" disabled={busy} onClick={() => void changePassword()}>
                  更新密码
                </Button>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        {/* 额度（仅托管运营开启时） */}
        {quotaOn && (
          <TabsContent value="quota" className="m-0 space-y-5">
            <Card className="border-border/50">
              <CardHeader>
                <CardTitle className="text-base">额度</CardTitle>
                <CardDescription>每次执行扣 1 次，卡密由运营方发放</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center gap-3">
                  <span className="text-3xl font-semibold tabular-nums">{remaining}</span>
                  <span className="text-muted-foreground text-sm">次可用</span>
                  <Badge variant={remaining > 5 ? "success" : "warning"}>
                    累计使用 {user?.quotaUsed ?? 0}
                  </Badge>
                </div>
                {/* 进度条 */}
                {user && user.quota > 0 && (
                  <div className="bg-muted h-2 w-full max-w-md overflow-hidden rounded-full">
                    <div
                      className={`h-full rounded-full transition-all ${
                        usedPct > 90 ? "bg-destructive" : usedPct > 70 ? "bg-amber-500" : "bg-primary"
                      }`}
                      style={{ width: `${usedPct}%` }}
                    />
                  </div>
                )}
                <div className="flex max-w-md gap-2">
                  <Input
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    placeholder="卡密，如 QJ-A1B2-C3D4"
                    className="flex-1"
                  />
                  <Button variant="outline" disabled={busy} onClick={() => void redeem()}>
                    兑换
                  </Button>
                </div>
                {redeemError && <p className="text-destructive text-sm">{redeemError}</p>}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {/* 管理员（role=admin 或单用户模式；其他人不可见也进不来） */}
        {isAdmin && (
          <TabsContent value="admin" className="m-0 space-y-5">
            <AdminPanel />
          </TabsContent>
        )}

        {/* 关于 */}
        <TabsContent value="about" className="m-0 space-y-5">
          <Card className="border-border/50">
            <CardHeader>
              <CardTitle className="text-base">关于</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <p className="text-muted-foreground tracking-label text-[10px] font-medium uppercase">当前版本</p>
                  <p className="tracking-headline font-mono text-lg font-semibold tabular-nums">
                    {info ? `v${info.appVersion}` : "v—"}
                  </p>
                </div>
                <Button variant="outline" size="sm" asChild>
                  <a href={QQ_GROUP_URL} target="_blank" rel="noopener noreferrer">
                    加入反馈群
                  </a>
                </Button>
              </div>
              {info && (
                <p className="text-muted-foreground text-xs">
                  API {info.apiVersion} · 会话 {info.tokenTTLDay} 天
                  {info.clientVersion ? ` · 模拟客户端 ${info.clientVersion}` : ""}
                </p>
              )}
              {info && (
                <div className="flex flex-wrap gap-1.5">
                  {info.features.map((f) => (
                    <Badge key={f} variant="secondary">
                      {f}
                    </Badge>
                  ))}
                </div>
              )}
              <p className="text-muted-foreground text-xs leading-relaxed">
                本项目仅供个人学习与研究。请使用本人或已获授权的账号，遵守学校规定与相关法律法规。
              </p>
              <div className="flex gap-2 pt-1">
                <a href="/api/selftest" target="_blank" rel="noreferrer">
                  <Button variant="outline" size="sm">
                    服务自检
                  </Button>
                </a>
                <a href="https://github.com/openilink/openilink-hub" target="_blank" rel="noreferrer">
                  <Button variant="ghost" size="sm">
                    界面设计致谢
                  </Button>
                </a>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

// ---- 管理员面板：运行时配置（诊断开关 / 识别服务）----
// 仅 role=admin 或单用户模式可到达；配置保存后服务端立即热生效。
const GIB = 1024 ** 3;

/** 字节 → 输入框里的 GB 数值（去掉无意义的尾零）。 */
function formatGB(bytes: number): string {
  const gb = bytes / GIB;
  if (!Number.isFinite(gb) || gb <= 0) return "1";
  return gb >= 10 ? String(Math.round(gb * 10) / 10) : gb.toFixed(2).replace(/\.?0+$/, "");
}

/** 字节 → 展示用容量（B / KB / MB / GB）。 */
function formatStorage(bytes: number): string {
  if (bytes >= GIB) return `${(bytes / GIB).toFixed(2).replace(/\.?0+$/, "")} GB`;
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1).replace(/\.0$/, "")} MB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${bytes} B`;
}

function AdminPanel() {
  const { toast } = useToast();
  const [cfg, setCfg] = useState<AdminConfig | null>(null);
  const [loadErr, setLoadErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [keepDays, setKeepDays] = useState("7");
  const [apiKey, setApiKey] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [model, setModel] = useState("");
  const [tlsHosts, setTlsHosts] = useState("");
  const [poolLimitGB, setPoolLimitGB] = useState("1");

  const load = async () => {
    try {
      const res = await api.getAdminConfig();
      setCfg(res);
      setKeepDays(String(res.diagKeepDays));
      setBaseURL(res.vision.baseURL);
      setModel(res.vision.model);
      setTlsHosts(res.server.tlsInsecureHosts);
      setPoolLimitGB(formatGB(res.photoPool.limitBytes));
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : "加载失败");
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const save = async (patch: Parameters<typeof api.saveAdminConfig>[0], title: string) => {
    setBusy(true);
    try {
      const res = await api.saveAdminConfig(patch);
      // 开多用户会重新签发会话：把新 token 存起来，避免保存完就被踢到登录页
      if (res.token) setToken(res.token);
      toast({ title, description: res.message });
      setApiKey("");
      await load();
    } catch (e) {
      toast({
        title: "保存失败",
        description: e instanceof Error ? e.message : undefined,
        variant: "destructive",
      });
    } finally {
      setBusy(false);
    }
  };

  if (loadErr) {
    return (
      <Card className="border-border/50">
        <CardContent className="text-destructive py-6 text-sm">{loadErr}</CardContent>
      </Card>
    );
  }

  const poolLimit = cfg?.photoPool.limitBytes ?? 0;
  const poolUsed = cfg?.photoPool.usedBytes ?? 0;
  const poolPct = poolLimit > 0 ? Math.min(100, Math.round((poolUsed / poolLimit) * 100)) : 0;

  const savePoolLimit = () => {
    const gb = Number(poolLimitGB);
    if (!Number.isFinite(gb) || gb <= 0 || gb > 1024) {
      toast({ title: "容量需在 1MB-1TB 之间", variant: "destructive" });
      return;
    }
    void save({ photoPoolLimitBytes: Math.round(gb * GIB) }, "照片池容量已更新");
  };

  return (
    <div className="grid items-start gap-5 lg:grid-cols-2">
      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-base">诊断日志</CardTitle>
          <CardDescription>
            记录对学校服务器的每次请求与响应（Cookie 与票据脱敏），按天落盘在服务器
            data/diag/ 下，保留期满自动清理；前端与 API 均不暴露。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">记录 HTTP 诊断日志</p>
              <p className="text-muted-foreground text-xs">出现未知错误时用于回放现场；关闭后不再记录</p>
            </div>
            <Switch
              checked={cfg?.diagEnabled ?? false}
              onCheckedChange={(v) => void save({ diagEnabled: v }, v ? "诊断日志已开启" : "诊断日志已关闭")}
            />
          </div>
          <div className="flex max-w-md items-center gap-2">
            <span className="text-muted-foreground w-24 text-xs">保留天数</span>
            <Input
              value={keepDays}
              onChange={(e) => setKeepDays(e.target.value)}
              inputMode="numeric"
              className="w-24"
            />
            <Button variant="outline" size="sm" disabled={busy} onClick={() => void save({ diagKeepDays: Number(keepDays) }, "保留天数已更新")}>
              保存
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-base">验证码识别服务（DeepSeek 多模态）</CardTitle>
          <CardDescription>
            部分学校的签到要求「以图判图」验证码，开启后自动识别并换取 ticket。留空 API Key 表示不修改。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3">
            <Field>
              <FieldLabel>API Key{cfg?.vision.hasKey ? `（已配置：${cfg.vision.keyMask}）` : "（未配置）"}</FieldLabel>
              <Input
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder={cfg?.vision.hasKey ? "留空保持不变" : "sk-..."}
                autoComplete="new-password"
              />
            </Field>
            <Field>
              <FieldLabel>Base URL（可选，默认官方）</FieldLabel>
              <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="https://api.deepseek.com" />
            </Field>
            <Field>
              <FieldLabel>模型（可选，默认 deepseek-flash）</FieldLabel>
              <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="deepseek-flash" />
            </Field>
          </div>
          <Button
            size="sm"
            disabled={busy}
            onClick={() =>
              void save(
                {
                  visionApiKey: apiKey || undefined,
                  visionBaseURL: baseURL,
                  visionModel: model,
                },
                "识别服务配置已更新",
              )
            }
          >
            保存
          </Button>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-base">访问模式</CardTitle>
          <CardDescription>
            多用户模式关闭时本机打开即用（推荐自用）；开启后需要账号密码登录，适合部署到服务器给多人使用。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">多用户模式</p>
              <p className="text-muted-foreground text-xs">
                开启后所有访问需要登录；开启时当前账号自动成为管理员并签发新会话
              </p>
            </div>
            <Switch
              checked={cfg?.server.multiUser ?? false}
              onCheckedChange={(v) => {
                if (!v) {
                  void save({ multiUser: false }, "已切换到单用户模式");
                  return;
                }
                // 单用户本地用户的密码是随机生成且从未展示的，
                // 不设一个可用密码，开启后会话一丢就登不回来。
                const pwd = window.prompt(
                  "开启多用户前，请为当前账号设置一个登录密码（至少 6 位，请牢记）：",
                );
                if (pwd === null) return;
                if (pwd.trim().length < 6) {
                  toast({ title: "管理员密码至少 6 位", variant: "destructive" });
                  return;
                }
                void save({ multiUser: true, adminPassword: pwd.trim() }, "已切换到多用户模式");
              }}
            />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">允许自助注册</p>
              <p className="text-muted-foreground text-xs">多用户模式下，登录页出现注册入口</p>
            </div>
            <Switch
              checked={cfg?.server.registerOpen ?? false}
              onCheckedChange={(v) => void save({ registerOpen: v }, v ? "自助注册已开启" : "自助注册已关闭")}
            />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">额度体系</p>
              <p className="text-muted-foreground text-xs">托管运营场景使用：执行扣次数、卡密兑换；个人使用保持关闭</p>
            </div>
            <Switch
              checked={cfg?.server.quotaEnabled ?? false}
              onCheckedChange={(v) => void save({ quotaEnabled: v }, v ? "额度体系已开启" : "额度体系已关闭")}
            />
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-base">照片池容量</CardTitle>
          <CardDescription>
            所有账号照片池加在一起的磁盘上限（不含单张兜底照片），默认 1GB。
            达到上限后新照片会被拒绝，已上传的照片不受影响。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-1.5">
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">已用</span>
              <span className="font-mono tabular-nums">
                {formatStorage(poolUsed)} / {formatStorage(poolLimit)} · {cfg?.photoPool.photoCount ?? 0} 张
              </span>
            </div>
            <div className="bg-muted h-1.5 overflow-hidden rounded-full">
              <div className="bg-primary h-full rounded-full transition-all" style={{ width: `${poolPct}%` }} />
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-muted-foreground w-16 text-xs">容量上限</span>
            <Input
              value={poolLimitGB}
              onChange={(e) => setPoolLimitGB(e.target.value)}
              inputMode="decimal"
              className="w-24"
            />
            <span className="text-muted-foreground text-xs">GB</span>
            <Button variant="outline" size="sm" disabled={busy} onClick={savePoolLimit}>
              保存
            </Button>
          </div>
          <p className="text-muted-foreground text-[11px]">范围 1MB-1TB；例如 0.5 表示 512MB。</p>
        </CardContent>
      </Card>

      <Card className="border-border/50">
        <CardHeader>
          <CardTitle className="text-base">证书校验白名单</CardTitle>
          <CardDescription>
            个别学校证书链不完整导致登录失败时，把学校域名后缀加进来（逗号分隔，如
            nut.campusphere.net）。白名单外仍做完整校验；留空表示全部严格校验。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <Input
            value={tlsHosts}
            onChange={(e) => setTlsHosts(e.target.value)}
            placeholder="如：nut.campusphere.net,example.edu.cn"
            className="max-w-md font-mono"
          />
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => void save({ tlsInsecureHosts: tlsHosts }, "证书白名单已更新")}
          >
            保存
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

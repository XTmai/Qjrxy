// 设置：URL 驱动的 tab（沿用 openilink settings 模式）。
// 外观 / 安全（改密）/ 额度（QINGJIN_QUOTA_ENABLED=true 时）/ 关于。
import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useInfo, useUser } from "@/hooks/use-auth";
import { useTheme } from "@/lib/theme";
import { QQ_GROUP_URL } from "@/lib/links";
import { api } from "@/api";
import { useToast } from "@/hooks/use-toast";
import { PageHeader } from "@/components/page-header";

const TABS = ["appearance", "security", "quota", "about"] as const;
type Tab = (typeof TABS)[number];

export function SettingsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const { toast } = useToast();
  const { data: user } = useUser();
  const { theme, setTheme } = useTheme();
  const { data: info } = useInfo();

  const tabFromUrl = location.pathname.split("/").pop() as Tab;
  const tab: Tab = TABS.includes(tabFromUrl) ? tabFromUrl : "appearance";
  const switchTab = (t: Tab) => navigate(`/settings/${t}`, { replace: true });

  const quotaOn = info?.quotaEnabled ?? false;
  const singleUser = info !== undefined && !info.multiUser;
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

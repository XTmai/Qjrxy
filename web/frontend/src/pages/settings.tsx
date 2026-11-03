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
  const remaining = user ? Math.max(0, user.quota - user.quotaUsed) : 0;

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
        <TabsList className="w-full">
          <TabsTrigger value="appearance" className="flex-1">
            外观
          </TabsTrigger>
          <TabsTrigger value="security" className="flex-1">
            安全
          </TabsTrigger>
          {quotaOn && (
            <TabsTrigger value="quota" className="flex-1">
              额度
            </TabsTrigger>
          )}
          <TabsTrigger value="about" className="flex-1">
            关于
          </TabsTrigger>
        </TabsList>

        {/* 外观 */}
        <TabsContent value="appearance">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">外观</CardTitle>
              <CardDescription>深色模式跟随系统，可手动切换</CardDescription>
            </CardHeader>
            <CardContent className="flex items-center justify-between">
              <p className="text-sm">当前主题：{theme === "dark" ? "深色" : "浅色"}</p>
              <Button variant="outline" size="sm" onClick={() => setTheme(theme === "dark" ? "light" : "dark")}>
                切换主题
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* 安全 */}
        <TabsContent value="security">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">修改登录密码</CardTitle>
              <CardDescription>用于登录本系统的密码，与今日校园的密码无关</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
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
        </TabsContent>

        {/* 额度（仅托管运营开启时） */}
        {quotaOn && (
          <TabsContent value="quota">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">额度</CardTitle>
                <CardDescription>每次执行扣 1 次，卡密由运营方发放</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center gap-2">
                  <span className="text-3xl font-semibold tabular-nums">{remaining}</span>
                  <span className="text-muted-foreground text-sm">次可用</span>
                  <Badge variant="secondary">累计使用 {user?.quotaUsed ?? 0}</Badge>
                </div>
                <div className="flex gap-2">
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
        <TabsContent value="about">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">关于</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              {info && (
                <p className="text-muted-foreground text-xs">
                  客户端协议 {info.appVersion} · API {info.apiVersion} · 会话 {info.tokenTTLDay} 天
                </p>
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
                <a
                  href="https://github.com/openilink/openilink-hub"
                  target="_blank"
                  rel="noreferrer"
                >
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

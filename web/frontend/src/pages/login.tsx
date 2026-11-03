// 登录 / 注册。基于 openilink 登录页布局（六边形背景 + 居中卡片），
// 认证通道只保留账号密码（本项目不接入微信/Passkey/OAuth）。
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { HexagonBackground } from "@/components/ui/hexagon-background";
import { useLogin, useRegister } from "@/hooks/use-auth";

export function LoginPage() {
  const navigate = useNavigate();
  const login = useLogin();
  const register = useRegister();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [nickname, setNickname] = useState("");
  const [error, setError] = useState("");

  const busy = login.isPending || register.isPending;

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    if (username.trim().length < 3) return setError("用户名至少 3 位");
    if (password.length < 6) return setError("密码至少 6 位");

    const action = mode === "login" ? login.mutateAsync : register.mutateAsync;
    const input = mode === "login"
      ? { username: username.trim(), password }
      : { username: username.trim(), password, nickname: nickname.trim() || undefined };

    action(input as never)
      .then(() => navigate("/", { replace: true }))
      .catch((err: Error) => setError(err.message || "操作失败，请重试"));
  };

  return (
    <div className="bg-background text-foreground relative flex min-h-screen items-center justify-center p-4">
      <HexagonBackground />

      <Card className="z-10 w-full max-w-sm">
        <CardHeader className="text-center">
          <div className="bg-primary text-primary-foreground mx-auto mb-3 flex size-12 items-center justify-center rounded-2xl text-xl font-bold">
            Q
          </div>
          <CardTitle className="text-xl tracking-tight">Qjrxy</CardTitle>
          <CardDescription>今日校园查寝助手 · 自部署</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="bg-muted mb-5 grid grid-cols-2 gap-1 rounded-lg p-1">
            {(["login", "register"] as const).map((m) => (
              <button
                key={m}
                type="button"
                onClick={() => {
                  setMode(m);
                  setError("");
                }}
                className={`rounded-md py-1.5 text-sm transition-colors ${
                  mode === m ? "bg-background text-foreground font-medium shadow-sm" : "text-muted-foreground"
                }`}
              >
                {m === "login" ? "登录" : "注册"}
              </button>
            ))}
          </div>

          <form onSubmit={submit} className="space-y-4">
            <Field>
              <FieldLabel>用户名</FieldLabel>
              <Input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="至少 3 位"
                autoComplete="username"
              />
            </Field>
            {mode === "register" && (
              <Field>
                <FieldLabel>
                  昵称 <span className="text-muted-foreground font-normal">（可选）</span>
                </FieldLabel>
                <Input value={nickname} onChange={(e) => setNickname(e.target.value)} placeholder="显示名称" />
              </Field>
            )}
            <Field>
              <FieldLabel>密码</FieldLabel>
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="至少 6 位"
                autoComplete={mode === "login" ? "current-password" : "new-password"}
              />
              {mode === "register" && (
                <FieldDescription>用于登录本系统，与今日校园的密码无关。</FieldDescription>
              )}
            </Field>
            {error && <p className="text-destructive text-sm">{error}</p>}
            <Button type="submit" variant="default" className="w-full" disabled={busy}>
              {busy ? "处理中…" : mode === "login" ? "登录" : "注册并登录"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <p className="text-muted-foreground absolute bottom-6 text-center text-xs leading-relaxed">
        仅供学习研究，请使用本人账号并遵守学校规定。
        <br />
        所有数据仅保存在你自己的服务器上。
      </p>
    </div>
  );
}

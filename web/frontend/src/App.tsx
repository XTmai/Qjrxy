import { Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "@/components/layout";
import { LoginPage } from "@/pages/login";
import { HomePage } from "@/pages/home";
import { AccountsPage } from "@/pages/accounts";
import { AuditPage } from "@/pages/audit";
import { SchedulesPage } from "@/pages/schedules";
import { SettingsPage } from "@/pages/settings";
import { useInfo, useUser } from "@/hooks/use-auth";
import { getToken } from "@/api/client";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";

function Gate({ children }: { children: React.ReactNode }) {
  // systemInfo 走 withUser 但单用户模式下无 token 也能取到（服务端自动落到本地用户），
  // 因此它是判定「单用户 / 多用户」的公开依据；多用户模式下无 token 时它 401（infoError）。
  const { data: info, isError: infoError } = useInfo();
  const single = !infoError && info !== undefined && !info.multiUser;
  const { data: user, isError, isPending, refetch } = useUser(single);

  if (!getToken()) {
    if (single) {
      // 单用户：继续走下面的 user 校验（me() 会返回本地用户）
    } else if (infoError) {
      return <Navigate to="/login" replace />;
    } else {
      return <Spinner />;
    }
  }
  if (isPending) return <Spinner />;
  if (isError || !user) {
    // 单用户模式下 me() 失败说明服务端异常，给重试入口而不是跳登录页（跳过去也是循环）
    if (single) return <LoadError onRetry={() => void refetch()} />;
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

function Spinner() {
  return (
    <div className="bg-background text-foreground flex min-h-screen items-center justify-center">
      <Loader2 className="size-5 animate-spin" />
    </div>
  );
}

function LoadError({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="bg-background text-foreground flex min-h-screen flex-col items-center justify-center gap-3">
      <p className="text-muted-foreground text-sm">本地服务暂时没有响应</p>
      <Button variant="outline" size="sm" onClick={onRetry}>
        重试
      </Button>
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <Gate>
            <Layout />
          </Gate>
        }
      >
        <Route path="/" element={<HomePage />} />
        <Route path="/accounts" element={<AccountsPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/schedules" element={<SchedulesPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/settings/:tab" element={<SettingsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

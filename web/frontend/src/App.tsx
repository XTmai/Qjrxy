import { useEffect, lazy, Suspense } from "react";
import { Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { Layout } from "@/components/layout";
import { LoginPage } from "@/pages/login";
import { HomePage } from "@/pages/home";
import { useInfo, useUser } from "@/hooks/use-auth";
import { UNAUTHORIZED_EVENT, getToken } from "@/api/client";
import { queryClient } from "@/lib/query-client";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";

// 除首页外的页面按需加载：首屏只解析首页 + 布局，弱机上首次可交互明显更快。
const AccountsPage = lazy(() => import("@/pages/accounts").then((m) => ({ default: m.AccountsPage })));
const AuditPage = lazy(() => import("@/pages/audit").then((m) => ({ default: m.AuditPage })));
const SchedulesPage = lazy(() => import("@/pages/schedules").then((m) => ({ default: m.SchedulesPage })));
const SettingsPage = lazy(() => import("@/pages/settings").then((m) => ({ default: m.SettingsPage })));

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
  const navigate = useNavigate();
  const location = useLocation();

  // 会话失效统一处理：清空缓存并回到登录页。
  // 之前只 dispatch 事件没有监听者，多用户模式下会话过期后只能手动清 localStorage。
  useEffect(() => {
    const onUnauthorized = () => {
      queryClient.clear();
      if (location.pathname !== "/login") navigate("/login", { replace: true });
    };
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
  }, [navigate, location.pathname]);

  return (
    <Suspense fallback={<Spinner />}>
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
    </Suspense>
  );
}

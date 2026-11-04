import { Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "@/components/layout";
import { LoginPage } from "@/pages/login";
import { HomePage } from "@/pages/home";
import { AccountsPage } from "@/pages/accounts";
import { AuditPage } from "@/pages/audit";
import { SchedulesPage } from "@/pages/schedules";
import { SettingsPage } from "@/pages/settings";
import { useUser } from "@/hooks/use-auth";
import { getToken } from "@/api/client";
import { Loader2 } from "lucide-react";

function Gate({ children }: { children: React.ReactNode }) {
  const { data: user, isError, isPending } = useUser();
  // 未登录（无 token）时 useUser 的查询被 enabled 禁用，isPending 恒为 true，
  // 必须在最前面判断，否则会永远停在加载动画上。
  if (!getToken()) return <Navigate to="/login" replace />;
  if (isPending) return <Spinner />;
  if (isError || !user) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function Spinner() {
  return (
    <div className="bg-background text-foreground flex min-h-screen items-center justify-center">
      <Loader2 className="size-5 animate-spin" />
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

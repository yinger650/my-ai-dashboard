import { useQuery } from "@tanstack/react-query";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { fetchSession } from "./api";
import { Layout } from "./components/Layout";
import { LoginPage } from "./pages/Login";
import { DashboardPage } from "./pages/Dashboard";
import { MachineDetailPage } from "./pages/MachineDetail";
import { ServiceDetailPage } from "./pages/ServiceDetail";
import { AccessPage } from "./pages/Access";
import { SettingsPage } from "./pages/Settings";

export default function App() {
  const location = useLocation();
  const { data: session, isLoading } = useQuery({
    queryKey: ["session"],
    queryFn: fetchSession,
    refetchInterval: 5 * 60 * 1000,
  });

  if (isLoading) {
    return <div className="flex h-full items-center justify-center text-slate-400">加载中…</div>;
  }

  const authed = !!session?.authenticated;

  if (!authed) {
    if (location.pathname === "/login") {
      return (
        <Routes>
          <Route path="/login" element={<LoginPage />} />
        </Routes>
      );
    }
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }

  return (
    <Layout>
      <Routes>
        <Route path="/login" element={<Navigate to="/" replace />} />
        <Route path="/" element={<DashboardPage />} />
        <Route path="/machines/:machineId" element={<MachineDetailPage />} />
        <Route path="/services/:serviceId" element={<ServiceDetailPage />} />
        <Route path="/access" element={<AccessPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  );
}

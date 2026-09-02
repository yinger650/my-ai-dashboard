import type { ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { NavLink, useNavigate } from "react-router-dom";
import { LogOut } from "lucide-react";
import { logout } from "../api";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";

const navItems = [
  { to: "/", label: "看板", end: true },
  { to: "/access", label: "访问记录", end: false },
  { to: "/settings", label: "设置", end: false },
];

export function Layout({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const qc = useQueryClient();

  async function handleLogout() {
    await logout();
    await qc.invalidateQueries({ queryKey: ["session"] });
    navigate("/login");
  }

  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-20 border-b border-[#1f2a44] bg-[#0b1020]/92 backdrop-blur">
        <div className="mx-auto flex max-w-[1600px] items-center justify-between px-4 py-2">
          <div className="flex min-w-0 items-center gap-3">
            <span className="text-sm font-semibold tracking-wide text-slate-100">
              <span className="mr-2 inline-block h-2 w-2 rounded-sm bg-indigo-400 align-middle" aria-hidden />
              AgentBoard
            </span>
            <nav className="ml-1 flex gap-1 overflow-x-auto">
              {navItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) =>
                    cn(
                      "rounded-md px-2.5 py-1 text-sm transition-colors",
                      isActive ? "bg-slate-800/80 text-white" : "text-slate-400 hover:text-white",
                    )
                  }
                >
                  {item.label}
                </NavLink>
              ))}
            </nav>
          </div>
          <Button variant="outline" size="sm" onClick={handleLogout}>
            <LogOut className="h-4 w-4" />
            <span className="hidden sm:inline">退出</span>
          </Button>
        </div>
      </header>
      <main className="mx-auto max-w-[1600px] px-4 py-5">{children}</main>
    </div>
  );
}

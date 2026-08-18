import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { login } from "../api";

export function LoginPage() {
  const [password, setPassword] = useState("");
  const [totp, setTotp] = useState("");
  const [needTotp, setNeedTotp] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  const qc = useQueryClient();

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await login(password, totp);
      await qc.invalidateQueries({ queryKey: ["session"] });
      navigate("/");
    } catch (err) {
      const e = err as Error & { code?: string };
      if (e.code === "totp_required") {
        setNeedTotp(true);
        setError("已启用双因素认证，请输入 TOTP 或恢复码");
      } else {
        setError(e.message || "登录失败");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <form onSubmit={onSubmit} className="card w-full max-w-sm p-6">
        <h1 className="mb-1 text-xl font-semibold text-indigo-400">◆ AgentBoard Personal</h1>
        <p className="mb-6 text-sm text-slate-400">管理员登录</p>

        <label className="mb-1 block text-sm text-slate-300">密码</label>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="mb-4 w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm outline-none focus:border-indigo-500"
          autoFocus
        />

        <label className="mb-1 block text-sm text-slate-300">TOTP / 恢复码{needTotp ? "（必填）" : "（如已启用）"}</label>
        <input
          type="text"
          inputMode="numeric"
          autoComplete="one-time-code"
          value={totp}
          onChange={(e) => setTotp(e.target.value)}
          placeholder={needTotp ? "6 位验证码或恢复码" : "未启用可留空"}
          className="mb-4 w-full rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm outline-none focus:border-indigo-500"
        />

        {error && <div className="mb-4 rounded-md bg-red-500/10 px-3 py-2 text-sm sev-error">{error}</div>}

        <button
          type="submit"
          disabled={busy || !password || (needTotp && !totp)}
          className="w-full rounded-md bg-indigo-600 px-3 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50"
        >
          {busy ? "登录中…" : "登录"}
        </button>
      </form>
    </div>
  );
}

import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { login } from "../api";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";

export function LoginPage() {
  const [password, setPassword] = useState("");
  const [totp, setTotp] = useState("");
  const [needTotp, setNeedTotp] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  const qc = useQueryClient();

  async function onSubmit(e: FormEvent) {
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
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-indigo-400">◆ AgentBoard Personal</CardTitle>
          <CardDescription>管理员登录</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit}>
            <Label htmlFor="password">密码</Label>
            <Input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="mb-4"
              autoFocus
            />
            <Label htmlFor="totp">TOTP / 恢复码{needTotp ? "（必填）" : "（如已启用）"}</Label>
            <Input
              id="totp"
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              value={totp}
              onChange={(e) => setTotp(e.target.value)}
              placeholder={needTotp ? "6 位验证码或恢复码" : "未启用可留空"}
              className="mb-4"
            />
            {error && <div className="mb-4 rounded-md bg-red-500/10 px-3 py-2 text-sm sev-error">{error}</div>}
            <Button type="submit" disabled={busy || !password || (needTotp && !totp)} className="w-full">
              {busy ? "登录中…" : "登录"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}

import { useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { fetchAuthMeta, login } from "../api";
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
  const meta = useQuery({ queryKey: ["auth-meta"], queryFn: fetchAuthMeta });
  const feishu = meta.data?.edition === "feishu";

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

  const params = new URLSearchParams(window.location.search);
  const oauthError = params.get("error");

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <p className="ab-eyebrow">AgentBoard {feishu ? "Feishu" : "Personal"}</p>
          <CardTitle className="mt-1">登录控制台</CardTitle>
          <CardDescription>
            {feishu ? "使用飞书账号进入你的看板" : "输入管理员密码进入看板"}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {feishu ? (
            <div className="space-y-4">
              {(oauthError === "feishu" || oauthError === "tenant") && (
                <div className="rounded-md bg-red-500/10 px-3 py-2 text-sm sev-error">
                  {oauthError === "tenant" ? "当前飞书企业不允许使用此看板" : "飞书登录失败，请重试"}
                </div>
              )}
              <Button className="w-full" asChild={false} onClick={() => { window.location.href = "/auth/feishu/login"; }}>
                飞书登录
              </Button>
            </div>
          ) : (
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
          )}
        </CardContent>
      </Card>
    </div>
  );
}

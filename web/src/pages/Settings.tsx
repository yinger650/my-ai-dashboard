import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { apiDelete, apiGet, apiPatch, apiPost } from "../api";
import type { Machine, TokenInfo } from "../types";
import { localTime } from "../format";

interface CreatedToken {
  id: string;
  token: string;
  prefix: string;
  scope: string;
}

export function SettingsPage() {
  const qc = useQueryClient();
  const [revealToken, setRevealToken] = useState<CreatedToken | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [totpSetup, setTotpSetup] = useState<{ secret: string; otpauth_url: string } | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [totpPassword, setTotpPassword] = useState("");

  const machines = useQuery({ queryKey: ["admin-machines"], queryFn: () => apiGet<Machine[]>("/api/v1/admin/machines") });
  const tokens = useQuery({ queryKey: ["admin-tokens"], queryFn: () => apiGet<TokenInfo[]>("/api/v1/admin/tokens") });
  const settings = useQuery({ queryKey: ["admin-settings"], queryFn: () => apiGet<Record<string, unknown>>("/api/v1/admin/settings") });
  const totp = useQuery({ queryKey: ["admin-totp"], queryFn: () => apiGet<{ enabled: boolean }>("/api/v1/admin/totp") });

  const [mKey, setMKey] = useState("");
  const [mName, setMName] = useState("");
  const [mKind, setMKind] = useState("physical");

  const createMachine = useMutation({
    mutationFn: () =>
      apiPost<{ machine: Machine; token?: CreatedToken }>("/api/v1/admin/machines", {
        machine_key: mKey,
        name: mName,
        kind: mKind,
        create_machine_token: true,
      }),
    onSuccess: (data) => {
      if (data.token) setRevealToken(data.token);
      setMKey("");
      setMName("");
      qc.invalidateQueries({ queryKey: ["admin-machines"] });
      qc.invalidateQueries({ queryKey: ["admin-tokens"] });
    },
  });

  const [tName, setTName] = useState("");
  const [tScope, setTScope] = useState("viewer");
  const createToken = useMutation({
    mutationFn: () => apiPost<CreatedToken>("/api/v1/admin/tokens", { name: tName, scope: tScope }),
    onSuccess: (data) => {
      setRevealToken(data);
      setTName("");
      qc.invalidateQueries({ queryKey: ["admin-tokens"] });
    },
  });

  const revoke = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/admin/tokens/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin-tokens"] }),
  });

  const [title, setTitle] = useState<string>("");
  const saveTitle = useMutation({
    mutationFn: () => apiPatch("/api/v1/admin/settings", { board_title: title }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin-settings"] }),
  });

  const startTotp = useMutation({
    mutationFn: () => apiPost<{ secret: string; otpauth_url: string }>("/api/v1/admin/totp/setup", {}),
    onSuccess: (data) => {
      setTotpSetup(data);
      setTotpCode("");
    },
  });
  const confirmTotp = useMutation({
    mutationFn: () => apiPost<{ enabled: boolean; recovery_codes: string[] }>("/api/v1/admin/totp/confirm", { code: totpCode }),
    onSuccess: (data) => {
      setRecoveryCodes(data.recovery_codes);
      setTotpSetup(null);
      setTotpCode("");
      qc.invalidateQueries({ queryKey: ["admin-totp"] });
      qc.invalidateQueries({ queryKey: ["session"] });
    },
  });
  const disableTotp = useMutation({
    mutationFn: () => apiPost("/api/v1/admin/totp/disable", { password: totpPassword, code: totpCode }),
    onSuccess: () => {
      setTotpCode("");
      setTotpPassword("");
      qc.invalidateQueries({ queryKey: ["admin-totp"] });
      qc.invalidateQueries({ queryKey: ["session"] });
    },
  });
  const regenRecovery = useMutation({
    mutationFn: () => apiPost<{ recovery_codes: string[] }>("/api/v1/admin/totp/recovery", { code: totpCode }),
    onSuccess: (data) => {
      setRecoveryCodes(data.recovery_codes);
      setTotpCode("");
    },
  });

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold">设置</h1>

      <section className="card p-4">
        <h2 className="mb-3 font-medium">看板设置</h2>
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <label className="mb-1 block text-xs text-slate-500">看板标题</label>
            <input
              value={title || String(settings.data?.board_title ?? "")}
              onChange={(e) => setTitle(e.target.value)}
              className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm"
            />
          </div>
          <button onClick={() => saveTitle.mutate()} className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-500">
            保存
          </button>
          <span className="text-xs text-slate-500">轮询间隔：{String(settings.data?.poll_interval_seconds ?? 15)}s</span>
        </div>
      </section>

      <section className="card p-4">
        <h2 className="mb-3 font-medium">双因素认证（TOTP）</h2>
        <p className="mb-3 text-sm text-slate-400">
          当前状态：{totp.data?.enabled ? "已启用" : "未启用"}。启用后登录必须提供验证器验证码或一次性恢复码。
        </p>
        {!totp.data?.enabled && !totpSetup && (
          <button onClick={() => startTotp.mutate()} className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-500">
            开始启用 TOTP
          </button>
        )}
        {totpSetup && (
          <div className="space-y-3">
            <p className="text-sm text-slate-300">在验证器中添加密钥，或打开 otpauth URL：</p>
            <div className="break-all rounded-md bg-slate-900 p-3 font-mono text-xs">{totpSetup.secret}</div>
            <div className="break-all rounded-md bg-slate-900 p-3 font-mono text-xs text-slate-400">{totpSetup.otpauth_url}</div>
            <input
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
              placeholder="输入 6 位验证码确认"
              className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm"
            />
            <button
              disabled={!totpCode || confirmTotp.isPending}
              onClick={() => confirmTotp.mutate()}
              className="ml-2 rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-500 disabled:opacity-50"
            >
              确认启用
            </button>
            {confirmTotp.error && <div className="text-sm sev-error">{(confirmTotp.error as Error).message}</div>}
          </div>
        )}
        {totp.data?.enabled && (
          <div className="flex flex-wrap items-end gap-2">
            <input
              type="password"
              value={totpPassword}
              onChange={(e) => setTotpPassword(e.target.value)}
              placeholder="当前密码"
              className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm"
            />
            <input
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
              placeholder="TOTP 或恢复码"
              className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm"
            />
            <button
              disabled={!totpPassword || !totpCode || disableTotp.isPending}
              onClick={() => disableTotp.mutate()}
              className="rounded-md border border-slate-700 px-3 py-1.5 text-sm hover:bg-slate-800"
            >
              关闭 TOTP
            </button>
            <button
              disabled={!totpCode || regenRecovery.isPending}
              onClick={() => regenRecovery.mutate()}
              className="rounded-md border border-slate-700 px-3 py-1.5 text-sm hover:bg-slate-800"
            >
              重新生成恢复码
            </button>
            {disableTotp.error && <div className="w-full text-sm sev-error">{(disableTotp.error as Error).message}</div>}
          </div>
        )}
      </section>

      <section className="card p-4">
        <h2 className="mb-3 font-medium">机器</h2>
        <div className="mb-4 flex flex-wrap items-end gap-2">
          <input value={mKey} onChange={(e) => setMKey(e.target.value)} placeholder="machine_key" className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm" />
          <input value={mName} onChange={(e) => setMName(e.target.value)} placeholder="名称" className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm" />
          <select value={mKind} onChange={(e) => setMKind(e.target.value)} className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm">
            <option value="physical">physical</option>
            <option value="vm">vm</option>
            <option value="container_host">container_host</option>
            <option value="virtual">virtual</option>
          </select>
          <button
            disabled={!mKey || !mName || createMachine.isPending}
            onClick={() => createMachine.mutate()}
            className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-500 disabled:opacity-50"
          >
            创建机器 + Token
          </button>
        </div>
        {createMachine.error && <div className="mb-2 text-sm sev-error">{(createMachine.error as Error).message}</div>}
        <table className="w-full text-left text-sm">
          <thead className="text-xs text-slate-500"><tr><th className="py-1">名称</th><th>key</th><th>类型</th><th>状态</th></tr></thead>
          <tbody>
            {(machines.data ?? []).map((m) => (
              <tr key={m.id} className="border-t border-slate-800/50">
                <td className="py-1.5">{m.name}</td>
                <td className="font-mono text-xs">{m.machine_key}</td>
                <td>{m.kind}</td>
                <td>{m.enabled ? "启用" : "禁用"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="card p-4">
        <h2 className="mb-3 font-medium">Token</h2>
        <div className="mb-4 flex flex-wrap items-end gap-2">
          <input value={tName} onChange={(e) => setTName(e.target.value)} placeholder="Token 名称" className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm" />
          <select value={tScope} onChange={(e) => setTScope(e.target.value)} className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm">
            <option value="viewer">viewer</option>
          </select>
          <button
            disabled={!tName || createToken.isPending}
            onClick={() => createToken.mutate()}
            className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-500 disabled:opacity-50"
          >
            创建 Viewer Token
          </button>
        </div>
        <table className="w-full text-left text-sm">
          <thead className="text-xs text-slate-500"><tr><th className="py-1">名称</th><th>前缀</th><th>scope</th><th>最后使用</th><th></th></tr></thead>
          <tbody>
            {(tokens.data ?? []).map((t) => (
              <tr key={t.id} className="border-t border-slate-800/50">
                <td className="py-1.5">{t.name}</td>
                <td className="font-mono text-xs">{t.token_prefix}…</td>
                <td>{t.scope}</td>
                <td className="text-xs text-slate-500">{t.last_used_at ? localTime(t.last_used_at) : "从未"}</td>
                <td className="text-right">
                  {t.revoked_at ? (
                    <span className="text-xs text-slate-500">已吊销</span>
                  ) : (
                    <button onClick={() => revoke.mutate(t.id)} className="text-xs sev-error hover:underline">吊销</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      {revealToken && (
        <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/60 p-4">
          <div className="card w-full max-w-lg p-6">
            <h3 className="mb-2 text-lg font-semibold">Token 已创建</h3>
            <p className="mb-3 text-sm sev-warning">请立即复制保存，关闭后无法再次查看。</p>
            <div className="mb-4 break-all rounded-md bg-slate-900 p-3 font-mono text-sm">{revealToken.token}</div>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => navigator.clipboard?.writeText(revealToken.token)}
                className="rounded-md border border-slate-700 px-3 py-1.5 text-sm hover:bg-slate-800"
              >
                复制
              </button>
              <button onClick={() => setRevealToken(null)} className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white">
                关闭
              </button>
            </div>
          </div>
        </div>
      )}

      {recoveryCodes && (
        <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/60 p-4">
          <div className="card w-full max-w-lg p-6">
            <h3 className="mb-2 text-lg font-semibold">恢复码</h3>
            <p className="mb-3 text-sm sev-warning">请立即抄写保存。每个恢复码只能使用一次，关闭后无法再查看。</p>
            <ul className="mb-4 space-y-1 font-mono text-sm">
              {recoveryCodes.map((c) => (
                <li key={c}>{c}</li>
              ))}
            </ul>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => navigator.clipboard?.writeText(recoveryCodes.join("\n"))}
                className="rounded-md border border-slate-700 px-3 py-1.5 text-sm hover:bg-slate-800"
              >
                复制全部
              </button>
              <button onClick={() => setRecoveryCodes(null)} className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white">
                关闭
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

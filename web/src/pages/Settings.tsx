import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Copy, Database, KeyRound, Server, Shield, Terminal } from "lucide-react";
import { apiDelete, apiGet, apiPatch, apiPost } from "../api";
import type { Board, Machine, TokenInfo } from "../types";
import { fmtBytes, localTime } from "../format";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Switch } from "../components/ui/switch";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "../components/ui/dialog";
import { DEFAULT_CARD_PINS } from "../lib/board-card";

interface CreatedToken {
  id: string;
  token: string;
  prefix: string;
  scope: string;
}

interface AdminSettings {
  board_title?: string;
  timezone?: string;
  poll_interval_seconds?: number;
  public_url?: string;
  board_txt_url?: string;
  ingest_url?: string;
  board_layout?: unknown;
  card_pin_keys?: string[];
  [k: string]: unknown;
}

export function SettingsPage() {
  const qc = useQueryClient();
  const [revealToken, setRevealToken] = useState<CreatedToken | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [totpSetup, setTotpSetup] = useState<{ secret: string; otpauth_url: string } | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [totpPassword, setTotpPassword] = useState("");

  const machines = useQuery({ queryKey: ["admin-machines"], queryFn: () => apiGet<Machine[]>("/api/v1/admin/machines") });
  const tokens = useQuery({ queryKey: ["admin-tokens"], queryFn: () => apiGet<TokenInfo[]>("/api/v1/admin/tokens") });
  const settings = useQuery({ queryKey: ["admin-settings"], queryFn: () => apiGet<AdminSettings>("/api/v1/admin/settings") });
  const totp = useQuery({ queryKey: ["admin-totp"], queryFn: () => apiGet<{ enabled: boolean }>("/api/v1/admin/totp") });
  const board = useQuery({ queryKey: ["board"], queryFn: () => apiGet<Board>("/api/v1/board") });

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
  const createToken = useMutation({
    mutationFn: () => apiPost<CreatedToken>("/api/v1/admin/tokens", { name: tName, scope: "viewer" }),
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
  const [poll, setPoll] = useState<string>("");
  const [tz, setTz] = useState<string>("");
  const [eventDays, setEventDays] = useState<string>("");
  const [quotaGB, setQuotaGB] = useState<string>("");
  const saveBoard = useMutation({
    mutationFn: () =>
      apiPatch("/api/v1/admin/settings", {
        board_title: title || String(settings.data?.board_title ?? ""),
        poll_interval_seconds: Number(poll || settings.data?.poll_interval_seconds || 15),
        timezone: tz || String(settings.data?.timezone ?? "UTC"),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin-settings"] });
      qc.invalidateQueries({ queryKey: ["board"] });
    },
  });

  const saveRetention = useMutation({
    mutationFn: () => {
      const days = Number(eventDays || settings.data?.event_retention_days || 30);
      const gb = Number(quotaGB || 5);
      return apiPatch("/api/v1/admin/settings", {
        event_retention_days: days,
        access_retention_days: days,
        raw_metric_retention_days: days,
        event_quota_bytes: Math.round(gb * 1024 * 1024 * 1024),
      });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin-settings"] }),
  });

  const runMaintenance = useMutation({
    mutationFn: () =>
      apiPost<{
        expired_sessions_deleted: number;
        events_deleted: number;
        access_deleted: number;
        runs_deleted: number;
        runs_closed: number;
        quota_deleted: number;
        events_bytes: number;
      }>("/api/v1/admin/maintenance/run", {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["board"] });
      qc.invalidateQueries({ queryKey: ["service-runs"] });
      qc.invalidateQueries({ queryKey: ["service-logs"] });
    },
  });

  const resetLayout = useMutation({
    mutationFn: () => apiPatch("/api/v1/admin/settings", { board_layout: null }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin-settings"] });
      qc.invalidateQueries({ queryKey: ["board"] });
    },
  });

  const savePins = useMutation({
    mutationFn: (keys: string[]) => apiPatch("/api/v1/admin/settings", { card_pin_keys: keys }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin-settings"] });
      qc.invalidateQueries({ queryKey: ["board"] });
    },
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

  function copy(text: string, id: string) {
    void navigator.clipboard?.writeText(text);
    setCopied(id);
    window.setTimeout(() => setCopied(null), 1500);
  }

  const publicUrl = String(settings.data?.public_url ?? "");
  const boardTxt = String(settings.data?.board_txt_url ?? `${publicUrl}/api/v1/board.txt`);
  const ingestUrl = String(settings.data?.ingest_url ?? `${publicUrl}/ingest/v1/events`);

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
        <p className="text-sm text-slate-400">配置看板、访问入口与 API Token。</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Server className="h-4 w-4 text-indigo-400" /> 看板
          </CardTitle>
          <CardDescription>标题、轮询间隔、网格布局与首页置顶</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap items-end gap-3">
            <div>
              <Label>看板标题</Label>
              <Input
                value={title || String(settings.data?.board_title ?? "")}
                onChange={(e) => setTitle(e.target.value)}
                className="w-64"
              />
            </div>
            <div>
              <Label>时区</Label>
              <Input
                value={tz || String(settings.data?.timezone ?? "UTC")}
                onChange={(e) => setTz(e.target.value)}
                className="w-40"
              />
            </div>
            <div>
              <Label>轮询间隔（秒）</Label>
              <Input
                type="number"
                min={5}
                value={poll || String(settings.data?.poll_interval_seconds ?? 15)}
                onChange={(e) => setPoll(e.target.value)}
                className="w-28"
              />
            </div>
            <Button onClick={() => saveBoard.mutate()} disabled={saveBoard.isPending}>
              保存
            </Button>
            <Button variant="outline" onClick={() => resetLayout.mutate()} disabled={resetLayout.isPending}>
              重置网格布局
            </Button>
          </div>
          <CardPinChecks
            selected={
              Array.isArray(settings.data?.card_pin_keys)
                ? settings.data.card_pin_keys
                : (board.data?.card_pin_keys ?? [])
            }
            candidates={board.data?.pin_candidates ?? []}
            pending={savePins.isPending}
            onToggle={(key, on) => {
              const next = new Set(
                Array.isArray(settings.data?.card_pin_keys)
                  ? settings.data.card_pin_keys
                  : (board.data?.card_pin_keys ?? []),
              );
              if (on) next.add(key);
              else next.delete(key);
              savePins.mutate([...next].sort());
            }}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Database className="h-4 w-4 text-indigo-400" /> 日志存储
          </CardTitle>
          <CardDescription>滚动日志、事件与访问记录最多保留一个月，容量上限 5 GiB。置顶当前态不会被清掉。</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap items-end gap-3">
            <div>
              <Label>保留天数</Label>
              <Input
                type="number"
                min={1}
                max={365}
                value={eventDays || String(settings.data?.event_retention_days ?? 30)}
                onChange={(e) => setEventDays(e.target.value)}
                className="w-28"
              />
            </div>
            <div>
              <Label>容量上限（GiB）</Label>
              <Input
                type="number"
                min={1}
                max={100}
                value={
                  quotaGB ||
                  String(
                    Math.round(Number(settings.data?.event_quota_bytes ?? 5 * 1024 * 1024 * 1024) / (1024 * 1024 * 1024)),
                  )
                }
                onChange={(e) => setQuotaGB(e.target.value)}
                className="w-28"
              />
            </div>
            <Button onClick={() => saveRetention.mutate()} disabled={saveRetention.isPending}>
              保存
            </Button>
            <Button variant="outline" onClick={() => runMaintenance.mutate()} disabled={runMaintenance.isPending}>
              {runMaintenance.isPending ? "清理中…" : "立即清理"}
            </Button>
          </div>
          {runMaintenance.data && (
            <p className="text-xs text-slate-400">
              已删事件 {runMaintenance.data.events_deleted} · 访问 {runMaintenance.data.access_deleted} · 超额{" "}
              {runMaintenance.data.quota_deleted} · 当前占用 {fmtBytes(runMaintenance.data.events_bytes)}
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Terminal className="h-4 w-4 text-indigo-400" /> 访问信息
          </CardTitle>
          <CardDescription>给 Agent / curl / TUI 使用的入口。只读密钥见下方 Viewer Token。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <CopyRow label="看板 URL" value={publicUrl || "（未配置 ABP_PUBLIC_URL）"} onCopy={() => copy(publicUrl, "url")} copied={copied === "url"} />
          <CopyRow
            label="纯文本 board.txt"
            value={`curl -H "Authorization: Bearer abp_v_…" ${boardTxt}`}
            onCopy={() => copy(`curl -H "Authorization: Bearer abp_v_…" ${boardTxt}`, "txt")}
            copied={copied === "txt"}
          />
          <CopyRow
            label="紧凑 TUI"
            value={`curl -H "Authorization: Bearer abp_v_…" "${boardTxt}?compact=1"`}
            onCopy={() => copy(`curl -H "Authorization: Bearer abp_v_…" "${boardTxt}?compact=1"`, "compact")}
            copied={copied === "compact"}
          />
          <CopyRow
            label="采集 Ingest"
            value={`POST ${ingestUrl}`}
            onCopy={() => copy(ingestUrl, "ingest")}
            copied={copied === "ingest"}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Shield className="h-4 w-4 text-indigo-400" /> 双因素认证（TOTP）
          </CardTitle>
          <CardDescription>
            当前状态：{totp.data?.enabled ? "已启用" : "未启用"}。启用后登录必须提供验证器验证码或一次性恢复码。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {!totp.data?.enabled && !totpSetup && (
            <Button onClick={() => startTotp.mutate()} disabled={startTotp.isPending}>
              开始启用 TOTP
            </Button>
          )}
          {totpSetup && (
            <div className="space-y-3">
              <p className="text-sm text-slate-300">在验证器中添加密钥，或打开 otpauth URL：</p>
              <div className="break-all rounded-md bg-slate-900 p-3 font-mono text-xs">{totpSetup.secret}</div>
              <div className="break-all rounded-md bg-slate-900 p-3 font-mono text-xs text-slate-400">{totpSetup.otpauth_url}</div>
              <div className="flex flex-wrap items-end gap-2">
                <Input
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  placeholder="输入 6 位验证码确认"
                  className="w-56"
                />
                <Button disabled={!totpCode || confirmTotp.isPending} onClick={() => confirmTotp.mutate()}>
                  确认启用
                </Button>
              </div>
              {confirmTotp.error && <div className="text-sm sev-error">{(confirmTotp.error as Error).message}</div>}
            </div>
          )}
          {totp.data?.enabled && (
            <div className="flex flex-wrap items-end gap-2">
              <Input
                type="password"
                value={totpPassword}
                onChange={(e) => setTotpPassword(e.target.value)}
                placeholder="当前密码"
                className="w-44"
              />
              <Input
                value={totpCode}
                onChange={(e) => setTotpCode(e.target.value)}
                placeholder="TOTP 或恢复码"
                className="w-44"
              />
              <Button
                variant="outline"
                disabled={!totpPassword || !totpCode || disableTotp.isPending}
                onClick={() => disableTotp.mutate()}
              >
                关闭 TOTP
              </Button>
              <Button variant="outline" disabled={!totpCode || regenRecovery.isPending} onClick={() => regenRecovery.mutate()}>
                重新生成恢复码
              </Button>
              {disableTotp.error && <div className="w-full text-sm sev-error">{(disableTotp.error as Error).message}</div>}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>机器</CardTitle>
          <CardDescription>创建机器时会同时生成一次性 Machine Token</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="mb-4 flex flex-wrap items-end gap-2">
            <Input value={mKey} onChange={(e) => setMKey(e.target.value)} placeholder="machine_key" className="w-44" />
            <Input value={mName} onChange={(e) => setMName(e.target.value)} placeholder="名称" className="w-44" />
            <select
              value={mKind}
              onChange={(e) => setMKind(e.target.value)}
              className="h-9 rounded-md border border-slate-700 bg-slate-900 px-3 text-sm"
            >
              <option value="physical">physical</option>
              <option value="vm">vm</option>
              <option value="container_host">container_host</option>
              <option value="virtual">virtual</option>
            </select>
            <Button disabled={!mKey || !mName || createMachine.isPending} onClick={() => createMachine.mutate()}>
              创建机器 + Token
            </Button>
          </div>
          {createMachine.error && <div className="mb-2 text-sm sev-error">{(createMachine.error as Error).message}</div>}
          <table className="w-full text-left text-sm">
            <thead className="text-xs text-slate-500">
              <tr>
                <th className="py-1">名称</th>
                <th>key</th>
                <th>类型</th>
                <th>状态</th>
              </tr>
            </thead>
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
          <div className="mt-6">
            <TokenGroup
              title="Machine Token"
              hint="前缀 abp_m_，绑定上面某一台机器，只能上报不能读看板。吊销后该机 ingest 会 401。"
              empty="还没有 Machine Token。"
              tokens={(tokens.data ?? []).filter((t) => t.scope === "machine_ingest")}
              machines={machines.data ?? []}
              onRevoke={(id) => revoke.mutate(id)}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="h-4 w-4 text-indigo-400" /> Viewer Token
          </CardTitle>
          <CardDescription>
            只读看板（<span className="font-mono">abp_v_</span>），给 curl / board.txt 用。不会随机器自动生成，全看板通常一把就够。Machine
            Token（<span className="font-mono">abp_m_</span>）在上方「机器」卡片里。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="mb-4 flex flex-wrap items-end gap-2">
            <Input value={tName} onChange={(e) => setTName(e.target.value)} placeholder="Viewer 名称" className="w-52" />
            <Button disabled={!tName || createToken.isPending} onClick={() => createToken.mutate()}>
              创建 Viewer Token
            </Button>
          </div>
          <TokenGroup
            title="只读 Viewer"
            hint="没有 Viewer 时这里是空的，不会把 Machine Token 混进来。"
            empty="还没有 Viewer Token。"
            tokens={(tokens.data ?? []).filter((t) => t.scope === "viewer")}
            onRevoke={(id) => revoke.mutate(id)}
          />
          {(tokens.data ?? []).some((t) => t.scope !== "viewer" && t.scope !== "machine_ingest") && (
            <TokenGroup
              title="其它"
              hint=""
              empty=""
              tokens={(tokens.data ?? []).filter((t) => t.scope !== "viewer" && t.scope !== "machine_ingest")}
              onRevoke={(id) => revoke.mutate(id)}
            />
          )}
        </CardContent>
      </Card>

      <Dialog open={!!revealToken} onOpenChange={(o) => !o && setRevealToken(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {revealToken?.scope === "viewer" ? "Viewer Token 已创建" : "Machine Token 已创建"}
            </DialogTitle>
            <DialogDescription>请立即复制保存，关闭后无法再次查看。Viewer 不能用来上报。</DialogDescription>
          </DialogHeader>
          <div className="mb-4 break-all rounded-md bg-slate-900 p-3 font-mono text-sm">{revealToken?.token}</div>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => revealToken && copy(revealToken.token, "tok")}>
              {copied === "tok" ? "已复制" : "复制"}
            </Button>
            <Button onClick={() => setRevealToken(null)}>关闭</Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={!!recoveryCodes} onOpenChange={(o) => !o && setRecoveryCodes(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>恢复码</DialogTitle>
            <DialogDescription>请立即抄写保存。每个恢复码只能使用一次，关闭后无法再查看。</DialogDescription>
          </DialogHeader>
          <ul className="mb-4 space-y-1 font-mono text-sm">
            {(recoveryCodes ?? []).map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => recoveryCodes && copy(recoveryCodes.join("\n"), "rec")}>
              {copied === "rec" ? "已复制" : "复制全部"}
            </Button>
            <Button onClick={() => setRecoveryCodes(null)}>关闭</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function CardPinChecks({
  selected,
  candidates,
  pending,
  onToggle,
}: {
  selected: string[];
  candidates: { service_key: string; name: string }[];
  pending: boolean;
  onToggle: (key: string, on: boolean) => void;
}) {
  const extra = new Set(selected);
  return (
    <div>
      <Label>首页置顶</Label>
      <p className="mb-2 text-xs text-slate-500">
        机器卡片日志区上方的置顶条。默认四项始终显示；自然语言扩展等其它服务有置顶后可在此勾选。
      </p>
      <div className="flex flex-col gap-2">
        {DEFAULT_CARD_PINS.map((p) => (
          <label key={p.key} className="flex items-center gap-2 text-sm text-slate-400">
            <Switch id={`pin-default-${p.key}`} checked disabled />
            <span>
              {p.name}
              <span className="ml-2 font-mono text-[11px] text-slate-600">{p.key}</span>
            </span>
          </label>
        ))}
        {candidates.map((c) => (
          <label key={c.service_key} className="flex items-center gap-2 text-sm text-slate-200">
            <Switch
              id={`pin-${c.service_key}`}
              checked={extra.has(c.service_key)}
              disabled={pending}
              onCheckedChange={(on) => onToggle(c.service_key, on)}
            />
            <span>
              {c.name || c.service_key}
              <span className="ml-2 font-mono text-[11px] text-slate-500">{c.service_key}</span>
            </span>
          </label>
        ))}
        {candidates.length === 0 && (
          <p className="text-xs text-slate-600">暂无其它可置顶服务。启用自然语言扩展并产生置顶后会出现在这里。</p>
        )}
      </div>
    </div>
  );
}

function CopyRow({
  label,
  value,
  onCopy,
  copied,
}: {
  label: string;
  value: string;
  onCopy: () => void;
  copied: boolean;
}) {
  return (
    <div className="flex flex-col gap-1 sm:flex-row sm:items-center">
      <div className="w-36 shrink-0 text-xs text-slate-500">{label}</div>
      <div className="min-w-0 flex-1 truncate rounded-md bg-slate-900 px-3 py-2 font-mono text-xs">{value}</div>
      <Button type="button" size="sm" variant="outline" onClick={onCopy}>
        <Copy className="h-3.5 w-3.5" />
        {copied ? "已复制" : "复制"}
      </Button>
    </div>
  );
}

export function tokenKindLabel(scope: string): string {
  switch (scope) {
    case "machine_ingest":
      return "上报";
    case "viewer":
      return "只读";
    case "service_ingest":
      return "单服务上报";
    default:
      return scope;
  }
}

function TokenGroup({
  title,
  hint,
  empty,
  tokens,
  machines,
  onRevoke,
}: {
  title: string;
  hint: string;
  empty: string;
  tokens: TokenInfo[];
  machines?: Machine[];
  onRevoke: (id: string) => void;
}) {
  const showMachine = Boolean(machines) && tokens.some((t) => t.machine_id);
  const machineKey = (id: string | null) => machines?.find((m) => m.id === id)?.machine_key ?? "—";
  return (
    <div className="mb-6 last:mb-0">
      <div className="mb-1 text-sm font-medium text-slate-200">{title}</div>
      {hint ? <p className="mb-2 text-xs text-slate-500">{hint}</p> : null}
      {tokens.length === 0 ? (
        <p className="text-sm text-slate-500">{empty}</p>
      ) : (
        <table className="w-full text-left text-sm">
          <thead className="text-xs text-slate-500">
            <tr>
              <th className="py-1">名称</th>
              <th>前缀</th>
              {showMachine ? <th>机器</th> : null}
              <th>类型</th>
              <th>最后使用</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {tokens.map((t) => (
              <tr key={t.id} className="border-t border-slate-800/50">
                <td className="py-1.5">{t.name}</td>
                <td className="font-mono text-xs">{t.token_prefix}…</td>
                {showMachine ? <td className="font-mono text-xs">{machineKey(t.machine_id)}</td> : null}
                <td>{tokenKindLabel(t.scope)}</td>
                <td className="text-xs text-slate-500">{t.last_used_at ? localTime(t.last_used_at) : "从未"}</td>
                <td className="text-right">
                  {t.revoked_at ? (
                    <span className="text-xs text-slate-500">已吊销</span>
                  ) : (
                    <Button variant="ghost" size="sm" className="sev-error" onClick={() => onRevoke(t.id)}>
                      吊销
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

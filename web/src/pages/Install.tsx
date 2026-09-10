import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { Link, NavLink, Route, Routes } from "react-router-dom";
import { Bot, Copy, Download, Server, Sparkles } from "lucide-react";
import { apiGet, apiPost } from "../api";
import type { Machine } from "../types";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import {
  LINUX_ASSETS,
  MACHINE_KEY_PLACEHOLDER,
  agentEnvSnippet,
  agentInstallPrompt,
  boardOrigin,
  clientDownloadUrl,
  hostEnvSnippet,
  hostInstallPrompt,
  hostYamlSnippet,
} from "../lib/install-prompt";
import { cn } from "../lib/utils";

interface AdminSettings {
  public_url?: string;
  ingest_origin?: string;
  workspace_slug?: string;
  edition?: string;
}

interface CreatedToken {
  token: string;
  prefix?: string;
  scope?: string;
}

const HOST_KINDS = [
  { value: "physical", label: "实体机" },
  { value: "vm", label: "虚拟机" },
  { value: "container_host", label: "Docker 容器" },
] as const;

export function InstallPage() {
  return (
    <Routes>
      <Route index element={<Chooser />} />
      <Route path="agent" element={<AgentGuide />} />
      <Route path="host" element={<HostGuide />} />
    </Routes>
  );
}

function Chooser() {
  return (
    <div className="mx-auto max-w-4xl">
      <h1 className="text-2xl font-semibold tracking-tight">安装</h1>
      <p className="mt-1 text-sm text-slate-400">选一种接入方式。两条路可以同时开，互不影响。</p>
      <div className="mt-6 grid gap-4 md:grid-cols-2">
        <ChoiceCard
          to="/install/agent"
          icon={<Bot className="h-6 w-6 text-indigo-300" />}
          title="AI Agent"
          hint="Cursor / Claude / Codex / OpenClaw…"
          body="对 Agent 说一句话，把当前项目的编码任务报到看板。"
        />
        <ChoiceCard
          to="/install/host"
          icon={<Server className="h-6 w-6 text-cyan-300" />}
          title="实体运行环境"
          hint="实体机 · 虚拟机 · Docker"
          body="下载 Linux 客户端，上报 CPU、磁盘、端口和本机作业。macOS / Windows 开发中。"
        />
      </div>
    </div>
  );
}

function ChoiceCard({
  to,
  icon,
  title,
  hint,
  body,
}: {
  to: string;
  icon: ReactNode;
  title: string;
  hint: string;
  body: string;
}) {
  return (
    <Link
      to={to}
      className="group rounded-2xl border border-slate-800 bg-[#0f1626] p-5 transition hover:border-indigo-500/50 hover:bg-indigo-500/5"
    >
      <div className="mb-3 flex h-11 w-11 items-center justify-center rounded-xl bg-slate-900 ring-1 ring-slate-800 group-hover:ring-indigo-500/40">
        {icon}
      </div>
      <div className="text-lg font-semibold">{title}</div>
      <div className="mt-0.5 text-xs uppercase tracking-[0.14em] text-slate-500">{hint}</div>
      <p className="mt-3 text-sm leading-relaxed text-slate-400">{body}</p>
    </Link>
  );
}

function GuideShell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-5 flex flex-wrap items-center gap-3 text-sm">
        <Link to="/install" className="text-slate-500 hover:text-white">
          ← 安装
        </Link>
        <span className="text-slate-700">/</span>
        <NavLink
          to="/install/agent"
          className={({ isActive }) => (isActive ? "text-white" : "text-slate-500 hover:text-white")}
        >
          AI Agent
        </NavLink>
        <span className="text-slate-700">·</span>
        <NavLink
          to="/install/host"
          className={({ isActive }) => (isActive ? "text-white" : "text-slate-500 hover:text-white")}
        >
          实体运行环境
        </NavLink>
      </div>
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="mt-1 text-sm text-slate-400">{subtitle}</p>
      <div className="mt-6 flex flex-col gap-6">{children}</div>
    </div>
  );
}

function useInstallContext() {
  const settings = useQuery({ queryKey: ["admin-settings"], queryFn: () => apiGet<AdminSettings>("/api/v1/admin/settings") });
  const machines = useQuery({ queryKey: ["admin-machines"], queryFn: () => apiGet<Machine[]>("/api/v1/admin/machines") });
  const publicUrl = String(settings.data?.public_url ?? "");
  const ingestOrigin = String(settings.data?.ingest_origin ?? publicUrl);
  const origin = boardOrigin(publicUrl, typeof window !== "undefined" ? window.location.origin : "");
  return {
    ready: settings.isSuccess,
    settings,
    machines,
    publicUrl,
    ingestOrigin: ingestOrigin || origin,
    origin,
  };
}

function SpellCard({ prompt, copied, onCopy }: { prompt: string; copied: boolean; onCopy: () => void }) {
  return (
    <div className="relative overflow-hidden rounded-2xl border border-indigo-500/35 bg-gradient-to-br from-indigo-950/90 via-[#0b1020] to-violet-950/70 p-5 shadow-[0_0_48px_rgba(99,102,241,0.18)]">
      <div className="pointer-events-none absolute -right-10 -top-10 h-36 w-36 rounded-full bg-indigo-500/20 blur-3xl" />
      <div className="mb-3 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-indigo-300">
        <Sparkles className="h-3.5 w-3.5" />
        对你的 AI 说这句话
      </div>
      <blockquote className="relative whitespace-pre-wrap font-mono text-[13px] leading-relaxed text-indigo-50/95">
        {prompt}
      </blockquote>
      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button type="button" onClick={onCopy}>
          <Copy className="h-4 w-4" />
          {copied ? "已复制，丢给 Agent" : "复制给 Agent"}
        </Button>
        <span className="text-xs text-slate-500">
          粘贴到 Cursor / Claude / Codex 等对话框即可安装。口令会让 Agent 按自身设置 PROVIDER，不要写死成 cursor。
        </span>
      </div>
    </div>
  );
}

function CopyBlock({ label, value, id, copied, onCopy }: { label: string; value: string; id: string; copied: string | null; onCopy: (text: string, id: string) => void }) {
  return (
    <div>
      <div className="mb-1 flex items-center justify-between gap-2">
        <span className="text-xs text-slate-500">{label}</span>
        <Button type="button" size="sm" variant="outline" onClick={() => onCopy(value, id)}>
          <Copy className="h-3.5 w-3.5" />
          {copied === id ? "已复制" : "复制"}
        </Button>
      </div>
      <pre className="overflow-x-auto rounded-md bg-slate-950 px-3 py-2 font-mono text-xs text-slate-200">{value}</pre>
    </div>
  );
}

function TokenBinder({
  kinds,
  defaultKind,
  defaultCode,
  machines,
  onIssued,
}: {
  kinds: { value: string; label: string }[];
  defaultKind: string;
  defaultCode: string;
  machines: Machine[];
  onIssued: (info: { token: string; machineKey: string }) => void;
}) {
  const qc = useQueryClient();
  const [code, setCode] = useState(defaultCode);
  const [name, setName] = useState("");
  const [kind, setKind] = useState(defaultKind);
  const [existingId, setExistingId] = useState("");
  const eligible = machines.filter((m) => kinds.some((k) => k.value === m.kind) && m.enabled);

  const createMachine = useMutation({
    mutationFn: () =>
      apiPost<{ machine: Machine; token?: CreatedToken }>("/api/v1/admin/machines", {
        machine_key: code,
        name: name || code,
        kind,
        create_machine_token: true,
      }),
    onSuccess: (data) => {
      if (data.token?.token) onIssued({ token: data.token.token, machineKey: data.machine.machine_key });
      qc.invalidateQueries({ queryKey: ["admin-machines"] });
      qc.invalidateQueries({ queryKey: ["admin-tokens"] });
    },
  });

  const mintToken = useMutation({
    mutationFn: () => {
      const m = eligible.find((x) => x.id === existingId);
      if (!m) throw new Error("请先选一台机器");
      return apiPost<CreatedToken>("/api/v1/admin/tokens", {
        name: `${m.machine_key} install`,
        scope: "machine_ingest",
        machine_id: m.id,
      }).then((tok) => ({ tok, machineKey: m.machine_key }));
    },
    onSuccess: ({ tok, machineKey }) => {
      if (tok.token) onIssued({ token: tok.token, machineKey });
      qc.invalidateQueries({ queryKey: ["admin-tokens"] });
    },
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>填入 {MACHINE_KEY_PLACEHOLDER}</CardTitle>
        <CardDescription>完整 Token 只显示一次。生成后会写进口令里的占位符。</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
          <div>
            <Label>机器代号</Label>
            <Input value={code} onChange={(e) => setCode(e.target.value)} placeholder={defaultCode} className="mt-1" />
          </div>
          <div>
            <Label>显示名</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="可中文" className="mt-1" />
          </div>
          {kinds.length > 1 ? (
            <select
              value={kind}
              onChange={(e) => setKind(e.target.value)}
              className="h-9 rounded-md border border-slate-700 bg-slate-900 px-3 text-sm"
            >
              {kinds.map((k) => (
                <option key={k.value} value={k.value}>
                  {k.label}
                </option>
              ))}
            </select>
          ) : null}
        </div>
        <Button
          type="button"
          disabled={!code || createMachine.isPending}
          onClick={() => createMachine.mutate()}
        >
          创建机器并生成 Token
        </Button>
        {createMachine.error && <p className="text-sm sev-error">{(createMachine.error as Error).message}</p>}
        {eligible.length > 0 && (
          <div className="flex flex-wrap items-end gap-2 border-t border-slate-800 pt-4">
            <div className="min-w-[12rem] flex-1">
              <Label>已有机器</Label>
              <select
                value={existingId}
                onChange={(e) => setExistingId(e.target.value)}
                className="mt-1 h-9 w-full rounded-md border border-slate-700 bg-slate-900 px-3 text-sm"
              >
                <option value="">选择一台…</option>
                {eligible.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name} ({m.machine_key})
                  </option>
                ))}
              </select>
            </div>
            <Button
              type="button"
              variant="outline"
              disabled={!existingId || mintToken.isPending}
              onClick={() => mintToken.mutate()}
            >
              为它生成新 Token
            </Button>
          </div>
        )}
        {mintToken.error && <p className="text-sm sev-error">{(mintToken.error as Error).message}</p>}
      </CardContent>
    </Card>
  );
}

function AgentGuide() {
  const ctx = useInstallContext();
  const [copied, setCopied] = useState<string | null>(null);
  const [issued, setIssued] = useState<{ token: string; machineKey: string } | null>(null);
  const prompt = agentInstallPrompt(ctx.ingestOrigin, issued?.token ?? "");
  const machines = ctx.machines.data ?? [];

  function copy(text: string, id: string) {
    void navigator.clipboard?.writeText(text);
    setCopied(id);
    window.setTimeout(() => setCopied(null), 1600);
  }

  if (!ctx.ready) {
    return (
      <GuideShell title="给 AI Agent 装上报" subtitle="编码会话的开始、进度、成功、失败、被打断，会出现在这台看板的 virtual 机器上。">
        <p className="text-slate-400">加载中…</p>
      </GuideShell>
    );
  }

  return (
    <GuideShell title="给 AI Agent 装上报" subtitle="编码会话的开始、进度、成功、失败、被打断，会出现在这台看板的 virtual 机器上。">
      <SpellCard prompt={prompt} copied={copied === "spell"} onCopy={() => copy(prompt, "spell")} />
      {issued && (
        <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
          Token 已写入上面的口令。关闭本页后无法再看完整密钥，请现在复制给 Agent。
        </p>
      )}
      <TokenBinder
        kinds={[{ value: "virtual", label: "virtual" }]}
        defaultKind="virtual"
        defaultCode="my-app"
        machines={machines}
        onIssued={setIssued}
      />
      <Card>
        <CardHeader>
          <CardTitle>手动安装</CardTitle>
          <CardDescription>不想让 Agent 动手时，三步即可。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 text-sm text-slate-300">
          <ol className="list-decimal space-y-3 pl-5">
            <li>上方创建一台 virtual 机器，拿到 {MACHINE_KEY_PLACEHOLDER}。</li>
            <li>
              仓库根目录 <code className="font-mono text-xs">.env</code> 只放 URL 和 Token（不要提交）。
              <code className="font-mono text-xs">AGENTBOARD_PROVIDER</code> 按当前 Agent 在跑{" "}
              <code className="font-mono text-xs">report.py</code> 前 export：cursor / codex / claude / openclaw / hermes / pi，不要写进共享 .env。
              <div className="mt-2">
                <CopyBlock
                  label=".env"
                  value={agentEnvSnippet(ctx.ingestOrigin, issued?.token ?? "")}
                  id="env"
                  copied={copied}
                  onCopy={copy}
                />
              </div>
            </li>
            <li>
              从{" "}
              <a className="text-indigo-300 hover:underline" href="https://github.com/yinger650/my-ai-dashboard" target="_blank" rel="noreferrer">
                GitHub 仓库
              </a>{" "}
              拷贝 <code className="font-mono text-xs">skills/agentboard-report</code>，把{" "}
              <code className="font-mono text-xs">always-on.md</code> 并入 <code className="font-mono text-xs">AGENTS.md</code>
              。之后每个会话：<code className="font-mono text-xs">report.py start</code> →{" "}
              <code className="font-mono text-xs">succeed</code> / <code className="font-mono text-xs">fail</code> /{" "}
              <code className="font-mono text-xs">interrupt</code>。
            </li>
          </ol>
        </CardContent>
      </Card>
    </GuideShell>
  );
}

function HostGuide() {
  const ctx = useInstallContext();
  const [copied, setCopied] = useState<string | null>(null);
  const [issued, setIssued] = useState<{ token: string; machineKey: string } | null>(null);
  const machines = ctx.machines.data ?? [];
  const amd64 = clientDownloadUrl(ctx.origin, "board-client-linux-amd64");
  const arm64 = clientDownloadUrl(ctx.origin, "board-client-linux-arm64");
  const prompt = hostInstallPrompt({
    boardUrl: ctx.ingestOrigin,
    machineKey: issued?.token ?? "",
    machineCode: issued?.machineKey ?? "home-server",
    downloadAmd64: amd64,
    downloadArm64: arm64,
  });

  function copy(text: string, id: string) {
    void navigator.clipboard?.writeText(text);
    setCopied(id);
    window.setTimeout(() => setCopied(null), 1600);
  }

  if (!ctx.ready) {
    return (
      <GuideShell title="给 Linux 主机装客户端" subtitle="实体机、虚拟机、Docker 容器同一套二进制。macOS 与 Windows 还在开发。">
        <p className="text-slate-400">加载中…</p>
      </GuideShell>
    );
  }

  return (
    <GuideShell title="给 Linux 主机装客户端" subtitle="实体机、虚拟机、Docker 容器同一套二进制。macOS 与 Windows 还在开发。">
      <SpellCard prompt={prompt} copied={copied === "spell"} onCopy={() => copy(prompt, "spell")} />
      {issued && (
        <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
          Token 已写入上面的口令与下面的命令。关闭本页后无法再看完整密钥。
        </p>
      )}
      <TokenBinder
        kinds={[...HOST_KINDS]}
        defaultKind="physical"
        defaultCode="home-server"
        machines={machines}
        onIssued={setIssued}
      />
      <Card>
        <CardHeader>
          <CardTitle>下载 Linux 客户端</CardTitle>
          <CardDescription>优先走本看板镜像（国内更稳）。镜像没有文件时再用 GitHub Release。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-2">
            {LINUX_ASSETS.map((asset) => (
              <a
                key={asset.id}
                href={clientDownloadUrl(ctx.origin, asset.file)}
                className="flex items-center justify-between rounded-xl border border-slate-800 bg-slate-950 px-4 py-3 text-sm hover:border-indigo-500/50"
              >
                <span>
                  <span className="block font-medium text-slate-100">{asset.label}</span>
                  <span className="font-mono text-[11px] text-slate-500">{asset.file}</span>
                </span>
                <Download className="h-4 w-4 text-indigo-300" />
              </a>
            ))}
            <DisabledDownload label="macOS" />
            <DisabledDownload label="Windows" />
          </div>
          <div className="flex flex-wrap gap-3 text-xs">
            {LINUX_ASSETS.map((asset) => (
              <a
                key={`gh-${asset.id}`}
                href={clientDownloadUrl(ctx.origin, asset.file, true)}
                className="text-slate-500 underline-offset-2 hover:text-indigo-300 hover:underline"
              >
                GitHub · {asset.label}
              </a>
            ))}
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>手动安装</CardTitle>
          <CardDescription>SSH 到目标 Linux，复制粘贴即可。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 text-sm text-slate-300">
          <ol className="list-decimal space-y-3 pl-5">
            <li>上方创建机器并生成 Token。机器代号必须和 YAML 里完全一致。</li>
            <li>
              安装二进制（按架构改文件名）：
              <div className="mt-2">
                <CopyBlock
                  label="安装"
                  value={[
                    "sudo install -d /opt/agentboard/bin /etc/agentboard",
                    `curl -fsSL ${amd64} -o /tmp/board-client`,
                    "sudo install -m 755 /tmp/board-client /opt/agentboard/bin/board-client",
                  ].join("\n")}
                  id="bin"
                  copied={copied}
                  onCopy={copy}
                />
              </div>
            </li>
            <li>
              Token 与最小配置：
              <div className="mt-2 space-y-2">
                <CopyBlock
                  label="/etc/agentboard/board-client.env"
                  value={hostEnvSnippet(issued?.token ?? "")}
                  id="cenv"
                  copied={copied}
                  onCopy={copy}
                />
                <CopyBlock
                  label="/etc/agentboard/client.yaml"
                  value={hostYamlSnippet(ctx.ingestOrigin, issued?.machineKey ?? "home-server")}
                  id="yaml"
                  copied={copied}
                  onCopy={copy}
                />
              </div>
              <p className="mt-2 text-xs text-slate-500">
                <code className="font-mono">sudo chmod 600 /etc/agentboard/board-client.env</code>
              </p>
            </li>
            <li>
              跑起来：
              <div className="mt-2">
                <CopyBlock
                  label="前台"
                  value="sudo /opt/agentboard/bin/board-client run --config /etc/agentboard/client.yaml"
                  id="run"
                  copied={copied}
                  onCopy={copy}
                />
              </div>
            </li>
            <li>回到看板，对应卡片出现且 CPU 会动即成功。更多勾选见设置页与仓库教程。</li>
          </ol>
        </CardContent>
      </Card>
    </GuideShell>
  );
}

function DisabledDownload({ label }: { label: string }) {
  return (
    <div
      className={cn(
        "flex items-center justify-between rounded-xl border border-dashed border-slate-800 bg-slate-950/40 px-4 py-3 text-sm text-slate-600",
      )}
    >
      <span>
        <span className="block font-medium">{label}</span>
        <span className="text-[11px]">开发中</span>
      </span>
    </div>
  );
}

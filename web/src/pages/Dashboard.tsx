import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useMemo, useState } from "react";
import { LayoutGrid, RefreshCw, Search } from "lucide-react";
import type { Layout } from "react-grid-layout";
import { apiGet, apiPatch } from "../api";
import type { Board } from "../types";
import { relativeTime } from "../format";
import { BoardGrid } from "../components/BoardGrid";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Switch } from "../components/ui/switch";
import { Label } from "../components/ui/label";

export function DashboardPage() {
  const qc = useQueryClient();
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [search, setSearch] = useState("");
  const [editMode, setEditMode] = useState(false);

  const { data, isLoading, error, dataUpdatedAt } = useQuery({
    queryKey: ["board"],
    queryFn: () => apiGet<Board>("/api/v1/board"),
    refetchInterval: (q) =>
      autoRefresh ? (q.state.data?.poll_interval_seconds ?? 15) * 1000 : false,
  });

  const saveLayout = useMutation({
    mutationFn: (layout: Layout) => apiPatch("/api/v1/admin/settings", { board_layout: layout }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin-settings"] }),
  });

  const machines = useMemo(
    () =>
      (data?.machines ?? []).filter((m) =>
        search
          ? m.name.toLowerCase().includes(search.toLowerCase()) || m.machine_key.includes(search)
          : true,
      ),
    [data?.machines, search],
  );

  const counts = { online: 0, degraded: 0, offline: 0 };
  for (const m of data?.machines ?? []) {
    if (m.health === "online") counts.online++;
    else if (m.health === "degraded" || m.health === "stale") counts.degraded++;
    else if (m.health === "offline") counts.offline++;
  }

  const savedLayout = Array.isArray(data?.layout) ? (data?.layout as Layout) : null;
  const pollMs = (data?.poll_interval_seconds ?? 15) * 1000;

  return (
    <div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{data?.title ?? "AgentBoard Personal"}</h1>
          <p className="text-sm text-slate-400">
            <span className="sev-normal">在线 {counts.online}</span> ·{" "}
            <span className="sev-warning">降级 {counts.degraded}</span> ·{" "}
            <span className="sev-offline">离线 {counts.offline}</span>
            {data && <> · 异常访问 {data.recent_abnormal}</>}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-slate-500" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索机器…"
              className="w-44 pl-8 sm:w-56"
            />
          </div>
          <div className="flex items-center gap-2">
            <Switch id="auto-refresh" checked={autoRefresh} onCheckedChange={setAutoRefresh} />
            <Label htmlFor="auto-refresh" className="mb-0 text-sm text-slate-400">
              自动刷新
            </Label>
          </div>
          <Button
            type="button"
            size="sm"
            variant={editMode ? "default" : "outline"}
            onClick={() => setEditMode((v) => !v)}
          >
            <LayoutGrid className="h-4 w-4" />
            {editMode ? "完成布局" : "调整布局"}
          </Button>
          <span className="hidden items-center gap-1 text-xs text-slate-500 sm:inline-flex">
            <RefreshCw className="h-3 w-3" />
            {dataUpdatedAt ? relativeTime(new Date(dataUpdatedAt).toISOString()) : ""}
          </span>
        </div>
      </div>

      {editMode && (
        <div className="mb-4 rounded-md border border-indigo-500/30 bg-indigo-500/10 px-3 py-2 text-xs text-indigo-200">
          拖动手柄移动卡片，右下角缩放。卡片会吸附到网格；手机端仍使用单列流式布局。
        </div>
      )}

      {error && (
        <div className="mb-4 rounded-md bg-amber-500/10 px-3 py-2 text-sm sev-warning">
          数据可能已过期：{(error as Error).message}
        </div>
      )}

      {isLoading && <div className="text-slate-400">加载中…</div>}

      {!isLoading && machines.length === 0 && (
        <div className="rounded-xl border border-slate-800 bg-[#0f1626] p-8 text-center text-slate-400">
          还没有机器。前往{" "}
          <Link className="text-indigo-400 underline" to="/settings">
            设置
          </Link>{" "}
          创建一台并生成 Token。
        </div>
      )}

      {machines.length > 0 && (
        <BoardGrid
          machines={machines}
          savedLayout={savedLayout}
          editMode={editMode}
          autoRefresh={autoRefresh}
          pollMs={pollMs}
          onLayoutChange={(layout) => saveLayout.mutate(layout)}
        />
      )}
    </div>
  );
}

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { apiGet } from "../api";
import type { AccessLog } from "../types";
import { localTime } from "../format";

export function AccessPage() {
  const [abnormalOnly, setAbnormalOnly] = useState(false);
  const { data, isLoading } = useQuery({
    queryKey: ["access", abnormalOnly],
    queryFn: () => apiGet<AccessLog[]>(`/api/v1/admin/access-logs${abnormalOnly ? "?abnormal=1" : ""}`),
    refetchInterval: 15000,
  });

  return (
    <div>
      <div className="mb-5 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">访问记录</h1>
        <label className="flex items-center gap-2 text-sm text-slate-400">
          <input type="checkbox" checked={abnormalOnly} onChange={(e) => setAbnormalOnly(e.target.checked)} />
          只看异常
        </label>
      </div>

      {isLoading && <div className="text-slate-400">加载中…</div>}

      <div className="card overflow-hidden">
        <table className="w-full text-left text-sm">
          <thead className="bg-slate-900/60 text-xs text-slate-400">
            <tr>
              <th className="px-3 py-2">时间</th>
              <th className="px-3 py-2">IP</th>
              <th className="px-3 py-2">主体</th>
              <th className="px-3 py-2">方法/路径</th>
              <th className="px-3 py-2">状态</th>
              <th className="px-3 py-2">原因</th>
              <th className="px-3 py-2">Request ID</th>
            </tr>
          </thead>
          <tbody>
            {(data ?? []).map((a) => (
              <tr
                key={a.id}
                className={a.is_abnormal ? "border-l-4 border-red-500 bg-red-500/5" : "border-b border-slate-800/50"}
              >
                <td className="px-3 py-2 text-slate-400">{localTime(a.occurred_at)}</td>
                <td className="px-3 py-2">{a.ip ?? "-"}</td>
                <td className="px-3 py-2">{a.actor_type}</td>
                <td className="px-3 py-2 font-mono text-xs">{a.method} {a.path}</td>
                <td className={`px-3 py-2 ${a.status_code >= 400 ? "sev-error" : "sev-normal"}`}>{a.status_code}</td>
                <td className="px-3 py-2 text-slate-400">{a.reason}</td>
                <td className="px-3 py-2 font-mono text-xs text-slate-500">{a.request_id.slice(0, 8)}</td>
              </tr>
            ))}
            {(data ?? []).length === 0 && !isLoading && (
              <tr><td colSpan={7} className="px-3 py-6 text-center text-slate-500">暂无记录</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

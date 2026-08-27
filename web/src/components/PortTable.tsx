import { cn } from "../lib/utils";

export interface ListenPort {
  protocol: string;
  address: string;
  port: number;
  pid?: number;
  process?: string;
}

export function PortTable({
  ports,
  occurredAt,
  emptyLabel = "暂无监听端口",
}: {
  ports: ListenPort[] | null | undefined;
  occurredAt?: string | null;
  emptyLabel?: string;
}) {
  const rows = ports ?? [];
  if (rows.length === 0) {
    return <div className="py-6 text-center text-sm text-slate-500">{emptyLabel}</div>;
  }
  return (
    <div>
      {occurredAt && <p className="mb-2 text-xs text-slate-500">快照 {new Date(occurredAt).toLocaleString()}</p>}
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="text-xs text-slate-500">
            <tr>
              <th className="pb-2 font-medium">进程</th>
              <th className="pb-2 font-medium">协议</th>
              <th className="pb-2 font-medium">绑定</th>
              <th className="pb-2 font-medium">端口</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800">
            {rows.map((p, i) => (
              <tr key={`${p.protocol}-${p.address}-${p.port}-${i}`} className={cn("text-slate-200")}>
                <td className="py-1.5">{p.process || "-"}</td>
                <td className="py-1.5 text-slate-400">{p.protocol}</td>
                <td className="py-1.5 font-mono text-xs text-slate-400">{p.address}</td>
                <td className="py-1.5 font-mono">{p.port}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

import { Area, AreaChart, ResponsiveContainer, YAxis } from "recharts";
import type { SparkPoint } from "../types";

export function Sparkline({ data, color = "#818cf8" }: { data: SparkPoint[]; color?: string }) {
  if (!data || data.length === 0) {
    return <div className="flex h-12 items-center text-xs text-slate-500">暂无数据</div>;
  }
  return (
    <ResponsiveContainer width="100%" height={48}>
      <AreaChart data={data} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
        <defs>
          <linearGradient id={`sg-${color}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity={0.5} />
            <stop offset="100%" stopColor={color} stopOpacity={0} />
          </linearGradient>
        </defs>
        <YAxis hide domain={[0, 100]} />
        <Area type="monotone" dataKey="cpu" stroke={color} strokeWidth={1.5} fill={`url(#sg-${color})`} isAnimationActive={false} />
      </AreaChart>
    </ResponsiveContainer>
  );
}

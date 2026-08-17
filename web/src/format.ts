export function fmtBytes(n: number | null | undefined): string {
  if (n == null) return "--";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}

export function fmtBps(n: number | null | undefined): string {
  if (n == null) return "--";
  return `${fmtBytes(n)}/s`;
}

export function fmtPct(n: number | null | undefined): string {
  if (n == null) return "--";
  return `${n.toFixed(1)}%`;
}

export function usagePct(used: number | null, total: number | null): number | null {
  if (used == null || total == null || total === 0) return null;
  return (used / total) * 100;
}

export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return "从未";
  const then = new Date(iso).getTime();
  const diff = Math.max(0, Date.now() - then);
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s} 秒前`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} 分钟前`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} 小时前`;
  return `${Math.floor(h / 24)} 天前`;
}

export function localTime(iso: string | null | undefined): string {
  if (!iso) return "--";
  return new Date(iso).toLocaleString();
}

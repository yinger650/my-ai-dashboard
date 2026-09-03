import type { MetricSample, StatusItem } from "../types";
import { usagePct } from "../format";

export interface PercentMetric {
  key: string;
  label: string;
  value: number;
  severity?: string;
}

const LABEL_BY_KEY: Record<string, string> = {
  cpu: "CPU",
  cpu_percent: "CPU",
  mem: "内存",
  memory: "内存",
  disk: "磁盘",
  gpu: "GPU",
  gpu_util: "GPU",
  gpu_percent: "GPU",
  gpu_mem: "显存",
  gpu_memory: "显存",
  gpu_mem_util: "显存",
  vram: "显存",
  data_dir: "目录占用",
  data_dir_pct: "目录占用",
};

const PERCENT_HINT = /percent|pct|util|gpu|vram|显存|利用率|占用/i;

export function metricLabel(key: string): string {
  return LABEL_BY_KEY[key] ?? key.replace(/_/g, " ");
}

export function parseStatusNumber(st: StatusItem): number | null {
  try {
    const v = JSON.parse(st.value_json);
    if (typeof v === "number" && Number.isFinite(v)) return v;
    if (typeof v === "boolean") return v ? 1 : 0;
    if (typeof v === "string" && v.trim() !== "") {
      const n = Number(v);
      return Number.isFinite(n) ? n : null;
    }
  } catch {
    /* ignore */
  }
  return null;
}

export function formatStatusValue(st: StatusItem): string {
  try {
    const v = JSON.parse(st.value_json);
    if (typeof v === "boolean") return v ? "是" : "否";
    return String(v);
  } catch {
    return st.value_json;
  }
}

export function isPercentStatus(st: StatusItem): boolean {
  const unit = (st.unit ?? "").toLowerCase().trim();
  if (unit === "%" || unit === "percent" || unit === "pct") return true;
  if (st.value_type === "progress") return true;
  const fmt = (st.display_format ?? "").toLowerCase();
  if (fmt === "percent" || fmt === "progress_bar") return true;
  if (st.value_type === "number" && PERCENT_HINT.test(`${st.status_key} ${st.label}`)) {
    return parseStatusNumber(st) != null;
  }
  return false;
}

export function collectPercentMetrics(input: {
  latest_metric?: MetricSample | null;
  heartbeat_metrics?: Record<string, number> | null;
  statuses?: StatusItem[] | null;
}): PercentMetric[] {
  const out: PercentMetric[] = [];
  const seen = new Set<string>();
  const add = (p: PercentMetric) => {
    if (!Number.isFinite(p.value) || seen.has(p.key)) return;
    seen.add(p.key);
    out.push(p);
  };

  const lm = input.latest_metric;
  if (lm?.cpu_percent != null) add({ key: "cpu", label: "CPU", value: lm.cpu_percent });
  const mem = usagePct(lm?.memory_used_bytes ?? null, lm?.memory_total_bytes ?? null);
  if (mem != null) add({ key: "mem", label: "内存", value: mem });
  const disk = usagePct(lm?.root_disk_used_bytes ?? null, lm?.root_disk_total_bytes ?? null);
  if (disk != null) add({ key: "disk", label: "磁盘", value: disk });

  for (const [key, value] of Object.entries(input.heartbeat_metrics ?? {})) {
    add({ key, label: metricLabel(key), value });
  }

  for (const st of input.statuses ?? []) {
    if (!isPercentStatus(st)) continue;
    const n = parseStatusNumber(st);
    if (n == null) continue;
    add({
      key: st.status_key,
      label: st.label || metricLabel(st.status_key),
      value: n,
      severity: st.severity,
    });
  }
  return out;
}

export function textStatuses(statuses: StatusItem[] | null | undefined): StatusItem[] {
  return (statuses ?? []).filter((s) => !isPercentStatus(s));
}

export function hasNetworkSample(lm: MetricSample | null | undefined): boolean {
  return lm?.network_rx_bps != null || lm?.network_tx_bps != null;
}

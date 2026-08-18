const HEALTH_LABEL: Record<string, string> = {
  online: "在线",
  offline: "离线",
  stale: "延迟",
  degraded: "降级",
  unknown: "未知",
  disabled: "已禁用",
};

const HEALTH_SEV: Record<string, string> = {
  online: "normal",
  offline: "offline",
  stale: "warning",
  degraded: "warning",
  unknown: "unknown",
  disabled: "unknown",
};

const HEALTH_ICON: Record<string, string> = {
  online: "●",
  offline: "○",
  stale: "◐",
  degraded: "▲",
  unknown: "?",
  disabled: "⊘",
};

export function HealthBadge({ health }: { health: string }) {
  const sev = HEALTH_SEV[health] ?? "unknown";
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium sev-${sev}`}
      style={{ borderColor: "currentColor" }}
      title={health}
    >
      <span aria-hidden>{HEALTH_ICON[health] ?? "?"}</span>
      {HEALTH_LABEL[health] ?? health}
    </span>
  );
}

export function SevDot({ severity }: { severity: string }) {
  return (
    <span
      className={`inline-block h-2.5 w-2.5 rounded-full bg-sev-${severity}`}
      title={severity}
      aria-label={severity}
    />
  );
}

export function SevText({ severity, children }: { severity: string; children: React.ReactNode }) {
  return <span className={`sev-${severity}`}>{children}</span>;
}
